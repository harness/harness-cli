package code

import (
	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/cmd/code/pr"
	"github.com/spf13/cobra"
)

func GetRootCmd(f *cmdutils.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "code",
		Short: "Harness Code repositories and pull requests",
		Long:  "Work with Harness Code Repository — pull requests, branches, and more.",
	}

	cmd.AddCommand(pr.GetRootCmd(f))

	return cmd
}

// GetPrAliasCmd returns a top-level "pr" command that mirrors "code pr".
func GetPrAliasCmd(f *cmdutils.Factory) *cobra.Command {
	cmd := pr.GetRootCmd(f)
	cmd.Long = "Work with pull requests (alias for 'code pr')."
	return cmd
}
