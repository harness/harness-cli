package command

import (
	"context"
	"fmt"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	client2 "github.com/harness/harness-cli/util/client"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func NewDeleteRegistryCmd(c *cmdutils.Factory) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "delete [name]",
		Short: "Delete registry",
		Long:  "Delete a registry from Harness Artifact Registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name = args[0]
			if len(name) == 0 {
				return fmt.Errorf("must specify registry name")
			}

			client := c.RegistryHttpClient()

			response, err := client.DeleteRegistryWithResponse(context.Background(),
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
