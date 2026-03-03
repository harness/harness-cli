package iacm

import (
	"github.com/harness/harness-cli/cmd/iacm/command"

	"github.com/spf13/cobra"
)

func GetRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "iacm",
		Short: "Infrastructure as Code Management commands",
		Long: `Commands for managing infrastructure as code with remote execution.
		
This allows you to run Terraform plans remotely using Harness IACM,
executing on Harness servers while streaming logs back to your CLI.`,
	}

	rootCmd.AddCommand(command.NewPlanCmd())

	return rootCmd
}