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

func NewMetadataSetCmd(f *cmdutils.Factory) *cobra.Command {
	var registry string
	var metadataStr string

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set metadata on a registry",
		Long:  "Set metadata key-value pairs on a Harness Artifact Registry",
		Example: `  hc registry metadata set --registry my-docker-reg --metadata "env:prod,region:us"
  hc registry metadata set --registry npm-packages --metadata "owner:team-a"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := progress.NewConsoleReporter()

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
				Metadata:           metadataItems,
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
				return fmt.Errorf("request failed with status: %d", response.StatusCode())
			}

			p.Success("Metadata updated successfully")
			return nil
		},
	}

	cmd.Flags().StringVar(&registry, "registry", "", "Registry identifier (required)")
	cmd.Flags().StringVar(&metadataStr, "metadata", "", "Metadata in key:value,key:value format (required)")
	cmd.MarkFlagRequired("registry")
	cmd.MarkFlagRequired("metadata")

	return cmd
}
