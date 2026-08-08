package command

import (
	"context"
	"fmt"

	"github.com/harness/harness-cli/cmd/cmdutils"
	client "github.com/harness/harness-cli/internal/api/ar"
	client2 "github.com/harness/harness-cli/util/client"
	"github.com/harness/harness-cli/util/common/printer"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// NewListArtifactCmd wires up:
//
//	hc artifact list [registry]
func NewListArtifactCmd(c *cmdutils.Factory) *cobra.Command {
	var registry string
	var pageSize int32
	var pageIndex int32
	cmd := &cobra.Command{
		Use:   "list [registry]",
		Short: "List all artifacts",
		Long: "Lists all artifacts in the Harness Artifact Registry. Pass a registry name " +
			"(or --registry) to scope the listing to a single registry; with neither, artifacts " +
			"across all registries in the account are listed.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				if len(registry) > 0 && registry != args[0] {
					return fmt.Errorf("conflicting registry arguments: positional '%s' and --registry '%s'",
						args[0], registry)
				}
				registry = args[0]
			}

			params := client.GetAllHarnessArtifactsParams{}
			if len(registry) > 0 {
				params.RegIdentifier = &[]string{registry}
			}

			if pageSize > 0 {
				size := int64(pageSize)
				params.Size = &size
			}
			if pageIndex > 0 {
				page := int64(pageIndex)
				params.Page = &page
			}

			httpClient := c.RegistryHttpClient()

			response, err := httpClient.GetAllHarnessArtifactsWithResponse(context.Background(), client2.GetScopeRef(), &params)
			if err != nil {
				return err
			}

			if response.JSON200 == nil || response.JSON200.Data.Artifacts == nil {
				log.Debug().Msgf("Unable to list artifacts")
				return fmt.Errorf("unable to list artifacts")
			}

			err = printer.Print(response.JSON200.Data.Artifacts, *response.JSON200.Data.PageIndex,
				*response.JSON200.Data.PageCount, *response.JSON200.Data.ItemCount, true, [][]string{
					{"name", "Artifact"},
					{"version", "Version"},
					{"artifactKey", "Artifact Key"},
					{"packageType", "Package Type"},
					{"registryIdentifier", "Registry"},
					{"downloadsCount", "Download Count"},
				})

			return err
		},
	}

	cmd.Flags().StringVar(&registry, "registry", "", "name of the registry")
	cmd.Flags().Int32Var(&pageSize, "page-size", 10, "number of items per page")
	cmd.Flags().Int32Var(&pageIndex, "page", 0, "page number (zero-indexed)")

	return cmd
}
