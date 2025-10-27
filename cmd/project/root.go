package project

import (
	"github.com/harness/harness-cli/cmd/project/command"

	"github.com/spf13/cobra"
)

func GetRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:     "project",
		Aliases: []string{"proj"},
		Short:   "Manage Harness Projects",
		Long:    `Commands to manage Harness Projects`,
	}

	// Add subcommands
	rootCmd.AddCommand(command.NewListProjectCmd())
	rootCmd.AddCommand(command.NewGetProjectCmd())
	rootCmd.AddCommand(command.NewCreateProjectCmd())
	rootCmd.AddCommand(command.NewDeleteProjectCmd())

	return rootCmd
}
