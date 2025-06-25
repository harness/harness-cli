package command

import (
	"context"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	client "github.com/harness/harness-cli/internal/api/ar"
	client2 "github.com/harness/harness-cli/util/client"
)

func NewDeleteArtifactCmd(c *client.ClientWithResponses) *cobra.Command {
	var name, registry string
	cmd := &cobra.Command{
		Use:   "artifact [name]",
		Short: "Delete an artifact from a registry",
		Long:  "Deletes a specific artifact and all its versions from the Harness Artifact Registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name = args[0]
			response, err := c.DeleteArtifactWithResponse(context.Background(),
				client2.GetRef(client2.GetScopeRef(), registry), name)
			if err != nil {
				return err
			}
			if response.JSON200 != nil {
				log.Info().Msgf("Deleted artifact %s; msg:%s", name, response.JSON200.Status)
			} else {
				log.Error().Msgf("failed to delete artifact %s %s", name, string(response.Body))
			}

			return nil
		},
	}

	// Common flags
	cmd.Flags().StringVar(&registry, "registry", "", "name of the registry")

	cmd.MarkFlagRequired("registry")

	return cmd
}
