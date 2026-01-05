package command

import (
	"context"
	"fmt"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar_v2"
	"github.com/harness/harness-cli/util/metadata"

	"github.com/spf13/cobra"
)

func NewRemoveMetadataCmd(f *cmdutils.Factory) *cobra.Command {
	var registry string
	var metadataStr string

	cmd := &cobra.Command{
		Use:   "remove-metadata",
		Short: "Remove metadata from a registry",
		Long:  "Remove metadata key-value pairs from a Harness Artifact Registry",
		Example: `  hc registry remove-metadata --registry my-docker-reg --metadata "env:prod"
  hc registry remove-metadata --registry npm-packages --metadata "owner:team-a,region:us"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			metadataItems, err := metadata.ParseMetadataString(metadataStr)
			if err != nil {
				return err
			}

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
				return err
			}

			if response.StatusCode() >= 400 {
				if response.JSON400 != nil {
					return fmt.Errorf("bad request: %s", response.JSON400.Message)
				}
				if response.JSON404 != nil {
					return fmt.Errorf("not found: %s", response.JSON404.Message)
				}
				return fmt.Errorf("request failed with status: %d", response.StatusCode())
			}

			fmt.Println("Metadata removed successfully")
			return nil
		},
	}

	cmd.Flags().StringVar(&registry, "registry", "", "Registry identifier (required)")
	cmd.Flags().StringVar(&metadataStr, "metadata", "", "Metadata in key:value,key:value format (required)")
	cmd.MarkFlagRequired("registry")
	cmd.MarkFlagRequired("metadata")

	return cmd
}
