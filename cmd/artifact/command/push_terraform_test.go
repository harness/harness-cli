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
