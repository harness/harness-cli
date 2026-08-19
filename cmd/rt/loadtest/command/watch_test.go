package command

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/harness/harness-cli/cmd/rt/loadtest/api"

	"github.com/spf13/cobra"
)

// watchAgainst points a watch at a stub run server and returns the error it
// gave up with. The handler decides how slowly a poll answers, which is what
// separates a deadline landing mid-request from one landing between polls.
func watchAgainst(t *testing.T, timeout time.Duration, handler http.HandlerFunc) error {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	cmd := &cobra.Command{}
	cmd.Flags().Duration("interval", 20*time.Millisecond, "")
	cmd.Flags().Duration("watch-timeout", timeout, "")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	// Cobra sets this during Execute; watchRun derives its deadline from it.
	cmd.SetContext(context.Background())

	sess := &session{
		client:  api.NewClient(api.Config{Server: server.URL}),
		printer: &resultPrinter{format: formatYAML, out: io.Discard},
	}

	return watchRun(cmd, sess, "checkout-load-a1b")
}

// The deadline expiring while a poll is in flight is the common case, not the
// rare one: the request occupies most of the interval. It used to escape as a
// transport error naming the whole URL, which reads like the run broke.
func TestWatchTimeoutDuringAPollReadsAsATimeout(t *testing.T) {
	err := watchAgainst(t, 80*time.Millisecond, func(w http.ResponseWriter, r *http.Request) {
		// Outlast the deadline so it lands here rather than between polls.
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte(`{"identity":"checkout-load-a1b","status":"Running"}`))
	})

	if err == nil {
		t.Fatal("expected watching to give up")
	}
	if strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("timeout surfaced as a transport error: %v", err)
	}
	if !strings.Contains(err.Error(), "stopped watching run") {
		t.Errorf("got %q, want the give-up message", err)
	}
	// A run outliving the watch is untouched, and saying so is the whole point
	// of the message.
	if !strings.Contains(err.Error(), "unaffected") {
		t.Errorf("message does not say the run is left alone: %q", err)
	}
}

// The same timeout landing between polls already worked; it must keep the
// status it last saw, since "it was still Running" is the useful part.
func TestWatchTimeoutBetweenPollsNamesTheLastStatus(t *testing.T) {
	err := watchAgainst(t, 60*time.Millisecond, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"identity":"checkout-load-a1b","status":"Running"}`))
	})

	if err == nil {
		t.Fatal("expected watching to give up")
	}
	if !strings.Contains(err.Error(), "still Running") {
		t.Errorf("got %q, want the last observed status", err)
	}
}

// A real failure is not a timeout and must not be dressed up as one, or a
// broken run looks like an impatient watch.
func TestWatchReportsATransportFailureAsItself(t *testing.T) {
	// 400 rather than 500: the read client retries a 5xx with backoff, which
	// would spend seconds proving nothing this test is about.
	err := watchAgainst(t, time.Minute, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadRequest)
	})

	if err == nil {
		t.Fatal("expected the failure to surface")
	}
	if strings.Contains(err.Error(), "stopped watching run") {
		t.Errorf("a server failure was reported as a timeout: %v", err)
	}
}

// The progress line is the only place a watcher sees numbers while the run is
// still going, so every percentile the tool measured has to reach it.
func TestProgressLineCarriesEveryReportedMetric(t *testing.T) {
	line := progressLine(&api.Run{
		Status:      api.RunRunning,
		TargetUsers: 10,
		LastMetrics: &api.MetricSnapshot{
			CurrentUsers:      4,
			TotalRPS:          2.5,
			TotalRequests:     120,
			TotalFailures:     3,
			ErrorRate:         2.5,
			AverageResponseMs: 80,
			P50ResponseMs:     70,
			P95ResponseMs:     105,
			P99ResponseMs:     140,
		},
	})

	for _, want := range []string{
		// Ramping up: four of the ten users are live, and both halves matter.
		"users=4/10",
		"rps=2.5", "requests=120", "failures=3", "errors=2.50%",
		"avg=80ms", "p50=70ms", "p95=105ms", "p99=140ms",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("progress line is missing %q: %s", want, line)
		}
	}
}

// Latency is time to first byte, which only JMeter measures. Printing a zero
// for the others would claim an instant response rather than no measurement.
func TestProgressLineOmitsLatencyWhenTheToolDoesNotMeasureIt(t *testing.T) {
	metrics := &api.MetricSnapshot{TotalRPS: 2.5, P50ResponseMs: 70}

	if line := progressLine(&api.Run{LastMetrics: metrics}); strings.Contains(line, "lat-") {
		t.Errorf("unmeasured latency reported as a value: %s", line)
	}

	metrics.AvgLatencyMs = 12
	metrics.P50LatencyMs = 10
	metrics.P95LatencyMs = 18
	metrics.P99LatencyMs = 25

	line := progressLine(&api.Run{LastMetrics: metrics})
	for _, want := range []string{"lat-avg=12ms", "lat-p50=10ms", "lat-p95=18ms", "lat-p99=25ms"} {
		if !strings.Contains(line, want) {
			t.Errorf("progress line is missing %q: %s", want, line)
		}
	}
}

// A tool that reports no user count at all must not be rendered as "0/10",
// which reads like every user died rather than like a missing field.
func TestProgressLineFallsBackToTheTargetUserCount(t *testing.T) {
	line := progressLine(&api.Run{
		TargetUsers: 10,
		LastMetrics: &api.MetricSnapshot{TotalRPS: 2.5},
	})

	if !strings.Contains(line, "users=10 ") {
		t.Errorf("got %q, want the target count on its own", line)
	}
}

// A run reaching a terminal state is the normal exit and must not be reported
// as giving up, however little time was left on the clock.
func TestWatchStopsCleanlyWhenTheRunFinishes(t *testing.T) {
	err := watchAgainst(t, time.Minute, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"identity":"checkout-load-a1b","status":"Finished"}`))
	})

	if err != nil {
		t.Errorf("a finished run should exit zero, got %v", err)
	}
}
