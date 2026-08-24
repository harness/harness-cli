package upload

import (
	"strings"
	"testing"

	ar "github.com/harness/harness-cli/internal/api/ar"
)

// ── getPusherInstance ────────────────────────────────────────────────────────

func TestGetPusherInstance_RAW_ReturnsPusher(t *testing.T) {
	p, err := getPusherInstance("RAW", UploaderConfig{SrcPattern: "*.bin"})
	if err != nil {
		t.Fatalf("unexpected error for RAW: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil Pusher for RAW")
	}
}

func TestGetPusherInstance_RAW_IsRawUploader(t *testing.T) {
	p, err := getPusherInstance("RAW", UploaderConfig{SrcPattern: "*.bin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := p.(*RawUploader); !ok {
		t.Errorf("expected *RawUploader, got %T", p)
	}
}

func TestGetPusherInstance_RAW_MapsConfigFields(t *testing.T) {
	cfg := UploaderConfig{
		SrcPattern: "src/**/*.zip",
		DryRun:     true,
		Flatten:    true,
		Include:    []string{"*.zip", "*.tar"},
		Exclude:    []string{"*test*"},
		PkgClient:  nil,
	}

	p, err := getPusherInstance("RAW", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ru, ok := p.(*RawUploader)
	if !ok {
		t.Fatalf("expected *RawUploader, got %T", p)
	}

	if ru.SrcPattern != cfg.SrcPattern {
		t.Errorf("SrcPattern: got %q, want %q", ru.SrcPattern, cfg.SrcPattern)
	}
	if ru.DryRun != cfg.DryRun {
		t.Errorf("DryRun: got %v, want %v", ru.DryRun, cfg.DryRun)
	}
	if ru.Flatten != cfg.Flatten {
		t.Errorf("Flatten: got %v, want %v", ru.Flatten, cfg.Flatten)
	}
	if len(ru.Include) != len(cfg.Include) {
		t.Errorf("Include len: got %d, want %d", len(ru.Include), len(cfg.Include))
	}
	for i, v := range cfg.Include {
		if ru.Include[i] != v {
			t.Errorf("Include[%d]: got %q, want %q", i, ru.Include[i], v)
		}
	}
	if len(ru.Exclude) != len(cfg.Exclude) {
		t.Errorf("Exclude len: got %d, want %d", len(ru.Exclude), len(cfg.Exclude))
	}
	for i, v := range cfg.Exclude {
		if ru.Exclude[i] != v {
			t.Errorf("Exclude[%d]: got %q, want %q", i, ru.Exclude[i], v)
		}
	}
	if ru.PkgClient != nil {
		t.Errorf("PkgClient: expected nil, got non-nil")
	}
}

func TestGetPusherInstance_RAW_SatisfiesPusherInterface(t *testing.T) {
	p, err := getPusherInstance("RAW", UploaderConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var _ Pusher = p
}

// ── Unsupported package types ─────────────────────────────────────────────────

func TestGetPusherInstance_Unsupported_ReturnsError(t *testing.T) {
	unsupported := []ar.PackageType{
		ar.DOCKER,
		ar.GENERIC,
		ar.HELM,
		ar.MAVEN,
		"NPM",
		"PYPI",
		"",
	}

	for _, pt := range unsupported {
		t.Run(string(pt), func(t *testing.T) {
			p, err := getPusherInstance(pt, UploaderConfig{})
			if err == nil {
				t.Errorf("expected error for package type %q, got nil (pusher: %T)", pt, p)
			}
			if p != nil {
				t.Errorf("expected nil pusher for unsupported type, got %T", p)
			}
			if !strings.Contains(err.Error(), "not supported") {
				t.Errorf("error should mention 'not supported', got: %v", err)
			}
		})
	}
}

func TestGetPusherInstance_Unsupported_ErrorContainsPackageType(t *testing.T) {
	_, err := getPusherInstance(ar.DOCKER, UploaderConfig{})
	if err == nil {
		t.Fatal("expected error for DOCKER")
	}
	if !strings.Contains(err.Error(), "DOCKER") {
		t.Errorf("error should name the package type, got: %v", err)
	}
}

// ── UploaderConfig zero value ─────────────────────────────────────────────────

func TestGetPusherInstance_RAW_ZeroConfig(t *testing.T) {
	p, err := getPusherInstance("RAW", UploaderConfig{})
	if err != nil {
		t.Fatalf("unexpected error with zero config: %v", err)
	}
	ru := p.(*RawUploader)
	if ru.SrcPattern != "" {
		t.Errorf("SrcPattern: got %q, want empty", ru.SrcPattern)
	}
	if ru.DryRun {
		t.Error("DryRun: expected false")
	}
	if ru.Flatten {
		t.Error("Flatten: expected false")
	}
	if ru.Include != nil {
		t.Errorf("Include: expected nil, got %v", ru.Include)
	}
	if ru.Exclude != nil {
		t.Errorf("Exclude: expected nil, got %v", ru.Exclude)
	}
}
