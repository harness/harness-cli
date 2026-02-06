package progress

import (
	"fmt"
	"os"
	"sync"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/harness/harness-cli/internal/style"
	"golang.org/x/term"
)

// StyledReporter implements Reporter with Charm-themed output.
// It uses lipgloss styles for colored output and a spinner animation
// while steps are in progress.
type StyledReporter struct {
	mu      sync.Mutex
	spinner *spinner.Model
	program *tea.Program
	running bool
}

// NewStyledReporter creates a reporter with lipgloss-styled output.
func NewStyledReporter() *StyledReporter {
	return &StyledReporter{}
}

// NewAutoReporter returns a StyledReporter when stdout is a TTY and colours
// are enabled, otherwise falls back to the plain ConsoleReporter.
// Use this in new code instead of NewConsoleReporter() to automatically
// get the best output for the environment.
func NewAutoReporter() Reporter {
	if term.IsTerminal(int(os.Stdout.Fd())) && style.Enabled {
		return NewStyledReporter()
	}
	return NewConsoleReporter()
}

var (
	startStyle   = lipgloss.NewStyle().Bold(true).Foreground(style.Cyan)
	stepStyle    = lipgloss.NewStyle().Foreground(style.Dim).PaddingLeft(2)
	errorStyle   = lipgloss.NewStyle().Foreground(style.Red).Bold(true).PaddingLeft(2)
	successStyle = lipgloss.NewStyle().Foreground(style.Green).Bold(true).PaddingLeft(2)
)

func (r *StyledReporter) Start(message string) {
	fmt.Println(startStyle.Render("⚡ " + message + "..."))
}

func (r *StyledReporter) Step(message string) {
	fmt.Println(stepStyle.Render("→ " + message + "..."))
}

func (r *StyledReporter) Error(message string) {
	fmt.Println(errorStyle.Render("✗ " + message))
}

func (r *StyledReporter) Success(message string) {
	fmt.Println(successStyle.Render("✓ " + message))
}

func (r *StyledReporter) End() {}
