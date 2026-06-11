package command

import (
	"context"
	"fmt"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar_v2"
	"github.com/harness/harness-cli/util/artifact"
	"github.com/harness/harness-cli/util/common/printer"
	"github.com/harness/harness-cli/util/common/progress"

	"github.com/spf13/cobra"
)

func NewMetadataGetCmd(f *cmdutils.Factory) *cobra.Command {
	var registry string
	var pkg string
	var version string
	var artifactKeyStr string

	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get metadata from a package or version",
		Long:  "Retrieve metadata key-value pairs from a package or specific version in Harness Artifact Registry",
		Example: `  # Package-level metadata
  hc artifact metadata get --registry r1 --package nginx

  # Version-level metadata
  hc artifact metadata get --registry r1 --package nginx --version 1.2.3

  # With artifact key (for unique artifact identification)
  hc artifact metadata get --registry deb11 --package 1oom --version 1.11.7-1 \
    --artifact-key "architecture=riscv64,distribution=bookworm3,component=test"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := progress.NewConsoleReporter()

			// Parse artifact key if provided
			var artifactKey artifact.ArtifactKey
			var err error
			if artifactKeyStr != "" {
				artifactKey, err = artifact.ParseArtifactKeyString(artifactKeyStr)
				if err != nil {
					p.Error("Failed to parse artifact key")
					return fmt.Errorf("invalid artifact key format: %w", err)
				}
			}

			p.Start("Fetching metadata")
			params := &ar_v2.GetMetadataParams{
				AccountIdentifier:  config.Global.AccountID,
				RegistryIdentifier: registry,
				Package:            &pkg,
			}

			if version != "" {
				params.Version = &version
			}

			// Add artifact key filters if provided
			if !artifactKey.IsEmpty() {
				filters := buildFiltersFromArtifactKey(artifactKey)
				params.Filters = &filters
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
				if len(response.Body) > 0 {
					return fmt.Errorf("request failed with status %d: %s", response.StatusCode(), string(response.Body))
				}
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
	cmd.Flags().StringVar(&artifactKeyStr, "artifact-key", "",
		"Artifact key filters as comma-separated key=value pairs\n"+
			"Example: architecture=amd64,distribution=focal,component=main\n"+
			"Accepts any key names - no validation is performed")
	cmd.MarkFlagRequired("registry")
	cmd.MarkFlagRequired("package")

	return cmd
}

// buildFiltersFromArtifactKey converts an ArtifactKey to query parameter format (array)
// Format: []string{"key:value", "key2:value2"}
// Used for GET requests where filters are query parameters
func buildFiltersFromArtifactKey(key artifact.ArtifactKey) ar_v2.FiltersParam {
	var filters []string

	for k, v := range key {
		filters = append(filters, fmt.Sprintf("%s:%s", k, v))
	}

	return filters
}

// buildFiltersMapFromArtifactKey converts an ArtifactKey to request body format (map)
// Format: map[string]string{"key": "value", "key2": "value2"}
// Used for POST/PUT/DELETE requests where filters are in the request body
func buildFiltersMapFromArtifactKey(key artifact.ArtifactKey) map[string]string {
	filters := make(map[string]string)

	for k, v := range key {
		filters[k] = v
	}

	return filters
}
