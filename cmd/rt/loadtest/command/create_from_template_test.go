package command

import (
	"net/http"
	"testing"

	"github.com/harness/harness-cli/cmd/rt/loadtest/api"
	"github.com/harness/harness-cli/config"

	"github.com/spf13/cobra"
)

// inScope pins config.Global to a fixed scope for one test and restores it
// afterwards, so a command built here resolves the same scope every run.
func inScope(t *testing.T) (org, project string) {
	t.Helper()

	const (
		account   = "test-account"
		orgID     = "perf"
		projectID = "checkout"
	)

	previous := config.Global
	t.Cleanup(func() { config.Global = previous })

	config.Global.AccountID = account
	config.Global.OrgID = orgID
	config.Global.ProjectID = projectID

	return orgID, projectID
}

// fromTemplateRequest parses a command line the way the command does. Each call
// gets a fresh command: pflag records a flag as set only on the first parse.
func fromTemplateRequest(t *testing.T, args ...string) (*cobra.Command, error) {
	t.Helper()

	cmd := &cobra.Command{Use: "create-from-template", RunE: func(*cobra.Command, []string) error { return nil }}
	registerFromTemplateFlags(cmd)
	cmd.SetArgs(args)
	cmd.SetOut(nopWriter{})
	cmd.SetErr(nopWriter{})

	return cmd, cmd.Execute()
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }

// imported drives "create-from-template" against a stub and returns the body
// it sent.
func imported(t *testing.T, routes map[call]any, args ...string) api.CreateFromTemplateRequest {
	t.Helper()

	if routes == nil {
		routes = map[call]any{}
	}
	routes[POST("/load-tests/from-template")] = aLoadTest("checkout-load")

	stub := serveLTM(t, routes)
	expectSuccess(t, NewCreateFromTemplateCmd(), args...)

	var body api.CreateFromTemplateRequest
	stub.find(POST("/load-tests/from-template")).decode(t, &body)
	return body
}

// importFlags is the minimum an import needs, so a test states only what it is
// varying.
func importFlags(extra ...string) []string {
	return append([]string{
		"--identity", "checkout-load",
		"--template-id", "standard-http", "--template-revision", "v3",
		"--hub-id", "harness-hub",
		"--infra-id", "perf-cluster", "--environment-id", "staging",
	}, extra...)
}

// A reference import is the default because it is the one that can be re-synced
// later; a local copy is the choice you have to make deliberately.
func TestFromTemplatePinsToTheTemplateByDefault(t *testing.T) {
	body := imported(t, nil, importFlags()...)

	if body.ImportType != api.ImportReference {
		t.Errorf("importType = %q, want REFERENCE by default", body.ImportType)
	}
	if body.TargetType != string(api.TargetKubernetes) {
		t.Errorf("targetType = %q, want kubernetes by default", body.TargetType)
	}
	if body.Name != "checkout-load" {
		t.Errorf("name = %q, want it to fall back to the identity", body.Name)
	}
	if body.TemplateReference.Identity != "standard-http" ||
		body.TemplateReference.Revision != "v3" ||
		body.TemplateReference.HubIdentity != "harness-hub" {
		t.Errorf("templateReference = %+v, want the template pinned by revision", body.TemplateReference)
	}
}

func TestFromTemplateCarriesTheOptionalDetail(t *testing.T) {
	body := imported(t, nil, importFlags(
		"--name", "Checkout load",
		"--description", "Imported from the standard profile",
		"--service-ref", "checkout-svc,cart-svc",
		"--target-type", "kubernetes",
		"--var", "baseUrl=https://eu.example.com", "--var", "users=250")...)

	if body.Description != "Imported from the standard profile" {
		t.Errorf("description = %q, want the one passed", body.Description)
	}
	if len(body.ServiceReferences) != 2 {
		t.Errorf("serviceReferences = %v, want both", body.ServiceReferences)
	}

	byName := map[string]api.Variable{}
	for _, variable := range body.Variables {
		byName[variable.Name] = variable
	}
	if byName["baseUrl"].Type != "String" || byName["users"].Type != "Integer" {
		t.Errorf("variables = %+v, want the types inferred from the values", body.Variables)
	}
}

// A template can leave infrastructure and environment as runtime inputs, but a
// load test cannot, so they are required here whatever the template says.
func TestFromTemplateInsistsOnWhatATemplateCannotSupply(t *testing.T) {
	for _, tc := range []struct {
		name  string
		strip string
		wants []string
	}{
		{name: "no identity", strip: "--identity", wants: []string{"--identity is required"}},
		{name: "no template", strip: "--template-id", wants: []string{"--template-id is required", "template list"}},
		{name: "no revision", strip: "--template-revision", wants: []string{"--template-revision is required", "list-revisions"}},
		{name: "no hub", strip: "--hub-id", wants: []string{"--hub-id is required"}},
		{name: "no infrastructure", strip: "--infra-id", wants: []string{"--infra-id is required", "template cannot supply"}},
		{name: "no environment", strip: "--environment-id", wants: []string{"--environment-id is required", "template cannot supply"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			serveLTM(t, map[call]any{})
			mustContain(t, "missing flag error",
				expectFailure(t, NewCreateFromTemplateCmd(), without(importFlags(), tc.strip)...), tc.wants...)
		})
	}
}

func TestFromTemplateRejectsAnImportTypeThatIsNotOne(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewCreateFromTemplateCmd(), importFlags("--import-type", "COPY")...)

	mustContain(t, "import type error", message, "COPY", "REFERENCE", "LOCAL")
}

// A reference import's configuration belongs to the template and is re-read
// from it, so an override would be accepted here and then quietly lost. Saying
// so names both ways out.
func TestFromTemplateRefusesATunableOverrideOnAReferenceImport(t *testing.T) {
	stub := serveLTM(t, map[call]any{})

	message := expectFailure(t, NewCreateFromTemplateCmd(),
		importFlags("--target-users", "500")...)

	mustContain(t, "reference override error", message,
		"--target-users", "REFERENCE", "--import-type LOCAL", "<+input>", "--runtime-value")
	if len(stub.requests()) != 0 {
		t.Errorf("made %v, want nothing sent for an override that cannot take effect", summarise(stub.requests()))
	}
}

// The message names every tunable flag that was passed, not just the first, so
// a caller does not fix them one round trip at a time.
func TestFromTemplateNamesEveryOverrideItRefuses(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewCreateFromTemplateCmd(),
		importFlags("--target-users", "500", "--duration", "900")...)

	mustContain(t, "reference override error", message, "--target-users", "--duration")
}

// A local import owns its configuration, so the override is applied — but on
// top of the whole template block, since the service replaces toolConfig
// wholesale and a lone tunable would drop the script with it.
func TestFromTemplateAppliesAnOverrideOverTheWholeTemplateBlock(t *testing.T) {
	body := imported(t, map[call]any{
		GET("/load-test-templates/standard-http/revisions/v3"): aTemplate("standard-http", "v3"),
	}, importFlags("--import-type", "LOCAL", "--target-users", "500")...)

	k6, ok := body.ToolConfig["k6"].(map[string]any)
	if !ok {
		t.Fatalf("toolConfig has no k6 block: %v", body.ToolConfig)
	}
	tunables, ok := k6["tunables"].(map[string]any)
	if !ok {
		t.Fatalf("the k6 block has no tunables: %v", k6)
	}
	if got := tunables["targetUsers"]; jsonNumber(got) != 500 {
		t.Errorf("targetUsers = %v, want the override", got)
	}
	if got := tunables["durationSeconds"]; jsonNumber(got) != 600 {
		t.Errorf("durationSeconds = %v, want the template value kept", got)
	}
	if _, found := k6["script"]; !found {
		t.Error("the template's script was dropped by a tunable override")
	}
}

// --mode is applied to the imported block before the other overrides, since it
// decides which of the two artifact shapes the rest of them write into.
func TestFromTemplateCanSwitchTheModeOfTheImportedBlock(t *testing.T) {
	body := imported(t, map[call]any{
		GET("/load-test-templates/standard-http/revisions/v3"): aTemplate("standard-http", "v3"),
	}, importFlags("--import-type", "LOCAL", "--mode", "image", "--target-users", "500")...)

	if got := digInto(t, body.ToolConfig, "k6", "mode"); got != "image" {
		t.Errorf("mode = %q, want image", got)
	}
	if got := digNumber(t, body.ToolConfig, "k6", "tunables", "targetUsers"); got != "500" {
		t.Errorf("targetUsers = %q, want the override applied alongside the mode", got)
	}
}

func TestFromTemplateRejectsAModeThatIsNotOne(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-test-templates/standard-http/revisions/v3"): aTemplate("standard-http", "v3"),
	})

	message := expectFailure(t, NewCreateFromTemplateCmd(),
		importFlags("--import-type", "LOCAL", "--mode", "ui")...)

	mustContain(t, "mode error", message, "no longer supported")
}

// A template stored without a tool config still has to accept an override:
// starting from nothing is not the same as failing.
func TestFromTemplateOverridesATemplateThatCarriesNoConfig(t *testing.T) {
	bare := aTemplate("standard-http", "v3")
	delete(bare, "toolConfig")

	body := imported(t, map[call]any{
		GET("/load-test-templates/standard-http/revisions/v3"): bare,
	}, importFlags("--import-type", "LOCAL", "--target-users", "500")...)

	if got := digNumber(t, body.ToolConfig, "k6", "tunables", "targetUsers"); got != "500" {
		t.Errorf("targetUsers = %q, want the override written into a fresh block", got)
	}
}

// The whole import can be held in version control, with flags amending it.
func TestFromTemplateTakesTheImportFromAConfigFile(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		POST("/load-tests/from-template"): aLoadTest("checkout-load"),
	})

	path := configFile(t, map[string]any{
		"identity":              "checkout-load",
		"infraIdentifier":       "perf-cluster",
		"environmentIdentifier": "staging",
		"importType":            "REFERENCE",
		"templateReference": map[string]any{
			"identity":               "standard-http",
			"revision":               "v3",
			"hubIdentity":            "harness-hub",
			"organizationIdentifier": "platform",
			"projectIdentifier":      "shared",
		},
	})

	expectSuccess(t, NewCreateFromTemplateCmd(), "--config", path, "--name", "Checkout load")

	var body api.CreateFromTemplateRequest
	stub.only().decode(t, &body)

	if body.Name != "Checkout load" {
		t.Errorf("name = %q, want the flag to win over the file", body.Name)
	}
	// The file named the template's scope, so defaulting must not overwrite it
	// with the session's; a template published elsewhere would stop resolving.
	if body.TemplateReference.OrganizationIdentifier != "platform" ||
		body.TemplateReference.ProjectIdentifier != "shared" {
		t.Errorf("templateReference scope = %q/%q, want the file's",
			body.TemplateReference.OrganizationIdentifier, body.TemplateReference.ProjectIdentifier)
	}
}

func TestFromTemplateReportsAConfigFileThatIsNotThere(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewCreateFromTemplateCmd(), "--config", "/no/such/import.json")

	mustContain(t, "missing config file error", message, "import.json")
}

// The variables are what a REFERENCE import fills the template's inputs with,
// so a malformed one has to be caught before the import is created.
func TestFromTemplateReportsAMalformedVariable(t *testing.T) {
	stub := serveLTM(t, map[call]any{})

	message := expectFailure(t, NewCreateFromTemplateCmd(), importFlags("--var", "REGION")...)

	mustContain(t, "variable error", message, "REGION", "NAME=VALUE")
	if got := len(stub.requests()); got != 0 {
		t.Errorf("an import was created despite the bad variable: %v", summarise(stub.requests()))
	}
}

// Without an override there is nothing to read the template for, so the extra
// round trip is not made.
func TestFromTemplateDoesNotReadTheTemplateWhenNothingIsOverridden(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		POST("/load-tests/from-template"): aLoadTest("checkout-load"),
	})

	expectSuccess(t, NewCreateFromTemplateCmd(), importFlags("--import-type", "LOCAL")...)

	stub.only() // fails if the template was read as well
}

// The template is read back at the session scope, so one published elsewhere
// cannot be merged into. Better to say that than to fail the read with a 404
// that reads as a missing template.
func TestFromTemplateRefusesAnOverrideOnATemplateOutsideTheScope(t *testing.T) {
	stub := serveLTM(t, map[call]any{})

	message := expectFailure(t, NewCreateFromTemplateCmd(),
		importFlags("--import-type", "LOCAL", "--template-org-id", "platform",
			"--template-project-id", "shared", "--target-users", "500")...)

	mustContain(t, "out-of-scope override error", message,
		"--target-users", "--template-org-id", "loadtest update")
	if len(stub.requests()) != 0 {
		t.Errorf("made %v, want nothing sent", summarise(stub.requests()))
	}
}

func TestFromTemplateSurfacesATemplateThatIsNotThere(t *testing.T) {
	serveLTM(t, map[call]any{
		POST("/load-tests/from-template"): http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"message":"template revision v3 not found in hub harness-hub"}`, http.StatusNotFound)
		}),
	})

	message := expectFailure(t, NewCreateFromTemplateCmd(), importFlags()...)

	mustContain(t, "missing template", message, "not found", "harness-hub")
}

// without drops a flag and its value from an argument list, so a table can
// state what is missing rather than repeat everything that is not.
func without(args []string, flag string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == flag {
			i++ // skip the value too
			continue
		}
		out = append(out, args[i])
	}
	return out
}

// The endpoint looks a template up at the scope named in the request, and an
// unnamed scope means the account, so a project template reads as missing.
func TestFromTemplateLooksInTheCurrentScopeByDefault(t *testing.T) {
	org, project := inScope(t)

	cmd, err := fromTemplateRequest(t, "--identity", "checkout-load", "--template-id", "standard-http",
		"--template-revision", "v3", "--hub-id", "harness-hub",
		"--infra-id", "perf-cluster", "--environment-id", "staging")
	if err != nil {
		t.Fatalf("parsing the flags: %v", err)
	}

	request, err := buildFromTemplateRequest(cmd)
	if err != nil {
		t.Fatalf("buildFromTemplateRequest: %v", err)
	}
	if request.TemplateReference.OrganizationIdentifier != org {
		t.Errorf("template organization is %q, want the current one, %q",
			request.TemplateReference.OrganizationIdentifier, org)
	}
	if request.TemplateReference.ProjectIdentifier != project {
		t.Errorf("template project is %q, want the current one, %q",
			request.TemplateReference.ProjectIdentifier, project)
	}
}

// Naming the scope is how a template kept elsewhere is reached, so what was
// asked for is what gets sent.
func TestFromTemplateKeepsAScopeThatWasNamed(t *testing.T) {
	inScope(t)

	cmd, err := fromTemplateRequest(t, "--identity", "checkout-load", "--template-id", "standard-http",
		"--template-revision", "v3", "--hub-id", "harness-hub",
		"--template-org-id", "platform", "--template-project-id", "shared",
		"--infra-id", "perf-cluster", "--environment-id", "staging")
	if err != nil {
		t.Fatalf("parsing the flags: %v", err)
	}

	request, err := buildFromTemplateRequest(cmd)
	if err != nil {
		t.Fatalf("buildFromTemplateRequest: %v", err)
	}
	if request.TemplateReference.OrganizationIdentifier != "platform" ||
		request.TemplateReference.ProjectIdentifier != "shared" {
		t.Fatalf("template scope was overwritten: %+v", request.TemplateReference)
	}
}

// Naming the organization alone means an org-scoped template, so the current
// project must not be filled in underneath it.
func TestFromTemplateDoesNotAddAProjectToANamedOrganization(t *testing.T) {
	inScope(t)

	cmd, err := fromTemplateRequest(t, "--identity", "checkout-load", "--template-id", "standard-http",
		"--template-revision", "v3", "--hub-id", "harness-hub", "--template-org-id", "platform",
		"--infra-id", "perf-cluster", "--environment-id", "staging")
	if err != nil {
		t.Fatalf("parsing the flags: %v", err)
	}

	request, err := buildFromTemplateRequest(cmd)
	if err != nil {
		t.Fatalf("buildFromTemplateRequest: %v", err)
	}
	if request.TemplateReference.ProjectIdentifier != "" {
		t.Fatalf("an org-scoped reference acquired project %q",
			request.TemplateReference.ProjectIdentifier)
	}
}

// An account-scoped template is reached by naming the empty scope, which is
// the only way left to ask for it once the default fills the current one in.
func TestFromTemplateReachesAccountScopeWhenAskedForItExplicitly(t *testing.T) {
	inScope(t)

	cmd, err := fromTemplateRequest(t, "--identity", "checkout-load", "--template-id", "standard-http",
		"--template-revision", "v3", "--hub-id", "harness-hub", "--template-org-id", "",
		"--infra-id", "perf-cluster", "--environment-id", "staging")
	if err != nil {
		t.Fatalf("parsing the flags: %v", err)
	}

	request, err := buildFromTemplateRequest(cmd)
	if err != nil {
		t.Fatalf("buildFromTemplateRequest: %v", err)
	}
	if request.TemplateReference.OrganizationIdentifier != "" ||
		request.TemplateReference.ProjectIdentifier != "" {
		t.Fatalf("an explicit account scope was filled in: %+v", request.TemplateReference)
	}
}

// A variable sent without a type is rejected and a flag has nowhere to state
// one, so it is inferred from the value as --runtime-value is.
func TestVariableTypeIsInferredFromTheValue(t *testing.T) {
	parsed, err := parseVariables([]string{
		"baseUrl=https://test-api.k6.io",
		"users=10",
		"errorBudget=0.05",
		"verbose=true",
	})
	if err != nil {
		t.Fatalf("parseVariables: %v", err)
	}

	want := map[string]string{
		"baseUrl":     "String",
		"users":       "Integer",
		"errorBudget": "Number",
		"verbose":     "Boolean",
	}
	for _, variable := range parsed {
		if variable.Type != want[variable.Name] {
			t.Errorf("%s was declared %s, want %s", variable.Name, variable.Type, want[variable.Name])
		}
	}
	if len(parsed) != len(want) {
		t.Fatalf("parsed %d variables, want %d", len(parsed), len(want))
	}
}

func TestParseVariablesRejectsAPairWithNoValue(t *testing.T) {
	for _, pair := range []string{"baseUrl", "=10", " =10"} {
		if _, err := parseVariables([]string{pair}); err == nil {
			t.Errorf("--var %q was accepted", pair)
		}
	}
}
