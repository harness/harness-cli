package command

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/harness/harness-cli/cmd/rt/loadtest/api"

	"github.com/spf13/cobra"
)

// NewCompositeCmd groups the composite load test operations. A composite is a
// Harness pipeline, hence no get, update, delete or run here.
func NewCompositeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "composite",
		Short: "Pair load tests with chaos probes",
		Long: `Manage composite load tests, which drive load while a chaos probe injects a
fault so a test measures resilience rather than throughput.

A composite is a Harness pipeline rather than a load test, so it is created
and listed here but executed from the Harness UI or the pipeline API.`,
	}

	cmd.AddCommand(newCompositeCreateCmd())
	cmd.AddCommand(newCompositeListCmd())

	return cmd
}

func newCompositeCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a composite load test",
		Long: `Pair an existing load test with a chaos probe so they run in parallel.

The result is a Harness pipeline with one stage that drives load while the
probe injects a fault, which is how a load test measures resilience rather
than throughput. Both halves must already exist: --load-test-ref names a load
test in the current scope and --probe-id names a chaos probe.

Leave --probe-infra-id out to have the generated step take the infrastructure
as a runtime input, chosen when the pipeline runs. That is usually what you
want for a pipeline shared across environments.

--identifier names the pipeline, so it follows pipeline naming rather than
load test naming: a letter first, then letters, digits, underscores or dollar
signs. Hyphens are valid in a load test identity but not here.

The pipeline is created but not executed. Run it from the Harness UI, or from
a pipeline trigger.`,
		Example: `  # Pair a load test with a probe, choosing the infrastructure at run time
  hc rt loadtest composite create --identifier checkout_resilience \
    --name "Checkout under fault" \
    --load-test-ref checkout-load --probe-id pod-delete-probe

  # Pin the probe to an infrastructure and cap its duration
  hc rt loadtest composite create --identifier checkout_resilience \
    --name "Checkout under fault" --load-test-ref checkout-load \
    --probe-id pod-delete-probe --probe-infra-id perf-cluster --probe-duration 10m \
    --objective "Hold p95 under 500ms while a pod is deleted"`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			request, err := buildCompositeRequest(cmd)
			if err != nil {
				return err
			}

			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			created, err := sess.client.CreateComposite(cmd.Context(), *request)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.ErrOrStderr(), "Created pipeline %q with stage %q. Run it from the Harness UI.\n",
				created.PipelineIdentifier, created.StageIdentifier)
			return sess.printer.Print(created, compositeCreatedTable(created))
		},
	}

	flags := cmd.Flags()

	registerConfigFlag(cmd, "Path to a JSON composite definition; flags override its values")

	// "identifier", not "identity": the composite endpoints follow
	// pipeline-service naming, and the flag matches the field it sets.
	flags.String("identifier", "", "Unique identifier for the composite pipeline; letters, digits, _ and $ only (required)")
	flags.String("name", "", "Display name (defaults to the identifier)")
	flags.String("description", "", "Description of what the composite exercises")
	flags.String("objective", "", "What the pairing is meant to prove, recorded on the pipeline")
	flags.StringToString("tag", nil, "Tag as KEY=VALUE; repeat for several")

	flags.String("load-test-ref", "", "Identity of the load test to drive traffic (required)")
	flags.String("probe-id", "", "Identity of the chaos probe to run alongside it (required)")
	flags.String("probe-infra-id", "", "Chaos infrastructure for the probe; left as a runtime input when omitted")
	flags.String("probe-duration", "", "How long the probe runs, for example 10m")

	return cmd
}

func buildCompositeRequest(cmd *cobra.Command) (*api.CreateCompositeRequest, error) {
	request := &api.CreateCompositeRequest{}

	configPath, _ := cmd.Flags().GetString(FlagConfig)
	if configPath != "" {
		if err := LoadConfigFile(configPath, request); err != nil {
			return nil, err
		}
	}

	set := changedFlags(cmd)
	str := func(name string) string { v, _ := cmd.Flags().GetString(name); return v }

	applyIf(set, "identifier", func() { request.Identifier = str("identifier") })
	applyIf(set, "name", func() { request.Name = str("name") })
	applyIf(set, "description", func() { request.Description = str("description") })
	applyIf(set, "objective", func() { request.Objective = str("objective") })
	applyIf(set, "tag", func() { request.Tags, _ = cmd.Flags().GetStringToString("tag") })
	applyIf(set, "load-test-ref", func() { request.LoadTest.LoadTestRef = str("load-test-ref") })
	applyIf(set, "probe-id", func() { request.Probe.Identity = str("probe-id") })
	applyIf(set, "probe-infra-id", func() { request.Probe.InfraReference = str("probe-infra-id") })
	applyIf(set, "probe-duration", func() { request.Probe.Duration = str("probe-duration") })

	if request.Identifier == "" {
		return nil, fmt.Errorf("--identifier is required")
	}
	if err := validateCompositeIdentifier(request.Identifier); err != nil {
		return nil, err
	}
	if request.Name == "" {
		request.Name = request.Identifier
	}
	if request.LoadTest.LoadTestRef == "" {
		return nil, fmt.Errorf("--load-test-ref is required: name an existing load test, as listed by \"hc rt loadtest list\"")
	}
	if request.Probe.Identity == "" {
		return nil, fmt.Errorf("--probe-id is required: name an existing chaos probe")
	}

	return request, nil
}

// A composite identifier becomes a pipeline identifier, which pipeline-service
// holds to its own naming rules. They are stricter than a load test identity:
// notably hyphens are fine there and rejected here.
var (
	compositeIdentifierPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_$]*$`)
	compositeIdentifierFix     = strings.NewReplacer("-", "_", ".", "_", " ", "_", "/", "_")
)

const compositeIdentifierMaxLen = 128

// validateCompositeIdentifier catches a name pipeline-service will refuse.
// Without it the rejection arrives as an HTTP 500 quoting a pipeline rule, with
// nothing to connect it back to the flag that caused it.
func validateCompositeIdentifier(identifier string) error {
	if len(identifier) > compositeIdentifierMaxLen {
		return fmt.Errorf("--identifier is %d characters: a pipeline identifier is capped at %d",
			len(identifier), compositeIdentifierMaxLen)
	}
	if compositeIdentifierPattern.MatchString(identifier) {
		return nil
	}

	const problem = "--identifier %q is not a valid pipeline identifier: it has to start with a letter and hold only letters, digits, underscores or dollar signs. A composite is a pipeline, so it will not take the hyphens a load test identity does"
	// Only offer a rewrite that actually passes: swapping the separators does
	// nothing for a name that opens with a digit.
	if fixed := compositeIdentifierFix.Replace(identifier); compositeIdentifierPattern.MatchString(fixed) {
		return fmt.Errorf(problem+"; try %q", identifier, fixed)
	}
	return fmt.Errorf(problem, identifier)
}

func newCompositeListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List composite load tests",
		Long: `List the composite load test pipelines in the current scope, with a summary
of their recent executions.

This listing comes from the pipeline service rather than the load test store,
so it takes a different set of filters: --search, --limit and --page work, but
there is no filter by tool, environment, status or tag, and --sort-field
accepts pipeline field names rather than the ones "hc rt loadtest list" takes.`,
		Example: `  hc rt loadtest composite list

  # Most recently executed first
  hc rt loadtest composite list --sort-field executionSummaryInfo.lastExecutionTs

  # Full detail, including every recent execution
  hc rt loadtest composite list --format json`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := cmd.Flags()
			sortField, _ := flags.GetString("sort-field")
			// A different service backs this list, with its own field names.
			// Reject an unsupported value rather than take back an opaque 400.
			if err := validateCompositeSortField(sortField); err != nil {
				return err
			}

			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			page, _ := flags.GetInt64("page")
			limit, _ := flags.GetInt64("limit")
			search, _ := flags.GetString("search")
			sortAscending, _ := flags.GetBool("sort-ascending")

			result, err := sess.client.ListComposites(cmd.Context(), api.ListOptions{
				Page:          page,
				Limit:         limit,
				Search:        search,
				SortField:     sortField,
				SortAscending: sortAscending,
			})
			if err != nil {
				return err
			}

			return sess.printer.Print(result, compositeTable(result.Items))
		},
	}

	flags := cmd.Flags()
	flags.Int64("page", 0, "Zero-based page number")
	flags.Int64("limit", 0, "Results per page, up to 100 (default 15)")
	flags.String("search", "", "Filter by a substring of the name")
	flags.String("sort-field", "", "Field to sort by: name, lastUpdatedAt or executionSummaryInfo.lastExecutionTs")
	flags.Bool("sort-ascending", false, "Sort ascending instead of descending")

	return cmd
}

func validateCompositeSortField(field string) error {
	if field == "" {
		return nil
	}
	for _, supported := range api.CompositeSortFields {
		if field == supported {
			return nil
		}
	}
	return fmt.Errorf("unsupported --sort-field %q: supported values are %s",
		field, strings.Join(api.CompositeSortFields, ", "))
}

func compositeCreatedTable(item *api.Composite) *Table {
	return &Table{
		Headers: []string{"PIPELINE", "STAGE", "IDENTIFIER", "NAME"},
		Rows: [][]string{{
			item.PipelineIdentifier,
			item.StageIdentifier,
			item.Identifier,
			item.Name,
		}},
	}
}

func compositeTable(items []*api.CompositeSummary) *Table {
	table := &Table{Headers: []string{"PIPELINE", "NAME", "LOAD TESTS", "PROBES", "LAST EXECUTION", "UPDATED"}}
	for _, item := range items {
		table.Rows = append(table.Rows, []string{
			item.PipelineIdentifier,
			item.Name,
			strconv.Itoa(item.LoadTestCount),
			strconv.Itoa(item.ProbeCount),
			lastExecution(item.RecentExecutions),
			formatMillis(item.LastUpdatedAt),
		})
	}
	return table
}

// lastExecution summarises the most recent execution for the table view. The
// full list is in the JSON output.
func lastExecution(executions []api.CompositeExecution) string {
	if len(executions) == 0 {
		return "-"
	}
	latest := executions[0]
	for _, execution := range executions[1:] {
		if execution.StartTs > latest.StartTs {
			latest = execution
		}
	}
	if latest.StartTs == 0 {
		return latest.Status
	}
	return fmt.Sprintf("%s (%s)", latest.Status, formatMillis(latest.StartTs))
}

// formatMillis renders a Unix-millisecond timestamp. The composite and usage
// endpoints report time this way, unlike the RFC 3339 strings used on runs.
func formatMillis(millis int64) string {
	if millis <= 0 {
		return "-"
	}
	return time.UnixMilli(millis).UTC().Format(time.RFC3339)
}
