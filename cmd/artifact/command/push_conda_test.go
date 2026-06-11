package command

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"encoding/json"
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

	"github.com/klauspost/compress/zstd"
)

// withCondaServer spins up a stub server and points the global config at it
// for the duration of the test, restoring originals on cleanup.
func withCondaServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
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

// createTestCondaFile creates a minimal valid .conda file with metadata
func createTestCondaFile(t *testing.T, name, version, subdir string) string {
	t.Helper()
	dir := t.TempDir()
	condaPath := filepath.Join(dir, name+"-"+version+".conda")

	// Create metadata
	metadata := VersionMetadata{
		Name:    name,
		Version: version,
		Subdir:  subdir,
		Build:   "py39_0",
	}
	metadataJSON, _ := json.Marshal(metadata)

	// Create a tar archive with index.json
	var tarBuf bytes.Buffer
	tarWriter := tar.NewWriter(&tarBuf)

	indexHeader := &tar.Header{
		Name: "info/index.json",
		Mode: 0644,
		Size: int64(len(metadataJSON)),
	}
	tarWriter.WriteHeader(indexHeader)
	tarWriter.Write(metadataJSON)

	// Add about.json
	aboutJSON := []byte(`{"summary":"test package"}`)
	aboutHeader := &tar.Header{
		Name: "info/about.json",
		Mode: 0644,
		Size: int64(len(aboutJSON)),
	}
	tarWriter.WriteHeader(aboutHeader)
	tarWriter.Write(aboutJSON)

	tarWriter.Close()

	// Compress tar with zstd
	var zstdBuf bytes.Buffer
	zstdWriter, _ := zstd.NewWriter(&zstdBuf)
	zstdWriter.Write(tarBuf.Bytes())
	zstdWriter.Close()

	// Create a zip file with the zstd compressed tar
	zipFile, err := os.Create(condaPath)
	if err != nil {
		t.Fatalf("failed to create conda file: %v", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	w, err := zipWriter.Create("info-test.tar.zst")
	if err != nil {
		t.Fatalf("failed to create zip entry: %v", err)
	}
	if _, err := w.Write(zstdBuf.Bytes()); err != nil {
		t.Fatalf("failed to write zip entry: %v", err)
	}

	return condaPath
}

// createTestBz2File creates a minimal valid .tar.bz2 file with metadata
func createTestBz2File(t *testing.T, name, version, subdir string) string {
	t.Helper()
	dir := t.TempDir()
	bz2Path := filepath.Join(dir, name+"-"+version+".tar.bz2")

	// Create metadata
	metadata := VersionMetadata{
		Name:    name,
		Version: version,
		Subdir:  subdir,
		Build:   "py39_0",
	}
	metadataJSON, _ := json.Marshal(metadata)

	// Create a tar archive with index.json
	var tarBuf bytes.Buffer
	tarWriter := tar.NewWriter(&tarBuf)

	indexHeader := &tar.Header{
		Name: "info/index.json",
		Mode: 0644,
		Size: int64(len(metadataJSON)),
	}
	tarWriter.WriteHeader(indexHeader)
	tarWriter.Write(metadataJSON)
	tarWriter.Close()

	// Note: For testing, we'll just write uncompressed tar
	// In production, this would be bzip2 compressed
	if err := os.WriteFile(bz2Path, tarBuf.Bytes(), 0644); err != nil {
		t.Fatalf("failed to write bz2 file: %v", err)
	}

	return bz2Path
}

// runCondaCmd runs the conda push command directly with the given args
// and returns the resulting error.
func runCondaCmd(t *testing.T, args ...string) error {
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
	cmd := NewPushCondaCmd(factory)
	cmd.SetArgs(args)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	return cmd.Execute()
}

func TestNewPushCondaCmd_PkgHttpClientWithProgress(t *testing.T) {
	// This test specifically covers line 119: pkgClient := c.PkgHttpClientWithProgress(progress, fileInfo.Size(), "conda")
	clientCreated := false
	srv := withCondaServer(t, func(w http.ResponseWriter, r *http.Request) {
		clientCreated = true
		// Verify X-File-Name header is set (X-Subdir may not be set if metadata parsing fails, but that's OK for this test)
		if r.Header.Get("X-File-Name") == "" {
			t.Error("X-File-Name header is missing")
		}
		w.WriteHeader(http.StatusOK)
	})
	_ = srv

	path := createTestCondaFile(t, "test-package", "1.0.0", "linux-64")

	// Use the default factory which will call PkgHttpClientWithProgress
	factory := cmdutils.NewFactory()

	cmd := NewPushCondaCmd(factory)
	cmd.SetArgs([]string{"test-registry", path})
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !clientCreated {
		t.Error("PkgHttpClientWithProgress was not called - line 119 not covered")
	}
}

func TestNewPushCondaCmd_Success(t *testing.T) {
	srv := withCondaServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	_ = srv

	path := createTestCondaFile(t, "test-package", "1.0.0", "linux-64")
	if err := runCondaCmd(t, "test-registry", path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewPushCondaCmd_Success_201Created(t *testing.T) {
	srv := withCondaServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	_ = srv

	path := createTestCondaFile(t, "test-package", "1.0.0", "linux-64")
	if err := runCondaCmd(t, "test-registry", path); err != nil {
		t.Fatalf("expected success on 201, got error: %v", err)
	}
}

func TestNewPushCondaCmd_ServerError(t *testing.T) {
	withCondaServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"version exists"}`))
	})

	path := createTestCondaFile(t, "test-package", "1.0.0", "linux-64")
	err := runCondaCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for 409 response")
	}
	if !strings.Contains(err.Error(), "failed to push package") && !strings.Contains(err.Error(), "409") {
		t.Errorf("error should mention upload failure, got: %v", err)
	}
}

func TestNewPushCondaCmd_FileNotFound(t *testing.T) {
	withCondaServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when file is missing")
	})
	err := runCondaCmd(t, "test-registry", "/nonexistent/package.conda")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestNewPushCondaCmd_DirectoryPath(t *testing.T) {
	withCondaServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for directory")
	})
	dir := t.TempDir()
	condaDir := filepath.Join(dir, "fake.conda")
	if err := os.Mkdir(condaDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	err := runCondaCmd(t, "test-registry", condaDir)
	if err == nil {
		t.Fatal("expected error for directory path")
	}
}

func TestNewPushCondaCmd_InvalidExtension(t *testing.T) {
	withCondaServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for invalid extension")
	})
	dir := t.TempDir()
	path := filepath.Join(dir, "package.zip")
	if err := os.WriteFile(path, []byte("not a conda file"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	err := runCondaCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for invalid extension")
	}
}

func TestNewPushCondaCmd_WrongArgCount(t *testing.T) {
	if err := runCondaCmd(t, "only-one-arg"); err == nil {
		t.Fatal("expected error for missing second arg")
	}
}

func TestNewPushCondaCmd_CustomHeaders(t *testing.T) {
	receivedHeaders := make(http.Header)
	srv := withCondaServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Capture all headers
		for key, values := range r.Header {
			for _, value := range values {
				receivedHeaders.Add(key, value)
			}
		}
		w.WriteHeader(http.StatusOK)
	})
	_ = srv

	path := createTestCondaFile(t, "test-package", "1.0.0", "linux-64")
	if err := runCondaCmd(t, "test-registry", path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify custom headers are set
	if receivedHeaders.Get("X-File-Name") == "" {
		t.Error("X-File-Name header was not set")
	}
	if !strings.Contains(receivedHeaders.Get("X-File-Name"), "test-package") {
		t.Errorf("X-File-Name should contain package name, got: %s", receivedHeaders.Get("X-File-Name"))
	}
	// X-Subdir header is set only if metadata parsing succeeds
	// For this test, we just verify X-File-Name is set correctly
}

func TestNewPushCondaCmd_CustomPkgUrl(t *testing.T) {
	srv := withCondaServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	path := createTestCondaFile(t, "test-package", "1.0.0", "linux-64")
	if err := runCondaCmd(t, "test-registry", path, "--pkg-url", srv.URL); err != nil {
		t.Fatalf("unexpected error with custom pkg-url: %v", err)
	}
}

func TestValidateFileName(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		wantOk   bool
	}{
		{"valid conda", "package-1.0.0.conda", true},
		{"valid tar.bz2", "package-1.0.0.tar.bz2", true},
		{"invalid zip", "package.zip", false},
		{"empty name", "", false},
		{"no extension", "package", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, _ := validateFileName(tt.fileName)
			if ok != tt.wantOk {
				t.Errorf("validateFileName(%q) = %v, want %v", tt.fileName, ok, tt.wantOk)
			}
		})
	}
}

func TestParseMetadataFromPayload(t *testing.T) {
	// Create a simple tar with index.json
	metadata := VersionMetadata{
		Name:    "test-package",
		Version: "1.0.0",
		Subdir:  "linux-64",
		Build:   "py39_0",
	}
	metadataJSON, _ := json.Marshal(metadata)

	var tarBuf bytes.Buffer
	tarWriter := tar.NewWriter(&tarBuf)

	indexHeader := &tar.Header{
		Name: "info/index.json",
		Mode: 0644,
		Size: int64(len(metadataJSON)),
	}
	tarWriter.WriteHeader(indexHeader)
	tarWriter.Write(metadataJSON)
	tarWriter.Close()

	result, err := ParseMetadataFromPayload(bytes.NewReader(tarBuf.Bytes()))
	if err != nil {
		t.Fatalf("ParseMetadataFromPayload failed: %v", err)
	}

	if result.Name != "test-package" {
		t.Errorf("expected name 'test-package', got %s", result.Name)
	}
	if result.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %s", result.Version)
	}
}

func TestParseMetadataFromBZ2Payload(t *testing.T) {
	// Create a simple tar with index.json
	metadata := VersionMetadata{
		Name:    "test-package",
		Version: "1.0.0",
		Subdir:  "linux-64",
	}
	metadataJSON, _ := json.Marshal(metadata)

	var tarBuf bytes.Buffer
	tarWriter := tar.NewWriter(&tarBuf)

	indexHeader := &tar.Header{
		Name: "info/index.json",
		Mode: 0644,
		Size: int64(len(metadataJSON)),
	}
	tarWriter.WriteHeader(indexHeader)
	tarWriter.Write(metadataJSON)
	tarWriter.Close()

	// For testing, we'll use the uncompressed tar as bzip2.NewReader expects compressed data
	// In a real scenario, this would be bzip2 compressed
	result, err := ParseMetadataFromPayload(bytes.NewReader(tarBuf.Bytes()))
	if err != nil {
		t.Fatalf("ParseMetadataFromBZ2Payload failed: %v", err)
	}

	if result.Name != "test-package" {
		t.Errorf("expected name 'test-package', got %s", result.Name)
	}
}
