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
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/util/common/auth"
)

func withRubyServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
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

func writeGemFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test-1.0.0.gem")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write gem file: %v", err)
	}
	return path
}

func runRubyCmd(t *testing.T, args ...string) error {
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
	cmd := NewPushRubyCmd(factory)
	cmd.SetArgs(args)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	return cmd.Execute()
}

func TestNewPushRubyCmd_Success(t *testing.T) {
	withRubyServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/ruby/api/v1/gems") {
			t.Errorf("expected path to contain /ruby/api/v1/gems, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"Success","name":"test","version":"1.0.0","platform":"ruby","sha256":"abc"}`))
	})

	path := writeGemFile(t, "test gem content")
	if err := runRubyCmd(t, "test-registry", path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewPushRubyCmd_ServerError(t *testing.T) {
	withRubyServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"version exists"}`))
	})

	path := writeGemFile(t, "test gem content")
	err := runRubyCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for 409 response")
	}
	if !strings.Contains(err.Error(), "failed to push package") {
		t.Errorf("error should mention upload failure, got: %v", err)
	}
}

func TestNewPushRubyCmd_FileNotFound(t *testing.T) {
	withRubyServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when file is missing")
	})
	err := runRubyCmd(t, "test-registry", "/nonexistent/test.gem")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestNewPushRubyCmd_NotAGem(t *testing.T) {
	withRubyServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for non-gem file")
	})
	dir := t.TempDir()
	path := filepath.Join(dir, "test.zip")
	if err := os.WriteFile(path, []byte("not a gem"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	err := runRubyCmd(t, "test-registry", path)
	if err == nil {
		t.Fatal("expected error for non-gem extension")
	}
}

func TestNewPushRubyCmd_DirectoryPath(t *testing.T) {
	withRubyServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for directory")
	})
	dir := t.TempDir()
	gemDir := filepath.Join(dir, "fake.gem")
	if err := os.Mkdir(gemDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	err := runRubyCmd(t, "test-registry", gemDir)
	if err == nil {
		t.Fatal("expected error for directory path")
	}
}

func TestNewPushRubyCmd_WrongArgCount(t *testing.T) {
	if err := runRubyCmd(t, "only-one-arg"); err == nil {
		t.Fatal("expected error for missing second arg")
	}
}

func TestNewPushRubyCmd_NoChecksumHeaders(t *testing.T) {
	withRubyServer(t, func(w http.ResponseWriter, r *http.Request) {
		for _, header := range []string{
			"X-Checksum-Md5", "X-Checksum-Sha1", "X-Checksum-Sha256", "X-Checksum-Sha512",
		} {
			if r.Header.Get(header) != "" {
				t.Errorf("unexpected %s header on ruby push request", header)
			}
		}
		w.WriteHeader(http.StatusOK)
	})

	path := writeGemFile(t, "test gem content")
	if err := runRubyCmd(t, "test-registry", path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
