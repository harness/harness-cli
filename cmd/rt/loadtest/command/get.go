package command

import (
	"github.com/harness/harness-cli/cmd/rt/loadtest/api"

	"github.com/spf13/cobra"
)

func NewGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <identity>",
		Short: "Show one load test",
		Long: `Show the full definition of a load test, including its tool configuration.

The table view is a summary. Use --format json or --format yaml to see the
tool configuration, variables and resource settings in full.`,
		Example: `  hc rt loadtest get checkout-load
  hc rt loadtest get checkout-load --format yaml`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			test, err := sess.client.Get(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			return sess.printer.Print(test, loadTestTable([]*api.LoadTest{test}))
		},
	}
}
