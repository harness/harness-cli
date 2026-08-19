package command

import (
	"fmt"
	"regexp"

	"github.com/harness/harness-cli/cmd/rt/loadtest/api"

	"github.com/spf13/cobra"
)

// NewRunCmd groups the operations on load test runs. Starting one is a verb on
// the load test itself, so it lives at "hc rt loadtest start", not here.
func NewRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Inspect and control load test runs",
		Long: `Work with the runs of a load test: list them, follow one to completion,
retune or stop one in flight, and read the results it produced.

Start a run with "hc rt loadtest start"; these commands act on runs that already
exist.`,
	}

	cmd.AddCommand(newRunListCmd())
	cmd.AddCommand(newRunGetCmd())
	cmd.AddCommand(newRunSummaryCmd())
	cmd.AddCommand(newRunGraphCmd())
	cmd.AddCommand(newRunMetricsCmd())
	cmd.AddCommand(newRunWatchCmd())
	cmd.AddCommand(newRunStopCmd())
	cmd.AddCommand(newRunUpdateCmd())
	cmd.AddCommand(newRunRerunCmd())

	return cmd
}

func newRunListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List load test runs",
		Long: `List runs in the current scope, newest first.

Without --load-test-id this covers every load test in the project. Pass
--status to see only the runs in one state, which is the quickest way to find
what is currently executing.`,
		Example: `  # Everything currently running in the project
  hc rt loadtest run list --status Running

  # The run history of one load test
  hc rt loadtest run list --load-test-id checkout-load --limit 20`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := cmd.Flags()
			status, _ := flags.GetString("status")
			if err := validateRunStatus(status); err != nil {
				return err
			}

			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			page, _ := flags.GetInt64("page")
			limit, _ := flags.GetInt64("limit")
			search, _ := flags.GetString("search")
			sortField, _ := flags.GetString("sort-field")
			sortAscending, _ := flags.GetBool("sort-ascending")
			environment, _ := flags.GetString("environment-id")

			opts := api.ListOptions{
				Page:                  page,
				Limit:                 limit,
				Search:                search,
				SortField:             sortField,
				SortAscending:         sortAscending,
				EnvironmentIdentifier: environment,
				Status:                status,
			}

			var result *api.RunList
			if loadTestID, _ := flags.GetString("load-test-id"); loadTestID != "" {
				result, err = sess.client.ListRuns(cmd.Context(), loadTestID, opts)
			} else {
				result, err = sess.client.ListAllRuns(cmd.Context(), opts)
			}
			if err != nil {
				return err
			}

			return sess.printer.Print(result, runTable(result.Items))
		},
	}

	flags := cmd.Flags()
	flags.String("load-test-id", "", "Only list runs of this load test")
	flags.String("status", "", "Filter by status: Pending, Running, Stopping, Stopped, Finished or Failed")
	flags.String("environment-id", "", "Filter by environment identifier")
	flags.Int64("page", 0, "Zero-based page number")
	flags.Int64("limit", 0, "Results per page, up to 100 (default 15)")
	flags.String("search", "", "Filter by a substring of the name")
	flags.String("sort-field", "", "Field to sort by: name, createdAt, updatedAt, runSequence or startedAt")
	flags.Bool("sort-ascending", false, "Sort ascending instead of descending")

	return cmd
}

func validateRunStatus(status string) error {
	switch api.RunStatus(status) {
	case "", api.RunPending, api.RunRunning, api.RunStopping,
		api.RunStopped, api.RunFinished, api.RunFailed:
		return nil
	}
	return fmt.Errorf("unsupported --status %q: supported values are Pending, Running, Stopping, Stopped, Finished, Failed", status)
}

func newRunGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <run-identity>",
		Short: "Show one run",
		Long: `Show the full record of a run, including its resolved configuration and
any errors it reported.

The table view is a summary. Use --format json or --format yaml to see the
resolved variables, the last metrics snapshot and the full error list.`,
		Example: `  hc rt loadtest run get run-01
  hc rt loadtest run get run-01 --format json`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			run, err := sess.client.GetRun(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			return sess.printer.Print(run, runTable([]*api.Run{run}))
		},
	}
}

func newRunSummaryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "summary <run-identity>",
		Short: "Show the aggregate results of a run",
		Long: `Show the aggregate result of a run: throughput, error rate, response time
percentiles and the per-endpoint breakdown.

This is the report you want after a run finishes. While a run is still going
the figures are partial and cover only what has completed so far.`,
		Example: `  hc rt loadtest run summary run-01
  hc rt loadtest run summary run-01 --format json`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			summary, err := sess.client.GetRunSummary(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			return sess.printer.Print(summary, nil)
		},
	}
}

func newRunGraphCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "graph <run-identity>",
		Short: "Show the run graph data",
		Long: `Show the data behind the run graph in the UI.

Use --from and --to to restrict the window. Both take an RFC 3339 timestamp,
for example 2026-08-09T10:00:00Z.`,
		Example: `  hc rt loadtest run graph run-01
  hc rt loadtest run graph run-01 --from 2026-08-09T10:00:00Z --to 2026-08-09T10:15:00Z`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			from, _ := cmd.Flags().GetString("from")
			to, _ := cmd.Flags().GetString("to")

			graph, err := sess.client.GetRunGraph(cmd.Context(), args[0], api.TimeRange{From: from, To: to})
			if err != nil {
				return err
			}

			return sess.printer.Print(graph, nil)
		},
	}

	cmd.Flags().String("from", "", "Start of the window as an RFC 3339 timestamp")
	cmd.Flags().String("to", "", "End of the window as an RFC 3339 timestamp")

	return cmd
}

func newRunMetricsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metrics <run-identity>",
		Short: "Show the metrics of a run",
		Long: `Show one of the metric projections of a run.

The timeseries view is throughput and response time over the life of the run,
scatter is per-request response times for spotting outliers, and aggregate is
the roll-up across the whole run.`,
		Example: `  hc rt loadtest run metrics run-01
  hc rt loadtest run metrics run-01 --view scatter
  hc rt loadtest run metrics run-01 --view aggregate --format json`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			view, _ := cmd.Flags().GetString("view")
			if err := validateMetricsView(view); err != nil {
				return err
			}

			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			from, _ := cmd.Flags().GetString("from")
			to, _ := cmd.Flags().GetString("to")

			metrics, err := sess.client.GetMetrics(cmd.Context(), args[0], api.MetricsView(view), api.TimeRange{From: from, To: to})
			if err != nil {
				return err
			}

			return sess.printer.Print(metrics, nil)
		},
	}

	cmd.Flags().String("view", string(api.MetricsTimeseries), "Which projection to fetch: timeseries, scatter or aggregate")
	cmd.Flags().String("from", "", "Start of the window as an RFC 3339 timestamp")
	cmd.Flags().String("to", "", "End of the window as an RFC 3339 timestamp")

	return cmd
}

func validateMetricsView(view string) error {
	for _, supported := range api.MetricsViews {
		if api.MetricsView(view) == supported {
			return nil
		}
	}
	return fmt.Errorf("unsupported --view %q: supported values are timeseries, scatter, aggregate", view)
}

func newRunStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop <run-identity>",
		Short: "Stop a running load test",
		Long: `Stop a run that is in progress.

The run moves to Stopping and then to Stopped. Metrics collected before the
stop are kept, so the summary and graphs remain available afterwards.`,
		Example:      `  hc rt loadtest run stop run-01`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			stopped, err := sess.client.StopRun(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			return sess.printer.Print(stopped, runTable([]*api.Run{stopped}))
		},
	}
}

func newRunUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <run-identity>",
		Short: "Retune a run that is in flight",
		Long: `Change the load applied by a run that is already in progress.

Only the user count and the spawn rate can be changed mid-run, and only while
the run is in the Running state. The change takes effect without restarting
the run, so the metrics timeline stays continuous across it.`,
		Example: `  # Push a running test harder
  hc rt loadtest run update run-01 --target-users 800

  # Slow down how quickly Locust adds users
  hc rt loadtest run update run-01 --spawn-rate 2.5`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			set := changedFlags(cmd)
			if !set["target-users"] && !set["spawn-rate"] {
				return fmt.Errorf("pass --target-users or --spawn-rate: there is nothing else to change on a running test")
			}

			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			scope := resolveScope()
			request := api.UpdateRunRequest{
				Identity:               args[0],
				AccountIdentifier:      scope.AccountID,
				OrganizationIdentifier: scope.OrgID,
				ProjectIdentifier:      scope.ProjectID,
			}
			if set["target-users"] {
				users, _ := cmd.Flags().GetInt("target-users")
				request.TargetUsers = &users
			}
			if set["spawn-rate"] {
				rate, _ := cmd.Flags().GetFloat64("spawn-rate")
				request.SpawnRate = &rate
			}

			updated, err := sess.client.UpdateRun(cmd.Context(), args[0], request)
			if err != nil {
				return err
			}

			return sess.printer.Print(updated, runTable([]*api.Run{updated}))
		},
	}

	cmd.Flags().Int("target-users", 0, "New concurrent user count")
	cmd.Flags().Float64("spawn-rate", 0, "New users started per second (Locust only)")

	return cmd
}

func newRunRerunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rerun <run-identity>",
		Short: "Start a new run with the settings of a past run",
		Long: `Start a fresh run of the load test that produced an earlier run.

The load test is re-read at start time, so a rerun picks up any change made to
the definition since. It is a convenience for "look up which test this run
belonged to, then run it again", not a replay of the exact earlier execution.`,
		Example: `  hc rt loadtest run rerun run-01
  hc rt loadtest run rerun run-01 --watch`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			previous, err := sess.client.GetRun(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			// The client translates the unique id back to an identity, so a
			// value still in that form means nothing in scope owns it.
			parent := previous.LoadTestIdentity
			switch {
			case parent == "":
				return fmt.Errorf("run %q does not record which load test it belonged to and cannot be rerun", args[0])
			case isUUID(parent):
				return fmt.Errorf("run %q belongs to load test %s, which is not in %s; it may have been deleted",
					args[0], parent, scopeDescription())
			}

			name, _ := cmd.Flags().GetString("run-name")
			if name == "" {
				name = "Rerun of " + args[0]
			}

			started, err := sess.client.CreateRun(cmd.Context(), parent, api.CreateRunRequest{Name: name})
			if err != nil {
				return err
			}

			watch, _ := cmd.Flags().GetBool("watch")
			if !watch {
				fmt.Fprintf(cmd.ErrOrStderr(), "Started run %s from load test %s.\n", started.Identity, parent)
				return sess.printer.Print(started, runTable([]*api.Run{started}))
			}

			return watchRun(cmd, sess, started.Identity)
		},
	}

	flags := cmd.Flags()
	flags.String("run-name", "", "Name for the new run, shown in listings")
	flags.Bool("watch", false, "Follow the new run until it reaches a terminal state")
	flags.Duration("interval", defaultPollInterval, "How often to poll while watching")
	flags.Duration("watch-timeout", 0, "Give up watching after this long (0 waits indefinitely)")

	return cmd
}

// uuidPattern matches the canonical 8-4-4-4-12 hexadecimal form.
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// isUUID separates an internal unique id from a user-facing identity, which the
// service constrains to a short alphanumeric slug.
func isUUID(value string) bool { return uuidPattern.MatchString(value) }
