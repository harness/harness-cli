package command

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
)

// withPuppetServer spins up a stub server and points the global config at it
// for the duration of the test, restoring originals on cleanup.
func withPuppetServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
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

func writePuppetTarball(t *testing.T, entries map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	for name, body := range entries {
		hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatalf("write body: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "module.tar.gz")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write tarball: %v", err)
	}
	return path
}

func TestIsPuppetTarball(t *testing.T) {
	cases := map[string]bool{
		"foo.tar.gz":         true,
		"FOO.TAR.GZ":         true,
		"module-1.0.0.tgz":   true,
		"module-1.0.0.zip":   false,
		"module-1.0.0.tar":   false,
		"module-1.0.0.tar.b": false,
	}
	for in, want := range cases {
		if got := isPuppetTarball(in); got != want {
			t.Errorf("isPuppetTarball(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestExtractPuppetMetadata_TopLevel(t *testing.T) {
	path := writePuppetTarball(t, map[string]string{
		"puppetlabs-apache-5.9.0/metadata.json": `{"name":"puppetlabs-apache","version":"5.9.0","author":"puppetlabs"}`,
		"puppetlabs-apache-5.9.0/README.md":     "readme",
	})
	meta, err := extractPuppetMetadata(path)
	if err != nil {
		t.Fatalf("extractPuppetMetadata: %v", err)
	}
	if meta.Name != "puppetlabs-apache" || meta.Version != "5.9.0" {
		t.Errorf("got %+v", meta)
	}
}

func TestExtractPuppetMetadata_SkipsNested(t *testing.T) {
	// metadata.json nested in a fixture must not be picked up; only
	// the top-level one (one separator deep) should match.
	path := writePuppetTarball(t, map[string]string{
		"puppetlabs-apache-5.9.0/spec/fixtures/metadata.json": `{"name":"x","version":"0.0.1"}`,
		"puppetlabs-apache-5.9.0/metadata.json":               `{"name":"puppetlabs-apache","version":"5.9.0"}`,
	})
	meta, err := extractPuppetMetadata(path)
	if err != nil {
		t.Fatalf("extractPuppetMetadata: %v", err)
	}
	if meta.Name != "puppetlabs-apache" {
		t.Errorf("nested metadata leaked: %+v", meta)
	}
}

func TestExtractPuppetMetadata_Missing(t *testing.T) {
	path := writePuppetTarball(t, map[string]string{
		"puppetlabs-apache-5.9.0/README.md": "no metadata",
	})
	if _, err := extractPuppetMetadata(path); err == nil {
		t.Fatal("expected error for missing metadata.json")
	}
}

func TestExtractPuppetMetadata_BadGzip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.tar.gz")
	if err := os.WriteFile(path, []byte("not gzip data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := extractPuppetMetadata(path); err == nil {
		t.Fatal("expected error for non-gzip file")
	}
}

func TestExtractPuppetMetadata_BadJSON(t *testing.T) {
	path := writePuppetTarball(t, map[string]string{
		"puppetlabs-apache-5.9.0/metadata.json": `{not valid json`,
	})
	if _, err := extractPuppetMetadata(path); err == nil {
		t.Fatal("expected error for malformed metadata.json")
	}
}

func TestExtractPuppetMetadata_OpenError(t *testing.T) {
	if _, err := extractPuppetMetadata("/nonexistent/path/file.tar.gz"); err == nil {
		t.Fatal("expected error for missing file")
	}
}

// puppetCmdArgs runs the puppet push command directly with the given args
// and returns the resulting error.
func runPuppetCmd(t *testing.T, args ...string) error {
	t.Helper()
	cmd := NewPushPuppetCmd(&cmdutils.Factory{})
	cmd.SetArgs(args)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	return cmd.Execute()
}

func TestNewPushPuppetCmd_Success(t *testing.T) {
	srv := withPuppetServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/puppet/upload") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	})
	_ = srv

	path := writePuppetTarball(t, map[string]string{
		"puppetlabs-apache-5.9.0/metadata.json": `{"name":"puppetlabs-apache","version":"5.9.0"}`,
	})
	if err := runPuppetCmd(t, "test-registry", path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewPushPuppetCmd_ServerError(t *testing.T) {
	withPuppetServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"version exists"}`))
	})

	path := writePuppetTarball(t, map[string]string{
		"puppetlabs-apache-5.9.0/metadata.json": `{"name":"puppetlabs-apache","version":"5.9.0"}`,
	})
	err := runPuppetCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for 409 response")
	}
	if !strings.Contains(err.Error(), "failed to upload") {
		t.Errorf("error should mention upload failure, got: %v", err)
	}
}

func TestNewPushPuppetCmd_FileNotFound(t *testing.T) {
	withPuppetServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when file is missing")
	})
	err := runPuppetCmd(t, "test-registry", "/nonexistent/module.tar.gz")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestNewPushPuppetCmd_NotATarball(t *testing.T) {
	withPuppetServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for non-tarball")
	})
	dir := t.TempDir()
	path := filepath.Join(dir, "module.zip")
	if err := os.WriteFile(path, []byte("not a tarball"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	err := runPuppetCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for non-tarball extension")
	}
	if !strings.Contains(err.Error(), "tarball") {
		t.Errorf("error should mention tarball, got: %v", err)
	}
}

func TestNewPushPuppetCmd_DirectoryPath(t *testing.T) {
	withPuppetServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for directory")
	})
	dir := t.TempDir()
	tarballDir := filepath.Join(dir, "fake.tar.gz")
	if err := os.Mkdir(tarballDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	err := runPuppetCmd(t, "test-registry", tarballDir)
	if err == nil {
		t.Fatal("expected error for directory path")
	}
}

func TestNewPushPuppetCmd_MissingMetadata(t *testing.T) {
	withPuppetServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when metadata is missing")
	})
	path := writePuppetTarball(t, map[string]string{
		"puppetlabs-apache-5.9.0/README.md": "no metadata here",
	})
	err := runPuppetCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for missing metadata.json")
	}
}

func TestNewPushPuppetCmd_EmptyMetadataFields(t *testing.T) {
	withPuppetServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when metadata is empty")
	})
	path := writePuppetTarball(t, map[string]string{
		"puppetlabs-apache-5.9.0/metadata.json": `{"name":"","version":""}`,
	})
	err := runPuppetCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for empty name/version")
	}
	if !strings.Contains(err.Error(), "non-empty") {
		t.Errorf("error should mention non-empty requirement, got: %v", err)
	}
}

func TestNewPushPuppetCmd_WrongArgCount(t *testing.T) {
	if err := runPuppetCmd(t, "only-one-arg"); err == nil {
		t.Fatal("expected error for missing second arg")
	}
}
