package command

import (
	"testing"

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
