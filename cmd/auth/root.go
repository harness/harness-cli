package auth

import (
	"github.com/spf13/cobra"
)

// Custom subcommand usage template
var subcommandUsageTemplate = `Usage:
  harness [options] auth <subcommand> [parameters]

{{if .HasAvailableSubCommands}}
Available Commands:{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}{{if (ne .Name "completion")}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}
{{end}}

{{if .HasAvailableLocalFlags}}Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}
{{end}}

{{if .HasAvailableSubCommands}}
Use "harness auth [command] --help" for more information about a command.
{{end}}
`

// GetRootCmd returns the auth command
func GetRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication commands for Harness CLI",
		Long:  `Manage authentication with Harness services`,
	}

	// Add subcommands
	rootCmd.AddCommand(getLoginCmd())
	rootCmd.AddCommand(getLogoutCmd())
	rootCmd.AddCommand(getStatusCmd())

	// Set custom usage template
	rootCmd.SetUsageTemplate(subcommandUsageTemplate)

	return rootCmd
}
