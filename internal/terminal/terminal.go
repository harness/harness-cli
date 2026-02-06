// Package terminal provides TTY detection and terminal capability helpers.
// It centralises all "is this a terminal?" logic so that commands and the TUI
// layer can make consistent decisions about colour, interactivity and output
// format without duplicating platform-specific checks.
package terminal

import (
	"os"
	"strings"

	"golang.org/x/term"
)

// Info holds the resolved terminal state for the current process.
// Create one at startup via Detect() and pass it down.
type Info struct {
	// IsTerminal is true when stdout is connected to a TTY.
	IsTerminal bool
	// StderrIsTerminal is true when stderr is connected to a TTY.
	StderrIsTerminal bool
	// ColorEnabled is true when ANSI colours should be emitted.
	ColorEnabled bool
	// InteractiveEnabled is true when interactive TUI prompts are allowed.
	InteractiveEnabled bool
	// ForceJSON is true when --json was explicitly passed.
	ForceJSON bool
}

// Detect inspects the environment and returns a populated Info.
// It takes the user-supplied flag values so callers don't have to repeat
// the precedence logic.
//
//	noColor      – true when --no-color was passed (or NO_COLOR env is set)
//	interactive  – true when --interactive / -i was passed
//	forceJSON    – true when --json was passed
func Detect(noColor, interactive, forceJSON bool) Info {
	stdoutFd := int(os.Stdout.Fd())
	stderrFd := int(os.Stderr.Fd())

	isTTY := term.IsTerminal(stdoutFd)
	stderrTTY := term.IsTerminal(stderrFd)

	// Honour the NO_COLOR convention (https://no-color.org/).
	envNoColor := os.Getenv("NO_COLOR") != ""

	colorOn := isTTY && !noColor && !envNoColor
	interactiveOn := isTTY && interactive

	return Info{
		IsTerminal:         isTTY,
		StderrIsTerminal:   stderrTTY,
		ColorEnabled:       colorOn,
		InteractiveEnabled: interactiveOn,
		ForceJSON:          forceJSON,
	}
}

// IsDumb returns true when the terminal is known to have no capabilities
// (e.g. TERM=dumb or running inside Emacs).
func IsDumb() bool {
	t := strings.ToLower(os.Getenv("TERM"))
	return t == "dumb" || t == ""
}

// IsCI returns true when a well-known CI environment variable is set.
func IsCI() bool {
	ciVars := []string{"CI", "GITHUB_ACTIONS", "JENKINS_URL", "GITLAB_CI", "CIRCLECI", "TRAVIS"}
	for _, v := range ciVars {
		if os.Getenv(v) != "" {
			return true
		}
	}
	return false
}
