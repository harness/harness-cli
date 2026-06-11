package command

import (
	"context"
	"fmt"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar_v2"
	"github.com/harness/harness-cli/util/artifact"
	"github.com/harness/harness-cli/util/common/progress"
	"github.com/harness/harness-cli/util/metadata"

	"github.com/spf13/cobra"
)

func NewMetadataSetCmd(f *cmdutils.Factory) *cobra.Command {
	var registry string
	var pkg string
	var version string
	var metadataStr string
	var artifactKeyStr string

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set metadata on a package or version",
		Long:  "Set metadata key-value pairs on a package or specific version in Harness Artifact Registry",
		Example: `  # Package-level metadata
  hc artifact metadata set --registry r1 --package nginx --metadata "owner:team-a"

  # Version-level metadata
  hc artifact metadata set --registry r1 --package nginx --version 1.2.3 --metadata "approved:true"

  # With artifact key (for unique artifact identification)
  hc artifact metadata set --registry deb11 --package 1oom --version 1.11.7-1 \
    --artifact-key "architecture=riscv64,distribution=bookworm3,component=test" \
    --metadata "approved:true"`,
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

			p.Start("Parsing metadata")
			metadataItems, err := metadata.ParseMetadataString(metadataStr)
			if err != nil {
				p.Error("Failed to parse metadata")
				return err
			}
			p.Success("Metadata parsed")

			p.Step("Updating metadata")
			params := &ar_v2.UpdateMetadataParams{
				AccountIdentifier: config.Global.AccountID,
			}

			body := ar_v2.UpdateMetadataJSONRequestBody{
				RegistryIdentifier: registry,
				Package:            &pkg,
				Metadata:           metadataItems,
			}

			if version != "" {
				body.Version = &version
			}

			// Add artifact key filters if provided
			if !artifactKey.IsEmpty() {
				filters := buildFiltersMapFromArtifactKey(artifactKey)
				body.ArtifactKeyFilters = &filters
			}

			response, err := f.RegistryV2HttpClient().UpdateMetadataWithResponse(
				context.Background(),
				params,
				body,
			)
			if err != nil {
				p.Error("Failed to update metadata")
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

			p.Success("Metadata updated successfully")
			return nil
		},
	}

	cmd.Flags().StringVar(&registry, "registry", "", "Registry identifier (required)")
	cmd.Flags().StringVar(&pkg, "package", "", "Package name (required)")
	cmd.Flags().StringVar(&version, "version", "", "Version (optional, for version-level metadata)")
	cmd.Flags().StringVar(&metadataStr, "metadata", "", "Metadata in key:value,key:value format (required)")
	cmd.Flags().StringVar(&artifactKeyStr, "artifact-key", "",
		"Artifact key filters as comma-separated key=value pairs\n"+
			"Example: architecture=amd64,distribution=focal,component=main\n"+
			"Accepts any key names - no validation is performed")
	cmd.MarkFlagRequired("registry")
	cmd.MarkFlagRequired("package")
	cmd.MarkFlagRequired("metadata")

	return cmd
}
