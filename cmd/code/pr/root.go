package pr

import (
	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/spf13/cobra"
)

func GetRootCmd(f *cmdutils.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr",
		Short: "Work with pull requests",
		Long:  "Create, list, view, merge, comment on, and check pull requests.",
	}

	cmd.AddCommand(newListCmd(f))
	cmd.AddCommand(newViewCmd(f))
	cmd.AddCommand(newCreateCmd(f))
	cmd.AddCommand(newMergeCmd(f))
	cmd.AddCommand(newCommentCmd(f))
	cmd.AddCommand(newChecksCmd(f))

	return cmd
}
