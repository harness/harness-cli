package dotnet

import (
	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/cmd/dotnet/command"
	"github.com/harness/harness-cli/cmd/pkgmgr"

	"github.com/spf13/cobra"
)

func GetRootCmd(f *cmdutils.Factory) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "dotnet [command] [args...]",
		Short: "dotnet wrapper with build-info and firewall support",
		Long: `Wrap dotnet commands with build-info collection and firewall integration.
Restore command gets build-info + firewall support.
All other commands are passed through to native dotnet.`,
	}

	rootCmd.AddCommand(command.NewDotnetRestoreCmd(f))

	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		return pkgmgr.RunNativeCommand("dotnet", args)
	}

	return rootCmd
}
