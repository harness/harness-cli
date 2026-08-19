package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <identity>",
		Short: "Delete a load test",
		Long: `Delete a load test and its script revisions.

This cannot be undone. Past run records are retained, but the definition and
its scripts are removed. Pass --force to skip the confirmation prompt in
scripts and pipelines.`,
		Example: `  hc rt loadtest delete checkout-load
  hc rt loadtest delete checkout-load --force`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			identity := args[0]

			force, _ := cmd.Flags().GetBool("force")
			if !force {
				confirmed, err := confirm(cmd, fmt.Sprintf("Delete load test %q and all of its script revisions?", identity))
				if err != nil {
					return err
				}
				if !confirmed {
					fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
					return nil
				}
			}

			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			if err := sess.client.Delete(cmd.Context(), identity); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Deleted load test %q.\n", identity)
			return nil
		},
	}

	cmd.Flags().Bool("force", false, "Delete without asking for confirmation")

	return cmd
}
