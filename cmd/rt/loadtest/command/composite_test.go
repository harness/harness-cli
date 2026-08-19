package command

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/harness/harness-cli/cmd/rt/loadtest/api"
)

// aComposite is the pipeline a create comes back as.
func aComposite(identifier string) map[string]any {
	return map[string]any{
		"pipelineIdentifier": identifier,
		"stageIdentifier":    identifier + "_stage",
		"name":               "Checkout under fault",
		"identifier":         identifier,
	}
}

// composed drives "composite create" against a stub and returns the body sent.
func composed(t *testing.T, args ...string) api.CreateCompositeRequest {
	t.Helper()

	stub := serveLTM(t, map[call]any{
		POST("/composite-load-tests"): aComposite("checkout_resilience"),
	})

	expectSuccess(t, NewCompositeCmd(), append([]string{"create"}, args...)...)

	var body api.CreateCompositeRequest
	stub.only().decode(t, &body)
	return body
}

// compositeFlags is the minimum a create needs, so a test states only what it
// is varying.
func compositeFlags(extra ...string) []string {
	return append([]string{
		"--identifier", "checkout_resilience",
		"--load-test-ref", "checkout-load",
		"--probe-id", "pod-delete-probe",
	}, extra...)
}

func TestCompositeCreatePairsALoadTestWithAProbe(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		POST("/composite-load-tests"): aComposite("checkout_resilience"),
	})

	out := expectSuccess(t, NewCompositeCmd(), append([]string{"create"}, compositeFlags()...)...)

	var body api.CreateCompositeRequest
	stub.only().decode(t, &body)

	if body.LoadTest.LoadTestRef != "checkout-load" {
		t.Errorf("loadTestRef = %q, want the test named", body.LoadTest.LoadTestRef)
	}
	if body.Probe.Identity != "pod-delete-probe" {
		t.Errorf("probe identity = %q, want the probe named", body.Probe.Identity)
	}
	// A composite is not run here, so saying where to run it is the whole
	// difference between a useful result and a bare identifier.
	mustContain(t, "create confirmation", out.stderr,
		"checkout_resilience", "checkout_resilience_stage", "Harness UI")
	mustContain(t, "create output", out.stdout, "checkout_resilience")
}

func TestCompositeCreateFallsBackToTheIdentifierForTheName(t *testing.T) {
	body := composed(t, compositeFlags()...)

	if body.Name != "checkout_resilience" {
		t.Errorf("name = %q, want it to fall back to the identifier", body.Name)
	}
}

func TestCompositeCreateCarriesTheOptionalDetail(t *testing.T) {
	body := composed(t, compositeFlags(
		"--name", "Checkout under fault",
		"--description", "Peak load while a pod is deleted",
		"--objective", "Hold p95 under 500ms",
		"--tag", "team=perf",
		"--probe-infra-id", "perf-cluster",
		"--probe-duration", "10m")...)

	if body.Name != "Checkout under fault" || body.Description != "Peak load while a pod is deleted" {
		t.Errorf("name/description = %q/%q", body.Name, body.Description)
	}
	if body.Objective != "Hold p95 under 500ms" {
		t.Errorf("objective = %q, want the one passed", body.Objective)
	}
	if body.Tags["team"] != "perf" {
		t.Errorf("tags = %v, want the pair passed", body.Tags)
	}
	if body.Probe.InfraReference != "perf-cluster" || body.Probe.Duration != "10m" {
		t.Errorf("probe = %+v, want the infrastructure and duration pinned", body.Probe)
	}
}

// Leaving the infrastructure out is deliberate: the generated step asks for it
// at run time, which is what a pipeline shared across environments needs. So it
// must be absent from the body rather than sent as empty.
func TestCompositeCreateLeavesTheProbeInfraOutWhenNotPinned(t *testing.T) {
	body := composed(t, compositeFlags()...)

	if body.Probe.InfraReference != "" {
		t.Errorf("infraReference = %q, want it left for a runtime input", body.Probe.InfraReference)
	}
}

func TestCompositeCreateInsistsOnBothHalves(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		wants []string
	}{
		{
			name:  "no identifier",
			args:  []string{"--load-test-ref", "checkout-load", "--probe-id", "pod-delete-probe"},
			wants: []string{"--identifier is required"},
		},
		{
			name:  "no load test",
			args:  []string{"--identifier", "checkout_resilience", "--probe-id", "pod-delete-probe"},
			wants: []string{"--load-test-ref is required", "loadtest list"},
		},
		{
			name:  "no probe",
			args:  []string{"--identifier", "checkout_resilience", "--load-test-ref", "checkout-load"},
			wants: []string{"--probe-id is required", "chaos probe"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			serveLTM(t, map[call]any{})
			message := expectFailure(t, NewCompositeCmd(), append([]string{"create"}, tc.args...)...)
			mustContain(t, "missing flag error", message, tc.wants...)
		})
	}
}

// The identifier is checked before anything is sent, so a hyphenated name costs
// a message naming the flag rather than a pipeline-service rejection.
func TestCompositeCreateValidatesTheIdentifierBeforeSending(t *testing.T) {
	stub := serveLTM(t, map[call]any{})

	message := expectFailure(t, NewCompositeCmd(), "create",
		"--identifier", "checkout-resilience",
		"--load-test-ref", "checkout-load", "--probe-id", "pod-delete-probe")

	mustContain(t, "identifier error", message, "--identifier", `"checkout_resilience"`)
	if len(stub.requests()) != 0 {
		t.Errorf("made %v, want nothing sent for an identifier that cannot work", summarise(stub.requests()))
	}
}

func TestCompositeCreateLetsFlagsOverrideTheConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "composite.json")
	contents, err := json.Marshal(map[string]any{
		"identifier":  "checkout_resilience",
		"name":        "From the file",
		"description": "Also from the file",
		"loadTest":    map[string]any{"loadTestRef": "checkout-load"},
		"probe":       map[string]any{"identity": "pod-delete-probe"},
	})
	if err != nil {
		t.Fatalf("building the config file: %v", err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("writing the config file: %v", err)
	}

	body := composed(t, "--config", path, "--name", "From the flag")

	if body.Name != "From the flag" {
		t.Errorf("name = %q, want the flag to win", body.Name)
	}
	if body.Description != "Also from the file" {
		t.Errorf("description = %q, want the file value where no flag was passed", body.Description)
	}
	if body.LoadTest.LoadTestRef != "checkout-load" {
		t.Errorf("loadTestRef = %q, want the file value", body.LoadTest.LoadTestRef)
	}
}

func TestCompositeCreateSurfacesARejectionFromTheService(t *testing.T) {
	serveLTM(t, map[call]any{
		POST("/composite-load-tests"): http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"message":"chaos probe pod-delete-probe does not exist"}`, http.StatusBadRequest)
		}),
	})

	message := expectFailure(t, NewCompositeCmd(), append([]string{"create"}, compositeFlags()...)...)

	mustContain(t, "probe rejection", message, "does not exist")
}

func TestCompositeListRendersThePipelinesAndTheirLastExecution(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/composite-load-tests"): map[string]any{
			"items": []any{
				map[string]any{
					"name":               "Checkout under fault",
					"pipelineIdentifier": "checkout_resilience",
					"loadTestCount":      1,
					"probeCount":         2,
					"lastUpdatedAt":      1754038800000,
					"recentExecutions": []any{
						map[string]any{"executionId": "e1", "status": "Failed", "startTs": 1753952400000},
						map[string]any{"executionId": "e2", "status": "Success", "startTs": 1754038800000},
					},
				},
			},
		},
	})

	globals().Format = formatTable
	out := expectSuccess(t, NewCompositeCmd(), "list")

	if got := stub.only().Path; got != "/composite-load-tests" {
		t.Errorf("called %s, want the composite route", got)
	}
	mustContain(t, "composite table", out.combined(),
		"PIPELINE", "checkout_resilience", "Checkout under fault", "LAST EXECUTION")
	// The most recent execution is the one worth showing, and the service does
	// not promise an order.
	mustContain(t, "last execution", out.combined(), "Success")
	mustNotContain(t, "last execution", out.combined(), "Failed")
}

// A scope with no composites is a normal state, not a failure, so it says so
// rather than printing an empty table or nothing at all.
func TestCompositeListRendersAProjectWithNoComposites(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/composite-load-tests"): map[string]any{"items": []any{}},
	})

	globals().Format = formatTable
	out := expectSuccess(t, NewCompositeCmd(), "list")

	mustContain(t, "empty composite table", out.combined(), "No results")
}

// A pipeline with no execution yet renders as a dash rather than as an empty
// cell, so the column reads as "nothing" rather than as missing data.
func TestCompositeListMarksAPipelineThatHasNeverRun(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/composite-load-tests"): map[string]any{
			"items": []any{
				map[string]any{
					"name":               "Checkout under fault",
					"pipelineIdentifier": "checkout_resilience",
					"recentExecutions":   []any{},
				},
			},
		},
	})

	globals().Format = formatTable
	out := expectSuccess(t, NewCompositeCmd(), "list")

	mustContain(t, "never-run pipeline", out.combined(), "-")
}

// A queued execution has a status but no start time yet. Formatting a zero
// timestamp would print the epoch, which reads as a run from 1970.
func TestALastExecutionThatHasNotStartedIsShownWithoutATime(t *testing.T) {
	got := lastExecution([]api.CompositeExecution{{Status: "Queued"}})

	if got != "Queued" {
		t.Errorf("lastExecution = %q, want the bare status with no time", got)
	}
}

// The listing does not order the executions, so the newest is found rather
// than assumed to be first.
func TestTheNewestExecutionIsTheOneSummarised(t *testing.T) {
	got := lastExecution([]api.CompositeExecution{
		{Status: "Failed", StartTs: 1_600_000_000_000},
		{Status: "Success", StartTs: 1_700_000_000_000},
		{Status: "Aborted", StartTs: 1_650_000_000_000},
	})

	if !strings.HasPrefix(got, "Success") {
		t.Errorf("lastExecution = %q, want the newest execution", got)
	}
}

// This listing comes from pipeline service, whose field names differ. An
// unsupported value would come back as an opaque 400, so it is caught here and
// the supported ones are named.
func TestCompositeListRejectsASortFieldFromTheOtherListing(t *testing.T) {
	stub := serveLTM(t, map[call]any{})

	message := expectFailure(t, NewCompositeCmd(), "list", "--sort-field", "createdAt")

	mustContain(t, "sort field error", message,
		"createdAt", "name", "lastUpdatedAt", "executionSummaryInfo.lastExecutionTs")
	if len(stub.requests()) != 0 {
		t.Errorf("made %v, want nothing sent for a field the service cannot sort on", summarise(stub.requests()))
	}
}

func TestCompositeListAcceptsTheSortFieldsPipelineServiceKnows(t *testing.T) {
	for _, field := range []string{"name", "lastUpdatedAt", "executionSummaryInfo.lastExecutionTs"} {
		t.Run(field, func(t *testing.T) {
			stub := serveLTM(t, map[call]any{
				GET("/composite-load-tests"): map[string]any{"items": []any{}},
			})

			expectSuccess(t, NewCompositeCmd(), "list",
				"--sort-field", field, "--sort-ascending", "--page", "2", "--limit", "50", "--search", "checkout")

			query := stub.only().Query
			if query.Get("sortField") != field {
				t.Errorf("sortField = %q, want %q", query.Get("sortField"), field)
			}
			if query.Get("search") != "checkout" || query.Get("limit") != "50" || query.Get("page") != "2" {
				t.Errorf("paging and search did not reach the request: %v", query)
			}
		})
	}
}

// The hyphens that every load test identity uses are exactly what a pipeline
// identifier refuses, so the natural name to reach for is the one that fails.
// Unchecked it comes back as an HTTP 500 quoting a pipeline rule, with nothing
// tying it to the flag.
func TestCompositeIdentifierRejectsHyphensWithARewrite(t *testing.T) {
	err := validateCompositeIdentifier("hc-qa-composite-1")
	if err == nil {
		t.Fatal("expected a hyphenated identifier to be refused")
	}
	if !strings.Contains(err.Error(), "--identifier") {
		t.Errorf("error does not name the flag: %v", err)
	}
	// Naming the fix matters more than naming the rule: the caller wants to
	// retry, not to learn pipeline-service grammar.
	if !strings.Contains(err.Error(), `"hc_qa_composite_1"`) {
		t.Errorf("got %q, want the corrected identifier offered", err)
	}
}

// A leading digit cannot be fixed by swapping separators, so offering a
// rewrite that fails the same way would send the caller round the loop twice.
func TestCompositeIdentifierOffersNoRewriteItCannotStandBehind(t *testing.T) {
	err := validateCompositeIdentifier("1-checkout")
	if err == nil {
		t.Fatal("expected an identifier starting with a digit to be refused")
	}
	if strings.Contains(err.Error(), "try ") {
		t.Errorf("suggested a rewrite that is still invalid: %v", err)
	}
}

func TestCompositeIdentifierAcceptsPipelineNaming(t *testing.T) {
	for _, identifier := range []string{"checkout_resilience", "Checkout1", "a", "a$b_C9"} {
		if err := validateCompositeIdentifier(identifier); err != nil {
			t.Errorf("%q is valid pipeline naming, got %v", identifier, err)
		}
	}
}

// The cap is reported as a length rather than as the character rule, since a
// name that is merely too long is otherwise well formed.
func TestCompositeIdentifierReportsTheLengthCapAsItself(t *testing.T) {
	err := validateCompositeIdentifier("a" + strings.Repeat("b", compositeIdentifierMaxLen))
	if err == nil {
		t.Fatal("expected an over-long identifier to be refused")
	}
	if !strings.Contains(err.Error(), "128") {
		t.Errorf("got %q, want the cap named", err)
	}
}
