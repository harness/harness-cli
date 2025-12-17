package command

import (
	"github.com/harness/harness-cli/cmd/cmdutils"

	"github.com/spf13/cobra"
)

// NewPushArtifactCmd creates a new cobra.Command for pushing artifacts
func NewPushArtifactCmd(f *cmdutils.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Push artifacts to Harness Artifact Registry",
		Long:  `Push artifacts to Harness Artifact Registry`,
	}

	// Add subcommands for different package types

	cmd.AddCommand(NewPushGenericCmd(f))
	cmd.AddCommand(NewPushMavenCmd(f))
	cmd.AddCommand(NewPushGoCmd(f))
	cmd.AddCommand(NewPushCondaCmd(f))
	cmd.AddCommand(NewPushComposerCmd(f))
	cmd.AddCommand(NewPushRpmCmd(f))
	cmd.AddCommand(NewPushCargoCmd(f))
	cmd.AddCommand(NewPushNugetCmd(f))
	cmd.AddCommand(NewPushNpmCmd(f))
	cmd.AddCommand(NewPushDartCmd(f))

	return cmd
}
