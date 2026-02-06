package command

import (
	"github.com/harness/harness-cli/cmd/cmdutils"

	"github.com/spf13/cobra"
)

func NewFirewallCmd(f *cmdutils.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "fw",
		Aliases: []string{"firewall"},
		Short:   "Manage firewall settings",
		Long:    "Commands to manage and view firewall settings for artifacts",
	}

	cmd.AddCommand(NewFirewallExplainCmd(f))

	return cmd
}
