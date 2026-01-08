package command

import (
	"context"
	"fmt"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar_v2"
	"github.com/harness/harness-cli/util/common/printer"
	"github.com/harness/harness-cli/util/common/progress"

	"github.com/spf13/cobra"
)

func NewMetadataGetCmd(f *cmdutils.Factory) *cobra.Command {
	var registry string
	var pkg string
	var version string

	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get metadata from a package or version",
		Long:  "Retrieve metadata key-value pairs from a package or specific version in Harness Artifact Registry",
		Example: `  # Package-level metadata
  hc artifact metadata get --registry r1 --package nginx

  # Version-level metadata
  hc artifact metadata get --registry r1 --package nginx --version 1.2.3`,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := progress.NewConsoleReporter()

			p.Start("Fetching metadata")
			params := &ar_v2.GetMetadataParams{
				AccountIdentifier:  config.Global.AccountID,
				RegistryIdentifier: registry,
				Package:            &pkg,
			}

			if version != "" {
				params.Version = &version
			}

			response, err := f.RegistryV2HttpClient().GetMetadataWithResponse(
				context.Background(),
				params,
			)
			if err != nil {
				p.Error("Failed to fetch metadata")
				return err
			}

			if response.StatusCode() >= 400 {
				if response.JSON400 != nil {
					p.Error("Bad request")
					return fmt.Errorf("bad request: %s", response.JSON400.Message)
				}
				if response.JSON404 != nil {
					p.Error("Not found")
					return fmt.Errorf("not found: %s", response.JSON404.Message)
				}
				p.Error("Request failed")
				return fmt.Errorf("request failed with status: %d", response.StatusCode())
			}

			p.Success("Metadata fetched")

			if response.JSON200 != nil && len(response.JSON200.Data.Metadata) > 0 {
				return printer.Print(response.JSON200.Data.Metadata, 0, 1, int64(len(response.JSON200.Data.Metadata)), false, [][]string{
					{"key", "Key"},
					{"value", "Value"},
				})
			}

			fmt.Println("No metadata found")
			return nil
		},
	}

	cmd.Flags().StringVar(&registry, "registry", "", "Registry identifier (required)")
	cmd.Flags().StringVar(&pkg, "package", "", "Package name (required)")
	cmd.Flags().StringVar(&version, "version", "", "Version (optional, for version-level metadata)")
	cmd.MarkFlagRequired("registry")
	cmd.MarkFlagRequired("package")

	return cmd
}
