package command

import (
	"context"

	client "github.com/harness/harness-cli/internal/api/ar"
	client2 "github.com/harness/harness-cli/util/client"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func NewDeleteArtifactCmd(c *client.ClientWithResponses) *cobra.Command {
	var name, registry, version string
	cmd := &cobra.Command{
		Use:   "delete [artifact-name]",
		Short: "Delete an artifact or a specific version",
		Long:  "Deletes an artifact and all its versions, or a specific version if --version flag is provided",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name = args[0]

			// If version flag is provided, delete specific version
			if version != "" {
				response, err := c.DeleteArtifactVersionWithResponse(context.Background(),
					client2.GetRef(client2.GetScopeRef(), registry), name, version)
				if err != nil {
					return err
				}
				if response.JSON200 != nil {
					log.Info().Msgf("Deleted artifact version %s/%s; msg:%s", name, version, response.JSON200.Status)
				} else {
					log.Error().Msgf("failed to delete artifact version %s/%s %s", name, version, string(response.Body))
				}
				return nil
			}

			// Otherwise, delete entire artifact (all versions)
			response, err := c.DeleteArtifactWithResponse(context.Background(),
				client2.GetRef(client2.GetScopeRef(), registry), name)
			if err != nil {
				return err
			}
			if response.JSON200 != nil {
				log.Info().Msgf("Deleted artifact %s and all its versions; msg:%s", name, response.JSON200.Status)
			} else {
				log.Error().Msgf("failed to delete artifact %s %s", name, string(response.Body))
			}

			return nil
		},
	}

	// Common flags
	cmd.Flags().StringVar(&registry, "registry", "", "name of the registry")
	cmd.Flags().StringVar(&version, "version", "", "specific version to delete (if not provided, deletes all versions)")

	cmd.MarkFlagRequired("registry")

	return cmd
}
