// Package progress provides progress reporting functionality
package progress

import (
	"fmt"
	"os"
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
type ConsoleReporter struct{}

// NewConsoleReporter creates a new ConsoleReporter
func NewConsoleReporter() *ConsoleReporter {
	return &ConsoleReporter{}
}

func (r *ConsoleReporter) Start(message string) {
	fmt.Printf("⚡ %s...\n", message)
}

func (r *ConsoleReporter) Step(message string) {
	fmt.Printf("  ▶ %s...\n", message)
}

func (r *ConsoleReporter) Error(message string) {
	fmt.Printf("  ❌ %s\n", message)
}

func (r *ConsoleReporter) Success(message string) {
	fmt.Printf("  ✅ %s\n", message)
}

func (r *ConsoleReporter) End() {}

// IsCI reports whether the process is running in a CI/non-interactive environment.
// It checks the CI environment variable, which is set automatically by GitHub Actions,
// GitLab CI, CircleCI, Travis CI, Jenkins, and most other CI systems.
func IsCI() bool {
	v := os.Getenv("CI")
	return v == "true" || v == "1" || v == "yes"
}

// NewReporterAuto returns a CIReporter when noProgress is true or when running in
// a CI environment (CI env var set); otherwise returns a ConsoleReporter for
// interactive terminal use.
func NewReporterAuto(noProgress bool) Reporter {
	if noProgress || IsCI() {
		return NewCIReporter()
	}
	return NewConsoleReporter()
}

// CIReporter implements Reporter for CI/non-interactive environments.
// It suppresses plan-style steps (Start, Step) but emits errors to stderr
// and final success as plain text to stdout.
type CIReporter struct{}

// NewCIReporter creates a new CIReporter.
func NewCIReporter() *CIReporter {
	return &CIReporter{}
}

func (r *CIReporter) Start(message string)   {}
func (r *CIReporter) Step(message string)    {}
func (r *CIReporter) Error(message string)   { fmt.Fprintf(os.Stderr, "ERROR: %s\n", message) }
func (r *CIReporter) Success(message string) { fmt.Println(message) }
func (r *CIReporter) End()                   {}

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
