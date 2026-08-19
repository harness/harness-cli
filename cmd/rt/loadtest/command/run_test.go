package command

import (
	"net/http"
	"strings"
	"testing"

	"github.com/harness/harness-cli/cmd/rt/loadtest/api"
)

// aRun is a run as the service returns one. The parent is a slug rather than a
// unique id so reading it costs a single request; the translation path has its
// own test.
func aRun(identity string, status api.RunStatus) map[string]any {
	return map[string]any{
		"uniqueId":         "9f1c2d3e-0000-4000-8000-000000000001",
		"identity":         identity,
		"loadTestIdentity": "checkout-load",
		"status":           string(status),
		"targetUsers":      50,
		"durationSeconds":  600,
		"runSequence":      3,
		"toolType":         "Locust",
	}
}

func TestRunListWithoutALoadTestCoversTheWholeProject(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/runs"): map[string]any{
			"items":      []any{aRun("checkout-load-a1b", api.RunRunning)},
			"pagination": map[string]any{"totalItems": 1},
		},
	})

	out := expectSuccess(t, NewRunCmd(), "list")

	if got := stub.only().Path; got != "/runs" {
		t.Errorf("listing every run should hit /runs, got %s", got)
	}
	mustContain(t, "run list output", out.stdout, "checkout-load-a1b", "Running")
}

// --load-test-id switches to a different endpoint rather than adding a filter,
// so the flag has to change the route, not just the query string.
func TestRunListScopedToOneLoadTestUsesTheNestedRoute(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load/runs"): map[string]any{
			"items": []any{aRun("checkout-load-a1b", api.RunFinished)},
		},
	})

	expectSuccess(t, NewRunCmd(), "list", "--load-test-id", "checkout-load")

	if got := stub.only().Path; got != "/load-tests/checkout-load/runs" {
		t.Errorf("got %s, want the nested runs route", got)
	}
}

// Every list flag has to reach the query string. A flag that parses but is
// never read is the failure this catches, and it is invisible from the output.
func TestRunListSendsEveryFilterItAccepts(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/runs"): map[string]any{"items": []any{}},
	})

	expectSuccess(t, NewRunCmd(), "list",
		"--status", "Running",
		"--environment-id", "staging",
		"--page", "2",
		"--limit", "50",
		"--search", "checkout",
		"--sort-field", "startedAt",
		"--sort-ascending",
	)

	query := stub.only().Query
	for key, want := range map[string]string{
		"status":                "Running",
		"environmentIdentifier": "staging",
		"page":                  "2",
		"limit":                 "50",
		"search":                "checkout",
		"sortField":             "startedAt",
	} {
		if got := query.Get(key); got != want {
			t.Errorf("query %s = %q, want %q (full query: %v)", key, got, want, query)
		}
	}
	if query.Get("sortOrder") == "" && query.Get("sortAscending") == "" {
		t.Errorf("--sort-ascending never reached the query string: %v", query)
	}
}

// The scope is not a flag on these commands; it comes from the resolved global
// configuration and has to travel as query parameters on every request.
func TestEveryRequestCarriesTheResolvedScope(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/runs"): map[string]any{"items": []any{}},
	})

	expectSuccess(t, NewRunCmd(), "list")

	query := stub.only().Query
	for key, want := range map[string]string{
		"accountIdentifier":      "acct1",
		"organizationIdentifier": "default",
		"projectIdentifier":      "perf",
	} {
		if got := query.Get(key); got != want {
			t.Errorf("scope %s = %q, want %q", key, got, want)
		}
	}
}

// A misspelled status is caught before the request rather than sent on to be
// rejected, so the message can name the values that do work.
func TestRunListRejectsAStatusTheServiceDoesNotHave(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewRunCmd(), "list", "--status", "Runnning")

	mustContain(t, "status error", message, "Runnning", "Pending", "Running", "Finished")
}

func TestRunListAcceptsEveryStatusTheServiceHas(t *testing.T) {
	for _, status := range []api.RunStatus{
		api.RunPending, api.RunRunning, api.RunStopping,
		api.RunStopped, api.RunFinished, api.RunFailed,
	} {
		if err := validateRunStatus(string(status)); err != nil {
			t.Errorf("%s is a real status but was rejected: %v", status, err)
		}
	}
	if err := validateRunStatus(""); err != nil {
		t.Errorf("no --status should mean no filter, got %v", err)
	}
}

func TestRunGetPrintsTheRun(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/runs/checkout-load-a1b"): aRun("checkout-load-a1b", api.RunFinished),
	})

	out := expectSuccess(t, NewRunCmd(), "get", "checkout-load-a1b")

	mustContain(t, "run get output", out.stdout, "checkout-load-a1b", "Finished")
}

// GET /runs/{id} answers with the parent's unique id, which no other route
// accepts. The client trades it back for the identity by walking the listing,
// so what the user sees is something they can pass to another command.
func TestRunGetTranslatesTheParentUniqueIDBackToAnIdentity(t *testing.T) {
	const uniqueID = "11111111-2222-4333-8444-555555555555"

	run := aRun("checkout-load-a1b", api.RunRunning)
	run["loadTestIdentity"] = uniqueID

	serveLTM(t, map[call]any{
		GET("/runs/checkout-load-a1b"): run,
		GET("/load-tests"): map[string]any{
			"items": []any{map[string]any{
				"uniqueId": uniqueID,
				"identity": "checkout-load",
				"name":     "Checkout load",
			}},
		},
	})

	out := expectSuccess(t, NewRunCmd(), "get", "checkout-load-a1b")

	mustContain(t, "run get output", out.stdout, "checkout-load")
	mustNotContain(t, "run get output", out.stdout, uniqueID)
}

// A parent that cannot be found is left as the unique id rather than blanked,
// and reading the run still has to succeed: the translation is a courtesy.
func TestRunGetSurvivesAParentItCannotResolve(t *testing.T) {
	const uniqueID = "11111111-2222-4333-8444-555555555555"

	run := aRun("checkout-load-a1b", api.RunRunning)
	run["loadTestIdentity"] = uniqueID

	serveLTM(t, map[call]any{
		GET("/runs/checkout-load-a1b"): run,
		GET("/load-tests"):             map[string]any{"items": []any{}},
	})

	out := expectSuccess(t, NewRunCmd(), "get", "checkout-load-a1b")

	mustContain(t, "run get output", out.stdout, uniqueID)
}

func TestRunSummaryPrintsWhatTheServiceReports(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/runs/checkout-load-a1b/summary"): map[string]any{
			"totalRequests": 12000,
			"errorRate":     0.4,
			"endpoints": []any{
				map[string]any{"name": "GET /cart", "p95ResponseMs": 210},
			},
		},
	})

	out := expectSuccess(t, NewRunCmd(), "summary", "checkout-load-a1b")

	mustContain(t, "summary output", out.stdout, "totalRequests", "12000", "GET /cart")
}

// A summary is nested deeply enough that a table would flatten it away, so the
// table format has to fall through to YAML rather than print one wide row.
func TestRunSummaryFallsBackToYAMLInTableFormat(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/runs/checkout-load-a1b/summary"): map[string]any{"totalRequests": 12000},
	})
	globals().Format = formatTable

	out := expectSuccess(t, NewRunCmd(), "summary", "checkout-load-a1b")

	mustContain(t, "summary output", out.stdout, "totalRequests: 12000")
}

func TestRunGraphPassesTheTimeWindowThrough(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/runs/checkout-load-a1b/graph"): map[string]any{"series": []any{}},
	})

	expectSuccess(t, NewRunCmd(), "graph", "checkout-load-a1b",
		"--from", "2026-08-09T10:00:00Z", "--to", "2026-08-09T10:15:00Z")

	query := stub.only().Query
	if query.Get("from") != "2026-08-09T10:00:00Z" || query.Get("to") != "2026-08-09T10:15:00Z" {
		t.Errorf("the --from/--to window did not reach the query string: %v", query)
	}
}

// The view is a path segment rather than a query parameter, so getting it
// wrong is a 404 rather than a silently ignored filter.
func TestRunMetricsDefaultsToTheTimeseriesView(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/runs/checkout-load-a1b/metrics/timeseries"): map[string]any{"points": []any{}},
	})

	expectSuccess(t, NewRunCmd(), "metrics", "checkout-load-a1b")

	if got := stub.only().Path; got != "/runs/checkout-load-a1b/metrics/timeseries" {
		t.Errorf("got %s, want the timeseries default", got)
	}
}

func TestRunMetricsAcceptsEveryViewTheServiceOffers(t *testing.T) {
	for _, view := range api.MetricsViews {
		path := "/runs/checkout-load-a1b/metrics/" + string(view)
		stub := serveLTM(t, map[call]any{
			GET(path): map[string]any{"points": []any{}},
		})

		expectSuccess(t, NewRunCmd(), "metrics", "checkout-load-a1b", "--view", string(view))

		if got := stub.only().Path; got != path {
			t.Errorf("got %s, want %s", got, path)
		}
	}
}

func TestRunMetricsPassesTheTimeWindowThrough(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/runs/checkout-load-a1b/metrics/scatter"): map[string]any{"points": []any{}},
	})

	expectSuccess(t, NewRunCmd(), "metrics", "checkout-load-a1b",
		"--view", "scatter", "--from", "2026-08-09T10:00:00Z", "--to", "2026-08-09T10:15:00Z")

	query := stub.only().Query
	if query.Get("from") != "2026-08-09T10:00:00Z" || query.Get("to") != "2026-08-09T10:15:00Z" {
		t.Errorf("the --from/--to window did not reach the query string: %v", query)
	}
}

func TestRunMetricsRejectsAViewThatDoesNotExist(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewRunCmd(), "metrics", "checkout-load-a1b", "--view", "histogram")

	mustContain(t, "view error", message, "histogram", "timeseries", "scatter", "aggregate")
}

// Stopping is two calls: the endpoint only acknowledges the request, so the run
// has to be read back for the caller to be told anything useful.
func TestRunStopReadsTheRunBackAfterAcknowledgement(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		POST("/runs/checkout-load-a1b/stop"): map[string]any{"acknowledged": true},
		GET("/runs/checkout-load-a1b"):       aRun("checkout-load-a1b", api.RunStopping),
	})

	out := expectSuccess(t, NewRunCmd(), "stop", "checkout-load-a1b")

	if seen := summarise(stub.requests()); len(seen) != 2 {
		t.Fatalf("expected a stop then a read, got %v", seen)
	}
	mustContain(t, "stop output", out.stdout, "Stopping")
}

// Only two things can change on a run in flight. Asking for neither is a
// mistake worth catching before a request goes out.
func TestRunUpdateInsistsOnSomethingToChange(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewRunCmd(), "update", "checkout-load-a1b")

	mustContain(t, "update error", message, "--target-users", "--spawn-rate")
}

// An unset flag must not be sent. Sending zero would read as "drop to no users"
// rather than "leave this alone", which is why the request uses pointers.
func TestRunUpdateSendsOnlyTheFlagsThatWereTyped(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		POST("/runs/checkout-load-a1b/update"): map[string]any{"acknowledged": true},
		GET("/runs/checkout-load-a1b"):         aRun("checkout-load-a1b", api.RunRunning),
	})

	expectSuccess(t, NewRunCmd(), "update", "checkout-load-a1b", "--target-users", "800")

	var sent map[string]any
	stub.find(POST("/runs/checkout-load-a1b/update")).decode(t, &sent)

	if sent["targetUsers"] != float64(800) {
		t.Errorf("targetUsers = %v, want 800", sent["targetUsers"])
	}
	if _, present := sent["spawnRate"]; present {
		t.Errorf("an untyped --spawn-rate was still sent: %v", sent)
	}
}

func TestRunUpdateSendsTheSpawnRateOnItsOwn(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		POST("/runs/checkout-load-a1b/update"): map[string]any{"acknowledged": true},
		GET("/runs/checkout-load-a1b"):         aRun("checkout-load-a1b", api.RunRunning),
	})

	expectSuccess(t, NewRunCmd(), "update", "checkout-load-a1b", "--spawn-rate", "2.5")

	var sent map[string]any
	stub.find(POST("/runs/checkout-load-a1b/update")).decode(t, &sent)

	if sent["spawnRate"] != 2.5 {
		t.Errorf("spawnRate = %v, want 2.5", sent["spawnRate"])
	}
	if _, present := sent["targetUsers"]; present {
		t.Errorf("an untyped --target-users was still sent: %v", sent)
	}
}

// This endpoint wants the scope in the body as well as the query string, which
// is unlike every other route and easy to drop.
func TestRunUpdateRepeatsTheScopeInTheBody(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		POST("/runs/checkout-load-a1b/update"): map[string]any{"acknowledged": true},
		GET("/runs/checkout-load-a1b"):         aRun("checkout-load-a1b", api.RunRunning),
	})

	expectSuccess(t, NewRunCmd(), "update", "checkout-load-a1b", "--target-users", "12")

	var sent api.UpdateRunRequest
	stub.find(POST("/runs/checkout-load-a1b/update")).decode(t, &sent)

	if sent.AccountIdentifier != "acct1" || sent.OrganizationIdentifier != "default" || sent.ProjectIdentifier != "perf" {
		t.Errorf("the scope is missing from the body: %+v", sent)
	}
	if sent.Identity != "checkout-load-a1b" {
		t.Errorf("identity = %q, want the run identity", sent.Identity)
	}
}

func TestRunRerunStartsAFreshRunOfTheSameLoadTest(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/runs/checkout-load-a1b"):         aRun("checkout-load-a1b", api.RunFinished),
		POST("/load-tests/checkout-load/runs"): aRun("checkout-load-v3u", api.RunPending),
	})

	out := expectSuccess(t, NewRunCmd(), "rerun", "checkout-load-a1b")

	var sent api.CreateRunRequest
	stub.find(POST("/load-tests/checkout-load/runs")).decode(t, &sent)

	if sent.Name != "Rerun of checkout-load-a1b" {
		t.Errorf("name = %q, want the generated rerun name", sent.Name)
	}
	mustContain(t, "rerun notice", out.stderr, "checkout-load-v3u", "checkout-load")
}

func TestRunRerunPrefersTheNameTheUserGave(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/runs/checkout-load-a1b"):         aRun("checkout-load-a1b", api.RunFinished),
		POST("/load-tests/checkout-load/runs"): aRun("checkout-load-v3u", api.RunPending),
	})

	expectSuccess(t, NewRunCmd(), "rerun", "checkout-load-a1b", "--run-name", "Nightly repeat")

	var sent api.CreateRunRequest
	stub.find(POST("/load-tests/checkout-load/runs")).decode(t, &sent)

	if sent.Name != "Nightly repeat" {
		t.Errorf("name = %q, want the name that was passed", sent.Name)
	}
}

// A parent still in unique-id form means nothing in scope owns it. Posting to
// that route would 404, so the message has to say what actually went wrong.
func TestRunRerunExplainsAParentThatIsNotInScope(t *testing.T) {
	run := aRun("checkout-load-a1b", api.RunFinished)
	run["loadTestIdentity"] = "11111111-2222-4333-8444-555555555555"

	serveLTM(t, map[call]any{
		GET("/runs/checkout-load-a1b"): run,
		GET("/load-tests"):             map[string]any{"items": []any{}},
	})

	message := expectFailure(t, NewRunCmd(), "rerun", "checkout-load-a1b")

	mustContain(t, "rerun error", message, "checkout-load-a1b", "deleted")
	// The scope is the likeliest explanation, so it has to be named.
	mustContain(t, "rerun error", message, "perf")
}

func TestRunRerunRefusesARunWithNoRecordedParent(t *testing.T) {
	run := aRun("checkout-load-a1b", api.RunFinished)
	run["loadTestIdentity"] = ""

	serveLTM(t, map[call]any{
		GET("/runs/checkout-load-a1b"): run,
	})

	message := expectFailure(t, NewRunCmd(), "rerun", "checkout-load-a1b")

	mustContain(t, "rerun error", message, "does not record", "cannot be rerun")
}

// --watch hands off to the same polling loop the watch command uses, so a
// rerun that is followed must not also print the started-run record.
func TestRunRerunFollowsTheNewRunWhenAskedTo(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/runs/checkout-load-a1b"):         aRun("checkout-load-a1b", api.RunFinished),
		POST("/load-tests/checkout-load/runs"): aRun("checkout-load-v3u", api.RunPending),
		GET("/runs/checkout-load-v3u"):         aRun("checkout-load-v3u", api.RunFinished),
	})

	out := expectSuccess(t, NewRunCmd(), "rerun", "checkout-load-a1b", "--watch", "--interval", "10ms")

	mustContain(t, "watch progress", out.stderr, "Finished")
	mustNotContain(t, "watch progress", out.stderr, "Started run")
}

// A run that ends Failed has to exit non-zero, or a pipeline gating on the load
// test would pass while the test was failing.
func TestRunWatchExitsNonZeroOnAFailedRun(t *testing.T) {
	failed := aRun("checkout-load-a1b", api.RunFailed)
	failed["errorMessage"] = "injector could not reach the target"

	serveLTM(t, map[call]any{
		GET("/runs/checkout-load-a1b"): failed,
	})

	message := expectFailure(t, NewRunCmd(), "watch", "checkout-load-a1b", "--interval", "10ms")

	mustContain(t, "failure message", message, "checkout-load-a1b", "injector could not reach the target")
}

// A failure the service does not explain still has to fail, just without
// inventing a reason for it.
func TestRunWatchStillFailsWhenTheServiceGivesNoReason(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/runs/checkout-load-a1b"): aRun("checkout-load-a1b", api.RunFailed),
	})

	message := expectFailure(t, NewRunCmd(), "watch", "checkout-load-a1b", "--interval", "10ms")

	if !strings.Contains(message, "failed") {
		t.Errorf("got %q, want it to report the run failed", message)
	}
}

// A stopped run is a deliberate act, not a failure, so it exits zero.
func TestRunWatchTreatsAStoppedRunAsASuccess(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/runs/checkout-load-a1b"): aRun("checkout-load-a1b", api.RunStopped),
	})

	expectSuccess(t, NewRunCmd(), "watch", "checkout-load-a1b", "--interval", "10ms")
}

// An error from the service reaches the user rather than being swallowed into
// an empty listing.
func TestRunGetReportsWhatTheServiceSaidWentWrong(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/runs/nope"): http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"message":"run not found"}`, http.StatusNotFound)
		}),
	})

	message := expectFailure(t, NewRunCmd(), "get", "nope")

	mustContain(t, "not-found error", message, "run not found")
}

func TestIsUUIDSeparatesAUniqueIDFromAnIdentity(t *testing.T) {
	if !isUUID("11111111-2222-4333-8444-555555555555") {
		t.Error("a canonical 8-4-4-4-12 value should read as a unique id")
	}
	// A run identity is a slug with a short suffix and shares the hyphens, so
	// the check has to be tighter than "contains a dash".
	for _, identity := range []string{"checkout-load-a1b", "checkout-load", "", "1111-2222"} {
		if isUUID(identity) {
			t.Errorf("%q is an identity, not a unique id", identity)
		}
	}
}
