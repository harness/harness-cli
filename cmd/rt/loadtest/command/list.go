package command

import (
	"github.com/harness/harness-cli/cmd/rt/loadtest/api"

	"github.com/spf13/cobra"
)

func NewListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List load tests in the current scope",
		Long: `List the load tests in the account, organization and project you are scoped to.

Results are paginated. Use --limit and --page to walk large projects, and
--search, --tool-type, --environment-id or --tag to narrow the set.`,
		Example: `  # Every load test in the current project
  hc rt loadtest list

  # Only JMeter tests in staging, newest first
  hc rt loadtest list --tool-type JMeter --environment-id staging --sort-field createdAt

  # Full detail as JSON, for scripting
  hc rt loadtest list --limit 100 --format json`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd)
		},
	}

	flags := cmd.Flags()
	flags.Int64("page", 0, "Zero-based page number")
	flags.Int64("limit", 0, "Results per page, up to 100 (default 15)")
	flags.String("search", "", "Filter by a substring of the name")
	flags.String("sort-field", "", "Field to sort by: name, createdAt or updatedAt")
	flags.Bool("sort-ascending", false, "Sort ascending instead of descending")
	flags.String("tool-type", "", "Filter by tool: JMeter, Locust or K6")
	flags.String("environment-id", "", "Filter by environment identifier")
	flags.StringSlice("tag", nil, "Filter by tag; repeat for several")

	return cmd
}

func runList(cmd *cobra.Command) error {
	sess, err := newSession(cmd)
	if err != nil {
		return err
	}

	flags := cmd.Flags()
	page, _ := flags.GetInt64("page")
	limit, _ := flags.GetInt64("limit")
	search, _ := flags.GetString("search")
	sortField, _ := flags.GetString("sort-field")
	sortAscending, _ := flags.GetBool("sort-ascending")
	toolType, _ := flags.GetString("tool-type")
	environment, _ := flags.GetString("environment-id")
	tags, _ := flags.GetStringSlice("tag")

	result, err := sess.client.List(cmd.Context(), api.ListOptions{
		Page:                  page,
		Limit:                 limit,
		Search:                search,
		SortField:             sortField,
		SortAscending:         sortAscending,
		ToolType:              toolType,
		EnvironmentIdentifier: environment,
		Tags:                  tags,
	})
	if err != nil {
		return err
	}

	return sess.printer.Print(result, loadTestTable(result.Items))
}
