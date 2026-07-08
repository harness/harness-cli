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
	cmd := NewPushNpmCmd(factory)
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

func TestNewPushNpmCmd_MetadataDownloadError(t *testing.T) {
	// Test lines 147-153: error handling when metadata download fails with non-200/404 status
	srv := withNpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			// Return 500 error for metadata download
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"internal server error"}`))
			return
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
	err := runNpmCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for metadata download failure")
	}
	// Verify error message includes status and response body (lines 147-153)
	if !strings.Contains(err.Error(), "failed to download NPM metadata") {
		t.Errorf("error should mention metadata download failure, got: %v", err)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should include status code, got: %v", err)
	}
}

func TestNewPushNpmCmd_MetadataDownloadBadGateway(t *testing.T) {
	// Test lines 147-153: error handling with 502 Bad Gateway
	srv := withNpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"bad gateway"}`))
			return
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
	err := runNpmCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for bad gateway response")
	}
	if !strings.Contains(err.Error(), "failed to download NPM metadata") {
		t.Errorf("error should mention metadata download failure, got: %v", err)
	}
}

func TestNewPushNpmCmd_PkgHttpClientWithProgress(t *testing.T) {
	// Test line 193: pkgClient = f.PkgHttpClientWithProgress(progress, bufferSize, fileInfo.Name())
	clientCreated := false
	srv := withNpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		clientCreated = true
		w.WriteHeader(http.StatusOK)
	})
	_ = srv

	path := writeNpmTarball(t, map[string]string{
		"package/package.json": `{
  "name": "test-package",
  "version": "1.0.0"
}`,
	})

	// Use the default factory which will call PkgHttpClientWithProgress
	factory := cmdutils.NewFactory()

	cmd := NewPushNpmCmd(factory)
	cmd.SetArgs([]string{"test-registry", path})
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !clientCreated {
		t.Error("PkgHttpClientWithProgress was not called - line 193 not covered")
	}
}

func TestNewPushNpmCmd_PackageAlreadyExists(t *testing.T) {
	// Test when package already exists (GET returns 200)
	srv := withNpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			// Return existing package metadata
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
  "name": "test-package",
  "versions": {
    "1.0.0": {
      "name": "test-package",
      "version": "1.0.0"
    }
  }
}`))
			return
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
	err := runNpmCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error when package version already exists")
	}
	if !strings.Contains(err.Error(), "already exist") {
		t.Errorf("error should mention package already exists, got: %v", err)
	}
}

func TestNewPushNpmCmd_Success_201Created(t *testing.T) {
	srv := withNpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})
	_ = srv

	path := writeNpmTarball(t, map[string]string{
		"package/package.json": `{
  "name": "test-package",
  "version": "1.0.0"
}`,
	})
	if err := runNpmCmd(t, "test-registry", path); err != nil {
		t.Fatalf("expected success on 201, got error: %v", err)
	}
}

func TestNewPushNpmCmd_CustomPkgUrl(t *testing.T) {
	srv := withNpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	path := writeNpmTarball(t, map[string]string{
		"package/package.json": `{
  "name": "test-package",
  "version": "1.0.0"
}`,
	})
	if err := runNpmCmd(t, "test-registry", path, "--pkg-url", srv.URL); err != nil {
		t.Fatalf("unexpected error with custom pkg-url: %v", err)
	}
}

func TestNewPushNpmCmd_ScopedPackage_Success(t *testing.T) {
	// Test lines 200-229: scoped package upload path
	srv := withNpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Verify the URL contains the scoped package structure
		if !strings.Contains(r.URL.Path, "/@test/") {
			t.Errorf("expected scoped package URL structure, got: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	})
	_ = srv

	path := writeNpmTarball(t, map[string]string{
		"package/package.json": `{
  "name": "@test/scoped-package",
  "version": "1.0.0"
}`,
	})
	if err := runNpmCmd(t, "test-registry", path); err != nil {
		t.Fatalf("unexpected error for scoped package: %v", err)
	}
}

func TestNewPushNpmCmd_ScopedPackage_InvalidFormat(t *testing.T) {
	// Test lines 203-206: invalid scoped package format
	// Note: We need to create a package.json with @ but no /,
	// but the check happens after metadata fetch, so we mock that failure path
	srv := withNpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			// Return 404 for metadata check
			w.WriteHeader(http.StatusNotFound)
			return
		}
		t.Fatal("server should not be hit for upload with invalid scoped package format")
	})
	_ = srv

	// Create a tarball with a malformed scoped name (@ without proper /)
	// We'll manually build this to bypass npm's validation
	path := writeNpmTarball(t, map[string]string{
		"package/package.json": `{
  "name": "@",
  "version": "1.0.0"
}`,
	})
	err := runNpmCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for invalid scoped package format")
	}
	// The error could be from the invalid name or from the split failing
	if !strings.Contains(err.Error(), "invalid") && !strings.Contains(err.Error(), "name") {
		t.Errorf("error should mention validation failure, got: %v", err)
	}
}

func TestNewPushNpmCmd_ScopedPackage_UploadError(t *testing.T) {
	// Test lines 224-227: scoped package upload error handling
	srv := withNpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"server error"}`))
	})
	_ = srv

	path := writeNpmTarball(t, map[string]string{
		"package/package.json": `{
  "name": "@test/scoped-package",
  "version": "1.0.0"
}`,
	})
	err := runNpmCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for scoped package upload failure")
	}
	if !strings.Contains(err.Error(), "failed to upload NPM package") {
		t.Errorf("error should mention upload failure, got: %v", err)
	}
}

func TestNewPushNpmCmd_MissingPkgUrl(t *testing.T) {
	// Test lines 123-126: missing pkg-url error
	origPkg := config.Global.Registry.PkgURL
	config.Global.Registry.PkgURL = ""
	defer func() {
		config.Global.Registry.PkgURL = origPkg
	}()

	path := writeNpmTarball(t, map[string]string{
		"package/package.json": `{
  "name": "test-package",
  "version": "1.0.0"
}`,
	})
	err := runNpmCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for missing pkg-url")
	}
	if !strings.Contains(err.Error(), "pkg-url must be set") {
		t.Errorf("error should mention pkg-url requirement, got: %v", err)
	}
}

func TestNewPushNpmCmd_InvalidMetadataJSON(t *testing.T) {
	// Test lines 160-162: JSON unmarshal error
	srv := withNpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{invalid json`))
			return
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
	err := runNpmCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for invalid metadata JSON")
	}
}

func TestNewPushNpmCmd_UnscopedPackage_UploadError(t *testing.T) {
	// Test lines 244-247: unscoped package upload error (ensure else branch is hit)
	srv := withNpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Return error for upload
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	})
	_ = srv

	path := writeNpmTarball(t, map[string]string{
		"package/package.json": `{
  "name": "unscoped-package",
  "version": "1.0.0"
}`,
	})
	err := runNpmCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for unscoped package upload failure")
	}
	if !strings.Contains(err.Error(), "failed to upload NPM package") {
		t.Errorf("error should mention upload failure, got: %v", err)
	}
}

func TestNewPushNpmCmd_ScopedPackage_201Created(t *testing.T) {
	// Test scoped package with 201 Created response
	srv := withNpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})
	_ = srv

	path := writeNpmTarball(t, map[string]string{
		"package/package.json": `{
  "name": "@test/scoped-package",
  "version": "1.0.0"
}`,
	})
	if err := runNpmCmd(t, "test-registry", path); err != nil {
		t.Fatalf("expected success on 201, got error: %v", err)
	}
}
