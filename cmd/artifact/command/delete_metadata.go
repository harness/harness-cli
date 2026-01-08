package command

import (
	"context"
	"fmt"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar_v2"
	"github.com/harness/harness-cli/util/common/progress"
	"github.com/harness/harness-cli/util/metadata"

	"github.com/spf13/cobra"
)

func NewMetadataDeleteCmd(f *cmdutils.Factory) *cobra.Command {
	var registry string
	var pkg string
	var version string
	var metadataStr string

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete metadata from a package or version",
		Long:  "Delete metadata key-value pairs from a package or specific version in Harness Artifact Registry",
		Example: `  # Package-level metadata
  hc artifact metadata delete --registry r1 --package nginx --metadata "owner:team-a"

  # Version-level metadata
  hc artifact metadata delete --registry r1 --package nginx --version 1.2.3 --metadata "approved:true"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := progress.NewConsoleReporter()

			p.Start("Parsing metadata")
			metadataItems, err := metadata.ParseMetadataString(metadataStr)
			if err != nil {
				p.Error("Failed to parse metadata")
				return err
			}
			p.Success("Metadata parsed")

			p.Step("Deleting metadata")
			params := &ar_v2.DeleteMetadataParams{
				AccountIdentifier: config.Global.AccountID,
			}

			body := ar_v2.DeleteMetadataJSONRequestBody{
				RegistryIdentifier: registry,
				Package:            &pkg,
				Metadata:           metadataItems,
			}

			if version != "" {
				body.Version = &version
			}

			response, err := f.RegistryV2HttpClient().DeleteMetadataWithResponse(
				context.Background(),
				params,
				body,
			)
			if err != nil {
				p.Error("Failed to delete metadata")
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

			p.Success("Metadata deleted successfully")
			return nil
		},
	}

	cmd.Flags().StringVar(&registry, "registry", "", "Registry identifier (required)")
	cmd.Flags().StringVar(&pkg, "package", "", "Package name (required)")
	cmd.Flags().StringVar(&version, "version", "", "Version (optional, for version-level metadata)")
	cmd.Flags().StringVar(&metadataStr, "metadata", "", "Metadata in key:value,key:value format (required)")
	cmd.MarkFlagRequired("registry")
	cmd.MarkFlagRequired("package")
	cmd.MarkFlagRequired("metadata")

	return cmd
}
