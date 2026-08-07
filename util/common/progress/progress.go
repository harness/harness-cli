// Package progress provides progress reporting functionality
package progress

import (
	"fmt"
	"os"

	"github.com/harness/harness-cli/config"
)

// Reporter defines the interface for reporting progress.
// It provides methods to report different stages of an operation
// and its status.
type Reporter interface {
	// Start begins progress reporting with an initial message
	Start(message string)

	// Step reports a new step in the operation
	Step(message string)

	// Error reports an error condition
	Error(message string)

	// Success reports successful completion
	Success(message string)

	// End finalizes progress reporting
	End()
}

// ConsoleReporter implements Reporter by printing messages to console
type ConsoleReporter struct {
	suppressLog bool
}

// NewConsoleReporter creates a new ConsoleReporter
func NewConsoleReporter() *ConsoleReporter {
	return &ConsoleReporter{
		suppressLog: config.Global.NoProgress || SuppressLog(),
	}
}

func (r *ConsoleReporter) Start(message string) {
	if !r.suppressLog {
		fmt.Printf("⚡ %s...\n", message)
	}
}

func (r *ConsoleReporter) Step(message string) {
	if !r.suppressLog {
		fmt.Printf("  ▶ %s...\n", message)
	}
}

func (r *ConsoleReporter) Error(message string) {
	if !r.suppressLog {
		fmt.Printf("  ❌ %s\n", message)
	} else {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", message)
	}
}

func (r *ConsoleReporter) Success(message string) {
	if !r.suppressLog {
		fmt.Printf("  ✅ %s\n", message)
	} else {
		fmt.Println(message)
	}
}

func (r *ConsoleReporter) End() {}

// SuppressLog reports whether the process is running in a CI/non-interactive environment.
// It checks the CI environment variable, which is set automatically by GitHub Actions,
// GitLab CI, CircleCI, Travis CI, Jenkins, and most other CI systems.
func SuppressLog() bool {
	v := os.Getenv("CI")
	return v == "true" || v == "1" || v == "yes"
}

// NopReporter implements Reporter with no-op operations
type NopReporter struct{}

// NewNopReporter creates a new NopReporter
func NewNopReporter() *NopReporter {
	return &NopReporter{}
}

func (r *NopReporter) Start(message string)   {}
func (r *NopReporter) Step(message string)    {}
func (r *NopReporter) Error(message string)   {}
func (r *NopReporter) Success(message string) {}
func (r *NopReporter) End()                   {}
