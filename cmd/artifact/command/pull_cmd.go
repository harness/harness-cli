package command

import (
	"github.com/harness/harness-cli/cmd/cmdutils"

	"github.com/spf13/cobra"
)

// NewPullArtifactCmd creates a new cobra.Command for pulling artifacts
func NewPullArtifactCmd(c *cmdutils.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Pull artifacts from Harness Artifact Registry",
		Long:  `Pull artifacts from Harness Artifact Registry`,
	}

	// Add subcommands for different package types
	cmd.AddCommand(NewPullGenericCmd(c))

	return cmd
}
