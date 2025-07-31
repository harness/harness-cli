package ar

import (
	commands "github.com/harness/harness-cli/cmd/ar/command"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar"
	"github.com/harness/harness-cli/util/common/auth"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func GetRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "ar",
		Short: "CLI tool for Harness Artifact Registry",
		Long:  `CLI tool for Harness Artifact Registry and migrate artifacts`,
	}

	client, err := ar.NewClientWithResponses(config.Global.APIBaseURL+"/gateway/har/api/v1", auth.GetXApiKeyOptionAR())
	if err != nil {
		log.Fatal().Msgf("Error creating client: %v", err)
	}

	rootCmd.AddCommand(
		getMigrateCmd(client),
	)

	rootCmd.AddCommand(
		getGetCommand(
			commands.NewGetRegistryCmd(client),
			commands.NewGetArtifactCmd(client),
			commands.NewGetVersionCmd(client),
			commands.NewFilesVersionCmd(client),
		),
	)

	rootCmd.AddCommand(
		getDeleteCmd(
			commands.NewDeleteRegistryCmd(client),
			commands.NewDeleteArtifactCmd(client),
			commands.NewDeleteVersionCmd(client),
		),
	)

	rootCmd.AddCommand(
		getPushCommand(
			commands.NewPushGenericCmd(client),
			commands.NewPushMavenCmd(client),
		),
	)

	rootCmd.AddCommand(
		getPullCommand(
			commands.NewPullGenericCmd(client),
		),
	)

	return rootCmd
}
