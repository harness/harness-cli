package command

import (
	"context"
	"fmt"
	"net/http"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	ar "github.com/harness/harness-cli/internal/api/ar"
	client2 "github.com/harness/harness-cli/util/client"
	"github.com/harness/harness-cli/util/common/printer"

	"github.com/spf13/cobra"
)

// NewGetRegistryCmd wires up:
//
//	hc registry get [name]
func NewGetRegistryCmd(f *cmdutils.Factory) *cobra.Command {
	var packageType string
	var pageSize int32
	var pageIndex int32
	cmd := &cobra.Command{
		Use:   "get [name]",
		Short: "Get registry details",
		Args:  cobra.MaximumNArgs(1),
		Long: "Retrieves detailed information about a specific Harness Artifact Registry. " +
			"When a name is given, exactly that registry is fetched and a missing registry is " +
			"an explicit error; without a name, registries are listed (optionally filtered).",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return getSingleRegistry(f, args[0])
			}

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

// getSingleRegistry fetches one registry by identifier. A missing registry is an
// explicit error, never an empty result.
func getSingleRegistry(f *cmdutils.Factory, name string) error {
	response, err := f.RegistryHttpClient().GetRegistryWithResponse(context.Background(),
		client2.GetRef(config.Global.AccountID, config.Global.OrgID, config.Global.ProjectID, name))
	if err != nil {
		return err
	}

	if response.StatusCode() == http.StatusNotFound {
		if response.JSON404 != nil && response.JSON404.Message != "" {
			return fmt.Errorf("registry '%s' not found: %s", name, response.JSON404.Message)
		}
		return fmt.Errorf("registry '%s' not found", name)
	}
	if response.StatusCode() != http.StatusOK || response.JSON200 == nil {
		return fmt.Errorf("failed to get registry '%s' (status: %s)", name, response.Status())
	}

	return printer.Print([]ar.Registry{response.JSON200.Data}, 0, 1, 1, false, [][]string{
		{"identifier", "Registry"},
		{"packageType", "Package Type"},
		{"description", "Description"},
		{"url", "Link"},
	})
}
