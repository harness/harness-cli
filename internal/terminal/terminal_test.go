package terminal

import (
	"os"
	"testing"
)

func TestDetect_NoColorFlag(t *testing.T) {
	info := Detect(true, false, false)
	if info.ColorEnabled {
		t.Error("expected ColorEnabled=false when noColor=true")
	}
}

func TestDetect_ForceJSON(t *testing.T) {
	info := Detect(false, false, true)
	if !info.ForceJSON {
		t.Error("expected ForceJSON=true when forceJSON=true")
	}
}

func TestDetect_InteractiveRequiresTTY(t *testing.T) {
	// In test environment, stdout is typically not a terminal
	info := Detect(false, true, false)
	// Interactive should only be enabled if stdout is actually a TTY
	if info.InteractiveEnabled && !info.IsTerminal {
		t.Error("InteractiveEnabled should be false when stdout is not a TTY")
	}
}

func TestDetect_NonTTY(t *testing.T) {
	// In test environments, stdout is not a terminal
	info := Detect(false, false, false)
	if info.IsTerminal {
		// This is okay in some test environments - skip if actually a TTY
		t.Skip("test stdout appears to be a TTY, skipping non-TTY test")
	}
	if info.ColorEnabled {
		t.Error("expected ColorEnabled=false when not a TTY")
	}
}

func TestDetect_NOCOLOREnv(t *testing.T) {
	os.Setenv("NO_COLOR", "1")
	defer os.Unsetenv("NO_COLOR")

	info := Detect(false, false, false)
	if info.ColorEnabled {
		t.Error("expected ColorEnabled=false when NO_COLOR env is set")
	}
}

func TestIsDumb(t *testing.T) {
	original := os.Getenv("TERM")
	defer os.Setenv("TERM", original)

	os.Setenv("TERM", "dumb")
	if !IsDumb() {
		t.Error("expected IsDumb()=true when TERM=dumb")
	}

	os.Setenv("TERM", "xterm-256color")
	if IsDumb() {
		t.Error("expected IsDumb()=false when TERM=xterm-256color")
	}
}

func TestIsCI(t *testing.T) {
	// Save and clear all CI variables
	ciVars := []string{"CI", "GITHUB_ACTIONS", "JENKINS_URL", "GITLAB_CI", "CIRCLECI", "TRAVIS"}
	saved := make(map[string]string)
	for _, v := range ciVars {
		saved[v] = os.Getenv(v)
		os.Unsetenv(v)
	}
	defer func() {
		for k, v := range saved {
			if v != "" {
				os.Setenv(k, v)
			}
		}
	}()

	if IsCI() {
		t.Error("expected IsCI()=false when no CI env vars are set")
	}

	os.Setenv("CI", "true")
	if !IsCI() {
		t.Error("expected IsCI()=true when CI=true")
	}
}
