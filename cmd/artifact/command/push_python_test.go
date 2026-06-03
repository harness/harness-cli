package command

import (
	"archive/tar"
	"archive/zip"
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

// withPythonServer spins up a stub server and points the global config at it
// for the duration of the test, restoring originals on cleanup.
func withPythonServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
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

// writePythonWhl creates a temporary .whl file with METADATA
func writePythonWhl(t *testing.T, name, version string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name+"-"+version+"-py3-none-any.whl")

	zipFile, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	defer zipFile.Close()

	zw := zip.NewWriter(zipFile)
	defer zw.Close()

	// Create METADATA file inside .dist-info directory
	metadata := "Name: " + name + "\nVersion: " + version + "\n"
	w, err := zw.Create(name + "-" + version + ".dist-info/METADATA")
	if err != nil {
		t.Fatalf("create metadata: %v", err)
	}
	if _, err := w.Write([]byte(metadata)); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	return path
}

// writePythonTarGz creates a temporary .tar.gz file with PKG-INFO
func writePythonTarGz(t *testing.T, name, version string) string {
	t.Helper()
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	// Create PKG-INFO
	pkgInfo := "Name: " + name + "\nVersion: " + version + "\n"
	hdr := &tar.Header{
		Name: name + "-" + version + "/PKG-INFO",
		Mode: 0o644,
		Size: int64(len(pkgInfo)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := tw.Write([]byte(pkgInfo)); err != nil {
		t.Fatalf("write body: %v", err)
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, name+"-"+version+".tar.gz")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write tarball: %v", err)
	}
	return path
}

// runPythonCmd runs the python push command directly with the given args
// and returns the resulting error.
func runPythonCmd(t *testing.T, args ...string) error {
	t.Helper()
	cmd := NewPushPythonCmd(&cmdutils.Factory{})
	cmd.SetArgs(args)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	return cmd.Execute()
}

func TestNewPushPythonCmd_Success_Whl(t *testing.T) {
	srv := withPythonServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Verify checksum headers are present
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

	path := writePythonWhl(t, "test-package", "1.0.0")
	if err := runPythonCmd(t, "test-registry", path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewPushPythonCmd_Success_TarGz(t *testing.T) {
	srv := withPythonServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Verify checksum headers are present
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

	path := writePythonTarGz(t, "test-package", "1.0.0")
	if err := runPythonCmd(t, "test-registry", path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewPushPythonCmd_Success_Directory(t *testing.T) {
	var uploadCount int
	srv := withPythonServer(t, func(w http.ResponseWriter, r *http.Request) {
		uploadCount++
		w.WriteHeader(http.StatusOK)
	})
	_ = srv

	dir := t.TempDir()
	// Create multiple python packages in directory
	whl1 := writePythonWhl(t, "pkg1", "1.0.0")
	whl2 := writePythonWhl(t, "pkg2", "2.0.0")
	targz1 := writePythonTarGz(t, "pkg3", "3.0.0")

	// Move them to the test directory
	os.Rename(whl1, filepath.Join(dir, filepath.Base(whl1)))
	os.Rename(whl2, filepath.Join(dir, filepath.Base(whl2)))
	os.Rename(targz1, filepath.Join(dir, filepath.Base(targz1)))

	if err := runPythonCmd(t, "test-registry", dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if uploadCount != 3 {
		t.Errorf("expected 3 uploads, got %d", uploadCount)
	}
}

func TestNewPushPythonCmd_ServerError(t *testing.T) {
	withPythonServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"version exists"}`))
	})

	path := writePythonWhl(t, "test-package", "1.0.0")
	err := runPythonCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for 409 response")
	}
	if !strings.Contains(err.Error(), "failed to upload") && !strings.Contains(err.Error(), "409") {
		t.Errorf("error should mention upload failure, got: %v", err)
	}
}

func TestNewPushPythonCmd_FileNotFound(t *testing.T) {
	withPythonServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when file is missing")
	})
	err := runPythonCmd(t, "test-registry", "/nonexistent/package.whl")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestNewPushPythonCmd_UnsupportedExtension(t *testing.T) {
	withPythonServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for unsupported file")
	})
	dir := t.TempDir()
	path := filepath.Join(dir, "package.zip")
	if err := os.WriteFile(path, []byte("not a python package"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	err := runPythonCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for unsupported extension")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("error should mention unsupported format, got: %v", err)
	}
}

func TestNewPushPythonCmd_DirectoryPath(t *testing.T) {
	withPythonServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for directory with .whl extension")
	})
	dir := t.TempDir()
	whlDir := filepath.Join(dir, "fake.whl")
	if err := os.Mkdir(whlDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	err := runPythonCmd(t, "test-registry", whlDir)
	if err == nil {
		t.Fatal("expected error for directory path")
	}
}

func TestNewPushPythonCmd_EmptyDirectory(t *testing.T) {
	withPythonServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for empty directory")
	})
	dir := t.TempDir()
	err := runPythonCmd(t, "test-registry", dir)
	if err == nil {
		t.Fatal("expected error for empty directory")
	}
	if !strings.Contains(err.Error(), "No python packages found") {
		t.Errorf("error should mention no packages found, got: %v", err)
	}
}

func TestNewPushPythonCmd_MissingMetadata_Whl(t *testing.T) {
	withPythonServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when metadata is missing")
	})

	dir := t.TempDir()
	path := filepath.Join(dir, "test-1.0.0-py3-none-any.whl")

	zipFile, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	defer zipFile.Close()

	zw := zip.NewWriter(zipFile)
	// Create a file but not METADATA
	w, _ := zw.Create("test-1.0.0.dist-info/RECORD")
	w.Write([]byte("dummy"))
	zw.Close()

	err = runPythonCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for missing metadata")
	}
	if !strings.Contains(err.Error(), "METADATA not found") {
		t.Errorf("error should mention missing metadata, got: %v", err)
	}
}

func TestNewPushPythonCmd_MissingMetadata_TarGz(t *testing.T) {
	withPythonServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when metadata is missing")
	})

	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	// Create a file but not PKG-INFO
	hdr := &tar.Header{Name: "test-1.0.0/README.md", Mode: 0o644, Size: 5}
	tw.WriteHeader(hdr)
	tw.Write([]byte("dummy"))
	tw.Close()
	gzw.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "test-1.0.0.tar.gz")
	os.WriteFile(path, buf.Bytes(), 0o644)

	err := runPythonCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for missing PKG-INFO")
	}
	if !strings.Contains(err.Error(), "PKG-INFO not found") {
		t.Errorf("error should mention missing PKG-INFO, got: %v", err)
	}
}

func TestNewPushPythonCmd_WrongArgCount(t *testing.T) {
	if err := runPythonCmd(t, "only-one-arg"); err == nil {
		t.Fatal("expected error for missing second arg")
	}
}

func TestNewPushPythonCmd_ChecksumHeadersSet(t *testing.T) {
	receivedHeaders := make(http.Header)
	srv := withPythonServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Capture all headers
		for key, values := range r.Header {
			for _, value := range values {
				receivedHeaders.Add(key, value)
			}
		}
		w.WriteHeader(http.StatusOK)
	})
	_ = srv

	path := writePythonWhl(t, "test-package", "1.0.0")
	if err := runPythonCmd(t, "test-registry", path); err != nil {
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
