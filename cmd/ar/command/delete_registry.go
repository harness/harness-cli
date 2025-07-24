package command

import (
	"context"
	"fmt"
	"github.com/harness/harness-go-sdk/harness/har"

	"github.com/harness/harness-cli/config"
	client "github.com/harness/harness-cli/internal/api/ar"
	client2 "github.com/harness/harness-cli/util/client"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func NewDeleteRegistryCmd(c *har.APIClient) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "registry [name]",
		Short: "Delete registry",
		Long:  "Delete a registry from Harness Artifact Registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name = args[0]
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
	return cmd
}
