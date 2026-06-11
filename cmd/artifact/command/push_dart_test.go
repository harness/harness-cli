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
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/util/common/auth"
)

// withDartServer spins up a stub server and points the global config at it
// for the duration of the test, restoring originals on cleanup.
func withDartServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
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

// writeDartTarball creates a temporary .tar.gz file with the given entries
func writeDartTarball(t *testing.T, entries map[string]string) string {
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
	path := filepath.Join(dir, "package.tar.gz")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write tarball: %v", err)
	}
	return path
}

// dartCmdArgs runs the dart push command directly with the given args
// and returns the resulting error.
func runDartCmd(t *testing.T, args ...string) error {
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
	cmd := NewPushDartCmd(factory)
	cmd.SetArgs(args)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	return cmd.Execute()
}

func TestNewPushDartCmd_Success(t *testing.T) {
	srv := withDartServer(t, func(w http.ResponseWriter, r *http.Request) {
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

	path := writeDartTarball(t, map[string]string{
		"pubspec.yaml": `name: test_package
version: 1.0.0
description: A test package`,
	})
	if err := runDartCmd(t, "test-registry", path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewPushDartCmd_ServerError(t *testing.T) {
	withDartServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"version exists"}`))
	})

	path := writeDartTarball(t, map[string]string{
		"pubspec.yaml": `name: test_package
version: 1.0.0`,
	})
	err := runDartCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for 409 response")
	}
	if !strings.Contains(err.Error(), "failed to upload Dart package") {
		t.Errorf("error should mention upload failure, got: %v", err)
	}
}

func TestNewPushDartCmd_FileNotFound(t *testing.T) {
	withDartServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when file is missing")
	})
	err := runDartCmd(t, "test-registry", "/nonexistent/package.tar.gz")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestNewPushDartCmd_NotATarball(t *testing.T) {
	withDartServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for non-tarball")
	})
	dir := t.TempDir()
	path := filepath.Join(dir, "package.zip")
	if err := os.WriteFile(path, []byte("not a tarball"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	err := runDartCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for non-tarball extension")
	}
}

func TestNewPushDartCmd_DirectoryPath(t *testing.T) {
	withDartServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for directory")
	})
	dir := t.TempDir()
	tarballDir := filepath.Join(dir, "fake.tar.gz")
	if err := os.Mkdir(tarballDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	err := runDartCmd(t, "test-registry", tarballDir)
	if err == nil {
		t.Fatal("expected error for directory path")
	}
}

func TestNewPushDartCmd_MissingPubspec(t *testing.T) {
	withDartServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when pubspec is missing")
	})
	path := writeDartTarball(t, map[string]string{
		"README.md": "no pubspec here",
	})
	err := runDartCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for missing pubspec.yaml")
	}
}

func TestNewPushDartCmd_EmptyPubspecFields(t *testing.T) {
	withDartServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when pubspec is empty")
	})
	path := writeDartTarball(t, map[string]string{
		"pubspec.yaml": `name: ""
version: ""`,
	})
	err := runDartCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for empty name/version")
	}
	if !strings.Contains(err.Error(), "non-empty") {
		t.Errorf("error should mention non-empty requirement, got: %v", err)
	}
}

func TestNewPushDartCmd_WrongArgCount(t *testing.T) {
	if err := runDartCmd(t, "only-one-arg"); err == nil {
		t.Fatal("expected error for missing second arg")
	}
}

func TestNewPushDartCmd_ChecksumHeadersSet(t *testing.T) {
	receivedHeaders := make(http.Header)
	srv := withDartServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Capture all headers
		for key, values := range r.Header {
			for _, value := range values {
				receivedHeaders.Add(key, value)
			}
		}
		w.WriteHeader(http.StatusOK)
	})
	_ = srv

	path := writeDartTarball(t, map[string]string{
		"pubspec.yaml": `name: test_package
version: 1.0.0`,
	})
	if err := runDartCmd(t, "test-registry", path); err != nil {
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
