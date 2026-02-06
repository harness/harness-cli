package style

import (
	"strings"
	"testing"
)

func TestInit_EnablesColor(t *testing.T) {
	Init(true)
	if !Enabled {
		t.Error("expected Enabled=true after Init(true)")
	}
}

func TestInit_DisablesColor(t *testing.T) {
	Init(false)
	if Enabled {
		t.Error("expected Enabled=false after Init(false)")
	}
	// Restore for other tests
	Init(true)
}

func TestSuccessIcon_WithColor(t *testing.T) {
	Init(true)
	icon := SuccessIcon()
	if !strings.Contains(icon, "✓") {
		t.Errorf("expected SuccessIcon to contain '✓', got %q", icon)
	}
}

func TestSuccessIcon_NoColor(t *testing.T) {
	Init(false)
	icon := SuccessIcon()
	if icon != "OK" {
		t.Errorf("expected SuccessIcon='OK' when color disabled, got %q", icon)
	}
	Init(true)
}

func TestErrorIcon_WithColor(t *testing.T) {
	Init(true)
	icon := ErrorIcon()
	if !strings.Contains(icon, "✗") {
		t.Errorf("expected ErrorIcon to contain '✗', got %q", icon)
	}
}

func TestErrorIcon_NoColor(t *testing.T) {
	Init(false)
	icon := ErrorIcon()
	if icon != "ERROR" {
		t.Errorf("expected ErrorIcon='ERROR' when color disabled, got %q", icon)
	}
	Init(true)
}

func TestWarningIcon_NoColor(t *testing.T) {
	Init(false)
	icon := WarningIcon()
	if icon != "WARN" {
		t.Errorf("expected WarningIcon='WARN' when color disabled, got %q", icon)
	}
	Init(true)
}

func TestHint(t *testing.T) {
	Init(false)
	h := Hint("run next command")
	if !strings.Contains(h, "run next command") {
		t.Errorf("expected Hint to contain message, got %q", h)
	}
	if !strings.Contains(h, "→") {
		t.Errorf("expected Hint to contain arrow, got %q", h)
	}
	Init(true)
}

func TestBanner(t *testing.T) {
	b := Banner()
	if len(b) == 0 {
		t.Error("expected Banner to be non-empty")
	}
	// The ASCII art contains "CLI" characters in the art
	if !strings.Contains(b, "___") {
		t.Error("expected Banner to contain ASCII art borders")
	}
}
