package command

import (
	"fmt"
	"strings"

	"github.com/harness/harness-cli/cmd/rt/loadtest/api"

	"github.com/spf13/cobra"
)

func NewCreateFromTemplateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create-from-template",
		Short: "Create a load test from a template",
		Long: `Create a load test from a published template revision.

A reference import stays pinned to the template: when a newer revision is
published, "hc rt loadtest get" reports templateUpdateAvailable and
"hc rt loadtest sync" pulls it in. A local import copies the template once and is
thereafter independent of it.

A template can leave its infrastructure and environment as runtime inputs, but
a load test always needs concrete values, so --infra-id and --environment-id
are required here even when the template supplies defaults.

The template is looked for in the current organization and project unless
--template-org-id or --template-project-id says otherwise. Pass
--template-org-id "" to reach a template published at account scope.

Tunable flags such as --target-url override what the template defines, but
only on a LOCAL import: a REFERENCE load test's configuration is owned by the
template and re-resolved from it on every read, so the service rejects an
override. To vary a REFERENCE import, author the value as "<+input>" in the
template and supply it per run with "hc rt loadtest start --runtime-value".`,
		Example: `  # Pinned to the template, so it can be re-synced later
  hc rt loadtest create-from-template --identity checkout-load \
    --name "Checkout load" --template-id standard-http --template-revision v3 \
    --hub-id harness-hub --infra-id perf-cluster --environment-id staging

  # An independent copy, retargeted at a different host
  hc rt loadtest create-from-template --identity checkout-canary \
    --template-id standard-http --template-revision v3 --hub-id harness-hub \
    --import-type LOCAL --infra-id perf-cluster --environment-id staging \
    --target-url https://canary.example.com`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			request, err := buildFromTemplateRequest(cmd)
			if err != nil {
				return err
			}

			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			if err := applyFromTemplateOverrides(cmd, sess, request); err != nil {
				return err
			}

			created, err := sess.client.CreateFromTemplate(cmd.Context(), *request)
			if err != nil {
				return err
			}

			return sess.printer.Print(created, loadTestTable([]*api.LoadTest{created}))
		},
	}

	registerFromTemplateFlags(cmd)

	return cmd
}

// registerFromTemplateFlags is separate from the constructor so a test can
// build its own command: pflag records a flag as set only on the first parse.
func registerFromTemplateFlags(cmd *cobra.Command) {
	flags := cmd.Flags()

	registerConfigFlag(cmd, "Path to a JSON definition; flags override its values")

	flags.String("identity", "", "Unique identifier for the new load test (required)")
	flags.String("name", "", "Display name (defaults to the identity)")
	flags.String("description", "", "Description of what the test exercises")
	flags.String("template-id", "", "Identifier of the template to import (required)")
	flags.String("template-revision", "", "Revision of the template to pin to (required)")
	flags.String("hub-id", "", "Identifier of the hub holding the template (required)")
	flags.String("template-org-id", "", "Organization of the template, when it lives outside this one (empty means account scope)")
	flags.String("template-project-id", "", "Project of the template, when it lives outside this one (empty means the organization)")
	flags.String("import-type", "", "REFERENCE to stay pinned to the template, or LOCAL to copy it (default REFERENCE)")
	flags.String("infra-id", "", "Identifier of the Kubernetes infrastructure to run on (required)")
	flags.String("environment-id", "", "Identifier of the environment the infrastructure belongs to (required)")
	flags.String("target-type", "", "Infrastructure family: kubernetes or linux (default kubernetes)")
	flags.StringSlice("service-ref", nil, "Chaos service to link; required when risk services are enabled")
	flags.StringArray("var", nil, "Override a template variable as NAME=VALUE; repeat for several")

	// Tunable overrides are legal only on a LOCAL import.
	registerToolConfigFlags(cmd)
}

// applyFromTemplateOverrides patches the whole template block into the request:
// the service replaces toolConfig wholesale, so a lone flag would drop the rest.
func applyFromTemplateOverrides(cmd *cobra.Command, sess *session, request *api.CreateFromTemplateRequest) error {
	set := changedFlags(cmd)
	if !touchesToolConfig(set) {
		return nil
	}

	if request.ImportType != api.ImportLocal {
		return fmt.Errorf("%s cannot be overridden on a REFERENCE import: the configuration belongs to template %q and is re-read from it. Pass --import-type LOCAL to take an independent copy, or author the value as \"<+input>\" in the template and set it per run with \"hc rt loadtest start --runtime-value\"",
			joinFlags(setToolConfigFlags(set)), request.TemplateReference.Identity)
	}
	// The template is read back at the session scope, so one kept elsewhere is
	// out of reach.
	scope := resolveScope()
	if request.TemplateReference.OrganizationIdentifier != scope.OrgID ||
		request.TemplateReference.ProjectIdentifier != scope.ProjectID {
		return fmt.Errorf("%s cannot be applied to a template outside the current scope: drop --template-org-id and --template-project-id to import a template published here, or create the load test and adjust it afterwards with \"hc rt loadtest update\"",
			joinFlags(setToolConfigFlags(set)))
	}

	template, err := sess.client.GetTemplateRevision(cmd.Context(), request.TemplateReference.HubIdentity,
		request.TemplateReference.Identity, request.TemplateReference.Revision)
	if err != nil {
		return err
	}

	if request.ToolConfig == nil {
		request.ToolConfig = template.ToolConfig
	}
	if request.ToolConfig == nil {
		request.ToolConfig = map[string]any{}
	}

	writer, err := newToolWriter(request.ToolConfig, template.ToolType)
	if err != nil {
		return err
	}
	if set["mode"] {
		mode, _ := cmd.Flags().GetString("mode")
		if err := writer.setMode(api.Mode(mode)); err != nil {
			return err
		}
	}
	return applyToolConfigFlags(cmd, writer, set)
}

// setToolConfigFlags lists the tunable flags actually passed, so an error can
// name what the caller typed.
func setToolConfigFlags(set map[string]bool) []string {
	var used []string
	for _, flag := range toolConfigFlags {
		if set[flag] {
			used = append(used, "--"+flag)
		}
	}
	return used
}

func joinFlags(flags []string) string {
	if len(flags) == 1 {
		return flags[0]
	}
	return strings.Join(flags, ", ")
}

func buildFromTemplateRequest(cmd *cobra.Command) (*api.CreateFromTemplateRequest, error) {
	request := &api.CreateFromTemplateRequest{}

	configPath, _ := cmd.Flags().GetString(FlagConfig)
	if configPath != "" {
		if err := LoadConfigFile(configPath, request); err != nil {
			return nil, err
		}
	}

	set := changedFlags(cmd)
	str := func(name string) string { v, _ := cmd.Flags().GetString(name); return v }

	applyIf(set, "identity", func() { request.Identity = str("identity") })
	applyIf(set, "name", func() { request.Name = str("name") })
	applyIf(set, "description", func() { request.Description = str("description") })
	applyIf(set, "infra-id", func() { request.InfraIdentifier = str("infra-id") })
	applyIf(set, "environment-id", func() { request.EnvironmentIdentifier = str("environment-id") })
	applyIf(set, "target-type", func() { request.TargetType = str("target-type") })
	applyIf(set, "import-type", func() { request.ImportType = api.ImportType(strings.ToUpper(str("import-type"))) })
	applyIf(set, "service-ref", func() { request.ServiceReferences, _ = cmd.Flags().GetStringSlice("service-ref") })
	applyIf(set, "template-id", func() { request.TemplateReference.Identity = str("template-id") })
	applyIf(set, "template-revision", func() { request.TemplateReference.Revision = str("template-revision") })
	applyIf(set, "hub-id", func() { request.TemplateReference.HubIdentity = str("hub-id") })
	applyIf(set, "template-org-id", func() { request.TemplateReference.OrganizationIdentifier = str("template-org-id") })
	applyIf(set, "template-project-id", func() { request.TemplateReference.ProjectIdentifier = str("template-project-id") })

	defaultTemplateScope(request, set)

	if request.Identity == "" {
		return nil, fmt.Errorf("--identity is required")
	}
	if request.Name == "" {
		request.Name = request.Identity
	}
	if request.TemplateReference.Identity == "" {
		return nil, fmt.Errorf("--template-id is required: list the available templates with \"hc rt loadtest template list\"")
	}
	if request.TemplateReference.Revision == "" {
		return nil, fmt.Errorf("--template-revision is required: list the revisions of template %q with \"hc rt loadtest template list-revisions %s\"",
			request.TemplateReference.Identity, request.TemplateReference.Identity)
	}
	if request.TemplateReference.HubIdentity == "" {
		return nil, fmt.Errorf("--hub-id is required: name the hub holding template %q", request.TemplateReference.Identity)
	}
	if request.InfraIdentifier == "" {
		return nil, fmt.Errorf("--infra-id is required; a template cannot supply it")
	}
	if request.EnvironmentIdentifier == "" {
		return nil, fmt.Errorf("--environment-id is required; a template cannot supply it")
	}
	if request.ImportType == "" {
		request.ImportType = api.ImportReference
	}
	if request.ImportType != api.ImportReference && request.ImportType != api.ImportLocal {
		return nil, fmt.Errorf("unsupported --import-type %q: supported values are REFERENCE, LOCAL", request.ImportType)
	}
	if request.TargetType == "" {
		request.TargetType = string(api.TargetKubernetes)
	}

	if set["var"] {
		pairs, _ := cmd.Flags().GetStringArray("var")
		overrides, err := parseVariables(pairs)
		if err != nil {
			return nil, err
		}
		request.Variables = mergeVariables(request.Variables, overrides)
	}

	return request, nil
}

// defaultTemplateScope aims the reference at the current scope; left empty it
// resolves to the account, so a template published here reads as missing.
func defaultTemplateScope(request *api.CreateFromTemplateRequest, set map[string]bool) {
	if set["template-org-id"] || set["template-project-id"] {
		return
	}
	if request.TemplateReference.OrganizationIdentifier != "" || request.TemplateReference.ProjectIdentifier != "" {
		return
	}

	scope := resolveScope()
	request.TemplateReference.OrganizationIdentifier = scope.OrgID
	request.TemplateReference.ProjectIdentifier = scope.ProjectID
}

func parseVariables(pairs []string) ([]api.Variable, error) {
	parsed := make([]api.Variable, 0, len(pairs))
	for _, pair := range pairs {
		name, value, found := strings.Cut(pair, "=")
		if !found || strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("invalid --var %q: expected NAME=VALUE", pair)
		}
		typed := typedValue(value)
		parsed = append(parsed, api.Variable{
			Name:  strings.TrimSpace(name),
			Value: typed,
			Type:  variableType(typed),
		})
	}
	return parsed, nil
}

// variableType infers what the service requires but a flag cannot state. To
// declare a numeric-looking value as text, give "type" in a --config file.
func variableType(value any) string {
	switch value.(type) {
	case int:
		return "Integer"
	case float64:
		return "Number"
	case bool:
		return "Boolean"
	default:
		return "String"
	}
}
