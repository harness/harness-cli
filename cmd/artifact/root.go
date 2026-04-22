package artifact

import (
	"github.com/harness/harness-cli/cmd/artifact/command"
	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/cmd/maven"
	"github.com/harness/harness-cli/cmd/npm"
	"github.com/harness/harness-cli/cmd/nuget"
	"github.com/harness/harness-cli/cmd/python"

	"github.com/spf13/cobra"
)

func GetRootCmd(f *cmdutils.Factory) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:     "artifact",
		Aliases: []string{"art"},
		Short:   "Manage Harness Artifacts",
		Long:    `Commands to manage artifacts in Harness Artifact Registry`,
	}
	// Add subcommands
	rootCmd.AddCommand(command.NewListArtifactCmd(f))
	rootCmd.AddCommand(command.NewGetArtifactCmd(f))
	rootCmd.AddCommand(command.NewCreateArtifactCmd(f))
	rootCmd.AddCommand(command.NewDeleteArtifactCmd(f))
	rootCmd.AddCommand(command.NewPullArtifactCmd(f))
	rootCmd.AddCommand(command.NewPushArtifactCmd(f))
	rootCmd.AddCommand(command.NewMetadataCmd(f))
	rootCmd.AddCommand(command.NewCopyArtifactCmd(f))
	rootCmd.AddCommand(npm.GetRootCmd(f))
	rootCmd.AddCommand(maven.GetRootCmd(f))
	rootCmd.AddCommand(python.GetRootCmd(f))
	rootCmd.AddCommand(nuget.GetRootCmd(f))

	return rootCmd
}
