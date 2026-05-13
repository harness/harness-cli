package mvn

import (
	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/cmd/mvn/command"
	"github.com/harness/harness-cli/cmd/pkgmgr"

	"github.com/spf13/cobra"
)

func GetRootCmd(f *cmdutils.Factory) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "mvn [command] [args...]",
		Short: "Maven wrapper with build-info and firewall support",
		Long: `Wrap Maven commands with build-info collection and firewall integration.
Install command gets build-info + firewall support.
All other commands are passed through to native mvn.`,
	}

	rootCmd.AddCommand(command.NewMvnInstallCmd(f))

	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		return pkgmgr.RunNativeCommand("mvn", args)
	}

	return rootCmd
}
