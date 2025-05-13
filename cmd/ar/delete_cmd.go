package ar

import (
	"github.com/spf13/cobra"
)

func getDeleteCmd(cmds ...*cobra.Command) *cobra.Command {
	// Registry command
	registryCmd := &cobra.Command{
		Use:   "delete",
		Short: "Registry management commands",
		Long:  `Commands to manage Harness Artifact Registry registries`,
	}
	for _, cmd := range cmds {
		registryCmd.AddCommand(cmd)
	}

	return registryCmd
}
