package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/harness/harness-cli/internal/style"
)

// StyledHelpTemplate returns a Cobra usage template with ANSI colour hints
// embedded. Cobra evaluates the template and replaces {{.xxx}} placeholders,
// so we inject lipgloss-styled literal text where safe.
//
// We avoid colouring dynamic content (command names, flag names) in the
// template itself because Cobra's template engine can't call lipgloss.
// Instead we style the fixed headings and chrome so the help feels modern
// while still being readable when piped.
func StyledHelpTemplate() string {
	if !style.Enabled {
		return "" // fall back to Cobra default
	}

	heading := lipgloss.NewStyle().Bold(true).Foreground(style.Cyan).Render
	dim := lipgloss.NewStyle().Foreground(style.Dim).Render

	return heading("Usage") + `:
  {{.UseLine}}
` + `{{if .HasAvailableSubCommands}}
` + heading("Available Commands") + `{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }}  {{.Short}}{{end}}{{end}}
{{end}}` + `{{if .HasAvailableLocalFlags}}
` + heading("Flags") + `
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}
{{end}}` + `{{if .HasAvailableInheritedFlags}}
` + heading("Global Flags") + `
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}
{{end}}` + `{{if .HasAvailableSubCommands}}
` + dim(`Use "{{.CommandPath}} [command] --help" for more information about a command.`) + `
{{end}}`
}
