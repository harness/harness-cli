package command

import (
	client "github.com/harness/harness-cli/internal/api/ar"

	"github.com/spf13/cobra"
)

// NewPullArtifactCmd creates a new cobra.Command for pulling artifacts
func NewPullArtifactCmd(c *client.ClientWithResponses) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Pull artifacts from Harness Artifact Registry",
		Long:  `Pull artifacts from Harness Artifact Registry`,
	}

	// Add subcommands for different package types
	cmd.AddCommand(NewPullGenericCmd(c))

	return cmd
}
