package command

import (
	"fmt"

	client "github.com/harness/harness-cli/internal/api/ar"

	"github.com/spf13/cobra"
)

// NewCreateArtifactCmd wires up:
//
//	hc artifact create
func NewCreateArtifactCmd(c *client.ClientWithResponses) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create artifact (use push command instead)",
		Long:  "Artifacts are typically created by pushing them to a registry. Use 'hc artifact push' instead.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("artifacts are created by pushing them to a registry. Use 'hc artifact push' command")
		},
	}

	return cmd
}
