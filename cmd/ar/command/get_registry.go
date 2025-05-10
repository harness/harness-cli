package command

import (
	"context"
	"github.com/spf13/cobra"
	"harness/cmd/common/printer"
	"harness/config"
	ar "harness/internal/api/ar"
	client2 "harness/util/client"
)

// newGetRegistryCmd wires up:
//
//	hns ar registry get <args>
func NewGetRegistryCmd(client *ar.ClientWithResponses) *cobra.Command {
	var name, packageType string
	var pageSize int32
	var pageIndex int32
	cmd := &cobra.Command{
		Use:   "registry",
		Short: "Get registry details",
		Long:  "Retrieves detailed information about a specific Harness Artifact Registry",
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
			if len(name) > 0 {
				params.SearchTerm = &name
			}
			if len(packageType) > 0 {
				params.PackageType = &[]string{packageType}
			}

			response, err := client.GetAllRegistriesWithResponse(context.Background(),
				client2.GetRef(config.Global.AccountID, config.Global.OrgID, config.Global.ProjectID),
				params)
			if err != nil {
				return err
			}

			err = printer.Print(response.JSON200.Data.Registries, *response.JSON200.Data.PageIndex,
				*response.JSON200.Data.PageCount, *response.JSON200.Data.ItemCount, true)

			return err
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "registry name")
	cmd.Flags().Int32Var(&pageSize, "page-size", 10, "number of items per page")
	cmd.Flags().Int32Var(&pageIndex, "page", 0, "page number (zero-indexed)")
	cmd.Flags().StringVar(&packageType, "package-type", "", "package type")

	return cmd
}
