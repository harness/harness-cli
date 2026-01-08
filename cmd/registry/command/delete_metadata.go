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
	var metadataStr string

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete metadata from a registry",
		Long:  "Delete metadata key-value pairs from a Harness Artifact Registry",
		Example: `  hc registry metadata delete --registry my-docker-reg --metadata "env:prod"
  hc registry metadata delete --registry npm-packages --metadata "owner:team-a,region:us"`,
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
				Metadata:           metadataItems,
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
	cmd.Flags().StringVar(&metadataStr, "metadata", "", "Metadata in key:value,key:value format (required)")
	cmd.MarkFlagRequired("registry")
	cmd.MarkFlagRequired("metadata")

	return cmd
}
