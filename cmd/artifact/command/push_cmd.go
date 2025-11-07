package command

import (
	client "github.com/harness/harness-cli/internal/api/ar"

	"github.com/spf13/cobra"
)

// NewPushArtifactCmd creates a new cobra.Command for pushing artifacts
func NewPushArtifactCmd(c *client.ClientWithResponses) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Push artifacts to Harness Artifact Registry",
		Long:  `Push artifacts to Harness Artifact Registry`,
	}

	// Add subcommands for different package types
	cmd.AddCommand(NewPushGenericCmd(c))
	cmd.AddCommand(NewPushMavenCmd(c))
	cmd.AddCommand(NewPushGoCmd(c))
	cmd.AddCommand(NewPushCondaCmd(c))

	return cmd
}
