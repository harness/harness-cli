package command

import (
	"github.com/spf13/cobra"
)

func NewVariablesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "variables <identity>",
		Short: "List the variables and runtime inputs of a load test",
		Long: `List everything a run of this load test can or must supply.

Two kinds of value are reported, distinguished by the KIND column.

A variable is a named value declared on the test and referenced from its
configuration as <+variable.NAME>. Set it for a run with
"hc rt loadtest start --value NAME=VALUE".

An input is a configuration leaf left as the "<+input>" sentinel. It has no
name, only a path, and the run will not start until every required one is
supplied with "hc rt loadtest start --runtime-value PATH=VALUE".

A test can have none of the first and several of the second, which is usual
for one built around a container image.

For a test imported from a template, both are resolved from the pinned
template revision.`,
		Example: `  hc rt loadtest variables checkout-load

  # Feed the paths straight into a run
  hc rt loadtest variables checkout-load --format json`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			result, err := sess.client.GetVariables(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			return sess.printer.Print(result, settableTable(result.Variables, result.Inputs))
		},
	}
}
