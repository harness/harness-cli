package command

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/harness/harness-cli/cmd/rt/loadtest/api"

	"github.com/spf13/cobra"
)

// NewUsageCmd groups the consumption reports. The endpoints read only
// accountIdentifier, so an org or project in scope does not narrow them.
func NewUsageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "usage",
		Short: "Report load testing consumption",
		Long: `Report how much load testing the account has consumed.

These figures are account-wide: an organization or project in scope does not
narrow them. Use "hc rt loadtest usage details" for the per-service breakdown.`,
	}

	cmd.AddCommand(newUsageGetCmd())
	cmd.AddCommand(newUsageDetailsCmd())
	cmd.AddCommand(newUsageReportCmd())

	return cmd
}

// registerUsageWindowFlags declares the time range shared by the three usage
// commands.
func registerUsageWindowFlags(cmd *cobra.Command) {
	flags := cmd.Flags()
	flags.String("start", "", "Start of the window, as a date (2026-01-01), a timestamp (2026-01-01T00:00:00Z) or unix millis")
	flags.String("end", "", "End of the window, in the same forms as --start (default now)")
	flags.String("last", "", "Window ending now, as a duration such as 30d, 24h or 90m; an alternative to --start")
}

// resolveUsageWindow turns the window flags into the unix-millisecond range the
// API takes. Both ends are required, hence --last and a defaulted --end.
func resolveUsageWindow(cmd *cobra.Command) (api.UsageWindow, error) {
	flags := cmd.Flags()
	startText, _ := flags.GetString("start")
	endText, _ := flags.GetString("end")
	lastText, _ := flags.GetString("last")

	if lastText != "" && startText != "" {
		return api.UsageWindow{}, fmt.Errorf("--last and --start are alternatives: --last measures back from now, --start names the beginning of the window")
	}
	if lastText == "" && startText == "" {
		return api.UsageWindow{}, fmt.Errorf("a time window is required: pass --last 30d, or --start and --end")
	}

	now := time.Now()
	end := now
	if endText != "" {
		parsed, err := parseUsageTime(endText, "--end")
		if err != nil {
			return api.UsageWindow{}, err
		}
		end = parsed
	}

	var start time.Time
	if lastText != "" {
		window, err := parseUsageDuration(lastText)
		if err != nil {
			return api.UsageWindow{}, err
		}
		start = end.Add(-window)
	} else {
		parsed, err := parseUsageTime(startText, "--start")
		if err != nil {
			return api.UsageWindow{}, err
		}
		start = parsed
	}

	if end.Before(start) {
		return api.UsageWindow{}, fmt.Errorf("the window ends before it starts: --end %s is earlier than --start %s",
			end.UTC().Format(time.RFC3339), start.UTC().Format(time.RFC3339))
	}

	return api.UsageWindow{
		StartMillis: start.UnixMilli(),
		EndMillis:   end.UnixMilli(),
	}, nil
}

// parseUsageTime takes a plain date, an RFC 3339 timestamp, or the unix
// milliseconds the API itself reports, so a response value can be pasted back.
func parseUsageTime(value, flag string) (time.Time, error) {
	trimmed := strings.TrimSpace(value)

	if millis, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return time.UnixMilli(millis), nil
	}
	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return parsed, nil
	}
	// Midnight UTC, so a window is reproducible wherever it is run from.
	if parsed, err := time.Parse("2006-01-02", trimmed); err == nil {
		return parsed.UTC(), nil
	}

	return time.Time{}, fmt.Errorf("invalid %s %q: expected a date (2026-01-01), a timestamp (2026-01-01T00:00:00Z) or unix milliseconds", flag, value)
}

// parseUsageDuration extends Go's duration syntax with days, since a usage
// window is normally spoken of in days and "720h" is not how anyone says it.
func parseUsageDuration(value string) (time.Duration, error) {
	trimmed := strings.TrimSpace(value)

	if days, found := strings.CutSuffix(trimmed, "d"); found {
		count, err := strconv.ParseFloat(days, 64)
		if err == nil && count > 0 {
			return time.Duration(count * float64(24*time.Hour)), nil
		}
		return 0, fmt.Errorf("invalid --last %q: expected a positive number of days, such as 30d", value)
	}

	parsed, err := time.ParseDuration(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid --last %q: expected a duration such as 30d, 24h or 90m", value)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("invalid --last %q: the window must be positive", value)
	}
	return parsed, nil
}

func newUsageGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Show total load testing usage for the account",
		Long: `Show the account's total load testing consumption over a time window, with
the weights it was calculated from.

Usage is reported for the whole account. An organization or project in scope,
from saved config or from --org and --project, does not narrow it; use
"hc rt loadtest usage details" for the per-service breakdown.

Total usage is a complexity figure, not a count of runs: run count, peak
users, worker count and duration are each weighted by the account's ratio
configuration, which is shown in full with --format json.`,
		Example: `  # The last 30 days
  hc rt loadtest usage get --last 30d

  # A specific month, with the weighting that produced the figure
  hc rt loadtest usage get --start 2026-01-01 --end 2026-02-01 --format json`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			window, err := resolveUsageWindow(cmd)
			if err != nil {
				return err
			}

			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			usage, err := sess.client.GetOverallUsage(cmd.Context(), window)
			if err != nil {
				return err
			}

			table := &Table{
				Headers: []string{"ACCOUNT", "SERVICES", "TOTAL USAGE", "FROM", "TO"},
				Rows: [][]string{{
					usage.AccountID,
					strconv.Itoa(usage.ServiceCount),
					strconv.FormatFloat(usage.TotalUsage, 'f', 2, 64),
					formatMillis(window.StartMillis),
					formatMillis(window.EndMillis),
				}},
			}
			return sess.printer.Print(usage, table)
		},
	}

	registerUsageWindowFlags(cmd)

	return cmd
}

func newUsageDetailsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "details",
		Short: "Break down load testing usage by service",
		Long: `List load testing consumption per service over a time window.

Every service in the account is listed, with the organization and project it
belongs to, whatever scope the command was run in. Complexity is the weighted
figure the service contributes to the account total; the raw counters it comes
from are listed alongside it.`,
		Example: `  hc rt loadtest usage details --last 30d

  # A quarter, in full
  hc rt loadtest usage details --start 2026-01-01 --end 2026-04-01 --format json`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			window, err := resolveUsageWindow(cmd)
			if err != nil {
				return err
			}

			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			details, err := sess.client.GetUsageDetails(cmd.Context(), window)
			if err != nil {
				return err
			}

			return sess.printer.Print(details, usageTable(details.Services))
		},
	}

	registerUsageWindowFlags(cmd)

	return cmd
}

func usageTable(items []api.ServiceUsage) *Table {
	table := &Table{Headers: []string{"SERVICE", "ORG", "PROJECT", "RUNS", "PEAK USERS", "WORKERS", "DURATION", "VU-SECONDS", "COMPLEXITY"}}
	for _, item := range items {
		table.Rows = append(table.Rows, []string{
			item.ServiceID,
			item.OrgID,
			item.ProjectID,
			strconv.FormatInt(item.RunCount, 10),
			strconv.Itoa(item.PeakUsers),
			strconv.Itoa(item.MaxWorkerCount),
			strconv.FormatInt(item.TotalDurationSec, 10) + "s",
			strconv.FormatInt(item.TotalVUSeconds, 10),
			strconv.FormatFloat(item.Complexity, 'f', 2, 64),
		})
	}
	return table
}

func newUsageReportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Export a usage report",
		Long: `Export the per-service usage breakdown as a report.

The service returns this one as rows rather than as objects: a header row,
one row per service and a total row. That makes it the form to hand to a
spreadsheet, so --output-file writes it as CSV. Without --output-file it is
printed as a table, and --format json returns the raw rows.

The figures are the same as "hc rt loadtest usage details"; only the shape
differs.`,
		Example: `  hc rt loadtest usage report --last 30d

  # Save last quarter as a spreadsheet
  hc rt loadtest usage report --start 2026-01-01 --end 2026-04-01 --output-file ./usage.csv`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			window, err := resolveUsageWindow(cmd)
			if err != nil {
				return err
			}

			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			rows, err := sess.client.GetUsageReport(cmd.Context(), window)
			if err != nil {
				return err
			}

			if destination, _ := cmd.Flags().GetString("output-file"); destination != "" {
				return writeCSV(cmd, destination, rows)
			}

			return sess.printer.Print(rows, reportTable(rows))
		},
	}

	registerUsageWindowFlags(cmd)
	cmd.Flags().String("output-file", "", "Write the report to this path as CSV")

	return cmd
}

// reportTable renders the report matrix, taking the first row as the header
// the service already supplied rather than inventing column names.
func reportTable(rows [][]string) *Table {
	if len(rows) == 0 {
		return &Table{}
	}
	return &Table{Headers: rows[0], Rows: rows[1:]}
}

func writeCSV(cmd *cobra.Command, destination string, rows [][]string) error {
	file, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("creating %q: %w", destination, err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	if err := writer.WriteAll(rows); err != nil {
		return fmt.Errorf("writing %q: %w", destination, err)
	}
	// WriteAll flushes, but the error it buffers surfaces only here.
	if err := writer.Error(); err != nil {
		return fmt.Errorf("writing %q: %w", destination, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("closing %q: %w", destination, err)
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "Wrote %d rows to %s.\n", len(rows), destination)
	return nil
}
