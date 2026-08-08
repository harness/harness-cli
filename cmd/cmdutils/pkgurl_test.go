package cmdutils

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/harness/harness-cli/config"

	"github.com/spf13/cobra"
)

func TestShouldDerivePkgURL(t *testing.T) {
	tests := []struct {
		name          string
		apiURLFlagSet bool
		currentPkgURL string
		want          bool
	}{
		{"api-url set, no pkg url configured", true, "", true},
		{"api-url set, whitespace pkg url", true, "   ", true},
		{"api-url set, pkg url from saved config", true, "https://pkg.example.com", false},
		{"api-url not set, no pkg url configured", false, "", false},
		{"api-url not set, pkg url from saved config", false, "https://pkg.example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldDerivePkgURL(tt.apiURLFlagSet, tt.currentPkgURL); got != tt.want {
				t.Errorf("ShouldDerivePkgURL(%v, %q) = %v, want %v",
					tt.apiURLFlagSet, tt.currentPkgURL, got, tt.want)
			}
		})
	}
}

// withGlobalConfig snapshots the global config fields ResolvePkgURL touches
// and restores them on cleanup.
func withGlobalConfig(t *testing.T) {
	t.Helper()
	orig := config.Global
	t.Cleanup(func() { config.Global = orig })
}

// newTestCmd builds a bare command with an --api-url flag, optionally marked
// as explicitly set, mirroring how the root command binds it in production.
func newTestCmd(t *testing.T, apiURL string, markChanged bool) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("api-url", "", "")
	if markChanged {
		if err := cmd.Flags().Set("api-url", apiURL); err != nil {
			t.Fatalf("failed to set api-url flag: %v", err)
		}
	}
	return cmd
}

func TestResolvePkgURL_ExplicitFlagWins(t *testing.T) {
	withGlobalConfig(t)
	config.Global.Registry.PkgURL = "https://saved.example.com"

	cmd := newTestCmd(t, "", false)
	if err := ResolvePkgURL(cmd, "pkg.override.example.com"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.Global.Registry.PkgURL != "https://pkg.override.example.com" {
		t.Errorf("expected --pkg-url to be normalized and win, got %q", config.Global.Registry.PkgURL)
	}
}

func TestResolvePkgURL_SavedConfigUntouched(t *testing.T) {
	withGlobalConfig(t)
	config.Global.Registry.PkgURL = "https://saved.example.com"

	cmd := newTestCmd(t, "", false)
	if err := ResolvePkgURL(cmd, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.Global.Registry.PkgURL != "https://saved.example.com" {
		t.Errorf("expected saved config to be left untouched, got %q", config.Global.Registry.PkgURL)
	}
}

func TestResolvePkgURL_DerivesFromAPIURL(t *testing.T) {
	withGlobalConfig(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/system/info") {
			t.Errorf("unexpected request path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("account_identifier"); got != "acct-1" {
			t.Errorf("expected account_identifier=acct-1, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"SUCCESS","data":{"registryUrl":"https://pkg.derived.example.com"}}`)
	}))
	defer srv.Close()

	config.Global.APIBaseURL = srv.URL
	config.Global.AccountID = "acct-1"
	config.Global.Registry.PkgURL = ""

	cmd := newTestCmd(t, srv.URL, true)
	if err := ResolvePkgURL(cmd, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.Global.Registry.PkgURL != "https://pkg.derived.example.com" {
		t.Errorf("expected derived pkg URL, got %q", config.Global.Registry.PkgURL)
	}
}

func TestResolvePkgURL_NoDerivationWithoutAPIURLFlag(t *testing.T) {
	withGlobalConfig(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must not be hit when --api-url was not explicitly set")
	}))
	defer srv.Close()

	config.Global.APIBaseURL = srv.URL
	config.Global.AccountID = "acct-1"
	config.Global.Registry.PkgURL = ""

	cmd := newTestCmd(t, "", false)
	err := ResolvePkgURL(cmd, "")
	if err == nil {
		t.Fatal("expected fail-fast error for empty pkg URL")
	}
	if !strings.Contains(err.Error(), "pkg-url must be set") {
		t.Errorf("error should mention pkg-url requirement, got: %v", err)
	}
}

func TestResolvePkgURL_SavedConfigWinsOverAPIURLFlag(t *testing.T) {
	withGlobalConfig(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must not be hit when a pkg URL is already configured")
	}))
	defer srv.Close()

	config.Global.APIBaseURL = srv.URL
	config.Global.AccountID = "acct-1"
	config.Global.Registry.PkgURL = "https://saved.example.com"

	cmd := newTestCmd(t, srv.URL, true)
	if err := ResolvePkgURL(cmd, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.Global.Registry.PkgURL != "https://saved.example.com" {
		t.Errorf("expected saved config to win over derivation, got %q", config.Global.Registry.PkgURL)
	}
}

func TestResolvePkgURL_DerivationFailure(t *testing.T) {
	withGlobalConfig(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"status":"ERROR","message":"boom"}`)
	}))
	defer srv.Close()

	config.Global.APIBaseURL = srv.URL
	config.Global.AccountID = "acct-1"
	config.Global.Registry.PkgURL = ""

	cmd := newTestCmd(t, srv.URL, true)
	err := ResolvePkgURL(cmd, "")
	if err == nil {
		t.Fatal("expected error when /system/info does not yield a registryUrl")
	}
	if !strings.Contains(err.Error(), "--pkg-url") {
		t.Errorf("error should point at --pkg-url escape hatch, got: %v", err)
	}
}
