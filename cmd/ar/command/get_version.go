package command

import (
	"context"
	"github.com/harness/harness-go-sdk/harness/har"

	client "github.com/harness/harness-cli/internal/api/ar"
	client2 "github.com/harness/harness-cli/util/client"
	"github.com/harness/harness-cli/util/common/printer"

	"github.com/spf13/cobra"
)

// newGetVersionCmd wires up:
//
//	hns ar version get <args>
func NewGetVersionCmd(c *har.APIClient) *cobra.Command {
	var name, registry, artifact string
	var pageSize int32
	var pageIndex int32
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Get artifact version details",
		Long:  "Retrieves detailed information about a specific version of an artifact in the Harness Artifact Registry",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				name = args[0]
			}

			params := client.GetAllArtifactVersionsParams{}
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

			response, err := c.GetAllArtifactVersionsWithResponse(context.Background(),
				client2.GetRef(client2.GetScopeRef(), registry), artifact, &params)
			if err != nil {
				return err
			}

			err = printer.Print(response.JSON200.Data.ArtifactVersions, *response.JSON200.Data.PageIndex,
				*response.JSON200.Data.PageCount, *response.JSON200.Data.ItemCount, true, [][]string{
					{"name", "Version"},
					{"registryIdentifier", "Registry"},
					{"packageType", "Package Type"},
					{"fileCount", "Files"},
					{"downloadsCount", "Download Count"},
					{"pullCommand", "Pull Command"},
				})

			return err
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
