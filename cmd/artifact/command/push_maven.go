package command

import (
	"context"

	"github.com/harness/harness-cli/cmd/cmdutils"
	client "github.com/harness/harness-cli/internal/api/ar"
	client2 "github.com/harness/harness-cli/util/client"
	"github.com/harness/harness-cli/util/common/printer"

	"github.com/spf13/cobra"
)

func NewPushMavenCmd(c *cmdutils.Factory) *cobra.Command {
	var name, registry string
	var pageSize int32
	var pageIndex int32
	cmd := &cobra.Command{
		Use:   "maven",
		Short: "Get artifact details",
		Long:  "Retrieves detailed information about a specific artifact in the Harness Artifact Registry",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := client.GetAllHarnessArtifactsParams{}
			if len(registry) > 0 {
				params.RegIdentifier = &[]string{registry}
			}
			if len(name) > 0 {
				params.SearchTerm = &name
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

			err = printer.Print(response.JSON200.Data.Artifacts, *response.JSON200.Data.PageIndex,
				*response.JSON200.Data.PageCount, *response.JSON200.Data.ItemCount, true, [][]string{
					{"name", "Artifact"},
					{"version", "Version"},
					{"packageType", "Package Type"},
					{"registryIdentifier", "Registry"},
					{"downloadsCount", "Download Count"},
				})

			return err
		},
	}

	// Common flags
	cmd.Flags().StringVar(&name, "name", "", "name of the artifact")
	cmd.Flags().StringVar(&registry, "registry", "", "name of the registry")
	cmd.Flags().Int32Var(&pageSize, "page-size", 10, "number of items per page")
	cmd.Flags().Int32Var(&pageIndex, "page", 0, "page number (zero-indexed)")

	return cmd
}
