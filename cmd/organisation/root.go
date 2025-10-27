package organisation

import (
	"github.com/harness/harness-cli/cmd/organisation/command"

	"github.com/spf13/cobra"
)

func GetRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:     "organisation",
		Aliases: []string{"org"},
		Short:   "Manage Harness Organisations",
		Long:    `Commands to manage Harness Organisations`,
	}

	// Add subcommands
	rootCmd.AddCommand(command.NewListOrganisationCmd())
	rootCmd.AddCommand(command.NewGetOrganisationCmd())
	rootCmd.AddCommand(command.NewCreateOrganisationCmd())
	rootCmd.AddCommand(command.NewDeleteOrganisationCmd())

	return rootCmd
}
