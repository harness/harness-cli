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

func NewSetMetadataCmd(f *cmdutils.Factory) *cobra.Command {
	var registry string
	var metadataStr string

	cmd := &cobra.Command{
		Use:   "set-metadata",
		Short: "Set metadata on a registry",
		Long:  "Set metadata key-value pairs on a Harness Artifact Registry",
		Example: `  hc registry set-metadata --registry my-docker-reg --metadata "env:prod,region:us"
  hc registry set-metadata --registry npm-packages --metadata "owner:team-a"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			metadataItems, err := metadata.ParseMetadataString(metadataStr)
			if err != nil {
				return err
			}

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

			fmt.Println("Metadata updated successfully")
			return nil
		},
	}

	cmd.Flags().StringVar(&registry, "registry", "", "Registry identifier (required)")
	cmd.Flags().StringVar(&metadataStr, "metadata", "", "Metadata in key:value,key:value format (required)")
	cmd.MarkFlagRequired("registry")
	cmd.MarkFlagRequired("metadata")

	return cmd
}
