package pip

import (
	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/cmd/pip/command"
	"github.com/harness/harness-cli/cmd/pkgmgr"

	"github.com/spf13/cobra"
)

func GetRootCmd(f *cmdutils.Factory) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "pip [command] [args...]",
		Short: "pip wrapper with build-info and firewall support",
		Long: `Wrap pip commands with build-info collection and firewall integration.
Install command gets build-info + firewall support.
All other commands are passed through to native pip.`,
	}

	rootCmd.AddCommand(command.NewPipInstallCmd(f))

	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		return pkgmgr.RunNativeCommand("pip", args)
	}

	return rootCmd
}
