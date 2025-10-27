package artifact

import (
	"github.com/harness/harness-cli/cmd/artifact/command"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar"
	"github.com/harness/harness-cli/util/common/auth"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func GetRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:     "artifact",
		Aliases: []string{"art"},
		Short:   "Manage Harness Artifacts",
		Long:    `Commands to manage artifacts in Harness Artifact Registry`,
	}

	client, err := ar.NewClientWithResponses(config.Global.APIBaseURL+"/gateway/har/api/v1", auth.GetXApiKeyOptionAR())
	if err != nil {
		log.Fatal().Msgf("Error creating client: %v", err)
	}

	// Add subcommands
	rootCmd.AddCommand(command.NewListArtifactCmd(client))
	rootCmd.AddCommand(command.NewGetArtifactCmd(client))
	rootCmd.AddCommand(command.NewCreateArtifactCmd(client))
	rootCmd.AddCommand(command.NewDeleteArtifactCmd(client))
	rootCmd.AddCommand(command.NewPullArtifactCmd(client))
	rootCmd.AddCommand(command.NewPushArtifactCmd(client))

	return rootCmd
}
