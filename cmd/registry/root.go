package registry

import (
	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/cmd/registry/command"

	"github.com/spf13/cobra"
)

func GetRootCmd(f *cmdutils.Factory) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:     "registry",
		Aliases: []string{"reg"},
		Short:   "Manage Harness Artifact Registries",
		Long:    `Commands to manage Harness Artifact Registry registries`,
	}

	// Add subcommands
	rootCmd.AddCommand(command.NewListRegistryCmd(f))
	rootCmd.AddCommand(command.NewGetRegistryCmd(f))
	rootCmd.AddCommand(command.NewCreateRegistryCmd(f))
	rootCmd.AddCommand(command.NewDeleteRegistryCmd(f))
	rootCmd.AddCommand(getMigrateCmd(f))
	rootCmd.AddCommand(command.NewMetadataCmd(f))

	return rootCmd
}
