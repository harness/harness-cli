package upload

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
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/util/common/auth"
)

// withUploadServer spins up a stub HTTP server, points config.Global at it,
// and restores the original config values on test cleanup.
func withUploadServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	orig := config.Global
	config.Global.Registry.PkgURL = srv.URL
	config.Global.AccountID = "test-account"
	config.Global.AuthToken = "pat.test-account.aaa.bbb"
	t.Cleanup(func() { config.Global = orig })

	return srv
}

// runUploadCmd executes NewUploadArtifactCmd with the given args and returns
// the error result. cobra's output streams are silenced to keep test output clean.
func runUploadCmd(t *testing.T, args ...string) error {
	t.Helper()
	factory := &cmdutils.Factory{
		PkgHttpClient: func() *pkgclient.ClientWithResponses {
			c, err := pkgclient.NewClientWithResponses(config.Global.Registry.PkgURL,
				auth.GetAuthOptionARPKG())
			if err != nil {
				t.Fatalf("create pkg client: %v", err)
			}
			return c
		},
	}
	cmd := NewUploadArtifactCmd(factory)
	cmd.SetArgs(args)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	return cmd.Execute()
}

// ── Command structure ─────────────────────────────────────────────────────────

func TestNewUploadArtifactCmd_Structure(t *testing.T) {
	cmd := NewUploadArtifactCmd(&cmdutils.Factory{})

	if !strings.HasPrefix(cmd.Use, "upload ") {
		t.Errorf("Use should start with 'upload', got %q", cmd.Use)
	}
	if cmd.Short == "" {
		t.Error("Short description must not be empty")
	}
	if cmd.Long == "" {
		t.Error("Long description must not be empty")
	}

	versionFlag := cmd.Flags().Lookup("version")
	if versionFlag == nil {
		t.Fatal("--version flag must be registered")
	}
	if versionFlag.DefValue != "1.0.0" {
		t.Errorf("--version default: got %q, want 1.0.0", versionFlag.DefValue)
	}
}

// ── Argument validation ───────────────────────────────────────────────────────

func TestNewUploadArtifactCmd_TooFewArgs(t *testing.T) {
	withUploadServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be reached with wrong arg count")
	})
	if err := runUploadCmd(t, "only-one-arg"); err == nil {
		t.Fatal("expected error for one argument")
	}
}

func TestNewUploadArtifactCmd_TooManyArgs(t *testing.T) {
	withUploadServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be reached with wrong arg count")
	})
	if err := runUploadCmd(t, "src", "reg/dest", "extra"); err == nil {
		t.Fatal("expected error for three arguments")
	}
}

func TestNewUploadArtifactCmd_ZeroArgs(t *testing.T) {
	withUploadServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be reached with zero args")
	})
	if err := runUploadCmd(t); err == nil {
		t.Fatal("expected error for zero arguments")
	}
}

// ── Target validation ─────────────────────────────────────────────────────────

func TestNewUploadArtifactCmd_InvalidTarget_NoSlash(t *testing.T) {
	withUploadServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be reached for invalid target")
	})
	dir := t.TempDir()
	f := filepath.Join(dir, "pkg.zip")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	err := runUploadCmd(t, f, "no-slash-registry")
	if err == nil {
		t.Fatal("expected error for target without '/'")
	}
	if !strings.Contains(err.Error(), "<registry>/<path>") {
		t.Errorf("error should describe expected format, got: %v", err)
	}
}

// ── No files matched ──────────────────────────────────────────────────────────

func TestNewUploadArtifactCmd_NoFilesMatched(t *testing.T) {
	withUploadServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be reached when no files matched")
	})
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	err := runUploadCmd(t, filepath.Join(dir, "*.jar"), "my-reg/libs")
	if err == nil {
		t.Fatal("expected error when pattern matches no files")
	}
	if !strings.Contains(err.Error(), "no files matched") {
		t.Errorf("error should say 'no files matched', got: %v", err)
	}
}

// ── Successful upload ─────────────────────────────────────────────────────────

func TestNewUploadArtifactCmd_Success_SingleFile(t *testing.T) {
	requestCount := 0
	withUploadServer(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	})

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.zip"), []byte("content"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := runUploadCmd(t, filepath.Join(dir, "*.zip"), "test-reg/mypackage"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if requestCount != 1 {
		t.Errorf("expected 1 upload request, got %d", requestCount)
	}
}

func TestNewUploadArtifactCmd_Success_MultipleFiles(t *testing.T) {
	requestCount := 0
	withUploadServer(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
	})

	dir := t.TempDir()
	for _, name := range []string{"a.zip", "b.zip", "c.zip"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("data"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	if err := runUploadCmd(t, filepath.Join(dir, "*.zip"), "test-reg/mypackage"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if requestCount != 3 {
		t.Errorf("expected 3 upload requests, got %d", requestCount)
	}
}

func TestNewUploadArtifactCmd_Success_CustomVersion(t *testing.T) {
	var capturedPath string
	withUploadServer(t, func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sdk.jar"), []byte("jar"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := runUploadCmd(t, filepath.Join(dir, "*.jar"), "test-reg/mylib", "--version", "3.2.1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(capturedPath, "3.2.1") {
		t.Errorf("expected version 3.2.1 in upload URL path %q", capturedPath)
	}
}

// ── Server error ──────────────────────────────────────────────────────────────

func TestNewUploadArtifactCmd_ServerError(t *testing.T) {
	withUploadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"internal server error"}`))
	})

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pkg.zip"), []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	err := runUploadCmd(t, filepath.Join(dir, "*.zip"), "test-reg/mypackage")
	if err == nil {
		t.Fatal("expected error for server 500 response")
	}
}
