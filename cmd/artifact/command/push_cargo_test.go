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

// withCargoServer spins up a stub server and points the global config at it
// for the duration of the test, restoring originals on cleanup.
func withCargoServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
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

// writeCargoFile creates a temporary .crate file (gzipped tarball) with Cargo.toml
func writeCargoFile(t *testing.T) string {
	t.Helper()
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	// Create Cargo.toml with package metadata
	cargoToml := `[package]
name = "test-package"
version = "1.0.0"
authors = ["Test Author"]
`
	hdr := &tar.Header{Name: "test-package-1.0.0/Cargo.toml", Mode: 0o644, Size: int64(len(cargoToml))}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := tw.Write([]byte(cargoToml)); err != nil {
		t.Fatalf("write body: %v", err)
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "test.crate")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write cargo file: %v", err)
	}
	return path
}

// cargoCmdArgs runs the cargo push command directly with the given args
// and returns the resulting error.
func runCargoCmd(t *testing.T, args ...string) error {
	t.Helper()
	cmd := NewPushCargoCmd(&cmdutils.Factory{})
	cmd.SetArgs(args)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	return cmd.Execute()
}

func TestNewPushCargoCmd_Success(t *testing.T) {
	srv := withCargoServer(t, func(w http.ResponseWriter, r *http.Request) {
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

	path := writeCargoFile(t)
	if err := runCargoCmd(t, "test-registry", path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewPushCargoCmd_ServerError(t *testing.T) {
	withCargoServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"version exists"}`))
	})

	path := writeCargoFile(t)
	err := runCargoCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for 409 response")
	}
	if !strings.Contains(err.Error(), "failed to push package") && !strings.Contains(err.Error(), "409") {
		t.Errorf("error should mention upload failure, got: %v", err)
	}
}

func TestNewPushCargoCmd_FileNotFound(t *testing.T) {
	withCargoServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when file is missing")
	})
	err := runCargoCmd(t, "test-registry", "/nonexistent/test.crate")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestNewPushCargoCmd_NotACrate(t *testing.T) {
	withCargoServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for non-crate file")
	})
	dir := t.TempDir()
	path := filepath.Join(dir, "test.tar")
	if err := os.WriteFile(path, []byte("not a crate"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	err := runCargoCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for non-crate extension")
	}
}

func TestNewPushCargoCmd_DirectoryPath(t *testing.T) {
	withCargoServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for directory")
	})
	dir := t.TempDir()
	crateDir := filepath.Join(dir, "fake.crate")
	if err := os.Mkdir(crateDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	err := runCargoCmd(t, "test-registry", crateDir)
	if err == nil {
		t.Fatal("expected error for directory path")
	}
}

func TestNewPushCargoCmd_WrongArgCount(t *testing.T) {
	if err := runCargoCmd(t, "only-one-arg"); err == nil {
		t.Fatal("expected error for missing second arg")
	}
}

func TestNewPushCargoCmd_ChecksumHeadersSet(t *testing.T) {
	receivedHeaders := make(http.Header)
	srv := withCargoServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Capture all headers
		for key, values := range r.Header {
			for _, value := range values {
				receivedHeaders.Add(key, value)
			}
		}
		w.WriteHeader(http.StatusOK)
	})
	_ = srv

	path := writeCargoFile(t)
	if err := runCargoCmd(t, "test-registry", path); err != nil {
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
