package command

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/harness/harness-cli/cmd/rt/loadtest/api"
)

// aTemplate is a template as the service returns one. The tool config is the
// shape the tunable flags patch, so an update test can tell an inherited value
// from an overwritten one.
func aTemplate(identity, revision string) map[string]any {
	return map[string]any{
		"uniqueId":    "3c4d5e6f-0000-4000-8000-000000000002",
		"identity":    identity,
		"revision":    revision,
		"name":        "Standard HTTP",
		"toolType":    "K6",
		"hubIdentity": "harness-hub",
		"updatedAt":   "2026-08-01T09:00:00Z",
		"toolConfig": map[string]any{
			"k6": map[string]any{
				"script":   map[string]any{"content": "ZXhwb3J0IGRlZmF1bHQ=", "source": "inline"},
				"tunables": map[string]any{"targetUsers": 100, "durationSeconds": 600},
			},
		},
	}
}

// scriptFile writes a throwaway script and returns its path, for the flags that
// read one off disk and base64 it into the request.
func scriptFile(t *testing.T, name, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing the test script: %v", err)
	}
	return path
}

func TestTemplateListSpansEveryHubWhenNoneIsNamed(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/load-test-templates"): map[string]any{
			"items": []any{aTemplate("standard-http", "v2")},
		},
	})

	out := expectSuccess(t, NewTemplateCmd(), "list")

	if got := stub.only().Query.Get("hubIdentity"); got != "" {
		t.Errorf("hubIdentity = %q, want it empty so the listing spans every hub", got)
	}
	mustContain(t, "template list output", out.stdout, "standard-http", "v2")
}

func TestTemplateListNarrowsToOneHub(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/load-test-templates"): map[string]any{"items": []any{}},
	})

	expectSuccess(t, NewTemplateCmd(), "list", "--hub-id", "harness-hub",
		"--tool-type", "K6", "--infra-type", "kubernetes", "--search", "http",
		"--sort-field", "updatedAt", "--page", "1", "--limit", "25")

	query := stub.only().Query
	for key, want := range map[string]string{
		"hubIdentity": "harness-hub",
		"toolType":    "K6",
		"infraType":   "kubernetes",
		"search":      "http",
		"sortField":   "updatedAt",
		"page":        "1",
		"limit":       "25",
	} {
		if got := query.Get(key); got != want {
			t.Errorf("query %s = %q, want %q (full query: %v)", key, got, want, query)
		}
	}
}

// The template store has no environment, tag or status filter. Offering one
// would be worse than not having it: the request would be accepted, the filter
// ignored, and the unfiltered result would read as filtered. So the flags are
// absent, and nothing reaches the query string either.
func TestTemplateListDropsFiltersTheTemplateStoreIgnores(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/load-test-templates"): map[string]any{"items": []any{}},
	})

	list, _, err := NewTemplateCmd().Find([]string{"list"})
	if err != nil {
		t.Fatalf("finding the list subcommand: %v", err)
	}
	for _, unsupported := range []string{"environment-id", "status", "tag"} {
		if list.Flags().Lookup(unsupported) != nil {
			t.Errorf("--%s is offered by template list, which cannot honour it", unsupported)
		}
	}

	expectSuccess(t, NewTemplateCmd(), "list", "--hub-id", "harness-hub", "--search", "http")

	query := stub.only().Query
	for _, unsupported := range []string{"environmentIdentifier", "status", "tags"} {
		if query.Has(unsupported) {
			t.Errorf("%s reached the template listing, which ignores it: %v", unsupported, query)
		}
	}
	if query.Get("search") != "http" {
		t.Errorf("a filter the store does support was dropped: %v", query)
	}
}

func TestTemplateGetResolvesToTheLatestRevision(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/load-test-templates/standard-http"): aTemplate("standard-http", "v2"),
	})

	out := expectSuccess(t, NewTemplateCmd(), "get", "standard-http", "--hub-id", "harness-hub")

	if got := stub.only().Query.Get("hubIdentity"); got != "harness-hub" {
		t.Errorf("hubIdentity = %q, want the hub to be matched exactly", got)
	}
	mustContain(t, "template get output", out.stdout, "standard-http", "v2")
}

func TestTemplateGetRevisionAsksForTheNamedVersion(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/load-test-templates/standard-http/revisions/v1"): aTemplate("standard-http", "v1"),
	})

	out := expectSuccess(t, NewTemplateCmd(), "get-revision", "standard-http", "v1", "--hub-id", "harness-hub")

	if got := stub.only().Path; !strings.HasSuffix(got, "/revisions/v1") {
		t.Errorf("got %s, want the named revision route", got)
	}
	mustContain(t, "get-revision output", out.stdout, "v1")
}

func TestTemplateListRevisionsPrintsEveryVersion(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-test-templates/standard-http/revisions"): []any{
			aTemplate("standard-http", "v2"),
			aTemplate("standard-http", "v1"),
		},
	})

	out := expectSuccess(t, NewTemplateCmd(), "list-revisions", "standard-http", "--hub-id", "harness-hub")

	mustContain(t, "list-revisions output", out.stdout, "v1", "v2")
}

func TestTemplateCreatePublishesTheFirstRevision(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		POST("/load-test-templates"): aTemplate("standard-http", "v1"),
	})

	expectSuccess(t, NewTemplateCmd(), "create",
		"--identity", "standard-http", "--revision", "v1",
		"--name", "Standard HTTP", "--tool-type", "K6", "--hub-id", "harness-hub",
		"--mode", "image", "--image", "ghcr.io/acme/k6-suite:1.4.0",
		"--target-users", "100", "--duration", "600")

	sent := stub.only()
	if got := sent.Query.Get("hubIdentity"); got != "harness-hub" {
		t.Errorf("hubIdentity = %q, want the hub the template is published into", got)
	}

	var body api.CreateTemplateRequest
	sent.decode(t, &body)

	if body.Identity != "standard-http" || body.Revision != "v1" {
		t.Errorf("identity/revision = %q/%q, want standard-http/v1", body.Identity, body.Revision)
	}
	if body.ToolType != "K6" {
		t.Errorf("toolType = %q, want K6", body.ToolType)
	}
	if body.ToolConfig == nil {
		t.Fatal("the tunable flags produced no tool config")
	}
}

// A template is versioned, so publishing one without naming the version is a
// mistake the CLI can catch before the request.
func TestTemplateCreateInsistsOnARevisionName(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewTemplateCmd(), "create",
		"--identity", "standard-http", "--tool-type", "K6", "--hub-id", "harness-hub",
		"--mode", "image", "--image", "ghcr.io/acme/k6:1")

	mustContain(t, "create error", message, "--revision is required", "v1")
}

func TestTemplateCreateInsistsOnAnIdentity(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewTemplateCmd(), "create",
		"--revision", "v1", "--tool-type", "K6", "--hub-id", "harness-hub")

	mustContain(t, "create error", message, "--identity is required")
}

func TestTemplateCreateInsistsOnATool(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewTemplateCmd(), "create",
		"--identity", "standard-http", "--revision", "v1", "--hub-id", "harness-hub")

	mustContain(t, "create error", message, "--tool-type is required", "JMeter", "Locust", "K6")
}

// The display name is optional and falls back to the identity, so a template
// never lists with an empty NAME column.
func TestTemplateCreateNamesATemplateAfterItsIdentity(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		POST("/load-test-templates"): aTemplate("standard-http", "v1"),
	})

	expectSuccess(t, NewTemplateCmd(), "create",
		"--identity", "standard-http", "--revision", "v1", "--tool-type", "K6",
		"--hub-id", "harness-hub", "--mode", "image", "--image", "ghcr.io/acme/k6:1")

	var body api.CreateTemplateRequest
	stub.only().decode(t, &body)

	if body.Name != "standard-http" {
		t.Errorf("name = %q, want it defaulted to the identity", body.Name)
	}
}

// A script is uploaded as base64 in the request rather than referenced, so the
// file has to be read and encoded before the call.
func TestTemplateCreateUploadsTheScriptItWasPointedAt(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		POST("/load-test-templates"): aTemplate("standard-http", "v1"),
	})

	expectSuccess(t, NewTemplateCmd(), "create",
		"--identity", "standard-http", "--revision", "v1", "--tool-type", "K6",
		"--hub-id", "harness-hub",
		"--script", scriptFile(t, "standard.js", "export default function () {}"))

	var body api.CreateTemplateRequest
	stub.only().decode(t, &body)

	content := digInto(t, body.ToolConfig, "k6", "script", "content")
	if content == "" {
		t.Fatalf("the script never reached the request: %v", body.ToolConfig)
	}
	if strings.Contains(content, "export default") {
		t.Errorf("the script was sent verbatim rather than base64 encoded: %q", content)
	}
}

func TestTemplateCreateReportsAScriptItCannotRead(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewTemplateCmd(), "create",
		"--identity", "standard-http", "--revision", "v1", "--tool-type", "K6",
		"--hub-id", "harness-hub", "--script", "/no/such/script.js")

	mustContain(t, "script error", message, "script.js")
}

// Each optional field is read back from its own flag by name, so a flag renamed
// without its reader following would publish a template missing that field
// rather than fail. One create exercising all of them is what catches that.
func TestTemplateCreateCarriesEveryOptionalFlagIntoTheRequest(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		POST("/load-test-templates"): aTemplate("standard-http", "v1"),
	})

	expectSuccess(t, NewTemplateCmd(), "create",
		"--identity", "standard-http", "--revision", "v1", "--tool-type", "K6",
		"--hub-id", "harness-hub", "--name", "Standard HTTP",
		"--description", "The shared HTTP smoke profile",
		"--infra-type", "kubernetes",
		"--infra-id", "perf-cluster",
		"--environment-id", "staging",
		"--tag", "team:perf", "--tag", "tier:gold",
		"--var", "REGION=eu-west-1", "--var", "USERS=250",
		"--mode", "image", "--image", "ghcr.io/acme/k6-suite:1.4.0")

	var body api.CreateTemplateRequest
	stub.only().decode(t, &body)

	for field, got := range map[string]string{
		"name":        body.Name,
		"description": body.Description,
		"infraType":   body.InfraType,
		"infraId":     body.InfraID,
		"envId":       body.EnvID,
	} {
		want := map[string]string{
			"name":        "Standard HTTP",
			"description": "The shared HTTP smoke profile",
			"infraType":   "kubernetes",
			"infraId":     "perf-cluster",
			"envId":       "staging",
		}[field]
		if got != want {
			t.Errorf("%s = %q, want %q", field, got, want)
		}
	}
	if got := strings.Join(body.Tags, ","); got != "team:perf,tier:gold" {
		t.Errorf("tags = %q, want both of them", got)
	}

	// A flag cannot state a variable's type, so it is inferred from the value.
	// Declaring USERS as a String would refuse a numeric override at run time.
	byName := map[string]api.Variable{}
	for _, variable := range body.Variables {
		byName[variable.Name] = variable
	}
	if got := byName["REGION"]; got.Value != "eu-west-1" || got.Type != "String" {
		t.Errorf("REGION = %v (%s), want eu-west-1 as a String", got.Value, got.Type)
	}
	if got := byName["USERS"]; jsonNumber(got.Value) != 250 || got.Type != "Integer" {
		t.Errorf("USERS = %v (%s), want 250 as an Integer", got.Value, got.Type)
	}
}

// A template definition can be held in version control as JSON and published
// from there, with flags amending it for the one field that varies.
func TestTemplateCreateStartsFromAConfigFileAndLetsFlagsWin(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		POST("/load-test-templates"): aTemplate("standard-http", "v1"),
	})

	path := configFile(t, map[string]any{
		"identity":    "standard-http",
		"revision":    "v1",
		"name":        "Standard HTTP",
		"description": "From the file",
		"toolType":    "K6",
		"infraId":     "perf-cluster",
		"toolConfig": map[string]any{
			"k6": map[string]any{
				"mode":     "image",
				"script":   map[string]any{"image": "ghcr.io/acme/k6-suite:1.4.0", "source": "image"},
				"tunables": map[string]any{"targetUsers": 100},
			},
		},
	})

	expectSuccess(t, NewTemplateCmd(), "create",
		"--config", path, "--hub-id", "harness-hub",
		"--description", "From the flag", "--target-users", "400")

	var body api.CreateTemplateRequest
	stub.only().decode(t, &body)

	if body.Identity != "standard-http" || body.Revision != "v1" || body.ToolType != "K6" {
		t.Errorf("the file's own fields did not survive: %+v", body)
	}
	if body.InfraID != "perf-cluster" {
		t.Errorf("infraId = %q, want the file's value; a flag left unset must not blank it", body.InfraID)
	}
	if body.Description != "From the flag" {
		t.Errorf("description = %q, want the flag to win over the file", body.Description)
	}
	if got := digNumber(t, body.ToolConfig, "k6", "tunables", "targetUsers"); got != "400" {
		t.Errorf("targetUsers = %q, want the flag's 400 patched into the file's config", got)
	}
}

// A variable is what a load test fills in when it imports the template, so a
// malformed one has to be refused before the template is published with it.
func TestTemplateCreateReportsAMalformedVariable(t *testing.T) {
	stub := serveLTM(t, map[call]any{})

	message := expectFailure(t, NewTemplateCmd(), "create",
		"--identity", "standard-http", "--revision", "v1", "--tool-type", "K6",
		"--hub-id", "harness-hub", "--mode", "image", "--image", "ghcr.io/acme/k6:1",
		"--var", "REGION")

	mustContain(t, "variable error", message, "REGION", "NAME=VALUE")
	if got := len(stub.requests()); got != 0 {
		t.Errorf("a template was published despite the bad variable: %v", summarise(stub.requests()))
	}
}

// An update edits a revision in place, so the stored config is the base. Only
// sending the changed leaf would drop the script along with everything else.
func TestTemplateUpdateKeepsTheStoredConfigItDidNotChange(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/load-test-templates/standard-http"): aTemplate("standard-http", "v2"),
		PUT("/load-test-templates/standard-http"): aTemplate("standard-http", "v2"),
	})

	expectSuccess(t, NewTemplateCmd(), "update", "standard-http",
		"--hub-id", "harness-hub", "--target-users", "250")

	var body api.UpdateTemplateRequest
	stub.find(PUT("/load-test-templates/standard-http")).decode(t, &body)

	if got := digInto(t, body.ToolConfig, "k6", "script", "content"); got == "" {
		t.Errorf("the stored script was dropped by an update that did not touch it: %v", body.ToolConfig)
	}
	if got := digNumber(t, body.ToolConfig, "k6", "tunables", "targetUsers"); got != "250" {
		t.Errorf("targetUsers = %q, want the new value 250", got)
	}
}

// Metadata-only edits have no reason to send a tool config at all, and sending
// a half-built one would overwrite what is stored.
func TestTemplateUpdateLeavesToolConfigAloneForAMetadataEdit(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/load-test-templates/standard-http"): aTemplate("standard-http", "v2"),
		PUT("/load-test-templates/standard-http"): aTemplate("standard-http", "v2"),
	})

	expectSuccess(t, NewTemplateCmd(), "update", "standard-http",
		"--hub-id", "harness-hub", "--description", "Updated for the new checkout")

	var body api.UpdateTemplateRequest
	stub.find(PUT("/load-test-templates/standard-http")).decode(t, &body)

	if body.Description != "Updated for the new checkout" {
		t.Errorf("description = %q, want the new text", body.Description)
	}
	if body.ToolConfig != nil {
		t.Errorf("a metadata edit sent a tool config: %v", body.ToolConfig)
	}
}

// The update builder reads a different set of flags from the create one — it
// has no --tool-type or --revision, since neither can be edited in place — so
// its readers need exercising in their own right.
func TestTemplateUpdateCarriesEveryOptionalFlagIntoTheRequest(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/load-test-templates/standard-http"): aTemplate("standard-http", "v2"),
		PUT("/load-test-templates/standard-http"): aTemplate("standard-http", "v2"),
	})

	expectSuccess(t, NewTemplateCmd(), "update", "standard-http",
		"--hub-id", "harness-hub",
		"--name", "Standard HTTP v2",
		"--description", "Now covers the new checkout",
		"--infra-type", "kubernetes",
		"--infra-id", "perf-cluster-2",
		"--environment-id", "prod",
		"--tag", "team:perf", "--tag", "tier:gold",
		"--var", "REGION=us-east-1")

	var body api.UpdateTemplateRequest
	stub.find(PUT("/load-test-templates/standard-http")).decode(t, &body)

	if body.Name != "Standard HTTP v2" || body.Description != "Now covers the new checkout" {
		t.Errorf("name/description = %q/%q, want the new values", body.Name, body.Description)
	}
	if body.InfraType != "kubernetes" || body.InfraID != "perf-cluster-2" || body.EnvID != "prod" {
		t.Errorf("infra/env = %q/%q/%q, want the new values", body.InfraType, body.InfraID, body.EnvID)
	}
	if got := strings.Join(body.Tags, ","); got != "team:perf,tier:gold" {
		t.Errorf("tags = %q, want both of them", got)
	}
	if len(body.Variables) != 1 || body.Variables[0].Name != "REGION" || body.Variables[0].Value != "us-east-1" {
		t.Errorf("variables = %v, want REGION replaced", body.Variables)
	}
}

// Switching an existing template from a script to a container is a mode change
// on the stored block, so the rest of that block has to survive it.
func TestTemplateUpdateCanSwitchTheModeOfTheStoredConfig(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/load-test-templates/standard-http"): aTemplate("standard-http", "v2"),
		PUT("/load-test-templates/standard-http"): aTemplate("standard-http", "v2"),
	})

	expectSuccess(t, NewTemplateCmd(), "update", "standard-http",
		"--hub-id", "harness-hub", "--mode", "image")

	var body api.UpdateTemplateRequest
	stub.find(PUT("/load-test-templates/standard-http")).decode(t, &body)

	if got := digInto(t, body.ToolConfig, "k6", "mode"); got != "image" {
		t.Errorf("mode = %q, want image", got)
	}
	if got := digNumber(t, body.ToolConfig, "k6", "tunables", "targetUsers"); got != "100" {
		t.Errorf("the stored tunables were dropped by a mode change: %v", body.ToolConfig)
	}
}

func TestTemplateUpdateRejectsAModeThatIsNotOne(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-test-templates/standard-http"): aTemplate("standard-http", "v2"),
	})

	message := expectFailure(t, NewTemplateCmd(), "update", "standard-http",
		"--hub-id", "harness-hub", "--mode", "ui")

	mustContain(t, "mode error", message, "no longer supported", "--script", "--mode image")
}

// A config file given to update carries a tool config of its own, which stands
// in for the stored one rather than being merged into it.
func TestTemplateUpdateStartsFromAConfigFile(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/load-test-templates/standard-http"): aTemplate("standard-http", "v2"),
		PUT("/load-test-templates/standard-http"): aTemplate("standard-http", "v2"),
	})

	path := configFile(t, map[string]any{
		"description": "Rewritten from the file",
		"toolConfig": map[string]any{
			"k6": map[string]any{
				"tunables": map[string]any{"targetUsers": 750},
			},
		},
	})

	expectSuccess(t, NewTemplateCmd(), "update", "standard-http",
		"--hub-id", "harness-hub", "--config", path)

	var body api.UpdateTemplateRequest
	stub.find(PUT("/load-test-templates/standard-http")).decode(t, &body)

	if body.Description != "Rewritten from the file" {
		t.Errorf("description = %q, want the file's text", body.Description)
	}
	if got := digNumber(t, body.ToolConfig, "k6", "tunables", "targetUsers"); got != "750" {
		t.Errorf("targetUsers = %q, want the file's 750", got)
	}
	// The file said what the config should be, so the stored script is not
	// carried into it; that is the difference between --config and a flag.
	if got := digInto(t, body.ToolConfig, "k6", "script", "content"); got != "" {
		t.Errorf("the stored script was merged into a config the file stated in full: %v", body.ToolConfig)
	}
}

func TestTemplateUpdateReportsAMalformedVariable(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/load-test-templates/standard-http"): aTemplate("standard-http", "v2"),
	})

	message := expectFailure(t, NewTemplateCmd(), "update", "standard-http",
		"--hub-id", "harness-hub", "--var", "=eu-west-1")

	mustContain(t, "variable error", message, "NAME=VALUE")
	for _, sent := range stub.requests() {
		if sent.Method == http.MethodPut {
			t.Errorf("the template was updated despite the bad variable")
		}
	}
}

func TestTemplateCreateRevisionInheritsWhatItWasNotGiven(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/load-test-templates/standard-http"):            aTemplate("standard-http", "v1"),
		POST("/load-test-templates/standard-http/revisions"): aTemplate("standard-http", "v2"),
	})

	out := expectSuccess(t, NewTemplateCmd(), "create-revision", "standard-http",
		"--revision", "v2", "--hub-id", "harness-hub", "--target-users", "500")

	var body api.CreateRevisionRequest
	stub.find(POST("/load-test-templates/standard-http/revisions")).decode(t, &body)

	if body.Revision != "v2" {
		t.Errorf("revision = %q, want v2", body.Revision)
	}
	// The script was not restated, so it has to have been carried over.
	if got := digInto(t, body.ToolConfig, "k6", "script", "content"); got == "" {
		t.Errorf("the inherited script is missing: %v", body.ToolConfig)
	}
	if got := digNumber(t, body.ToolConfig, "k6", "tunables", "targetUsers"); got != "500" {
		t.Errorf("targetUsers = %q, want the new value 500", got)
	}
	mustContain(t, "publish notice", out.stderr, "standard-http", "v2")
}

func TestTemplateCreateRevisionInsistsOnAName(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-test-templates/standard-http"): aTemplate("standard-http", "v1"),
	})

	message := expectFailure(t, NewTemplateCmd(), "create-revision", "standard-http", "--hub-id", "harness-hub")

	mustContain(t, "create-revision error", message, "--revision is required", "v2")
}

// A revision is immutable, so reusing the current name would silently do
// nothing or fail obscurely. The message has to point at the in-place edit.
func TestTemplateCreateRevisionRefusesToReuseTheLatestName(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-test-templates/standard-http"): aTemplate("standard-http", "v2"),
	})

	message := expectFailure(t, NewTemplateCmd(), "create-revision", "standard-http",
		"--revision", "v2", "--hub-id", "harness-hub")

	mustContain(t, "create-revision error", message, "v2", "immutable", "template update")
}

// A revision inherits every field it does not restate, so each optional flag
// has to be read for the new revision to differ from the old one at all.
func TestTemplateCreateRevisionCarriesEveryOptionalFlagIntoTheRequest(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/load-test-templates/standard-http"):            aTemplate("standard-http", "v1"),
		POST("/load-test-templates/standard-http/revisions"): aTemplate("standard-http", "v2"),
	})

	expectSuccess(t, NewTemplateCmd(), "create-revision", "standard-http",
		"--hub-id", "harness-hub", "--revision", "v2",
		"--name", "Standard HTTP v2",
		"--description", "Adds the checkout journey",
		"--infra-id", "perf-cluster-2",
		"--environment-id", "prod",
		"--tag", "team:perf", "--tag", "tier:gold",
		"--var", "REGION=us-east-1", "--var", "STRICT=true",
		"--script", scriptFile(t, "checkout.js", "export default function () {}"))

	var body api.CreateRevisionRequest
	stub.find(POST("/load-test-templates/standard-http/revisions")).decode(t, &body)

	if body.Name != "Standard HTTP v2" || body.Description != "Adds the checkout journey" {
		t.Errorf("name/description = %q/%q, want the new values", body.Name, body.Description)
	}
	if body.InfraID != "perf-cluster-2" || body.EnvID != "prod" {
		t.Errorf("infra/env = %q/%q, want the new values", body.InfraID, body.EnvID)
	}
	if got := strings.Join(body.Tags, ","); got != "team:perf,tier:gold" {
		t.Errorf("tags = %q, want both of them", got)
	}

	byName := map[string]api.Variable{}
	for _, variable := range body.Variables {
		byName[variable.Name] = variable
	}
	if got := byName["STRICT"]; got.Value != true || got.Type != "Boolean" {
		t.Errorf("STRICT = %v (%s), want true as a Boolean", got.Value, got.Type)
	}

	// The new script has to replace the inherited one rather than sit beside
	// it, or the revision would publish the version it was meant to supersede.
	content := digInto(t, body.ToolConfig, "k6", "script", "content")
	if content == "" || content == "ZXhwb3J0IGRlZmF1bHQ=" {
		t.Errorf("script content = %q, want the newly uploaded file", content)
	}
}

func TestTemplateCreateRevisionStartsFromAConfigFile(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/load-test-templates/standard-http"):            aTemplate("standard-http", "v1"),
		POST("/load-test-templates/standard-http/revisions"): aTemplate("standard-http", "v2"),
	})

	path := configFile(t, map[string]any{
		"revision":    "v2",
		"description": "From the file",
		"toolConfig": map[string]any{
			"k6": map[string]any{
				"tunables": map[string]any{"targetUsers": 900},
			},
		},
	})

	expectSuccess(t, NewTemplateCmd(), "create-revision", "standard-http",
		"--hub-id", "harness-hub", "--config", path)

	var body api.CreateRevisionRequest
	stub.find(POST("/load-test-templates/standard-http/revisions")).decode(t, &body)

	if body.Revision != "v2" || body.Description != "From the file" {
		t.Errorf("revision/description = %q/%q, want the file's values", body.Revision, body.Description)
	}
	if got := digNumber(t, body.ToolConfig, "k6", "tunables", "targetUsers"); got != "900" {
		t.Errorf("targetUsers = %q, want the file's 900", got)
	}
}

func TestTemplateCreateRevisionReportsAMalformedVariable(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/load-test-templates/standard-http"): aTemplate("standard-http", "v1"),
	})

	message := expectFailure(t, NewTemplateCmd(), "create-revision", "standard-http",
		"--hub-id", "harness-hub", "--revision", "v2", "--var", "REGION")

	mustContain(t, "variable error", message, "REGION", "NAME=VALUE")
	for _, sent := range stub.requests() {
		if sent.Method == http.MethodPost {
			t.Errorf("a revision was published despite the bad variable")
		}
	}
}

// The tunable flags write into whichever tool the stored template uses, so a
// flag belonging to another engine has to be refused rather than written into
// a block the service will not read.
func TestTemplateCreateRevisionRefusesAFlagBelongingToAnotherEngine(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/load-test-templates/standard-http"): aTemplate("standard-http", "v1"),
	})

	message := expectFailure(t, NewTemplateCmd(), "create-revision", "standard-http",
		"--hub-id", "harness-hub", "--revision", "v2", "--spawn-rate", "10")

	mustContain(t, "engine mismatch error", message, "spawn-rate")
	for _, sent := range stub.requests() {
		if sent.Method == http.MethodPost {
			t.Errorf("a revision was published with a flag the engine does not have")
		}
	}
}

func TestTemplateDeleteRemovesTheWholeTemplate(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		DELETE("/load-test-templates/standard-http"): "",
	})

	out := expectSuccess(t, NewTemplateCmd(), "delete", "standard-http", "--hub-id", "harness-hub", "--force")

	if got := stub.only().Method; got != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", got)
	}
	mustContain(t, "delete confirmation", out.stdout, "Deleted template", "standard-http")
}

func TestTemplateDeleteRevisionLeavesTheOthers(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		DELETE("/load-test-templates/standard-http/revisions/v1"): "",
	})

	out := expectSuccess(t, NewTemplateCmd(), "delete-revision", "standard-http", "v1",
		"--hub-id", "harness-hub", "--force")

	if got := stub.only().Path; got != "/load-test-templates/standard-http/revisions/v1" {
		t.Errorf("got %s, want only the named revision to be deleted", got)
	}
	mustContain(t, "delete confirmation", out.stdout, "Deleted revision", "v1", "standard-http")
}

// Without --force a delete asks first. There is no terminal in a pipeline or a
// test, and guessing "yes" on a destructive call is not an option, so it has to
// refuse and say how to proceed deliberately.
func TestTemplateDeleteWithoutForceRefusesWithNoTerminal(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewTemplateCmd(), "delete", "standard-http", "--hub-id", "harness-hub")

	mustContain(t, "confirmation refusal", message, "not a terminal", "--force")
}

func TestTemplateDeleteRevisionWithoutForceRefusesWithNoTerminal(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewTemplateCmd(), "delete-revision", "standard-http", "v1", "--hub-id", "harness-hub")

	mustContain(t, "confirmation refusal", message, "not a terminal", "--force")
}

// The export is YAML, so it has to bypass the printer entirely: re-encoding it
// as JSON because --format said so would defeat the point of an export.
func TestTemplateExportYAMLPrintsTheDocumentUntouched(t *testing.T) {
	const document = "identity: standard-http\nrevision: v2\n"

	serveLTM(t, map[call]any{
		GET("/load-test-templates/standard-http/yaml"): document,
	})

	out := expectSuccess(t, NewTemplateCmd(), "export-yaml", "standard-http", "--hub-id", "harness-hub")

	if out.stdout != document {
		t.Errorf("the exported YAML was re-rendered:\ngot  %q\nwant %q", out.stdout, document)
	}
}

func TestTemplateExportYAMLWritesTheFileItWasAskedFor(t *testing.T) {
	const document = "identity: standard-http\nrevision: v2\n"

	serveLTM(t, map[call]any{
		GET("/load-test-templates/standard-http/yaml"): document,
	})
	destination := filepath.Join(t.TempDir(), "standard-http.yaml")

	out := expectSuccess(t, NewTemplateCmd(), "export-yaml", "standard-http",
		"--hub-id", "harness-hub", "--output-file", destination)

	written, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("the export wrote no file: %v", err)
	}
	if string(written) != document {
		t.Errorf("file holds %q, want %q", written, document)
	}
	// The notice belongs on stderr so redirecting stdout to a file still works.
	mustContain(t, "export notice", out.stderr, "standard-http.yaml")
	if out.stdout != "" {
		t.Errorf("the document was printed as well as written: %q", out.stdout)
	}
}

func TestTemplateExportYAMLReportsAPathItCannotWrite(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-test-templates/standard-http/yaml"): "identity: standard-http\n",
	})

	message := expectFailure(t, NewTemplateCmd(), "export-yaml", "standard-http",
		"--hub-id", "harness-hub", "--output-file", filepath.Join(t.TempDir(), "no-such-dir", "out.yaml"))

	mustContain(t, "write error", message, "out.yaml")
}

// Variables and inputs are different things: a variable is a named value, an
// input is a config leaf left as "<+input>" and addressed by path. The table
// leads with inputs because those are what block a run.
func TestTemplateVariablesListsTheInputsThatBlockARun(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-test-templates/standard-http/variables"): map[string]any{
			"variables": []any{
				map[string]any{"name": "region", "value": "eu-west-1", "type": "string"},
			},
			"inputs": []any{
				map[string]any{"path": "k6.tunables.targetUsers", "type": "number", "required": true},
			},
		},
	})
	globals().Format = formatTable

	out := expectSuccess(t, NewTemplateCmd(), "variables", "standard-http", "--hub-id", "harness-hub")

	mustContain(t, "variables table", out.stdout, "k6.tunables.targetUsers", "PATH")
}

func TestTemplateVariablesCanDescribeAnOlderRevision(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/load-test-templates/standard-http/variables"): map[string]any{"inputs": []any{}},
	})

	expectSuccess(t, NewTemplateCmd(), "variables", "standard-http",
		"--hub-id", "harness-hub", "--revision", "v1")

	if got := stub.only().Query.Get("revision"); got != "v1" {
		t.Errorf("revision = %q, want v1", got)
	}
}

// Omitting --revision means the latest, which the service resolves. Sending an
// empty revision would ask for one named "".
func TestTemplateVariablesOmitsTheRevisionForTheLatest(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/load-test-templates/standard-http/variables"): map[string]any{"inputs": []any{}},
	})

	expectSuccess(t, NewTemplateCmd(), "variables", "standard-http", "--hub-id", "harness-hub")

	if stub.only().Query.Has("revision") {
		t.Error("an empty revision was sent rather than omitted")
	}
}

func TestTemplateGetReportsWhatTheServiceSaidWentWrong(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-test-templates/nope"): http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"message":"template not found"}`, http.StatusNotFound)
		}),
	})

	message := expectFailure(t, NewTemplateCmd(), "get", "nope", "--hub-id", "harness-hub")

	mustContain(t, "not-found error", message, "template not found")
}

// digInto reads a string leaf out of a decoded tool config, so a test can
// assert on one value without unpacking the whole tree.
func digInto(t *testing.T, config map[string]any, path ...string) string {
	t.Helper()
	value := dig(config, path...)
	text, _ := value.(string)
	return text
}

// digNumber renders a numeric leaf the way formatNumber does, so a whole
// number compares as "250" rather than "250.000000".
func digNumber(t *testing.T, config map[string]any, path ...string) string {
	t.Helper()
	value := dig(config, path...)
	if value == nil {
		return ""
	}
	return formatNumber(value)
}

func dig(config map[string]any, path ...string) any {
	var current any = config
	for _, key := range path {
		nested, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = nested[key]
	}
	return current
}
