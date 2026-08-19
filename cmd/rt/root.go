package rt

import (
	"github.com/harness/harness-cli/cmd/rt/loadtest"

	"github.com/spf13/cobra"
)

// GetRootCmd builds the "hc rt" tree. Resiliency Testing is the module and load
// testing is one feature of it, so the feature is a group rather than a
// top-level command; chaos joins it here when those commands move across.
func GetRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "rt",
		Short: "Manage Harness RT (Resiliency Testing)",
		Long: `Commands for Harness RT, short for Resiliency Testing.

Harness RT covers load testing today. Chaos experiments are still served by
their own CLI and will move under this group.`,
	}

	rootCmd.AddCommand(loadtest.NewCmd())

	return rootCmd
}
