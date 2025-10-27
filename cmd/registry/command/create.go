package command

import (
	"fmt"

	client "github.com/harness/harness-cli/internal/api/ar"

	"github.com/spf13/cobra"
)

// NewCreateRegistryCmd wires up:
//
//	hc registry create
func NewCreateRegistryCmd(c *client.ClientWithResponses) *cobra.Command {
	var description, packageType string
	cmd := &cobra.Command{
		Use:   "create [identifier]",
		Short: "Create a new registry",
		Long:  "Create a new registry in Harness Artifact Registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("registry create command not yet implemented")
		},
	}
	cmd.Flags().StringVar(&description, "description", "", "Registry description")
	cmd.Flags().StringVar(&packageType, "package-type", "DOCKER", "Package type (DOCKER, MAVEN, NPM, etc.)")

	return cmd
}
