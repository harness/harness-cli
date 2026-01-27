package command

import (
	"context"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	ar "github.com/harness/harness-cli/internal/api/ar"
	client2 "github.com/harness/harness-cli/util/client"
	"github.com/harness/harness-cli/util/common/printer"

	"github.com/spf13/cobra"
)

// NewListRegistryCmd wires up:
//
//	hc registry list
func NewListRegistryCmd(f *cmdutils.Factory) *cobra.Command {
	var packageType string
	var pageSize int32
	var pageIndex int32
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all registries",
		Long:  "Lists all Harness Artifact Registries",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Create params for pagination if needed
			params := &ar.GetAllRegistriesParams{}
			if pageSize > 0 {
				size := int64(pageSize)
				params.Size = &size
			}
			if pageIndex > 0 {
				page := int64(pageIndex)
				params.Page = &page
			}
			if len(packageType) > 0 {
				params.PackageType = &[]string{packageType}
			}

			response, err := f.RegistryHttpClient().GetAllRegistriesWithResponse(context.Background(),
				client2.GetRef(config.Global.AccountID, config.Global.OrgID, config.Global.ProjectID),
				params)
			if err != nil {
				return err
			}

			err = printer.Print(response.JSON200.Data.Registries, *response.JSON200.Data.PageIndex,
				*response.JSON200.Data.PageCount, *response.JSON200.Data.ItemCount, true, [][]string{
					{"identifier", "Registry"},
					{"packageType", "Package Type"},
					{"registrySize", "Size"},
					{"type", "Registry Type"},
					{"description", "Description"},
					{"url", "Link"},
				})

			return err
		},
	}

	cmd.Flags().Int32Var(&pageSize, "page-size", 10, "number of items per page")
	cmd.Flags().Int32Var(&pageIndex, "page", 0, "page number (zero-indexed)")
	cmd.Flags().StringVar(&packageType, "package-type", "", "package type")

	return cmd
}
