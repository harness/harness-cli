package command

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/harness/harness-cli/cmd/rt/loadtest/api"
)

// updated drives "update" against a stub that answers the read and the write,
// and returns the body it sent. Every update is a read-then-merge, so the read
// has to be stubbed even when the test only cares about what was written.
func updated(t *testing.T, args ...string) api.UpdateRequest {
	t.Helper()

	stub := serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load"): aLoadTest("checkout-load"),
		PUT("/load-tests/checkout-load"): aLoadTest("checkout-load"),
	})

	expectSuccess(t, NewUpdateCmd(), append([]string{"checkout-load"}, args...)...)

	var body api.UpdateRequest
	stub.find(PUT("/load-tests/checkout-load")).decode(t, &body)
	return body
}

// Only what was passed changes. A rename that also blanked the description
// would be a silent data loss, so the fields left alone are absent from the
// body rather than sent as empty.
func TestUpdateSendsOnlyTheFieldsThatWerePassed(t *testing.T) {
	body := updated(t, "--name", "Checkout load (EU)")

	if body.Name != "Checkout load (EU)" {
		t.Errorf("name = %q, want the new one", body.Name)
	}
	for what, got := range map[string]string{
		"description":    body.Description,
		"infra-id":       body.InfraIdentifier,
		"environment-id": body.EnvironmentIdentifier,
		"target-type":    body.TargetType,
	} {
		if got != "" {
			t.Errorf("%s = %q, want it left alone when the flag was not passed", what, got)
		}
	}
}

func TestUpdateCarriesEveryScalarFlagItOffers(t *testing.T) {
	body := updated(t,
		"--name", "Checkout load (EU)",
		"--description", "Peak season profile",
		"--infra-id", "prod-cluster",
		"--environment-id", "production",
		"--target-type", "kubernetes",
		"--cleanup-policy", "retain")

	for what, got := range map[string]struct{ have, want string }{
		"name":          {body.Name, "Checkout load (EU)"},
		"description":   {body.Description, "Peak season profile"},
		"infraId":       {body.InfraIdentifier, "prod-cluster"},
		"environmentId": {body.EnvironmentIdentifier, "production"},
		"targetType":    {body.TargetType, "kubernetes"},
		"cleanupPolicy": {string(body.CleanupPolicy), "retain"},
	} {
		if got.have != got.want {
			t.Errorf("%s = %q, want %q", what, got.have, got.want)
		}
	}
}

func TestUpdateReplacesTagsAndServiceReferencesWholesale(t *testing.T) {
	body := updated(t, "--tag", "nightly,eu", "--service-ref", "checkout-svc,cart-svc")

	if len(body.Tags) != 2 || body.Tags[0] != "nightly" || body.Tags[1] != "eu" {
		t.Errorf("tags = %v, want the set replaced with both", body.Tags)
	}
	if len(body.ServiceReferences) != 2 {
		t.Errorf("serviceReferences = %v, want both", body.ServiceReferences)
	}
}

func TestUpdateRejectsACleanupPolicyThatIsNotOne(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load"): aLoadTest("checkout-load"),
	})

	message := expectFailure(t, NewUpdateCmd(), "checkout-load", "--cleanup-policy", "keep")

	mustContain(t, "cleanup policy error", message, "keep", "delete", "retain")
}

// A tunable patches one leaf of a nested block. Sending only that leaf would
// drop everything else in the block, including the script the test runs.
func TestUpdateMergesATunableIntoTheStoredToolConfig(t *testing.T) {
	body := updated(t, "--target-users", "500")

	k6, ok := body.ToolConfig["k6"].(map[string]any)
	if !ok {
		t.Fatalf("toolConfig has no k6 block: %v", body.ToolConfig)
	}
	tunables, ok := k6["tunables"].(map[string]any)
	if !ok {
		t.Fatalf("the k6 block has no tunables: %v", k6)
	}
	if got := tunables["targetUsers"]; jsonNumber(got) != 500 {
		t.Errorf("targetUsers = %v, want the flag value", got)
	}
	if got := tunables["durationSeconds"]; jsonNumber(got) != 600 {
		t.Errorf("durationSeconds = %v, want the stored value kept", got)
	}
	if _, found := k6["script"]; !found {
		t.Error("the stored script was dropped by a tunable update")
	}
}

// Moving an existing test from a script to a container is a mode change on the
// stored block, so everything else in that block has to survive it.
func TestUpdateCanSwitchTheModeOfTheStoredConfig(t *testing.T) {
	body := updated(t, "--mode", "image", "--image", "ghcr.io/acme/k6-suite:1.4.0")

	if got := digInto(t, body.ToolConfig, "k6", "mode"); got != "image" {
		t.Errorf("mode = %q, want image", got)
	}
	if got := digNumber(t, body.ToolConfig, "k6", "tunables", "targetUsers"); got != "100" {
		t.Errorf("the stored tunables were dropped by a mode change: %v", body.ToolConfig)
	}
}

func TestUpdateRejectsAModeThatIsNotOne(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load"): aLoadTest("checkout-load"),
	})

	message := expectFailure(t, NewUpdateCmd(), "checkout-load", "--mode", "container")

	mustContain(t, "mode error", message, "unsupported --mode", "script, image")
}

// Renaming a test has nothing to do with its configuration, so the block is
// omitted entirely rather than echoed back.
func TestUpdateLeavesToolConfigOutWhenNoTunableChanged(t *testing.T) {
	body := updated(t, "--name", "Checkout load (EU)")

	if body.ToolConfig != nil {
		t.Errorf("toolConfig = %v, want it omitted when nothing in it changed", body.ToolConfig)
	}
}

// Resources are a block too. Raising one limit must not clear the other three.
func TestUpdatePatchesResourcesRatherThanReplacingThem(t *testing.T) {
	body := updated(t, "--memory-limit", "4Gi")

	if body.Resources == nil {
		t.Fatal("resources were dropped")
	}
	if got := body.Resources.Limits["memory"]; got != "4Gi" {
		t.Errorf("limits.memory = %q, want the new value", got)
	}
	if got := body.Resources.Limits["cpu"]; got != "2" {
		t.Errorf("limits.cpu = %q, want the stored value kept", got)
	}
	if got := body.Resources.Requests["memory"]; got != "512Mi" {
		t.Errorf("requests.memory = %q, want the whole requests list kept", got)
	}
}

// Adding a variable must not drop the ones already stored, and a flag cannot
// state a type, so a numeric-looking value is inferred rather than sent as text.
func TestUpdateMergesVariablesIntoTheStoredSet(t *testing.T) {
	body := updated(t, "--var", "TIMEOUT=30")

	byName := map[string]api.Variable{}
	for _, variable := range body.Variables {
		byName[variable.Name] = variable
	}

	if got := jsonNumber(byName["TIMEOUT"].Value); got != 30 {
		t.Errorf("TIMEOUT = %v, want the new variable as the number 30", byName["TIMEOUT"].Value)
	}
	if got := byName["TIMEOUT"].Type; got != "Integer" {
		t.Errorf("TIMEOUT type = %q, want Integer inferred from the value", got)
	}
	if got := byName["REGION"].Value; got != "eu-west-1" {
		t.Errorf("REGION = %v, want the stored variable kept", got)
	}
}

func TestUpdateRejectsAVariableWithoutAValue(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load"): aLoadTest("checkout-load"),
	})

	message := expectFailure(t, NewUpdateCmd(), "checkout-load", "--var", "TIMEOUT")

	mustContain(t, "variable error", message, "TIMEOUT")
}

// A config file is the base and flags win, so one file can serve several
// updates without being edited between them.
func TestUpdateLetsFlagsOverrideTheConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.json")
	contents, err := json.Marshal(map[string]any{
		"name":            "From the file",
		"description":     "Also from the file",
		"infraIdentifier": "file-cluster",
	})
	if err != nil {
		t.Fatalf("building the config file: %v", err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("writing the config file: %v", err)
	}

	body := updated(t, "--config", path, "--name", "From the flag")

	if body.Name != "From the flag" {
		t.Errorf("name = %q, want the flag to win", body.Name)
	}
	if body.Description != "Also from the file" {
		t.Errorf("description = %q, want the file value where no flag was passed", body.Description)
	}
	if body.InfraIdentifier != "file-cluster" {
		t.Errorf("infraIdentifier = %q, want the file value", body.InfraIdentifier)
	}
}

func TestUpdateReportsAConfigFileThatIsNotThere(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load"): aLoadTest("checkout-load"),
	})

	missing := filepath.Join(t.TempDir(), "absent.json")
	message := expectFailure(t, NewUpdateCmd(), "checkout-load", "--config", missing)

	mustContain(t, "missing config error", message, "absent.json")
}

// The read comes first, so a test that is not there fails before anything is
// sent — there is nothing to merge into.
func TestUpdateStopsWhenTheTestIsNotThere(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/load-tests/ghost"): http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"message":"load test not found"}`, http.StatusNotFound)
		}),
	})

	message := expectFailure(t, NewUpdateCmd(), "ghost", "--name", "New name")

	mustContain(t, "not found error", message, "not found")
	if len(stub.requests()) != 1 {
		t.Errorf("made %v, want the update abandoned after a failed read", summarise(stub.requests()))
	}
}

func TestSyncPullsTheLatestTemplateRevision(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		POST("/load-tests/checkout-load/sync-template"): aLoadTest("checkout-load"),
	})

	out := expectSuccess(t, NewSyncCmd(), "checkout-load")

	if got := stub.only().Path; got != "/load-tests/checkout-load/sync-template" {
		t.Errorf("called %s, want the sync route", got)
	}
	mustContain(t, "sync confirmation", out.stderr, "Synced", "checkout-load")
	mustContain(t, "sync output", out.stdout, "checkout-load")
}

// Only a reference import can be synced, and the service is the one that knows
// which. Its refusal has to reach the user rather than read as a success.
func TestSyncSurfacesATestThatCannotBeSynced(t *testing.T) {
	serveLTM(t, map[call]any{
		POST("/load-tests/checkout-load/sync-template"): http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"message":"load test was not imported by reference"}`, http.StatusBadRequest)
		}),
	})

	message := expectFailure(t, NewSyncCmd(), "checkout-load")

	mustContain(t, "sync refusal", message, "not imported by reference")
}

// jsonNumber reads a value that came back through JSON, where every number is
// a float64 regardless of how it was written.
func jsonNumber(value any) float64 {
	if number, ok := value.(float64); ok {
		return number
	}
	return -1
}
