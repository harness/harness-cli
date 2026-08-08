package command

import (
	"context"
	"fmt"
	"net/http"

	"github.com/harness/harness-cli/cmd/cmdutils"
	ar "github.com/harness/harness-cli/internal/api/ar"
	client2 "github.com/harness/harness-cli/util/client"
	"github.com/harness/harness-cli/util/common/printer"

	"github.com/spf13/cobra"
)

// NewCreateRegistryCmd wires up:
//
//	hc registry create [identifier]
func NewCreateRegistryCmd(c *cmdutils.Factory) *cobra.Command {
	var description, packageType string
	cmd := &cobra.Command{
		Use:   "create [identifier]",
		Short: "Create a new registry",
		Long: "Create a new virtual registry in Harness Artifact Registry. " +
			"For upstream registries or advanced configuration (cleanup policies, patterns, " +
			"proxies), use the Harness REST API.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			identifier := args[0]

			body := ar.RegistryRequest{
				Identifier:  identifier,
				PackageType: ar.PackageType(packageType),
				Config: &ar.RegistryConfig{
					Type: ar.RegistryTypeVIRTUAL,
				},
			}
			if len(description) > 0 {
				body.Description = &description
			}

			spaceRef := client2.GetScopeRef()
			response, err := c.RegistryHttpClient().CreateRegistryWithResponse(context.Background(),
				&ar.CreateRegistryParams{SpaceRef: &spaceRef}, body)
			if err != nil {
				return err
			}

			if response.StatusCode() != http.StatusCreated || response.JSON201 == nil {
				return fmt.Errorf("failed to create registry '%s' (status: %s): %s",
					identifier, response.Status(), createRegistryErrorMessage(response))
			}

			return printer.Print([]ar.Registry{response.JSON201.Data}, 0, 1, 1, false, [][]string{
				{"identifier", "Registry"},
				{"packageType", "Package Type"},
				{"description", "Description"},
				{"url", "Link"},
			})
		},
	}
	cmd.Flags().StringVar(&description, "description", "", "Registry description")
	cmd.Flags().StringVar(&packageType, "package-type", "DOCKER", "Package type (DOCKER, MAVEN, NPM, etc.)")

	return cmd
}

// createRegistryErrorMessage extracts the server's error message when available,
// falling back to the raw body.
func createRegistryErrorMessage(response *ar.CreateRegistryResp) string {
	switch {
	case response.JSON400 != nil && response.JSON400.Message != "":
		return response.JSON400.Message
	case response.JSON401 != nil && response.JSON401.Message != "":
		return response.JSON401.Message
	case response.JSON403 != nil && response.JSON403.Message != "":
		return response.JSON403.Message
	case response.JSON500 != nil && response.JSON500.Message != "":
		return response.JSON500.Message
	default:
		return string(response.Body)
	}
}
