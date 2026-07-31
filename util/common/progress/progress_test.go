package progress

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdout redirects os.Stdout, runs f, then returns what was written.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = orig

	var buf bytes.Buffer
	io.Copy(&buf, r) //nolint:errcheck
	return buf.String()
}

// captureStderr redirects os.Stderr, runs f, then returns what was written.
func captureStderr(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w

	f()

	w.Close()
	os.Stderr = orig

	var buf bytes.Buffer
	io.Copy(&buf, r) //nolint:errcheck
	return buf.String()
}

// setCI sets the CI environment variable and returns a cleanup function.
func setCI(t *testing.T, value string) {
	t.Helper()
	if value == "" {
		t.Setenv("CI", "")
	} else {
		t.Setenv("CI", value)
	}
}

// --- IsCI ---

func TestIsCI(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"true literal", "true", true},
		{"1", "1", true},
		{"yes", "yes", true},
		{"empty", "", false},
		{"false", "false", false},
		{"0", "0", false},
		{"uppercase TRUE", "TRUE", false},
		{"random string", "enabled", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setCI(t, tc.value)
			if got := IsCI(); got != tc.want {
				t.Errorf("IsCI() = %v, want %v (CI=%q)", got, tc.want, tc.value)
			}
		})
	}
}

// --- NewReporterAuto ---

func TestNewReporterAuto_NoProgressFlag(t *testing.T) {
	setCI(t, "")

	r := NewReporterAuto(true)
	if _, ok := r.(*CIReporter); !ok {
		t.Errorf("expected *CIReporter when noProgress=true, got %T", r)
	}
}

func TestNewReporterAuto_CIEnvVar(t *testing.T) {
	for _, val := range []string{"true", "1", "yes"} {
		t.Run("CI="+val, func(t *testing.T) {
			setCI(t, val)
			r := NewReporterAuto(false)
			if _, ok := r.(*CIReporter); !ok {
				t.Errorf("expected *CIReporter when CI=%q, got %T", val, r)
			}
		})
	}
}

func TestNewReporterAuto_Interactive(t *testing.T) {
	setCI(t, "")
	r := NewReporterAuto(false)
	if _, ok := r.(*ConsoleReporter); !ok {
		t.Errorf("expected *ConsoleReporter when noProgress=false and CI unset, got %T", r)
	}
}

func TestNewReporterAuto_NoProgressOverridesCIUnset(t *testing.T) {
	setCI(t, "false")
	r := NewReporterAuto(true)
	if _, ok := r.(*CIReporter); !ok {
		t.Errorf("expected *CIReporter when noProgress=true even if CI=false, got %T", r)
	}
}

// --- CIReporter ---

func TestCIReporter_StartProducesNoOutput(t *testing.T) {
	r := NewCIReporter()
	out := captureStdout(t, func() { r.Start("doing something") })
	if out != "" {
		t.Errorf("CIReporter.Start() wrote %q, want empty", out)
	}
}

func TestCIReporter_StepProducesNoOutput(t *testing.T) {
	r := NewCIReporter()
	out := captureStdout(t, func() { r.Step("some step") })
	if out != "" {
		t.Errorf("CIReporter.Step() wrote %q, want empty", out)
	}
}

func TestCIReporter_EndProducesNoOutput(t *testing.T) {
	r := NewCIReporter()
	out := captureStdout(t, func() { r.End() })
	if out != "" {
		t.Errorf("CIReporter.End() wrote %q, want empty", out)
	}
}

func TestCIReporter_ErrorWritesToStderr(t *testing.T) {
	r := NewCIReporter()
	errOut := captureStderr(t, func() { r.Error("something went wrong") })
	if !strings.Contains(errOut, "ERROR:") {
		t.Errorf("CIReporter.Error() stderr = %q, want it to contain 'ERROR:'", errOut)
	}
	if !strings.Contains(errOut, "something went wrong") {
		t.Errorf("CIReporter.Error() stderr = %q, want it to contain the message", errOut)
	}
}

func TestCIReporter_ErrorDoesNotWriteToStdout(t *testing.T) {
	r := NewCIReporter()
	stdout := captureStdout(t, func() { r.Error("oops") })
	if stdout != "" {
		t.Errorf("CIReporter.Error() wrote to stdout: %q", stdout)
	}
}

func TestCIReporter_SuccessWritesToStdout(t *testing.T) {
	r := NewCIReporter()
	out := captureStdout(t, func() { r.Success("all done") })
	if !strings.Contains(out, "all done") {
		t.Errorf("CIReporter.Success() stdout = %q, want it to contain the message", out)
	}
}

func TestCIReporter_SuccessDoesNotWriteToStderr(t *testing.T) {
	r := NewCIReporter()
	errOut := captureStderr(t, func() { r.Success("all done") })
	if errOut != "" {
		t.Errorf("CIReporter.Success() wrote to stderr: %q", errOut)
	}
}

// --- ConsoleReporter ---

func TestConsoleReporter_StartFormat(t *testing.T) {
	r := NewConsoleReporter()
	out := captureStdout(t, func() { r.Start("loading") })
	if !strings.Contains(out, "loading") {
		t.Errorf("ConsoleReporter.Start() = %q, want message included", out)
	}
}

func TestConsoleReporter_StepFormat(t *testing.T) {
	r := NewConsoleReporter()
	out := captureStdout(t, func() { r.Step("processing") })
	if !strings.Contains(out, "processing") {
		t.Errorf("ConsoleReporter.Step() = %q, want message included", out)
	}
}

func TestConsoleReporter_ErrorFormat(t *testing.T) {
	r := NewConsoleReporter()
	out := captureStdout(t, func() { r.Error("failed") })
	if !strings.Contains(out, "failed") {
		t.Errorf("ConsoleReporter.Error() = %q, want message included", out)
	}
}

func TestConsoleReporter_SuccessFormat(t *testing.T) {
	r := NewConsoleReporter()
	out := captureStdout(t, func() { r.Success("done") })
	if !strings.Contains(out, "done") {
		t.Errorf("ConsoleReporter.Success() = %q, want message included", out)
	}
}
