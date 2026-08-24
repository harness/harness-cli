package upload

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	ar "github.com/harness/harness-cli/internal/api/ar"
)

// ── getRegistryName ───────────────────────────────────────────────────────────

func TestGetRegistryName_WithSlash_ReturnsFirstSegment(t *testing.T) {
	name, err := getRegistryName("my-registry/path/to/dest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "my-registry" {
		t.Errorf("got %q, want %q", name, "my-registry")
	}
}

func TestGetRegistryName_WithoutSlash_ReturnsFullString(t *testing.T) {
	name, err := getRegistryName("my-registry")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "my-registry" {
		t.Errorf("got %q, want %q", name, "my-registry")
	}
}

func TestGetRegistryName_EmptyString_ReturnsError(t *testing.T) {
	_, err := getRegistryName("")
	if err == nil {
		t.Fatal("expected error for empty target, got nil")
	}
	if !strings.Contains(err.Error(), "registry name must not be empty") {
		t.Errorf("error should mention 'registry name must not be empty', got: %v", err)
	}
}

func TestGetRegistryName_LeadingSlash_EmptyName_ReturnsError(t *testing.T) {
	_, err := getRegistryName("/dest/path")
	if err == nil {
		t.Fatal("expected error when registry name is empty (leading slash), got nil")
	}
	if !strings.Contains(err.Error(), "registry name must not be empty") {
		t.Errorf("error should mention 'registry name must not be empty', got: %v", err)
	}
}

func TestGetRegistryName_TrailingSlash_ReturnsName(t *testing.T) {
	name, err := getRegistryName("my-registry/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "my-registry" {
		t.Errorf("got %q, want %q", name, "my-registry")
	}
}

func TestGetRegistryName_MultipleSlashes_ReturnsFirstSegment(t *testing.T) {
	name, err := getRegistryName("reg/a/b/c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "reg" {
		t.Errorf("got %q, want %q", name, "reg")
	}
}

// ── validateRegistry ──────────────────────────────────────────────────────────

// registryTestResponse is a helper that builds a minimal RegistryResponse JSON payload.
func registryTestResponse(pkgType ar.PackageType) []byte {
	resp := ar.RegistryResponse{
		Data: ar.Registry{
			Identifier:  "test-reg",
			PackageType: pkgType,
			Url:         "https://example.com",
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

// newTestFactory creates a *cmdutils.Factory whose RegistryHttpClient points at srv.
func newTestFactory(t *testing.T, srv *httptest.Server) *cmdutils.Factory {
	t.Helper()
	client, err := ar.NewClientWithResponses(srv.URL)
	if err != nil {
		t.Fatalf("failed to create AR test client: %v", err)
	}
	return &cmdutils.Factory{
		RegistryHttpClient: func() *ar.ClientWithResponses {
			return client
		},
	}
}

func TestValidateRegistry_Success_ReturnsPackageType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(registryTestResponse("RAW"))
	}))
	defer srv.Close()

	factory := newTestFactory(t, srv)

	pkgType, err := validateRegistry(context.Background(), "test-reg", factory)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pkgType != "RAW" {
		t.Errorf("PackageType: got %q, want %q", pkgType, "RAW")
	}
}

func TestValidateRegistry_HTTP404_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	factory := newTestFactory(t, srv)

	_, err := validateRegistry(context.Background(), "missing-reg", factory)
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
	if !strings.Contains(err.Error(), "not found or inaccessible") {
		t.Errorf("error should mention 'not found or inaccessible', got: %v", err)
	}
}

func TestValidateRegistry_HTTP500_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	factory := newTestFactory(t, srv)

	_, err := validateRegistry(context.Background(), "test-reg", factory)
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestValidateRegistry_NetworkError_ReturnsError(t *testing.T) {
	// Point at a server that is already closed.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	client, err := ar.NewClientWithResponses(url)
	if err != nil {
		t.Fatalf("failed to create test client: %v", err)
	}
	factory := &cmdutils.Factory{
		RegistryHttpClient: func() *ar.ClientWithResponses { return client },
	}

	_, err = validateRegistry(context.Background(), "test-reg", factory)
	if err == nil {
		t.Fatal("expected error for closed server, got nil")
	}
	if !strings.Contains(err.Error(), "failed to reach registry") {
		t.Errorf("error should mention 'failed to reach registry', got: %v", err)
	}
}

func TestValidateRegistry_HTTP200_NilJSON200_ReturnsError(t *testing.T) {
	// Respond 200 but with non-JSON body so JSON200 stays nil.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	factory := newTestFactory(t, srv)

	_, err := validateRegistry(context.Background(), "test-reg", factory)
	if err == nil {
		t.Fatal("expected error when JSON200 is nil, got nil")
	}
}

func TestValidateRegistry_UsesRegistryRef(t *testing.T) {
	origAccount := config.Global.AccountID
	origOrg := config.Global.OrgID
	origProject := config.Global.ProjectID
	config.Global.AccountID = "acc"
	config.Global.OrgID = "org"
	config.Global.ProjectID = "proj"
	defer func() {
		config.Global.AccountID = origAccount
		config.Global.OrgID = origOrg
		config.Global.ProjectID = origProject
	}()

	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(registryTestResponse("RAW"))
	}))
	defer srv.Close()

	factory := newTestFactory(t, srv)
	_, _ = validateRegistry(context.Background(), "my-reg", factory)

	if !strings.Contains(capturedPath, "my-reg") {
		t.Errorf("expected request path to contain registry name, got %q", capturedPath)
	}
}

// ── NewUploadArtifactCmd – args validation ────────────────────────────────────

func TestNewUploadArtifactCmd_TooFewArgs_ReturnsError(t *testing.T) {
	cmd := NewUploadArtifactCmd(&cmdutils.Factory{})
	cmd.SetArgs([]string{"only-one-arg"})
	err := cmd.Args(cmd, []string{"only-one-arg"})
	if err == nil {
		t.Fatal("expected error for too few args, got nil")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg(s)") {
		t.Errorf("error should mention expected arg count, got: %v", err)
	}
}

func TestNewUploadArtifactCmd_TooManyArgs_ReturnsError(t *testing.T) {
	cmd := NewUploadArtifactCmd(&cmdutils.Factory{})
	err := cmd.Args(cmd, []string{"src", "dest", "extra"})
	if err == nil {
		t.Fatal("expected error for too many args, got nil")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg(s)") {
		t.Errorf("error should mention expected arg count, got: %v", err)
	}
}

func TestNewUploadArtifactCmd_ExactArgs_NoError(t *testing.T) {
	cmd := NewUploadArtifactCmd(&cmdutils.Factory{})
	err := cmd.Args(cmd, []string{"*.jar", "my-registry/libs/"})
	if err != nil {
		t.Errorf("unexpected error for exactly 2 args: %v", err)
	}
}

func TestNewUploadArtifactCmd_ZeroArgs_ReturnsError(t *testing.T) {
	cmd := NewUploadArtifactCmd(&cmdutils.Factory{})
	err := cmd.Args(cmd, []string{})
	if err == nil {
		t.Fatal("expected error for zero args, got nil")
	}
}

func TestNewUploadArtifactCmd_HasExpectedFlags(t *testing.T) {
	cmd := NewUploadArtifactCmd(&cmdutils.Factory{})

	flags := []string{"version", "dry-run", "flatten", "include", "exclude"}
	for _, name := range flags {
		if f := cmd.Flags().Lookup(name); f == nil {
			t.Errorf("expected flag --%s to be registered", name)
		}
	}
}

func TestNewUploadArtifactCmd_DryRunDefault_IsFalse(t *testing.T) {
	cmd := NewUploadArtifactCmd(&cmdutils.Factory{})
	f := cmd.Flags().Lookup("dry-run")
	if f == nil {
		t.Fatal("flag --dry-run not found")
	}
	if f.DefValue != "false" {
		t.Errorf("--dry-run default: got %q, want %q", f.DefValue, "false")
	}
}

func TestNewUploadArtifactCmd_VersionDefault_Is100(t *testing.T) {
	cmd := NewUploadArtifactCmd(&cmdutils.Factory{})
	f := cmd.Flags().Lookup("version")
	if f == nil {
		t.Fatal("flag --version not found")
	}
	if f.DefValue != "1.0.0" {
		t.Errorf("--version default: got %q, want %q", f.DefValue, "1.0.0")
	}
}

func TestNewUploadArtifactCmd_FlattenDefault_IsFalse(t *testing.T) {
	cmd := NewUploadArtifactCmd(&cmdutils.Factory{})
	f := cmd.Flags().Lookup("flatten")
	if f == nil {
		t.Fatal("flag --flatten not found")
	}
	if f.DefValue != "false" {
		t.Errorf("--flatten default: got %q, want %q", f.DefValue, "false")
	}
}
