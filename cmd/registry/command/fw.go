package command

import (
	"github.com/harness/harness-cli/cmd/cmdutils"

	"github.com/spf13/cobra"
)

func NewFirewallCmd(f *cmdutils.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "fw",
		Aliases: []string{"firewall"},
		Short:   "Firewall and security commands",
		Long:    "Commands to audit and analyze dependencies for security and compliance",
	}

	cmd.AddCommand(NewFirewallAuditCmd(f))
	cmd.AddCommand(NewFirewallExplainCmd(f))

	return cmd
}
