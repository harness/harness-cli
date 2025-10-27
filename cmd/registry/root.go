package registry

import (
	"github.com/harness/harness-cli/cmd/registry/command"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar"
	"github.com/harness/harness-cli/util/common/auth"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func GetRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:     "registry",
		Aliases: []string{"reg"},
		Short:   "Manage Harness Artifact Registries",
		Long:    `Commands to manage Harness Artifact Registry registries`,
	}

	client, err := ar.NewClientWithResponses(config.Global.APIBaseURL+"/gateway/har/api/v1", auth.GetXApiKeyOptionAR())
	if err != nil {
		log.Fatal().Msgf("Error creating client: %v", err)
	}

	// Add subcommands
	rootCmd.AddCommand(command.NewListRegistryCmd(client))
	rootCmd.AddCommand(command.NewGetRegistryCmd(client))
	rootCmd.AddCommand(command.NewCreateRegistryCmd(client))
	rootCmd.AddCommand(command.NewDeleteRegistryCmd(client))
	rootCmd.AddCommand(getMigrateCmd(client))

	return rootCmd
}
