package command

import (
	"bytes"
	"crypto/md5" //nolint:gosec // Mirrors the command's Conan revision derivation.
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
	conanutil "github.com/harness/harness-cli/module/ar/migrate/util"
	"github.com/harness/harness-cli/util/common/auth"
)

// withConanServer spins up a stub server and points the global config at it
// for the duration of the test, restoring originals on cleanup.
func withConanServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	origPkg := config.Global.Registry.PkgURL
	origAcct := config.Global.AccountID
	config.Global.Registry.PkgURL = srv.URL
	config.Global.AccountID = "test-account"
	t.Cleanup(func() {
		config.Global.Registry.PkgURL = origPkg
		config.Global.AccountID = origAcct
	})
	return srv
}

func runConanCmd(t *testing.T, args ...string) error {
	t.Helper()
	factory := &cmdutils.Factory{
		PkgHttpClient: func() *pkgclient.ClientWithResponses {
			client, err := pkgclient.NewClientWithResponses(config.Global.Registry.PkgURL,
				auth.GetAuthOptionARPKG())
			if err != nil {
				t.Fatalf("failed to create pkg client: %v", err)
			}
			return client
		},
	}
	cmd := NewPushConanCmd(factory)
	cmd.SetArgs(args)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	return cmd.Execute()
}

// writeConanDir writes the given filename->content map into a fresh temp dir and
// returns the directory path.
func writeConanDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func TestParseConanReference(t *testing.T) {
	cases := []struct {
		in                           string
		wantErr                      bool
		name, version, user, channel string
	}{
		{in: "zlib/1.3", name: "zlib", version: "1.3", user: "_", channel: "_"},
		{in: "zlib/1.3@myuser/stable", name: "zlib", version: "1.3", user: "myuser", channel: "stable"},
		{in: "zlib", wantErr: true},
		{in: "zlib/1.3@myuser", wantErr: true},
		{in: "Zlib/1.3", wantErr: true}, // uppercase not allowed
		{in: "zlib/1.3@/stable", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, tc := range cases {
		ref, err := parseConanReference(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseConanReference(%q) expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseConanReference(%q) unexpected error: %v", tc.in, err)
			continue
		}
		if ref.Name != tc.name || ref.Version != tc.version || ref.User != tc.user || ref.Channel != tc.channel {
			t.Errorf("parseConanReference(%q) = %+v", tc.in, ref)
		}
	}
}

func TestOrderConanFiles_ManifestLast(t *testing.T) {
	in := []string{"/x/conanmanifest.txt", "/x/conanfile.py", "/x/conan_export.tgz"}
	got := orderConanFiles(in)
	if filepath.Base(got[len(got)-1]) != conanutil.ConanManifestFile {
		t.Errorf("conanmanifest.txt must be last, got order: %v", got)
	}
	// Non-manifest files must be sorted deterministically.
	nonManifest := got[:len(got)-1]
	if !sort.StringsAreSorted(nonManifest) {
		t.Errorf("non-manifest files not sorted: %v", nonManifest)
	}
}

func TestConanRevisionFromManifest(t *testing.T) {
	dir := writeConanDir(t, map[string]string{conanutil.ConanManifestFile: "1700000000\nconanfile.py: abc\n"})
	got, err := conanRevisionFromManifest(dir)
	if err != nil {
		t.Fatalf("conanRevisionFromManifest: %v", err)
	}
	sum := md5.Sum([]byte("1700000000\nconanfile.py: abc\n")) //nolint:gosec
	if want := hex.EncodeToString(sum[:]); got != want {
		t.Errorf("rrev = %s, want %s", got, want)
	}
}

func TestCollectConanLayerFiles_RequiresManifest(t *testing.T) {
	dir := writeConanDir(t, map[string]string{"conanfile.py": "content"})
	if _, _, err := collectConanLayerFiles(dir, conanutil.IsConanRecipeFile); err == nil {
		t.Fatal("expected error when conanmanifest.txt missing")
	}
}

func TestCollectConanLayerFiles_SkipsUnknownFiles(t *testing.T) {
	dir := writeConanDir(t, map[string]string{
		"conanfile.py":              "x",
		conanutil.ConanManifestFile: "1700000000\nconanfile.py: abc\n",
		"conan_export.tgz":          "tgz",
		".DS_Store":                 "junk",
		"README.md":                 "docs",
	})
	files, skipped, err := collectConanLayerFiles(dir, conanutil.IsConanRecipeFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 3 {
		t.Errorf("expected 3 valid files, got %d: %v", len(files), files)
	}
	if len(skipped) != 2 {
		t.Errorf("expected 2 skipped files, got %d: %v", len(skipped), skipped)
	}
}

func TestIsValidConanLayerFile(t *testing.T) {
	recipeValid := []string{"conanfile.py", "conanmanifest.txt", "conan_export.tgz", "conan_sources.txz", "conan_sources.tzst"}
	for _, n := range recipeValid {
		if !conanutil.IsConanRecipeFile(n) {
			t.Errorf("conanutil.IsConanRecipeFile(%q) = false, want true", n)
		}
	}
	recipeInvalid := []string{"conaninfo.txt", "conan_package.tgz", ".DS_Store", "conanfile.txt", "conan_export.tar.gz", "conan_export.zip"}
	for _, n := range recipeInvalid {
		if conanutil.IsConanRecipeFile(n) {
			t.Errorf("conanutil.IsConanRecipeFile(%q) = true, want false", n)
		}
	}
	pkgValid := []string{"conaninfo.txt", "conanmanifest.txt", "conan_package.tgz"}
	for _, n := range pkgValid {
		if !conanutil.IsConanPackageFile(n) {
			t.Errorf("conanutil.IsConanPackageFile(%q) = false, want true", n)
		}
	}
	pkgInvalid := []string{"conanfile.py", "conan_export.tgz", "conan_package.zip"}
	for _, n := range pkgInvalid {
		if conanutil.IsConanPackageFile(n) {
			t.Errorf("conanutil.IsConanPackageFile(%q) = true, want false", n)
		}
	}
}

func TestNewPushConanCmd_SkipsStrayFiles(t *testing.T) {
	var uploaded []string
	withConanServer(t, func(w http.ResponseWriter, r *http.Request) {
		uploaded = append(uploaded, filepath.Base(r.URL.Path))
		w.WriteHeader(http.StatusOK)
	})
	dir := writeConanDir(t, map[string]string{
		"conanfile.py":              "x",
		conanutil.ConanManifestFile: "1700000000\nconanfile.py: abc\n",
		".DS_Store":                 "junk",
	})
	if err := runConanCmd(t, "test-registry", "zlib/1.3", dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, name := range uploaded {
		if name == ".DS_Store" {
			t.Fatalf(".DS_Store should have been skipped, uploads: %v", uploaded)
		}
	}
	if len(uploaded) != 2 {
		t.Errorf("expected 2 uploads (conanfile.py, conanmanifest.txt), got %d: %v", len(uploaded), uploaded)
	}
}

func TestNewPushConanCmd_RecipeOnly_Success(t *testing.T) {
	var uploaded []string
	withConanServer(t, func(w http.ResponseWriter, r *http.Request) {
		uploaded = append(uploaded, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	})

	dir := writeConanDir(t, map[string]string{
		"conanfile.py":              "from conan import ConanFile",
		conanutil.ConanManifestFile: "1700000000\nconanfile.py: abc\n",
		"conan_export.tgz":          "fake-tgz",
	})

	if err := runConanCmd(t, "test-registry", "zlib/1.3@myuser/stable", dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(uploaded) != 3 {
		t.Fatalf("expected 3 uploads, got %d: %v", len(uploaded), uploaded)
	}
	// Last upload must be the manifest (finalization marker).
	if !strings.HasSuffix(uploaded[len(uploaded)-1], "/"+conanutil.ConanManifestFile) {
		t.Errorf("last upload should be conanmanifest.txt, got: %s", uploaded[len(uploaded)-1])
	}
	// Path must contain the reference coordinates + recipe revision.
	if !strings.Contains(uploaded[0], "/conan/v2/conans/zlib/1.3/myuser/stable/revisions/") {
		t.Errorf("unexpected upload path: %s", uploaded[0])
	}
}

func TestNewPushConanCmd_RecipeAndPackage_Success(t *testing.T) {
	var recipePaths, packagePaths []string
	withConanServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/packages/") {
			packagePaths = append(packagePaths, r.URL.Path)
		} else {
			recipePaths = append(recipePaths, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	})

	recipeDir := writeConanDir(t, map[string]string{
		"conanfile.py":              "from conan import ConanFile",
		conanutil.ConanManifestFile: "1700000000\nconanfile.py: abc\n",
	})
	pkgDir := writeConanDir(t, map[string]string{
		"conaninfo.txt":             "[settings]",
		conanutil.ConanManifestFile: "1700000001\nconaninfo.txt: def\n",
		"conan_package.tgz":         "fake-pkg-tgz",
	})
	pkgID := strings.Repeat("a", 40)

	if err := runConanCmd(t, "test-registry", "zlib/1.3", recipeDir,
		"--package-dir", pkgDir, "--package-id", pkgID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recipePaths) != 2 {
		t.Fatalf("expected 2 recipe uploads, got %d: %v", len(recipePaths), recipePaths)
	}
	if len(packagePaths) != 3 {
		t.Fatalf("expected 3 package uploads, got %d: %v", len(packagePaths), packagePaths)
	}
	// Absent user/channel -> "_" placeholder in the path.
	if !strings.Contains(recipePaths[0], "/conan/v2/conans/zlib/1.3/_/_/revisions/") {
		t.Errorf("expected placeholder user/channel, got: %s", recipePaths[0])
	}
	if !strings.Contains(packagePaths[0], "/packages/"+pkgID+"/revisions/") {
		t.Errorf("unexpected package path: %s", packagePaths[0])
	}
}

func TestNewPushConanCmd_ServerError(t *testing.T) {
	withConanServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"exists"}`))
	})
	dir := writeConanDir(t, map[string]string{
		"conanfile.py":              "x",
		conanutil.ConanManifestFile: "1700000000\nconanfile.py: abc\n",
	})
	err := runConanCmd(t, "test-registry", "zlib/1.3", dir)
	if err == nil {
		t.Fatal("expected error for 409 response")
	}
	if !strings.Contains(err.Error(), "failed to push") {
		t.Errorf("error should mention push failure, got: %v", err)
	}
}

func TestNewPushConanCmd_InvalidReference(t *testing.T) {
	withConanServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for invalid reference")
	})
	dir := writeConanDir(t, map[string]string{
		"conanfile.py":              "x",
		conanutil.ConanManifestFile: "1700000000\nconanfile.py: abc\n",
	})
	if err := runConanCmd(t, "test-registry", "not-a-valid-ref", dir); err == nil {
		t.Fatal("expected error for invalid reference")
	}
}

func TestNewPushConanCmd_PackageDirRequiresPackageID(t *testing.T) {
	withConanServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when package-id is missing")
	})
	recipeDir := writeConanDir(t, map[string]string{
		"conanfile.py":              "x",
		conanutil.ConanManifestFile: "1700000000\nconanfile.py: abc\n",
	})
	pkgDir := writeConanDir(t, map[string]string{
		"conaninfo.txt":             "[settings]",
		conanutil.ConanManifestFile: "1700000001\nconaninfo.txt: def\n",
	})
	err := runConanCmd(t, "test-registry", "zlib/1.3", recipeDir, "--package-dir", pkgDir)
	if err == nil {
		t.Fatal("expected error when --package-id is missing")
	}
	if !strings.Contains(err.Error(), "package-id") {
		t.Errorf("error should mention package-id, got: %v", err)
	}
}

func TestNewPushConanCmd_WrongArgCount(t *testing.T) {
	if err := runConanCmd(t, "only-two", "args"); err == nil {
		t.Fatal("expected error for wrong arg count")
	}
}

func TestNewPushConanCmd_ChecksumHeaderSet(t *testing.T) {
	var sawSha1 string
	withConanServer(t, func(w http.ResponseWriter, r *http.Request) {
		if v := r.Header.Get("X-Checksum-Sha1"); v != "" {
			sawSha1 = v
		}
		w.WriteHeader(http.StatusOK)
	})
	dir := writeConanDir(t, map[string]string{
		"conanfile.py":              "x",
		conanutil.ConanManifestFile: "1700000000\nconanfile.py: abc\n",
	})
	if err := runConanCmd(t, "test-registry", "zlib/1.3", dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sawSha1 == "" {
		t.Error("X-Checksum-Sha1 header was not set")
	}
}
