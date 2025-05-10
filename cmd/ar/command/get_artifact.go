package command

import (
	"context"

	"github.com/spf13/cobra"
	"harness/cmd/common/printer"
	client "harness/internal/api/ar"
	client2 "harness/util/client"
)

// newGetArtifactCmd wires up:
//
//	hns ar artifact get <args>
func NewGetArtifactCmd(c *client.ClientWithResponses) *cobra.Command {
	var name, registry string
	var pageSize int32
	var pageIndex int32
	cmd := &cobra.Command{
		Use:   "artifact",
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

			response, err := c.GetAllHarnessArtifactsWithResponse(context.Background(), client2.GetScopeRef(), &params)
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
