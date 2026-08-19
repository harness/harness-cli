package command

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/harness/harness-cli/cmd/rt/loadtest/api"

	"github.com/spf13/cobra"
)

const defaultPollInterval = 10 * time.Second

func newRunWatchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "watch <run-identity>",
		Short: "Follow a run until it finishes",
		Long: `Poll a run and report its progress until it reaches a terminal state.

Progress lines go to stderr and the final run record to stdout, so the output
can be redirected without the progress noise. The exit status reflects the
outcome: zero when the run finishes or is stopped, and non-zero when it fails,
which makes this usable as a pipeline gate.

Each line carries whatever the tool reported: user count, throughput, error
rate and the response-time percentiles. The lat-* fields are time to first
byte and appear for JMeter only, since no other tool measures them.`,
		Example: `  hc rt loadtest run watch run-01
  hc rt loadtest run watch run-01 --interval 5s --watch-timeout 30m`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := newSession(cmd)
			if err != nil {
				return err
			}
			return watchRun(cmd, sess, args[0])
		},
	}

	cmd.Flags().Duration("interval", defaultPollInterval, "How often to poll")
	cmd.Flags().Duration("watch-timeout", 0, "Give up after this long (0 waits indefinitely)")

	return cmd
}

// watchRun polls until the run reaches a terminal state, printing a progress
// line whenever the status or the headline metrics change.
func watchRun(cmd *cobra.Command, sess *session, identity string) error {
	interval, _ := cmd.Flags().GetDuration("interval")
	if interval <= 0 {
		interval = defaultPollInterval
	}

	ctx := cmd.Context()
	timeout, _ := cmd.Flags().GetDuration("watch-timeout")
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	progress := cmd.ErrOrStderr()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var previous string
	var status api.RunStatus

	for {
		run, err := sess.client.GetRun(ctx, identity)
		if err != nil {
			// The deadline can land mid-request as easily as between polls,
			// and the two mean the same thing to the caller. Without this the
			// timeout surfaces as a transport error naming the whole URL.
			if timeout > 0 && errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return watchTimeout(identity, timeout, status)
			}
			return err
		}
		status = run.Status

		if line := progressLine(run); line != previous {
			fmt.Fprintln(progress, line)
			previous = line
		}

		if run.Status.Terminal() {
			if err := sess.printer.Print(run, runTable([]*api.Run{run})); err != nil {
				return err
			}
			return terminalError(run)
		}

		select {
		case <-ctx.Done():
			return watchTimeout(identity, timeout, status)
		case <-ticker.C:
		}
	}
}

// watchTimeout reports giving up on a run that is still going. Watching is a
// read, so the wording has to make clear the run itself was left alone.
func watchTimeout(identity string, timeout time.Duration, status api.RunStatus) error {
	if status == "" {
		return fmt.Errorf("stopped watching run %s after %s: the run itself is unaffected and can be followed again",
			identity, timeout)
	}
	return fmt.Errorf("stopped watching run %s after %s, it was still %s: the run itself is unaffected and can be followed again",
		identity, timeout, status)
}

// progressLine renders one poll as a single line, so that a watch reads as a
// timeline rather than a series of blocks. Fields a tool does not measure are
// left out instead of printed as zero, which would read as a real measurement.
func progressLine(run *api.Run) string {
	metrics := run.LastMetrics
	if metrics == nil {
		return fmt.Sprintf("%-9s users=%d", run.Status, run.TargetUsers)
	}

	var line strings.Builder
	fmt.Fprintf(&line, "%-9s users=", run.Status)
	// During ramp-up the users actually generating load trail the target, and
	// the gap is the thing worth watching. Not every tool reports the current
	// count, so fall back to the target alone rather than showing 0.
	if metrics.CurrentUsers > 0 {
		fmt.Fprintf(&line, "%d/%d", metrics.CurrentUsers, run.TargetUsers)
	} else {
		fmt.Fprintf(&line, "%d", run.TargetUsers)
	}

	fmt.Fprintf(&line, " rps=%.1f requests=%d failures=%d errors=%.2f%%",
		metrics.TotalRPS, metrics.TotalRequests, metrics.TotalFailures, metrics.ErrorRate)
	fmt.Fprintf(&line, " avg=%.0fms p50=%.0fms p95=%.0fms p99=%.0fms",
		metrics.AverageResponseMs, metrics.P50ResponseMs, metrics.P95ResponseMs, metrics.P99ResponseMs)

	// Latency is time to first byte and only JMeter measures it; the other
	// tools leave these zero, where "lat-p99=0ms" would claim an instant reply.
	if hasLatency(metrics) {
		fmt.Fprintf(&line, " lat-avg=%.0fms lat-p50=%.0fms lat-p95=%.0fms lat-p99=%.0fms",
			metrics.AvgLatencyMs, metrics.P50LatencyMs, metrics.P95LatencyMs, metrics.P99LatencyMs)
	}

	return line.String()
}

func hasLatency(metrics *api.MetricSnapshot) bool {
	return metrics.AvgLatencyMs > 0 || metrics.P50LatencyMs > 0 ||
		metrics.P95LatencyMs > 0 || metrics.P99LatencyMs > 0
}

// terminalError turns a failed run into a non-zero exit status, so a pipeline
// step gating on the load test fails when the run does.
func terminalError(run *api.Run) error {
	if run.Status != api.RunFailed {
		return nil
	}
	if run.ErrorMessage != "" {
		return fmt.Errorf("run %s failed: %s", run.Identity, run.ErrorMessage)
	}
	return fmt.Errorf("run %s failed", run.Identity)
}
