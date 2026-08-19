package loadtest

import (
	"github.com/harness/harness-cli/cmd/rt/loadtest/command"

	"github.com/spf13/cobra"
)

// NewCmd builds the load test tree: verbs on a load test sit directly under it,
// secondary nouns are grouped, as "hc registry" and "hc artifact" do.
func NewCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:     "loadtest",
		Aliases: []string{"lt"},
		Short:   "Manage Harness RT load tests",
		Long: `Create, run and inspect Harness RT load tests.

A load test pairs a JMeter, Locust or K6 workload with the infrastructure to
run it on. Define one from a script, a container image, a declarative endpoint
spec or a published template, start runs of it and read the results back.

Every command works in the account, organization and project resolved from
--account, --org and --project or from the saved configuration.`,
	}

	// Verbs on the load test itself.
	rootCmd.AddCommand(command.NewCreateCmd())
	rootCmd.AddCommand(command.NewCreateFromJSONCmd())
	rootCmd.AddCommand(command.NewCreateFromTemplateCmd())
	rootCmd.AddCommand(command.NewListCmd())
	rootCmd.AddCommand(command.NewGetCmd())
	rootCmd.AddCommand(command.NewUpdateCmd())
	rootCmd.AddCommand(command.NewUpdateJSONSpecCmd())
	rootCmd.AddCommand(command.NewDeleteCmd())
	rootCmd.AddCommand(command.NewVariablesCmd())
	rootCmd.AddCommand(command.NewExportYAMLCmd())
	rootCmd.AddCommand(command.NewStartCmd())
	rootCmd.AddCommand(command.NewSyncCmd())

	// Secondary nouns.
	rootCmd.AddCommand(command.NewScriptCmd())
	rootCmd.AddCommand(command.NewRunCmd())
	rootCmd.AddCommand(command.NewTemplateCmd())
	rootCmd.AddCommand(command.NewCompositeCmd())
	rootCmd.AddCommand(command.NewUsageCmd())

	return rootCmd
}
