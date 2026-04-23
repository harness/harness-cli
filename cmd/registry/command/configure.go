package command

import (
	"github.com/harness/harness-cli/cmd/cmdutils"

	"github.com/spf13/cobra"
)

func NewConfigureCmd(f *cmdutils.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Configure package manager clients",
		Long:  "Configure package manager clients to work with Harness Artifact Registry",
	}

	cmd.AddCommand(NewConfigureNpmCmd(f))

	return cmd
}
