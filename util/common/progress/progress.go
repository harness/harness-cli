// Package progress provides progress reporting functionality
package progress

import "fmt"

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
