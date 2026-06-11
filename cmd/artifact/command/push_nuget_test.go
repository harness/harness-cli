package command

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/harness/harness-cli/cmd/artifact/command/utils"
	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/util/common/auth"
)

// withNugetServer spins up a stub server and points the global config at it
// for the duration of the test, restoring originals on cleanup.
func withNugetServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
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

// writeNugetFile creates a temporary .nupkg file with the given content
func writeNugetFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.nupkg")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write nuget file: %v", err)
	}
	return path
}

// nugetCmdArgs runs the nuget push command directly with the given args
// and returns the resulting error.
func runNugetCmd(t *testing.T, args ...string) error {
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
	cmd := NewPushNugetCmd(factory)
	cmd.SetArgs(args)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	return cmd.Execute()
}

func TestNewPushNugetCmd_Success(t *testing.T) {
	srv := withNugetServer(t, func(w http.ResponseWriter, r *http.Request) {
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

	path := writeNugetFile(t, "test nuget content")
	if err := runNugetCmd(t, "test-registry", path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewPushNugetCmd_ServerError(t *testing.T) {
	withNugetServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"version exists"}`))
	})

	path := writeNugetFile(t, "test nuget content")
	err := runNugetCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for 409 response")
	}
	if !strings.Contains(err.Error(), "failed to push package") && !strings.Contains(err.Error(), "409") {
		t.Errorf("error should mention upload failure, got: %v", err)
	}
}

func TestNewPushNugetCmd_FileNotFound(t *testing.T) {
	withNugetServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when file is missing")
	})
	err := runNugetCmd(t, "test-registry", "/nonexistent/test.nupkg")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestNewPushNugetCmd_NotANuget(t *testing.T) {
	withNugetServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for non-nupkg file")
	})
	dir := t.TempDir()
	path := filepath.Join(dir, "test.zip")
	if err := os.WriteFile(path, []byte("not a nupkg"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	err := runNugetCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for non-nupkg extension")
	}
}

func TestNewPushNugetCmd_DirectoryPath(t *testing.T) {
	withNugetServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for directory")
	})
	dir := t.TempDir()
	nugetDir := filepath.Join(dir, "fake.nupkg")
	if err := os.Mkdir(nugetDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	err := runNugetCmd(t, "test-registry", nugetDir)
	if err == nil {
		t.Fatal("expected error for directory path")
	}
}

func TestNewPushNugetCmd_WrongArgCount(t *testing.T) {
	if err := runNugetCmd(t, "only-one-arg"); err == nil {
		t.Fatal("expected error for missing second arg")
	}
}

func TestNewPushNugetCmd_ChecksumHeadersSet(t *testing.T) {
	receivedHeaders := make(http.Header)
	srv := withNugetServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Capture all headers
		for key, values := range r.Header {
			for _, value := range values {
				receivedHeaders.Add(key, value)
			}
		}
		w.WriteHeader(http.StatusOK)
	})
	_ = srv

	path := writeNugetFile(t, "test nuget content for checksums")
	if err := runNugetCmd(t, "test-registry", path); err != nil {
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

func TestNewPushNugetCmd_NestedPath(t *testing.T) {
	receivedPath := ""
	srv := withNugetServer(t, func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})
	_ = srv

	path := writeNugetFile(t, "test nuget content")
	if err := runNugetCmd(t, "test-registry", path, "--path", "nested/folder"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the path includes the nested directory
	if !strings.Contains(receivedPath, "nested/folder") {
		t.Errorf("expected path to contain 'nested/folder', got: %s", receivedPath)
	}
}

func TestNewPushNugetCmd_CustomPkgUrl(t *testing.T) {
	srv := withNugetServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	path := writeNugetFile(t, "test nuget content")
	if err := runNugetCmd(t, "test-registry", path, "--pkg-url", srv.URL); err != nil {
		t.Fatalf("unexpected error with custom pkg-url: %v", err)
	}
}

func TestNewPushNugetCmd_Success_201Created(t *testing.T) {
	srv := withNugetServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	_ = srv

	path := writeNugetFile(t, "test nuget content")
	if err := runNugetCmd(t, "test-registry", path); err != nil {
		t.Fatalf("expected success on 201, got error: %v", err)
	}
}

func TestNewPushNugetCmd_PkgHttpClientWithProgress(t *testing.T) {
	// This test specifically covers line 143: pkgClient := c.PkgHttpClientWithProgress(...)
	clientCreated := false
	srv := withNugetServer(t, func(w http.ResponseWriter, r *http.Request) {
		clientCreated = true
		w.WriteHeader(http.StatusOK)
	})
	_ = srv

	path := writeNugetFile(t, "test nuget content")

	// Use the default factory which will call PkgHttpClientWithProgress
	factory := cmdutils.NewFactory()

	cmd := NewPushNugetCmd(factory)
	cmd.SetArgs([]string{"test-registry", path})
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !clientCreated {
		t.Error("PkgHttpClientWithProgress was not called - line 143 not covered")
	}
}

func TestNewPushNugetCmd_MultipartFormData(t *testing.T) {
	receivedContentType := ""
	srv := withNugetServer(t, func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	})
	_ = srv

	path := writeNugetFile(t, "test nuget content")
	if err := runNugetCmd(t, "test-registry", path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify multipart form data is used
	if !strings.HasPrefix(receivedContentType, "multipart/form-data") {
		t.Errorf("expected multipart/form-data content type, got: %s", receivedContentType)
	}
}

func TestNewPushNugetCmd_EmptyFile(t *testing.T) {
	srv := withNugetServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	_ = srv

	// Create empty nupkg file
	path := writeNugetFile(t, "")
	if err := runNugetCmd(t, "test-registry", path); err != nil {
		t.Fatalf("unexpected error for empty file: %v", err)
	}
}

func TestNewPushNugetCmd_LargeFile(t *testing.T) {
	srv := withNugetServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	_ = srv

	// Create a larger file (1MB)
	largeContent := strings.Repeat("A", 1024*1024)
	path := writeNugetFile(t, largeContent)
	if err := runNugetCmd(t, "test-registry", path); err != nil {
		t.Fatalf("unexpected error for large file: %v", err)
	}
}

func TestNewPushNugetCmd_NestedPathServerError(t *testing.T) {
	srv := withNugetServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"server error"}`))
	})
	_ = srv

	path := writeNugetFile(t, "test nuget content")
	err := runNugetCmd(t, "test-registry", path, "--path", "nested/folder")
	if err == nil {
		t.Fatal("expected error for server error with nested path")
	}
	// Error message can be either "upload failed" or "request failed" due to retries
	if !strings.Contains(err.Error(), "failed") {
		t.Errorf("error should mention failure, got: %v", err)
	}
}

func TestUploadNugetPackageDirect(t *testing.T) {
	// Test the uploadNugetPackageDirect function directly
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT method, got %s", r.Method)
		}
		if r.Header.Get("x-api-key") == "" {
			t.Error("x-api-key header is missing")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	origAuthToken := config.Global.AuthToken
	config.Global.AuthToken = "test-api-key"
	defer func() { config.Global.AuthToken = origAuthToken }()

	body := bytes.NewBufferString("test content")
	checksums := utils.FileChecksums{
		MD5:    "test-md5",
		SHA1:   "test-sha1",
		SHA256: "test-sha256",
		SHA512: "test-sha512",
	}

	progress := &mockProgressReporter{}
	err := uploadNugetPackageDirect(
		context.Background(),
		srv.URL,
		"multipart/form-data",
		body,
		config.Global.AuthToken,
		progress,
		int64(body.Len()),
		checksums,
	)

	if err != nil {
		t.Fatalf("uploadNugetPackageDirect failed: %v", err)
	}
}

// mockProgressReporter implements progress.Reporter for testing
type mockProgressReporter struct{}

func (m *mockProgressReporter) Start(message string) {}
func (m *mockProgressReporter) End()                 {}
func (m *mockProgressReporter) Step(msg string)      {}
func (m *mockProgressReporter) Success(msg string)   {}
func (m *mockProgressReporter) Error(msg string)     {}
func (m *mockProgressReporter) Warn(msg string)      {}
