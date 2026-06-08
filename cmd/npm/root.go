package npm

import (
	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/cmd/npm/command"
	"github.com/harness/harness-cli/cmd/pkgmgr"

	"github.com/spf13/cobra"
)

func GetRootCmd(f *cmdutils.Factory) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "npm [command] [args...]",
		Short: "npm wrapper with build-info and firewall support",
		Long: `Wrap npm commands with build-info collection and firewall integration.
Install and ci commands get build-info + firewall support.
All other commands are passed through to native npm.`,
	}

	rootCmd.AddCommand(command.NewNpmInstallCmd(f))
	rootCmd.AddCommand(command.NewNpmCiCmd(f))
	rootCmd.AddCommand(command.NewNpmAuditCmd(f))

	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		return pkgmgr.RunNativeCommand("npm", args)
	}

	return rootCmd
}
