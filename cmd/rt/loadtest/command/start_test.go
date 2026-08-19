package command

import (
	"net/http"
	"testing"

	"github.com/harness/harness-cli/cmd/rt/loadtest/api"
)

// anInput is a "<+input>" leaf as the variables endpoint reports one. The path
// is absolute and names the tool, which is what start has to reconstruct from
// whatever abbreviation the user typed.
func anInput(path string, value any) map[string]any {
	return map[string]any{
		"name":  path,
		"path":  path,
		"value": value,
		"type":  "Integer",
	}
}

// started drives "start" against a stub answering the run creation, and returns
// the body it sent.
func started(t *testing.T, routes map[call]any, args ...string) api.CreateRunRequest {
	t.Helper()

	if routes == nil {
		routes = map[call]any{}
	}
	routes[POST("/load-tests/checkout-load/runs")] = aRun("checkout-load-a1b", api.RunRunning)

	stub := serveLTM(t, routes)
	expectSuccess(t, NewStartCmd(), append([]string{"checkout-load"}, args...)...)

	var body api.CreateRunRequest
	stub.find(POST("/load-tests/checkout-load/runs")).decode(t, &body)
	return body
}

// Nothing was overridden, so the run carries the definition as it stands. In
// particular it must not ask for the variables it has no use for.
func TestStartSendsARunAndReturnsWithoutWatching(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		POST("/load-tests/checkout-load/runs"): aRun("checkout-load-a1b", api.RunRunning),
	})

	out := expectSuccess(t, NewStartCmd(), "checkout-load")

	if got := stub.only().Path; got != "/load-tests/checkout-load/runs" {
		t.Errorf("called %s, want the run route of the named test", got)
	}
	mustContain(t, "start confirmation", out.stderr, "Started run", "checkout-load-a1b", "run watch")
	mustContain(t, "start output", out.stdout, "checkout-load-a1b")
}

// A run needs an identity unique in scope and the user has no reason to invent
// one, so the client generates it rather than leaving the field empty.
func TestStartNamesTheRunItself(t *testing.T) {
	body := started(t, nil)

	if body.Identity == "" {
		t.Error("the run was sent with no identity, which the service rejects")
	}
	if body.Identity == "checkout-load" {
		t.Error("the run took the load test's identity, which is not unique per run")
	}
}

func TestStartCarriesARunName(t *testing.T) {
	body := started(t, nil, "--run-name", "release-2.4 baseline")

	if body.Name != "release-2.4 baseline" {
		t.Errorf("name = %q, want the one passed", body.Name)
	}
}

// A variable override is by name and needs no lookup, so it goes straight
// through with the value as written.
func TestStartOverridesVariablesByName(t *testing.T) {
	body := started(t, nil, "--value", "tier=premium", "--value", "REGION=us-east-1")

	byName := map[string]any{}
	for _, value := range body.Values {
		byName[value.Name] = value.Value
	}
	if byName["tier"] != "premium" || byName["REGION"] != "us-east-1" {
		t.Errorf("values = %v, want both overrides", body.Values)
	}
}

func TestStartRejectsAValueThatIsNotAPair(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewStartCmd(), "checkout-load", "--value", "tier")

	mustContain(t, "value error", message, "tier", "NAME=VALUE")
}

func TestStartRejectsARuntimeValueThatIsNotAPair(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewStartCmd(), "checkout-load", "--runtime-value", "targetUsers")

	mustContain(t, "runtime value error", message, "targetUsers", "PATH=VALUE")
}

// The service keys inputs by their full path. Making the user type it in full
// every time would be a transcription exercise, so a unique tail is expanded.
func TestStartExpandsAnAbbreviatedRuntimePath(t *testing.T) {
	for _, abbreviation := range []string{
		".toolConfig.k6.tunables.targetUsers",
		"toolConfig.k6.tunables.targetUsers",
		"tunables.targetUsers",
		"targetUsers",
	} {
		t.Run(abbreviation, func(t *testing.T) {
			body := started(t, map[call]any{
				GET("/load-tests/checkout-load/variables"): map[string]any{
					"loadTestIdentity": "checkout-load",
					"inputs": []any{
						anInput(".toolConfig.k6.tunables.targetUsers", "<+input>"),
						anInput(".toolConfig.k6.tunables.rampUpTimeSec", "<+input>"),
					},
				},
			}, "--runtime-value", abbreviation+"=500")

			value, found := body.RuntimeValues[".toolConfig.k6.tunables.targetUsers"]
			if !found {
				t.Fatalf("runtimeValues = %v, want the path expanded in full", body.RuntimeValues)
			}
			if jsonNumber(value) != 500 {
				t.Errorf("targetUsers = %v, want the number 500 rather than text", value)
			}
		})
	}
}

// An input bound to an integer field rejects a quoted number, and a flag cannot
// state a type, so it is read from the value.
func TestStartTypesRuntimeValuesByWhatTheyLookLike(t *testing.T) {
	body := started(t, map[call]any{
		GET("/load-tests/checkout-load/variables"): map[string]any{
			"inputs": []any{
				anInput(".toolConfig.k6.tunables.targetUsers", "<+input>"),
				anInput(".toolConfig.k6.tunables.rpsLimit", "<+input>"),
				anInput(".toolConfig.k6.options.noConnectionReuse", "<+input>"),
				anInput(".toolConfig.k6.tunables.targetUrl", "<+input>"),
			},
		},
	},
		"--runtime-value", "targetUsers=500",
		"--runtime-value", "rpsLimit=12.5",
		"--runtime-value", "noConnectionReuse=true",
		"--runtime-value", "targetUrl=https://eu.example.com")

	values := body.RuntimeValues
	if got := values[".toolConfig.k6.tunables.targetUsers"]; jsonNumber(got) != 500 {
		t.Errorf("targetUsers = %#v, want an integer", got)
	}
	if got := values[".toolConfig.k6.tunables.rpsLimit"]; jsonNumber(got) != 12.5 {
		t.Errorf("rpsLimit = %#v, want a float", got)
	}
	if got := values[".toolConfig.k6.options.noConnectionReuse"]; got != true {
		t.Errorf("noConnectionReuse = %#v, want a bool", got)
	}
	if got := values[".toolConfig.k6.tunables.targetUrl"]; got != "https://eu.example.com" {
		t.Errorf("targetUrl = %#v, want the text unchanged", got)
	}
}

// The path is checked before the run is sent, so a typo costs a listing rather
// than a run that starts and then ignores the value.
func TestStartNamesTheInputsItAcceptsWhenThePathIsWrong(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load/variables"): map[string]any{
			"inputs": []any{
				anInput(".toolConfig.k6.tunables.targetUsers", "<+input>"),
				anInput(".toolConfig.k6.tunables.rampUpTimeSec", "<+input>"),
			},
		},
	})

	message := expectFailure(t, NewStartCmd(), "checkout-load",
		"--runtime-value", "targetUser=500")

	mustContain(t, "unknown input error", message, "targetUser",
		".toolConfig.k6.tunables.targetUsers", ".toolConfig.k6.tunables.rampUpTimeSec")
	if len(stub.requests()) != 1 {
		t.Errorf("made %v, want the run abandoned before it was sent", summarise(stub.requests()))
	}
}

// Guessing between two matches would silently set the wrong one, so the tail is
// rejected and both candidates are named.
func TestStartRefusesAnAmbiguousRuntimePath(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load/variables"): map[string]any{
			"inputs": []any{
				anInput(".toolConfig.k6.tunables.targetUsers", "<+input>"),
				anInput(".toolConfig.jmeter.tunables.targetUsers", "<+input>"),
			},
		},
	})

	message := expectFailure(t, NewStartCmd(), "checkout-load",
		"--runtime-value", "tunables.targetUsers=500")

	mustContain(t, "ambiguous input error", message,
		".toolConfig.k6.tunables.targetUsers", ".toolConfig.jmeter.tunables.targetUsers", "in full")
}

// A test with no inputs at all is a different mistake from a mistyped path: the
// definition is missing a sentinel, so the message says how to add one.
func TestStartExplainsATestWithNoRuntimeInputsAtAll(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load/variables"): map[string]any{
			"loadTestIdentity": "checkout-load",
			"inputs":           []any{},
		},
	})

	message := expectFailure(t, NewStartCmd(), "checkout-load",
		"--runtime-value", "targetUsers=500")

	mustContain(t, "no inputs error", message, "no runtime inputs", "<+input>", "--value")
}

// Without an override there is nothing to resolve, so the extra round trip is
// not made at all.
func TestStartSkipsTheVariablesLookupWhenNothingNeedsResolving(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		POST("/load-tests/checkout-load/runs"): aRun("checkout-load-a1b", api.RunRunning),
	})

	expectSuccess(t, NewStartCmd(), "checkout-load", "--value", "tier=premium")

	stub.only() // fails if the variables endpoint was called as well
}

// --watch polls until the run reaches a terminal state, and a failed run has to
// make the command fail or a pipeline gate on it would pass.
func TestStartWatchesToTheEndAndFailsOnAFailedRun(t *testing.T) {
	polls := 0
	serveLTM(t, map[call]any{
		POST("/load-tests/checkout-load/runs"): aRun("checkout-load-a1b", api.RunRunning),
		GET("/runs/checkout-load-a1b"): http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			polls++
			status := api.RunRunning
			if polls > 1 {
				status = api.RunFailed
			}
			writeJSON(w, aRun("checkout-load-a1b", status))
		}),
	})

	message := expectFailure(t, NewStartCmd(), "checkout-load",
		"--watch", "--interval", "10ms")

	mustContain(t, "watch failure", message, "checkout-load-a1b")
	if polls < 2 {
		t.Errorf("polled %d times, want it to keep polling while the run was running", polls)
	}
}

func TestStartSurfacesARejectedRun(t *testing.T) {
	serveLTM(t, map[call]any{
		POST("/load-tests/checkout-load/runs"): http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"message":"infrastructure perf-cluster is not connected"}`, http.StatusBadRequest)
		}),
	})

	message := expectFailure(t, NewStartCmd(), "checkout-load")

	mustContain(t, "rejected run", message, "not connected")
}
