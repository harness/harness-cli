package command

import (
	"context"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	client "harness/internal/api/ar"
	client2 "harness/util/client"
)

// newDeleteVersionCmd wires up:
//
//	hns ar version delete <args>
func NewDeleteVersionCmd(c *client.ClientWithResponses) *cobra.Command {
	var name, registry, artifact string
	var pageSize int32
	var pageIndex int32
	cmd := &cobra.Command{
		Use:   "version [name]",
		Short: "Delete an artifact version",
		Long:  "Deletes a specific version of an artifact from the Harness Artifact Registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name = args[0]
			response, err := c.DeleteArtifactVersionWithResponse(context.Background(),
				client2.GetRef(client2.GetScopeRef(), registry), artifact, name)

			if err != nil {
				return err
			}
			if response.JSON200 != nil {
				log.Info().Msgf("Deleted version %s; msg:%s", name, response.JSON200.Status)
			} else {
				log.Error().Msgf("failed to delete version %s %s", name, string(response.Body))
			}

			return nil
		},
	}

	// Common flags
	cmd.Flags().StringVar(&registry, "registry", "", "registry name")
	cmd.Flags().StringVar(&artifact, "artifact", "", "artifact name")
	cmd.Flags().Int32Var(&pageSize, "page-size", 10, "number of items per page")
	cmd.Flags().Int32Var(&pageIndex, "page", 0, "page number (zero-indexed)")

	err := cmd.MarkFlagRequired("registry")
	if err != nil {
		return nil
	}
	err = cmd.MarkFlagRequired("artifact")
	if err != nil {
		return nil
	}

	return cmd
}
