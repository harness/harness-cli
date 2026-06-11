package command

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
)

// withDebianServer spins up a stub server and points the global config at it
// for the duration of the test, restoring originals on cleanup.
func withDebianServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
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

// writeDebFile creates a temporary .deb file with the given content
func writeDebFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.deb")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write deb file: %v", err)
	}
	return path
}

// writeDscFile creates a temporary .dsc file with valid metadata
func writeDscFile(t *testing.T, source, version string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.dsc")
	content := fmt.Sprintf(`Format: 3.0 (quilt)
Source: %s
Version: %s
Binary: test-package
Maintainer: Test User <test@example.com>
Architecture: any
`, source, version)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write dsc file: %v", err)
	}
	return path
}

// writeTarXzFile creates a temporary tar.xz file
func writeTarXzFile(t *testing.T, filename, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write tar.xz file: %v", err)
	}
	return path
}

// runDebianCmd runs the debian push command directly with the given args
// and returns the resulting error.
func runDebianCmd(t *testing.T, args ...string) error {
	t.Helper()
	cmd := NewPushDebianCmd(&cmdutils.Factory{})
	cmd.SetArgs(args)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	return cmd.Execute()
}

func TestNewPushDebianCmd_Success(t *testing.T) {
	srv := withDebianServer(t, func(w http.ResponseWriter, r *http.Request) {
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

		// Verify query parameters
		query := r.URL.Query()
		if query.Get("distribution") != "focal" {
			t.Errorf("expected distribution=focal, got %s", query.Get("distribution"))
		}
		if query.Get("component") != "main" {
			t.Errorf("expected component=main, got %s", query.Get("component"))
		}

		w.WriteHeader(http.StatusOK)
	})
	_ = srv

	path := writeDebFile(t, "test deb content")
	if err := runDebianCmd(t, "test-registry", path, "--distribution=focal", "--component=main"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewPushDebianCmd_ServerError(t *testing.T) {
	withDebianServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"package exists"}`))
	})

	path := writeDebFile(t, "test deb content")
	err := runDebianCmd(t, "test-registry", path, "--distribution=focal", "--component=main")
	if err == nil {
		t.Fatal("expected error for 409 response")
	}
	if !strings.Contains(err.Error(), "failed to push package") {
		t.Errorf("error should mention upload failure, got: %v", err)
	}
}

func TestNewPushDebianCmd_FileNotFound(t *testing.T) {
	withDebianServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when file is missing")
	})
	err := runDebianCmd(t, "test-registry", "/nonexistent/test.deb", "--distribution=focal", "--component=main")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestNewPushDebianCmd_NotADebFile(t *testing.T) {
	withDebianServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for non-deb file")
	})
	dir := t.TempDir()
	path := filepath.Join(dir, "test.zip")
	if err := os.WriteFile(path, []byte("not a deb"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	err := runDebianCmd(t, "test-registry", path, "--distribution=focal", "--component=main")
	if err == nil {
		t.Fatal("expected error for non-deb extension")
	}
}

func TestNewPushDebianCmd_DirectoryPath(t *testing.T) {
	withDebianServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for directory")
	})
	dir := t.TempDir()
	debDir := filepath.Join(dir, "fake.deb")
	if err := os.Mkdir(debDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	err := runDebianCmd(t, "test-registry", debDir, "--distribution=focal", "--component=main")
	if err == nil {
		t.Fatal("expected error for directory path")
	}
}

func TestNewPushDebianCmd_MissingDistribution(t *testing.T) {
	withDebianServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when distribution is missing")
	})
	path := writeDebFile(t, "test deb content")
	err := runDebianCmd(t, "test-registry", path, "--component=main")
	if err == nil {
		t.Fatal("expected error for missing distribution")
	}
}

func TestNewPushDebianCmd_MissingComponent(t *testing.T) {
	withDebianServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when component is missing")
	})
	path := writeDebFile(t, "test deb content")
	err := runDebianCmd(t, "test-registry", path, "--distribution=focal")
	if err == nil {
		t.Fatal("expected error for missing component")
	}
}

func TestNewPushDebianCmd_WrongArgCount(t *testing.T) {
	if err := runDebianCmd(t, "only-one-arg"); err == nil {
		t.Fatal("expected error for missing second arg")
	}
}

func TestNewPushDebianCmd_ChecksumHeadersSet(t *testing.T) {
	receivedHeaders := make(http.Header)
	srv := withDebianServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Capture all headers
		for key, values := range r.Header {
			for _, value := range values {
				receivedHeaders.Add(key, value)
			}
		}
		w.WriteHeader(http.StatusOK)
	})
	_ = srv

	path := writeDebFile(t, "test deb content for checksums")
	if err := runDebianCmd(t, "test-registry", path, "--distribution=focal", "--component=main"); err != nil {
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

// Tests for Debian Source Package (.dsc) functionality

func TestNewPushDebianCmd_DscSuccess(t *testing.T) {
	uploadCount := 0
	srv := withDebianServer(t, func(w http.ResponseWriter, r *http.Request) {
		uploadCount++
		// Verify checksum headers are present
		if r.Header.Get("X-Checksum-Md5") == "" {
			t.Error("X-Checksum-Md5 header is missing")
		}
		if r.Header.Get("X-Checksum-Sha256") == "" {
			t.Error("X-Checksum-Sha256 header is missing")
		}

		// Verify query parameters
		query := r.URL.Query()
		if query.Get("distribution") != "focal" {
			t.Errorf("expected distribution=focal, got %s", query.Get("distribution"))
		}
		if query.Get("component") != "main" {
			t.Errorf("expected component=main, got %s", query.Get("component"))
		}

		// For src uploads, verify package and version
		if strings.Contains(r.URL.Path, "/src") {
			if query.Get("package") == "" {
				t.Error("package query parameter is missing for src upload")
			}
			if query.Get("version") == "" {
				t.Error("version query parameter is missing for src upload")
			}
		}

		w.WriteHeader(http.StatusOK)
	})
	_ = srv

	dscPath := writeDscFile(t, "test-package", "1.2.3-4")
	tarXzPath := writeTarXzFile(t, "test.tar.xz", "test tar content")

	if err := runDebianCmd(t, "test-registry", dscPath,
		"--distribution=focal", "--component=main", "--source-file="+tarXzPath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have uploaded DSC + tar.xz = 2 files
	if uploadCount != 2 {
		t.Errorf("expected 2 uploads, got %d", uploadCount)
	}
}

func TestNewPushDebianCmd_DscWithOrigTarXz(t *testing.T) {
	uploadCount := 0
	var receivedVersions []string

	srv := withDebianServer(t, func(w http.ResponseWriter, r *http.Request) {
		uploadCount++
		query := r.URL.Query()

		// Capture version for src uploads
		if strings.Contains(r.URL.Path, "/src") {
			receivedVersions = append(receivedVersions, query.Get("version"))
		}

		w.WriteHeader(http.StatusOK)
	})
	_ = srv

	dscPath := writeDscFile(t, "test-package", "1.2.3-4ubuntu1")
	origTarXzPath := writeTarXzFile(t, "test.orig.tar.xz", "upstream source")

	if err := runDebianCmd(t, "test-registry", dscPath,
		"--distribution=focal", "--component=main", "--origin-source-file="+origTarXzPath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have uploaded DSC + orig.tar.xz = 2 files
	if uploadCount != 2 {
		t.Errorf("expected 2 uploads, got %d", uploadCount)
	}

	// Verify upstream version extraction (1.2.3-4ubuntu1 -> 1.2.3)
	if len(receivedVersions) > 0 && receivedVersions[0] != "1.2.3" {
		t.Errorf("expected upstream version 1.2.3, got %s", receivedVersions[0])
	}
}

func TestNewPushDebianCmd_DscBothSourceFiles(t *testing.T) {
	uploadCount := 0
	srv := withDebianServer(t, func(w http.ResponseWriter, r *http.Request) {
		uploadCount++
		w.WriteHeader(http.StatusOK)
	})
	_ = srv

	dscPath := writeDscFile(t, "test-package", "2.0.0-1")
	tarXzPath := writeTarXzFile(t, "test.debian.tar.xz", "debian patches")
	origTarXzPath := writeTarXzFile(t, "test.orig.tar.xz", "upstream source")

	if err := runDebianCmd(t, "test-registry", dscPath,
		"--distribution=focal", "--component=main",
		"--source-file="+tarXzPath, "--origin-source-file="+origTarXzPath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have uploaded DSC + tar.xz + orig.tar.xz = 3 files
	if uploadCount != 3 {
		t.Errorf("expected 3 uploads, got %d", uploadCount)
	}
}

func TestNewPushDebianCmd_DscMissingDscFile(t *testing.T) {
	withDebianServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when dsc file is missing")
	})
	err := runDebianCmd(t, "test-registry", "/nonexistent/test.dsc",
		"--distribution=focal", "--component=main", "--source-file=/tmp/test.tar.xz")
	if err == nil {
		t.Fatal("expected error for missing dsc file")
	}
}

func TestNewPushDebianCmd_DscNoSourceFiles(t *testing.T) {
	withDebianServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when no source files provided")
	})

	dscPath := writeDscFile(t, "test-package", "1.0.0-1")
	err := runDebianCmd(t, "test-registry", dscPath,
		"--distribution=focal", "--component=main")
	if err == nil {
		t.Fatal("expected error for missing source files")
	}
	if !strings.Contains(err.Error(), "at least one") {
		t.Errorf("error should mention missing source files, got: %v", err)
	}
}

func TestNewPushDebianCmd_DscInvalidDscFile(t *testing.T) {
	withDebianServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for invalid dsc file")
	})

	dir := t.TempDir()
	dscPath := filepath.Join(dir, "test.dsc")
	// Write invalid DSC without Source/Version fields
	if err := os.WriteFile(dscPath, []byte("Invalid content"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	tarXzPath := writeTarXzFile(t, "test.tar.xz", "content")

	err := runDebianCmd(t, "test-registry", dscPath,
		"--distribution=focal", "--component=main", "--source-file="+tarXzPath)
	if err == nil {
		t.Fatal("expected error for invalid dsc file")
	}
}

func TestNewPushDebianCmd_UnsupportedFileExtension(t *testing.T) {
	withDebianServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for unsupported file type")
	})

	dir := t.TempDir()
	path := filepath.Join(dir, "test.rpm")
	if err := os.WriteFile(path, []byte("not supported"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	err := runDebianCmd(t, "test-registry", path, "--distribution=focal", "--component=main")
	if err == nil {
		t.Fatal("expected error for unsupported file extension")
	}
	if !strings.Contains(err.Error(), "must be either .deb or .dsc") {
		t.Errorf("error should mention supported file types, got: %v", err)
	}
}

func TestParseDscFile(t *testing.T) {
	dscPath := writeDscFile(t, "mypackage", "1.2.3-4ubuntu1")

	metadata, err := parseDscFile(dscPath)
	if err != nil {
		t.Fatalf("parseDscFile failed: %v", err)
	}

	if metadata.Source != "mypackage" {
		t.Errorf("expected Source=mypackage, got %s", metadata.Source)
	}
	if metadata.Version != "1.2.3-4ubuntu1" {
		t.Errorf("expected Version=1.2.3-4ubuntu1, got %s", metadata.Version)
	}
}
