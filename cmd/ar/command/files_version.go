package command

import (
	"context"

	client "github.com/harness/harness-cli/internal/api/ar"
	client2 "github.com/harness/harness-cli/util/client"
	"github.com/harness/harness-cli/util/common/printer"

	"github.com/spf13/cobra"
)

// newFilesVersionCmd wires up:
//
//	hns ar version files <args>
func NewFilesVersionCmd(c *client.ClientWithResponses) *cobra.Command {
	var name string
	var version, registry, artifact string
	var pageSize int32
	var pageIndex int32
	cmd := &cobra.Command{
		Use:   "file",
		Short: "Get artifact file for a version",
		Long:  "Retrieves detailed list of files for a version from Harness Artifact Registry",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				name = args[0]
			}

			params := &client.GetArtifactFilesParams{}
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

			response, err := c.GetArtifactFilesWithResponse(context.Background(),
				client2.GetRef(client2.GetScopeRef(), registry), artifact, version, params)
			if err != nil {
				return err
			}

			err = printer.Print(response.JSON200.Files, *response.JSON200.PageIndex, *response.JSON200.PageCount,
				*response.JSON200.ItemCount, true, [][]string{
					{"name", "File"},
					{"size", "Size"},
				})

			return err
		},
	}

	// Common flags
	cmd.Flags().StringVar(&version, "version", "", "version")
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
	err = cmd.MarkFlagRequired("version")
	if err != nil {
		return nil
	}

	return cmd
}
