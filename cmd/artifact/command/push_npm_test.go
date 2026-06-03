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

// withNpmServer spins up a stub server and points the global config at it
// for the duration of the test, restoring originals on cleanup.
func withNpmServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
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

// writeNpmTarball creates a temporary .tgz file with the given entries
func writeNpmTarball(t *testing.T, entries map[string]string) string {
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
	path := filepath.Join(dir, "package.tgz")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write tarball: %v", err)
	}
	return path
}

// npmCmdArgs runs the npm push command directly with the given args
// and returns the resulting error.
func runNpmCmd(t *testing.T, args ...string) error {
	t.Helper()
	cmd := NewPushNpmCmd(&cmdutils.Factory{})
	cmd.SetArgs(args)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	return cmd.Execute()
}

func TestNewPushNpmCmd_Success(t *testing.T) {
	srv := withNpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		// NPM first checks if package exists (GET request)
		if r.Method == http.MethodGet {
			// Return 404 to indicate package doesn't exist
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Then uploads the package (PUT request)
		// Verify checksum headers are present on upload
		if r.Header.Get("X-Checksum-Md5") == "" {
			t.Error("X-Checksum-Md5 header is missing")
		}
		if r.Header.Get("X-Checksum-Sha1") == "" {
			t.Error("X-Checksum-Sha1 header is missing")
		}
		if r.Header.Get("X-Checksum-Sha256") == "" {
			t.Error("X-Checksum-Sha256 header is missing")
		}
		if r.Header.Get("X-Checksum-Sha512") == "" {
			t.Error("X-Checksum-Sha512 header is missing")
		}
		w.WriteHeader(http.StatusOK)
	})
	_ = srv

	path := writeNpmTarball(t, map[string]string{
		"package/package.json": `{
  "name": "test-package",
  "version": "1.0.0",
  "description": "A test package"
}`,
	})
	if err := runNpmCmd(t, "test-registry", path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewPushNpmCmd_ServerError(t *testing.T) {
	withNpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"version exists"}`))
	})

	path := writeNpmTarball(t, map[string]string{
		"package/package.json": `{
  "name": "test-package",
  "version": "1.0.0"
}`,
	})
	err := runNpmCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for 409 response")
	}
	if !strings.Contains(err.Error(), "failed to upload NPM package") && !strings.Contains(err.Error(), "409") {
		t.Errorf("error should mention upload failure, got: %v", err)
	}
}

func TestNewPushNpmCmd_FileNotFound(t *testing.T) {
	withNpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when file is missing")
	})
	err := runNpmCmd(t, "test-registry", "/nonexistent/package.tgz")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestNewPushNpmCmd_NotATarball(t *testing.T) {
	withNpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for non-tarball")
	})
	dir := t.TempDir()
	path := filepath.Join(dir, "package.zip")
	if err := os.WriteFile(path, []byte("not a tarball"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	err := runNpmCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for non-tarball extension")
	}
}

func TestNewPushNpmCmd_DirectoryPath(t *testing.T) {
	withNpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for directory")
	})
	dir := t.TempDir()
	tarballDir := filepath.Join(dir, "fake.tgz")
	if err := os.Mkdir(tarballDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	err := runNpmCmd(t, "test-registry", tarballDir)
	if err == nil {
		t.Fatal("expected error for directory path")
	}
}

func TestNewPushNpmCmd_MissingPackageJson(t *testing.T) {
	withNpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when package.json is missing")
	})
	path := writeNpmTarball(t, map[string]string{
		"package/README.md": "no package.json here",
	})
	err := runNpmCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for missing package.json")
	}
}

func TestNewPushNpmCmd_EmptyPackageJsonFields(t *testing.T) {
	withNpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when package.json is empty")
	})
	path := writeNpmTarball(t, map[string]string{
		"package/package.json": `{
  "name": "",
  "version": ""
}`,
	})
	err := runNpmCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for empty name/version")
	}
	if !strings.Contains(err.Error(), "package.json must contain") && !strings.Contains(err.Error(), "name") {
		t.Errorf("error should mention validation requirement, got: %v", err)
	}
}

func TestNewPushNpmCmd_WrongArgCount(t *testing.T) {
	if err := runNpmCmd(t, "only-one-arg"); err == nil {
		t.Fatal("expected error for missing second arg")
	}
}

func TestNewPushNpmCmd_ChecksumHeadersSet(t *testing.T) {
	receivedHeaders := make(http.Header)
	srv := withNpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Capture all headers from upload request
		for key, values := range r.Header {
			for _, value := range values {
				receivedHeaders.Add(key, value)
			}
		}
		w.WriteHeader(http.StatusOK)
	})
	_ = srv

	path := writeNpmTarball(t, map[string]string{
		"package/package.json": `{
  "name": "test-package",
  "version": "1.0.0"
}`,
	})
	if err := runNpmCmd(t, "test-registry", path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify checksum headers are set
	if receivedHeaders.Get("X-Checksum-Md5") == "" {
		t.Error("X-Checksum-Md5 header was not set")
	}
	if receivedHeaders.Get("X-Checksum-Sha1") == "" {
		t.Error("X-Checksum-Sha1 header was not set")
	}
	if receivedHeaders.Get("X-Checksum-Sha256") == "" {
		t.Error("X-Checksum-Sha256 header was not set")
	}
	if receivedHeaders.Get("X-Checksum-Sha512") == "" {
		t.Error("X-Checksum-Sha512 header was not set")
	}
}
