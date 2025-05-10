package command

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"harness/config"
	client "harness/internal/api/ar"
	client2 "harness/util/client"
)

func NewDeleteRegistryCmd(c *client.ClientWithResponses) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "registry",
		Short: "Delete registry",
		Long:  "Delete a registry from Harness Artifact Registry",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Create params for pagination if needed
			if len(name) == 0 {
				return fmt.Errorf("must specify registry name")
			}

			response, err := c.DeleteRegistryWithResponse(context.Background(),
				client2.GetRef(config.Global.AccountID, config.Global.OrgID, config.Global.ProjectID, name))
			if err != nil {
				return err
			}
			if response.JSON200 != nil {
				log.Info().Msgf("Deleted registry %s", name)
			} else {
				log.Error().Msgf("failed to delete registry %s %s", name, string(response.Body))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "registry name")

	return cmd
}
