package ar

import (
	"github.com/spf13/cobra"
)

func GetRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "ar",
		Short: "CLI tool for Harness Artifact Registry",
		Long:  `CLI tool for Harness Artifact Registry and migrate artifacts`,
	}

	rootCmd.AddCommand(getMigrateCmd())
	rootCmd.AddCommand(getArtifactsCmd())
	rootCmd.AddCommand(getRegistryCmds())

	return rootCmd
}
