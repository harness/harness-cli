package command

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
)

// withRpmServer spins up a stub server and points the global config at it
// for the duration of the test, restoring originals on cleanup.
func withRpmServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
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

// writeRpmFile creates a temporary .rpm file with the given content
func writeRpmFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.rpm")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write rpm file: %v", err)
	}
	return path
}

// rpmCmdArgs runs the rpm push command directly with the given args
// and returns the resulting error.
func runRpmCmd(t *testing.T, args ...string) error {
	t.Helper()
	cmd := NewPushRpmCmd(&cmdutils.Factory{})
	cmd.SetArgs(args)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	return cmd.Execute()
}

func TestNewPushRpmCmd_Success(t *testing.T) {
	srv := withRpmServer(t, func(w http.ResponseWriter, r *http.Request) {
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

	path := writeRpmFile(t, "test rpm content")
	if err := runRpmCmd(t, "test-registry", path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewPushRpmCmd_ServerError(t *testing.T) {
	withRpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"version exists"}`))
	})

	path := writeRpmFile(t, "test rpm content")
	err := runRpmCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for 409 response")
	}
	if !strings.Contains(err.Error(), "failed to push package") {
		t.Errorf("error should mention upload failure, got: %v", err)
	}
}

func TestNewPushRpmCmd_FileNotFound(t *testing.T) {
	withRpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when file is missing")
	})
	err := runRpmCmd(t, "test-registry", "/nonexistent/test.rpm")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestNewPushRpmCmd_NotAnRpm(t *testing.T) {
	withRpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for non-rpm file")
	})
	dir := t.TempDir()
	path := filepath.Join(dir, "test.zip")
	if err := os.WriteFile(path, []byte("not an rpm"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	err := runRpmCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for non-rpm extension")
	}
}

func TestNewPushRpmCmd_DirectoryPath(t *testing.T) {
	withRpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for directory")
	})
	dir := t.TempDir()
	rpmDir := filepath.Join(dir, "fake.rpm")
	if err := os.Mkdir(rpmDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	err := runRpmCmd(t, "test-registry", rpmDir)
	if err == nil {
		t.Fatal("expected error for directory path")
	}
}

func TestNewPushRpmCmd_WrongArgCount(t *testing.T) {
	if err := runRpmCmd(t, "only-one-arg"); err == nil {
		t.Fatal("expected error for missing second arg")
	}
}

func TestNewPushRpmCmd_ChecksumHeadersSet(t *testing.T) {
	receivedHeaders := make(http.Header)
	srv := withRpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Capture all headers
		for key, values := range r.Header {
			for _, value := range values {
				receivedHeaders.Add(key, value)
			}
		}
		w.WriteHeader(http.StatusOK)
	})
	_ = srv

	path := writeRpmFile(t, "test rpm content for checksums")
	if err := runRpmCmd(t, "test-registry", path); err != nil {
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
