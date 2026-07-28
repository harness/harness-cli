package command

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	p "github.com/harness/harness-cli/util/common/progress"
)

// withTerraformServer spins up a stub server and points the global config at
// it for the duration of the test, restoring originals on cleanup.
func withTerraformServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
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

func writeTempFile(t *testing.T, name string, body []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return path
}

func runTerraformCmd(t *testing.T, args ...string) error {
	t.Helper()
	factory := &cmdutils.Factory{}
	cmd := NewPushTerraformCmd(factory)
	cmd.SetArgs(args)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	return cmd.Execute()
}

func TestIsTerraformModule(t *testing.T) {
	cases := map[string]bool{
		"module.tar.gz":    true,
		"MODULE.TAR.GZ":    true,
		"module-1.0.0.tgz": true,
		"module-1.0.0.zip": false,
		"module-1.0.0.tar": false,
		"terraform-provider-aws_1.0.0_linux_amd64.zip": false,
	}
	for in, want := range cases {
		if got := isTerraformModule(in); got != want {
			t.Errorf("isTerraformModule(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestIsTerraformProvider(t *testing.T) {
	cases := map[string]bool{
		"terraform-provider-aws_1.0.0_linux_amd64.zip": true,
		"module.tar.gz": false,
		"module.tgz":    false,
		"random.zip":    true, // extension check alone; content validated separately
	}
	for in, want := range cases {
		if got := isTerraformProvider(in); got != want {
			t.Errorf("isTerraformProvider(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseProviderFilename_Valid(t *testing.T) {
	typeName, version, osName, arch, err := parseProviderFilename("terraform-provider-alicloud_0.0.1_linux_amd64.zip")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if typeName != "alicloud" || version != "0.0.1" || osName != "linux" || arch != "amd64" {
		t.Errorf("got type=%s version=%s os=%s arch=%s", typeName, version, osName, arch)
	}
}

func TestParseProviderFilename_PrereleaseVersion(t *testing.T) {
	typeName, version, osName, arch, err := parseProviderFilename("terraform-provider-aws_1.2.3-beta.1_darwin_arm64.zip")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if typeName != "aws" || version != "1.2.3-beta.1" || osName != "darwin" || arch != "arm64" {
		t.Errorf("got type=%s version=%s os=%s arch=%s", typeName, version, osName, arch)
	}
}

func TestParseProviderFilename_Invalid(t *testing.T) {
	cases := []string{
		"not-a-provider.zip",
		"terraform-provider-aws.zip",
		"terraform-provider-aws_1.0.0.zip",
		"terraform-provider-aws_1.0.0_linux.zip",
		"terraform-provider-aws_notsemver_linux_amd64.zip",
	}
	for _, in := range cases {
		if _, _, _, _, err := parseProviderFilename(in); err == nil {
			t.Errorf("parseProviderFilename(%q) expected error, got nil", in)
		}
	}
}

func TestNewPushTerraformCmd_ModuleSuccess(t *testing.T) {
	var gotPath string
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusCreated)
	})

	path := writeTempFile(t, "module.tar.gz", []byte("fake tar.gz content"))
	err := runTerraformCmd(t, "test-registry", path,
		"--namespace", "aliyun", "--name", "vpc", "--provider", "alicloud", "--version", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantPath := "/pkg/test-account/test-registry/terraform/v1/modules/aliyun/vpc/alicloud/1.0.0"
	if gotPath != wantPath {
		t.Errorf("path = %s, want %s", gotPath, wantPath)
	}
}

func TestNewPushTerraformCmd_ModuleMissingFlags(t *testing.T) {
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when flags are missing")
	})
	path := writeTempFile(t, "module.tar.gz", []byte("data"))

	cases := []struct {
		name string
		args []string
	}{
		{"missing name", []string{"test-registry", path, "--namespace", "aliyun", "--provider", "alicloud", "--version", "1.0.0"}},
		{"missing provider", []string{"test-registry", path, "--namespace", "aliyun", "--name", "vpc", "--version", "1.0.0"}},
		{"missing version", []string{"test-registry", path, "--namespace", "aliyun", "--name", "vpc", "--provider", "alicloud"}},
		{"missing namespace", []string{"test-registry", path, "--name", "vpc", "--provider", "alicloud", "--version", "1.0.0"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := runTerraformCmd(t, c.args...); err == nil {
				t.Fatal("expected error for missing required flag")
			}
		})
	}
}

func TestNewPushTerraformCmd_ModuleBadVersion(t *testing.T) {
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for invalid version")
	})
	path := writeTempFile(t, "module.tar.gz", []byte("data"))
	err := runTerraformCmd(t, "test-registry", path,
		"--namespace", "aliyun", "--name", "vpc", "--provider", "alicloud", "--version", "not-a-version")
	if err == nil {
		t.Fatal("expected error for invalid semver")
	}
	if !strings.Contains(err.Error(), "SemVer") {
		t.Errorf("error should mention SemVer, got: %v", err)
	}
}

func TestNewPushTerraformCmd_ProviderSuccess(t *testing.T) {
	var gotPath string
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusCreated)
	})

	path := writeTempFile(t, "terraform-provider-alicloud_0.0.1_linux_amd64.zip", []byte("fake zip content"))
	err := runTerraformCmd(t, "test-registry", path, "--namespace", "aliyun")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantPath := "/pkg/test-account/test-registry/terraform/v1/providers/aliyun/alicloud/0.0.1/terraform-provider-alicloud_0.0.1_linux_amd64.zip"
	if gotPath != wantPath {
		t.Errorf("path = %s, want %s", gotPath, wantPath)
	}
}

func TestNewPushTerraformCmd_ProviderMissingNamespace(t *testing.T) {
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when namespace is missing")
	})
	path := writeTempFile(t, "terraform-provider-alicloud_0.0.1_linux_amd64.zip", []byte("data"))
	err := runTerraformCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for missing namespace")
	}
}

func TestNewPushTerraformCmd_ProviderBadFilename(t *testing.T) {
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for malformed provider filename")
	})
	path := writeTempFile(t, "not-a-provider.zip", []byte("data"))
	err := runTerraformCmd(t, "test-registry", path, "--namespace", "aliyun")
	if err == nil {
		t.Fatal("expected error for malformed filename")
	}
}

// TestNewPushTerraformCmd_ZipWithModuleFlagsRejected asserts that a .zip file
// is never treated as a module upload, even when module identity flags are
// supplied — the module registry only ever accepts .tar.gz/.tgz server-side,
// and a non-provider-named .zip must fail as "unsupported", not be silently
// misrouted through the provider filename parser.
func TestNewPushTerraformCmd_ZipWithModuleFlagsRejected(t *testing.T) {
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit: .zip modules are not supported")
	})
	path := writeTempFile(t, "module.zip", []byte("data"))
	err := runTerraformCmd(t, "test-registry", path,
		"--namespace", "aliyun", "--name", "vpc", "--provider", "alicloud", "--version", "1.0.0")
	if err == nil {
		t.Fatal("expected error: .zip is not a supported module extension")
	}
	if !strings.Contains(err.Error(), "Invalid provider filename") && !strings.Contains(err.Error(), "does not match required convention") {
		t.Errorf("expected provider-filename-convention error, got: %v", err)
	}
}

func TestNewPushTerraformCmd_ServerConflict(t *testing.T) {
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"version exists"}`))
	})
	path := writeTempFile(t, "module.tar.gz", []byte("data"))
	err := runTerraformCmd(t, "test-registry", path,
		"--namespace", "aliyun", "--name", "vpc", "--provider", "alicloud", "--version", "1.0.0")
	if err == nil {
		t.Fatal("expected error for 409 response")
	}
	if !strings.Contains(err.Error(), "failed to upload") {
		t.Errorf("error should mention upload failure, got: %v", err)
	}
}

func TestNewPushTerraformCmd_FileNotFound(t *testing.T) {
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when file is missing")
	})
	err := runTerraformCmd(t, "test-registry", "/nonexistent/module.tar.gz", "--namespace", "aliyun")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestNewPushTerraformCmd_UnsupportedExtension(t *testing.T) {
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for unsupported extension")
	})
	path := writeTempFile(t, "module.txt", []byte("data"))
	err := runTerraformCmd(t, "test-registry", path, "--namespace", "aliyun")
	if err == nil {
		t.Fatal("expected error for unsupported extension")
	}
}

func TestNewPushTerraformCmd_ChecksumHeadersSet(t *testing.T) {
	receivedHeaders := make(http.Header)
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		for key, values := range r.Header {
			for _, value := range values {
				receivedHeaders.Add(key, value)
			}
		}
		w.WriteHeader(http.StatusCreated)
	})

	path := writeTempFile(t, "module.tar.gz", []byte("fake tar.gz content"))
	err := runTerraformCmd(t, "test-registry", path,
		"--namespace", "aliyun", "--name", "vpc", "--provider", "alicloud", "--version", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedHeaders.Get("X-Checksum-Sha256") == "" {
		t.Error("X-Checksum-Sha256 header was not set")
	}
	if receivedHeaders.Get("Content-Type") != "application/octet-stream" {
		t.Errorf("Content-Type = %s, want application/octet-stream", receivedHeaders.Get("Content-Type"))
	}
}

func TestNewPushTerraformCmd_WrongArgCount(t *testing.T) {
	if err := runTerraformCmd(t, "only-one-arg"); err == nil {
		t.Fatal("expected error for missing second arg")
	}
}

func TestNewPushTerraformCmd_ModuleFromDirectory(t *testing.T) {
	var gotPath string
	var gotBody []byte
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
	})

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(`resource "null_resource" "x" {}`), 0o644); err != nil {
		t.Fatalf("write main.tf: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".terraform"), 0o755); err != nil {
		t.Fatalf("mkdir .terraform: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".terraform", "junk"), []byte("junk"), 0o644); err != nil {
		t.Fatalf("write junk: %v", err)
	}

	err := runTerraformCmd(t, "test-registry", dir,
		"--namespace", "aliyun", "--name", "vpc", "--provider", "alicloud", "--version", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantPath := "/pkg/test-account/test-registry/terraform/v1/modules/aliyun/vpc/alicloud/1.0.0"
	if gotPath != wantPath {
		t.Errorf("path = %s, want %s", gotPath, wantPath)
	}

	gzReader, err := gzip.NewReader(bytes.NewReader(gotBody))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	tarReader := tar.NewReader(gzReader)
	var names []string
	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar read: %v", err)
		}
		names = append(names, hdr.Name)
	}
	if !containsName(names, "main.tf") {
		t.Errorf("archive missing main.tf, got entries: %v", names)
	}
	if containsName(names, filepath.ToSlash(filepath.Join(".terraform", "junk"))) {
		t.Errorf("archive should not contain .terraform contents, got entries: %v", names)
	}
}

func containsName(names []string, target string) bool {
	for _, n := range names {
		if n == target {
			return true
		}
	}
	return false
}

func TestNewPushTerraformCmd_DirectoryMissingFlags(t *testing.T) {
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when flags are missing")
	})
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write main.tf: %v", err)
	}
	err := runTerraformCmd(t, "test-registry", dir, "--namespace", "aliyun")
	if err == nil {
		t.Fatal("expected error for missing name/provider/version")
	}
}

func TestNewPushTerraformCmd_DirectoryNoTfFiles(t *testing.T) {
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when directory has no .tf files")
	})
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	err := runTerraformCmd(t, "test-registry", dir,
		"--namespace", "aliyun", "--name", "vpc", "--provider", "alicloud", "--version", "1.0.0")
	if err == nil {
		t.Fatal("expected error for directory with no .tf files")
	}
}

// --- packageModuleDir uncovered branches ---

func TestPackageModuleDir_MissingProvider(t *testing.T) {
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit")
	})
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runTerraformCmd(t, "test-registry", dir,
		"--namespace", "ns", "--name", "mod", "--version", "1.0.0")
	if err == nil || !strings.Contains(err.Error(), "--provider is required") {
		t.Fatalf("expected --provider error, got: %v", err)
	}
}

func TestPackageModuleDir_MissingVersion(t *testing.T) {
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit")
	})
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runTerraformCmd(t, "test-registry", dir,
		"--namespace", "ns", "--name", "mod", "--provider", "aws")
	if err == nil || !strings.Contains(err.Error(), "--version is required") {
		t.Fatalf("expected --version error, got: %v", err)
	}
}

func TestPackageModuleDir_BadSemver(t *testing.T) {
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit")
	})
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runTerraformCmd(t, "test-registry", dir,
		"--namespace", "ns", "--name", "mod", "--provider", "aws", "--version", "bad-ver")
	if err == nil || !strings.Contains(err.Error(), "SemVer") {
		t.Fatalf("expected SemVer error, got: %v", err)
	}
}

func TestPackageModuleDir_TfOnlyInSubdir(t *testing.T) {
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when .tf is only in a subdirectory")
	})
	dir := t.TempDir()
	subdir := filepath.Join(dir, "submodule")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "main.tf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runTerraformCmd(t, "test-registry", dir,
		"--namespace", "ns", "--name", "mod", "--provider", "aws", "--version", "1.0.0")
	if err == nil || !strings.Contains(err.Error(), "root level") {
		t.Fatalf("expected root level error, got: %v", err)
	}
}

func TestPackageModuleDir_TrailingSlash(t *testing.T) {
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runTerraformCmd(t, "test-registry", dir+string(filepath.Separator),
		"--namespace", "ns", "--name", "mod", "--provider", "aws", "--version", "1.0.0")
	if err != nil {
		t.Fatalf("trailing slash should not cause 'no .tf files' error: %v", err)
	}
}

func TestPackageModuleDir_TfOnlyInTerraformDir(t *testing.T) {
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when .tf is only in .terraform/")
	})
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".terraform"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".terraform", "hidden.tf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runTerraformCmd(t, "test-registry", dir,
		"--namespace", "ns", "--name", "mod", "--provider", "aws", "--version", "1.0.0")
	if err == nil || !strings.Contains(err.Error(), "root level") {
		t.Fatalf("expected root level error, got: %v", err)
	}
}

// --- pushTerraformProvider uncovered branches ---

func TestNewPushTerraformCmd_ProviderServerConflict(t *testing.T) {
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"platform already exists"}`))
	})
	path := writeTempFile(t, "terraform-provider-aws_1.0.0_linux_amd64.zip", []byte("data"))
	err := runTerraformCmd(t, "test-registry", path, "--namespace", "aliyun")
	if err == nil || !strings.Contains(err.Error(), "failed to upload") {
		t.Fatalf("expected upload failure error, got: %v", err)
	}
}

func TestNewPushTerraformCmd_ProviderServer401(t *testing.T) {
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Unauthorized"}`))
	})
	path := writeTempFile(t, "terraform-provider-alicloud_0.0.1_linux_amd64.zip", []byte("data"))
	err := runTerraformCmd(t, "test-registry", path, "--namespace", "aliyun")
	if err == nil || !strings.Contains(err.Error(), "failed to upload") {
		t.Fatalf("expected upload failure, got: %v", err)
	}
}

func TestNewPushTerraformCmd_ProviderChecksumHeadersSet(t *testing.T) {
	receivedHeaders := make(http.Header)
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		for k, v := range r.Header {
			for _, val := range v {
				receivedHeaders.Add(k, val)
			}
		}
		w.WriteHeader(http.StatusCreated)
	})
	path := writeTempFile(t, "terraform-provider-alicloud_0.0.1_linux_amd64.zip", []byte("zip content"))
	err := runTerraformCmd(t, "test-registry", path, "--namespace", "aliyun")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedHeaders.Get("X-Checksum-Sha256") == "" {
		t.Error("X-Checksum-Sha256 header was not set for provider upload")
	}
}

// --- writeModuleArchive and archive content ---

func TestWriteModuleArchive_SkipsTfstate(t *testing.T) {
	var gotNames []string
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gzr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			w.WriteHeader(http.StatusCreated)
			return
		}
		tr := tar.NewReader(gzr)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
			gotNames = append(gotNames, hdr.Name)
		}
		w.WriteHeader(http.StatusCreated)
	})

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "terraform.tfstate"), []byte("state"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "terraform.tfstate.backup"), []byte("backup"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := runTerraformCmd(t, "test-registry", dir,
		"--namespace", "ns", "--name", "mod", "--provider", "aws", "--version", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsName(gotNames, "main.tf") {
		t.Errorf("archive missing main.tf, entries: %v", gotNames)
	}
	for _, n := range gotNames {
		if strings.Contains(n, "tfstate") {
			t.Errorf("archive should not contain tfstate files, found: %s", n)
		}
	}
}

func TestWriteModuleArchive_SkipsDSStore(t *testing.T) {
	var gotNames []string
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gzr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			w.WriteHeader(http.StatusCreated)
			return
		}
		tr := tar.NewReader(gzr)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
			gotNames = append(gotNames, hdr.Name)
		}
		w.WriteHeader(http.StatusCreated)
	})

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte("mac junk"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := runTerraformCmd(t, "test-registry", dir,
		"--namespace", "ns", "--name", "mod", "--provider", "aws", "--version", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, n := range gotNames {
		if n == ".DS_Store" {
			t.Errorf("archive should not contain .DS_Store")
		}
	}
}

// --- module server error branches ---

func TestNewPushTerraformCmd_ModuleServer401(t *testing.T) {
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Unauthorized"}`))
	})
	path := writeTempFile(t, "module.tar.gz", []byte("data"))
	err := runTerraformCmd(t, "test-registry", path,
		"--namespace", "aliyun", "--name", "vpc", "--provider", "alicloud", "--version", "1.0.0")
	if err == nil || !strings.Contains(err.Error(), "failed to upload") {
		t.Fatalf("expected upload failure, got: %v", err)
	}
}

func TestNewPushTerraformCmd_ModuleServer500(t *testing.T) {
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"internal error"}`))
	})
	path := writeTempFile(t, "module.tar.gz", []byte("data"))
	err := runTerraformCmd(t, "test-registry", path,
		"--namespace", "aliyun", "--name", "vpc", "--provider", "alicloud", "--version", "1.0.0")
	if err == nil || !strings.Contains(err.Error(), "failed to upload") {
		t.Fatalf("expected upload failure, got: %v", err)
	}
}

// --- URL shape tests ---

func TestNewPushTerraformCmd_ModuleURLShape(t *testing.T) {
	var gotMethod, gotPath string
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})
	path := writeTempFile(t, "mod.tgz", []byte("data"))
	err := runTerraformCmd(t, "reg", path,
		"--namespace", "ns", "--name", "mymod", "--provider", "gcp", "--version", "2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %s, want PUT", gotMethod)
	}
	want := "/pkg/test-account/reg/terraform/v1/modules/ns/mymod/gcp/2.3.4"
	if gotPath != want {
		t.Errorf("path = %s, want %s", gotPath, want)
	}
}

func TestNewPushTerraformCmd_ProviderURLShape(t *testing.T) {
	var gotMethod, gotPath string
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})
	path := writeTempFile(t, "terraform-provider-gcp_2.3.4_windows_amd64.zip", []byte("data"))
	err := runTerraformCmd(t, "reg", path, "--namespace", "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %s, want PUT", gotMethod)
	}
	want := "/pkg/test-account/reg/terraform/v1/providers/ns/gcp/2.3.4/terraform-provider-gcp_2.3.4_windows_amd64.zip"
	if gotPath != want {
		t.Errorf("path = %s, want %s", gotPath, want)
	}
}

// --- direct function tests for error paths ---

func TestPushTerraformModule_ChecksumError(t *testing.T) {
	// Create file so Stat passes, then remove so ComputeFileChecksums fails
	dir := t.TempDir()
	path := filepath.Join(dir, "mod.tar.gz")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(path)

	factory := &cmdutils.Factory{}
	nop := p.NewNopReporter()
	ctx := t.Context()
	err = pushTerraformModule(ctx, factory, nop, "reg", path, info, "ns", "mod", "aws", "1.0.0")
	if err == nil || !strings.Contains(err.Error(), "failed to compute checksums") {
		t.Fatalf("expected checksum error, got: %v", err)
	}
}

func TestPushTerraformProvider_ChecksumError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "terraform-provider-aws_1.0.0_linux_amd64.zip")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(path)

	factory := &cmdutils.Factory{}
	nop := p.NewNopReporter()
	ctx := t.Context()
	err = pushTerraformProvider(ctx, factory, nop, "reg", path, info, "ns")
	if err == nil || !strings.Contains(err.Error(), "failed to compute checksums") {
		t.Fatalf("expected checksum error, got: %v", err)
	}
}

func TestPushTerraformModule_MissingName(t *testing.T) {
	factory := &cmdutils.Factory{}
	nop := p.NewNopReporter()
	ctx := t.Context()
	info, _ := os.Stat(os.TempDir())
	err := pushTerraformModule(ctx, factory, nop, "reg", "/tmp/mod.tar.gz", info, "ns", "", "aws", "1.0.0")
	if err == nil || !strings.Contains(err.Error(), "--name is required") {
		t.Fatalf("expected --name error, got: %v", err)
	}
}

func TestPushTerraformModule_BadSemver(t *testing.T) {
	factory := &cmdutils.Factory{}
	nop := p.NewNopReporter()
	ctx := t.Context()
	info, _ := os.Stat(os.TempDir())
	err := pushTerraformModule(ctx, factory, nop, "reg", "/tmp/mod.tar.gz", info, "ns", "mod", "aws", "not-semver")
	if err == nil || !strings.Contains(err.Error(), "SemVer") {
		t.Fatalf("expected SemVer error, got: %v", err)
	}
}

// --- NewPushTerraformCmd error branches ---

func TestNewPushTerraformCmd_GlobNoMatch(t *testing.T) {
	withTerraformServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit")
	})
	// A glob pattern that matches nothing
	dir := t.TempDir()
	pattern := filepath.Join(dir, "*.tar.gz")
	err := runTerraformCmd(t, "test-registry", pattern,
		"--namespace", "ns", "--name", "mod", "--provider", "aws", "--version", "1.0.0")
	if err == nil {
		t.Fatal("expected error for glob with no matches")
	}
}

// --- packageModuleDir direct tests for hard-to-reach paths ---

func TestPackageModuleDir_WriteArchiveToReadOnlyDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping permission test: running as root")
	}
	// Make the destination (tmpDir inside packageModuleDir) unwritable by making
	// TMPDIR point to a read-only directory so os.MkdirTemp inside the function fails.
	// Instead: provide an unreadable source dir so filepath.Walk errors.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Remove read permission on the directory itself so Walk will error on entry
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	nop := p.NewNopReporter()
	_, err := packageModuleDir(nop, dir, "ns", "mod", "aws", "1.0.0")
	if err == nil {
		t.Fatal("expected error from unreadable directory")
	}
}

func TestPackageModuleDir_WriteArchiveFailure(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping permission test: running as root")
	}
	// Archive write path: create a source dir with a .tf file but make the archive
	// output path non-writable by pointing archive to a read-only dir's subpath.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// We can't easily control where MkdirTemp writes. Instead test writeModuleArchive directly.
	roDir := t.TempDir()
	if err := os.Chmod(roDir, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o755) })

	err := writeModuleArchive(filepath.Join(roDir, "out.tar.gz"), dir)
	if err == nil || !strings.Contains(err.Error(), "failed to create archive file") {
		t.Fatalf("expected archive create failure, got: %v", err)
	}
}

func TestWriteModuleArchive_WalkError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping permission test: running as root")
	}
	// Source dir with a file we can't read during Walk
	srcDir := t.TempDir()
	subDir := filepath.Join(srcDir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "main.tf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "nested.tf"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Remove read on sub so Walk errors when trying to read it
	if err := os.Chmod(subDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(subDir, 0o755) })

	archiveDir := t.TempDir()
	err := writeModuleArchive(filepath.Join(archiveDir, "out.tar.gz"), srcDir)
	if err == nil {
		t.Fatal("expected walk error for unreadable subdirectory")
	}
}

func TestWriteModuleArchive_WithSubdirectory(t *testing.T) {
	// Exercises the info.IsDir() branch inside Walk (line 250) for non-skip dirs
	var gotNames []string
	srcDir := t.TempDir()
	subDir := filepath.Join(srcDir, "modules")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "main.tf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "child.tf"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(t.TempDir(), "out.tar.gz")
	if err := writeModuleArchive(archivePath, srcDir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	f, _ := os.Open(archivePath)
	defer f.Close()
	gzr, _ := gzip.NewReader(f)
	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read archive: %v", err)
		}
		gotNames = append(gotNames, hdr.Name)
	}
	if !containsName(gotNames, "main.tf") {
		t.Errorf("archive missing main.tf, entries: %v", gotNames)
	}
	if !containsName(gotNames, "modules/child.tf") {
		t.Errorf("archive missing modules/child.tf, entries: %v", gotNames)
	}
}

func TestWriteModuleArchive_FileOpenErrorDuringWalk(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping permission test: running as root")
	}
	// Create a file, start Walk (which sees it), then remove it so os.Open fails
	// We need a file that passes the Walk filter but is deleted before Open.
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "main.tf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Make the file unreadable (not deleted, but permission denied on open)
	secretPath := filepath.Join(srcDir, "secret.tf")
	if err := os.WriteFile(secretPath, []byte("s"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(secretPath, 0o644) })

	archiveDir := t.TempDir()
	err := writeModuleArchive(filepath.Join(archiveDir, "out.tar.gz"), srcDir)
	if err == nil {
		t.Fatal("expected error when file cannot be opened")
	}
}
