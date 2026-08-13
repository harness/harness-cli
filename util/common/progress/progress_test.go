package progress

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/harness/harness-cli/config"
)

func captureStdout(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	f()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String()
}

func captureStderr(f func()) string {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	f()
	w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String()
}

func TestSuppressLog(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected bool
	}{
		{"CI=true", "true", true},
		{"CI=1", "1", true},
		{"CI=yes", "yes", true},
		{"CI=false", "false", false},
		{"CI=0", "0", false},
		{"CI empty", "", false},
		{"CI=other", "other", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CI", tt.envValue)
			result := SuppressLog()
			if result != tt.expected {
				t.Errorf("SuppressLog() = %v, want %v (CI=%q)", result, tt.expected, tt.envValue)
			}
		})
	}
}

func TestNewConsoleReporter_SuppressedWhenNoProgress(t *testing.T) {
	t.Setenv("CI", "")
	config.Global.NoProgress = true
	defer func() { config.Global.NoProgress = false }()

	r := NewConsoleReporter()
	if !r.suppressLog {
		t.Errorf("expected suppressLog=true when NoProgress=true, got false")
	}
}

func TestNewConsoleReporter_SuppressedWhenCI(t *testing.T) {
	t.Setenv("CI", "true")
	config.Global.NoProgress = false

	r := NewConsoleReporter()
	if !r.suppressLog {
		t.Errorf("expected suppressLog=true when CI=true, got false")
	}
}

func TestNewConsoleReporter_ActiveWhenDefault(t *testing.T) {
	t.Setenv("CI", "")
	config.Global.NoProgress = false

	r := NewConsoleReporter()
	if r.suppressLog {
		t.Errorf("expected suppressLog=false when CI unset and NoProgress=false, got true")
	}
}

func TestConsoleReporter_Start_Active(t *testing.T) {
	r := &ConsoleReporter{suppressLog: false}
	out := captureStdout(func() { r.Start("hello") })
	if !strings.Contains(out, "hello") {
		t.Errorf("expected stdout to contain 'hello', got %q", out)
	}
}

func TestConsoleReporter_Start_Suppressed(t *testing.T) {
	r := &ConsoleReporter{suppressLog: true}
	out := captureStdout(func() { r.Start("hello") })
	if out != "" {
		t.Errorf("expected no stdout when suppressed, got %q", out)
	}
}

func TestConsoleReporter_Step_Active(t *testing.T) {
	r := &ConsoleReporter{suppressLog: false}
	out := captureStdout(func() { r.Step("step one") })
	if !strings.Contains(out, "step one") {
		t.Errorf("expected stdout to contain 'step one', got %q", out)
	}
}

func TestConsoleReporter_Step_Suppressed(t *testing.T) {
	r := &ConsoleReporter{suppressLog: true}
	out := captureStdout(func() { r.Step("step one") })
	if out != "" {
		t.Errorf("expected no stdout when suppressed, got %q", out)
	}
}

func TestConsoleReporter_Error_Active(t *testing.T) {
	r := &ConsoleReporter{suppressLog: false}
	out := captureStdout(func() { r.Error("something failed") })
	if !strings.Contains(out, "something failed") {
		t.Errorf("expected stdout to contain 'something failed', got %q", out)
	}
}

func TestConsoleReporter_Error_Suppressed(t *testing.T) {
	r := &ConsoleReporter{suppressLog: true}
	errOut := captureStderr(func() { r.Error("something failed") })
	if !strings.Contains(errOut, "something failed") {
		t.Errorf("expected stderr to contain 'something failed', got %q", errOut)
	}
}

func TestConsoleReporter_Success_Active(t *testing.T) {
	r := &ConsoleReporter{suppressLog: false}
	out := captureStdout(func() { r.Success("all done") })
	if !strings.Contains(out, "all done") {
		t.Errorf("expected stdout to contain 'all done', got %q", out)
	}
}

func TestConsoleReporter_Success_Suppressed(t *testing.T) {
	r := &ConsoleReporter{suppressLog: true}
	out := captureStdout(func() { r.Success("all done") })
	if !strings.Contains(out, "all done") {
		t.Errorf("expected stdout to contain 'all done' even when suppressed, got %q", out)
	}
}

func TestConsoleReporter_End(t *testing.T) {
	r := &ConsoleReporter{}
	r.End()
}

func TestNopReporter_ImplementsReporter(t *testing.T) {
	var _ Reporter = NewNopReporter()
}

func TestNopReporter_NoPanic(t *testing.T) {
	r := NewNopReporter()
	r.Start("x")
	r.Step("x")
	r.Error("x")
	r.Success("x")
	r.End()
}
