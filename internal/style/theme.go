// Package style defines the visual theme for the Harness CLI.
// All colours, borders and text styles are defined here so that every TUI
// component and formatted output uses a consistent look-and-feel.
//
// Call Init(colorEnabled) once at startup. After that, use the exported
// styles and helper functions freely.
package style

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// ─── Colour palette ──────────────────────────────────────────────────────────

var (
	// Brand / primary
	Blue   = lipgloss.Color("#0078D4")
	Cyan   = lipgloss.Color("#00B4D8")
	Indigo = lipgloss.Color("#6366F1")

	// Semantic
	Green  = lipgloss.Color("#22C55E")
	Yellow = lipgloss.Color("#FACC15")
	Red    = lipgloss.Color("#EF4444")
	Orange = lipgloss.Color("#F97316")

	// Neutral
	White   = lipgloss.Color("#FAFAFA")
	Dim     = lipgloss.Color("#6B7280")
	Subtle  = lipgloss.Color("#374151")
	Surface = lipgloss.Color("#1F2937")
	Base    = lipgloss.Color("#111827")
)

// ─── Reusable text styles ────────────────────────────────────────────────────

var (
	// Title is used for top-level headings and the main menu banner.
	Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(Blue).
		PaddingBottom(1)

	// Subtitle is used for section headers.
	Subtitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(Cyan)

	// Success style for positive confirmations.
	Success = lipgloss.NewStyle().
		Foreground(Green).
		Bold(true)

	// Warning style for non-fatal alerts.
	Warning = lipgloss.NewStyle().
		Foreground(Yellow)

	// Error style for error messages.
	Error = lipgloss.NewStyle().
		Foreground(Red).
		Bold(true)

	// DimText is used for hints, secondary info and disabled items.
	DimText = lipgloss.NewStyle().
		Foreground(Dim)

	// Code style for inline code / identifiers.
	Code = lipgloss.NewStyle().
		Foreground(Indigo)

	// Bold is a simple bold helper.
	Bold = lipgloss.NewStyle().Bold(true)
)

// ─── Component styles ────────────────────────────────────────────────────────

var (
	// MenuTitle is the style for the main interactive-menu title bar.
	MenuTitle = lipgloss.NewStyle().
			Background(Blue).
			Foreground(White).
			Bold(true).
			Padding(0, 2)

	// MenuItem is the style for unselected menu items.
	MenuItem = lipgloss.NewStyle().
			PaddingLeft(2)

	// MenuItemSelected is the style for the currently highlighted menu item.
	MenuItemSelected = lipgloss.NewStyle().
				Foreground(Cyan).
				Bold(true).
				PaddingLeft(2)

	// MenuItemDescription is the dim description below an item.
	MenuItemDescription = lipgloss.NewStyle().
				Foreground(Dim).
				PaddingLeft(4)

	// StatusBar is the bottom bar of the TUI.
	StatusBar = lipgloss.NewStyle().
			Foreground(Dim).
			PaddingTop(1)

	// TableHeader styles table column headers.
	TableHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(Cyan).
			BorderBottom(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(Subtle)

	// TableCell is the default table cell style.
	TableCell = lipgloss.NewStyle().
			PaddingRight(2)

	// Box is a bordered container used around forms and detail views.
	Box = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Subtle).
		Padding(1, 2)

	// HelpKey is the style for key bindings shown in help text.
	HelpKey = lipgloss.NewStyle().
		Foreground(Cyan).
		Bold(true)

	// HelpDesc is the style for help text descriptions.
	HelpDesc = lipgloss.NewStyle().
			Foreground(Dim)

	// Spinner is the colour used for spinner animations.
	SpinnerColor = Cyan
)

// ─── Banner ──────────────────────────────────────────────────────────────────

// Banner returns the Harness CLI ASCII banner.
func Banner() string {
	banner := `
 _   _                                    ____ _     ___
| | | | __ _ _ __ _ __   ___  ___ ___    / ___| |   |_ _|
| |_| |/ _` + "`" + ` | '__| '_ \ / _ \/ __/ __|  | |   | |    | |
|  _  | (_| | |  | | | |  __/\__ \__ \  | |___| |___ | |
|_| |_|\__,_|_|  |_| |_|\___||___/___/   \____|_____|___|`

	return lipgloss.NewStyle().Foreground(Blue).Bold(true).Render(banner)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// Enabled tracks whether styles should render ANSI output.
// When false, all styles degrade to plain text.
var Enabled = true

// Init configures the style package. Call once at startup.
func Init(colorEnabled bool) {
	Enabled = colorEnabled
	if !colorEnabled {
		lipgloss.SetColorProfile(termenv.Ascii)
	}
}

// SuccessIcon returns a themed check mark.
func SuccessIcon() string {
	if Enabled {
		return Success.Render("✓")
	}
	return "OK"
}

// ErrorIcon returns a themed X mark.
func ErrorIcon() string {
	if Enabled {
		return Error.Render("✗")
	}
	return "ERROR"
}

// WarningIcon returns a themed warning indicator.
func WarningIcon() string {
	if Enabled {
		return Warning.Render("!")
	}
	return "WARN"
}

// Hint renders a "next step" hint message.
func Hint(msg string) string {
	return DimText.Render("→ " + msg)
}
