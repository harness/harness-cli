package command

import (
	"fmt"
	"os"

	"github.com/harness/harness-cli/cmd/rt/loadtest/api"

	"github.com/spf13/cobra"
)

// NewTemplateCmd groups the operations on load test templates. Two hubs may
// hold different templates under one identity, so --hub-id is on every command.
func NewTemplateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "template",
		Short: "Publish and manage reusable load test templates",
		Long: `Manage the reusable load test definitions published into a hub.

A template is versioned: it exists as a series of immutable revisions, and
"the template" without a revision means its latest. Adopt one with
"hc rt loadtest create-from-template".

Every command here takes --hub-id, since two hubs may hold different templates
under the same identity. Only "list" treats it as optional, where omitting it
spans every hub; everywhere else the hub is matched exactly, so leaving it off
looks for a template filed under no hub at all.`,
	}

	cmd.AddCommand(newTemplateCreateCmd())
	cmd.AddCommand(newTemplateListCmd())
	cmd.AddCommand(newTemplateGetCmd())
	cmd.AddCommand(newTemplateUpdateCmd())
	cmd.AddCommand(newTemplateDeleteCmd())
	cmd.AddCommand(newTemplateVariablesCmd())
	cmd.AddCommand(newTemplateExportYAMLCmd())
	cmd.AddCommand(newTemplateListRevisionsCmd())
	cmd.AddCommand(newTemplateGetRevisionCmd())
	cmd.AddCommand(newTemplateCreateRevisionCmd())
	cmd.AddCommand(newTemplateDeleteRevisionCmd())

	return cmd
}

// registerHubFlag declares the hub selector shared by every template command.
func registerHubFlag(cmd *cobra.Command) {
	cmd.Flags().String("hub-id", "", "Identifier of the hub holding the template")
}

func hubID(cmd *cobra.Command) string {
	value, _ := cmd.Flags().GetString("hub-id")
	return value
}

func newTemplateCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Publish a load test template",
		Long: `Publish a reusable load test definition into a hub.

A template is versioned, so --revision is required: this call creates the
template and its first revision together. Publish later versions with
"hc rt loadtest template create-revision".

The definition is authored exactly as a load test is, from flags or from a
--config file, with the same tunable and threshold flags. A template is
infrastructure-agnostic, though: --infra-id and --environment-id are optional
here and only supply defaults, because a load test created from the template
must always name concrete values of its own.

Any tunable may be left as "<+input>" for whoever creates a load test from the
template to fill in. "hc rt loadtest template variables" lists what a given
revision still needs.`,
		Example: `  # Publish a K6 template from a script
  hc rt loadtest template create --identity standard-http --revision v1 \
    --name "Standard HTTP" --tool-type K6 --hub-id harness-hub \
    --script ./standard.js --target-users 100 --duration 600

  # Leave the load level to whoever adopts the template
  hc rt loadtest template create --identity spike --revision v1 \
    --tool-type Locust --hub-id harness-hub --script ./spike.py \
    --target-users '<+input>' --duration '<+input>'

  # From a container image, with a pass/fail threshold baked in
  hc rt loadtest template create --identity api-suite --revision v1 \
    --tool-type K6 --hub-id harness-hub --mode image \
    --image ghcr.io/acme/k6-suite:1.4.0 \
    --threshold "metric=http_req_duration,stat=p95,operator=<,value=500"`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if skeleton, _ := cmd.Flags().GetBool(flagGenerateSkeleton); skeleton {
				toolType, _ := cmd.Flags().GetString("tool-type")
				mode, _ := cmd.Flags().GetString("mode")
				return printSkeleton(cmd, toolType, mode)
			}

			request, err := buildCreateTemplateRequest(cmd)
			if err != nil {
				return err
			}

			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			created, err := sess.client.CreateTemplate(cmd.Context(), hubID(cmd), *request)
			if err != nil {
				return err
			}

			return sess.printer.Print(created, templateTable([]*api.Template{created}))
		},
	}

	flags := cmd.Flags()

	registerConfigFlag(cmd, "Path to a JSON template definition; flags override its values")
	registerHubFlag(cmd)

	flags.String("identity", "", "Unique identifier for the template (required)")
	flags.String("revision", "", "Revision to publish, for example v1 (required)")
	flags.String("name", "", "Display name (defaults to the identity)")
	flags.String("description", "", "Description of what the template exercises")
	flags.String("tool-type", "", "Load testing tool: JMeter, Locust or K6 (required)")
	flags.String("infra-type", "", "Infrastructure family the template targets: kubernetes or linux")
	flags.StringSlice("tag", nil, "Tag to attach; repeat for several")
	flags.String("infra-id", "", "Default infrastructure identifier; a load test may still override it")
	flags.String("environment-id", "", "Default environment identifier; a load test may still override it")

	registerToolConfigFlags(cmd)

	flags.String("script", "", "Path to the local script file to upload (script mode)")
	flags.String("test-plan", "", "Name of the main .jmx inside a JMeter .zip workspace")
	flags.StringArray("var", nil, "Template variable as NAME=VALUE, referenced from config as <+variable.NAME>; repeat for several")

	registerSkeletonFlag(cmd)

	return cmd
}

func buildCreateTemplateRequest(cmd *cobra.Command) (*api.CreateTemplateRequest, error) {
	request := &api.CreateTemplateRequest{}

	configPath, _ := cmd.Flags().GetString(FlagConfig)
	if configPath != "" {
		if err := LoadConfigFile(configPath, request); err != nil {
			return nil, err
		}
	}

	set := changedFlags(cmd)
	str := func(name string) string { v, _ := cmd.Flags().GetString(name); return v }

	applyIf(set, "identity", func() { request.Identity = str("identity") })
	applyIf(set, "revision", func() { request.Revision = str("revision") })
	applyIf(set, "name", func() { request.Name = str("name") })
	applyIf(set, "description", func() { request.Description = str("description") })
	applyIf(set, "tool-type", func() { request.ToolType = api.ToolType(str("tool-type")) })
	applyIf(set, "infra-type", func() { request.InfraType = str("infra-type") })
	applyIf(set, "infra-id", func() { request.InfraID = str("infra-id") })
	applyIf(set, "environment-id", func() { request.EnvID = str("environment-id") })
	applyIf(set, "tag", func() { request.Tags, _ = cmd.Flags().GetStringSlice("tag") })

	if request.Identity == "" {
		return nil, fmt.Errorf("--identity is required")
	}
	if request.Revision == "" {
		return nil, fmt.Errorf("--revision is required: a template is published as a named revision, for example v1")
	}
	if request.Name == "" {
		request.Name = request.Identity
	}
	if request.ToolType == "" {
		return nil, fmt.Errorf("--tool-type is required: supported values are %s", joinTools(api.SupportedToolTypes))
	}

	if request.ToolConfig == nil {
		request.ToolConfig = map[string]any{}
	}
	writer, err := newToolWriter(request.ToolConfig, request.ToolType)
	if err != nil {
		return nil, err
	}
	mode, err := applyArtifactFlags(cmd, writer, set)
	if err != nil {
		return nil, err
	}
	if err := applyToolConfigFlags(cmd, writer, set); err != nil {
		return nil, err
	}
	scriptPath, _ := cmd.Flags().GetString("script")
	if err := validateArtifact(writer, mode, scriptPath); err != nil {
		return nil, err
	}

	if set["var"] {
		pairs, _ := cmd.Flags().GetStringArray("var")
		parsed, err := parseVariables(pairs)
		if err != nil {
			return nil, err
		}
		request.Variables = mergeVariables(request.Variables, parsed)
	}

	return request, nil
}

func newTemplateListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List load test templates",
		Long: `List the templates visible in the current scope.

Only the latest revision of each template is listed. Use
"hc rt loadtest template list-revisions" to see the versions of one.

Omitting --hub-id spans every hub in the scope; the HUB column names the one
each template came from. This is the only template command that treats the hub
as optional — the rest match it exactly.`,
		Example: `  hc rt loadtest template list --hub-id harness-hub

  # Only K6 templates, newest first
  hc rt loadtest template list --hub-id harness-hub --tool-type K6 --sort-field updatedAt`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			flags := cmd.Flags()
			page, _ := flags.GetInt64("page")
			limit, _ := flags.GetInt64("limit")
			search, _ := flags.GetString("search")
			sortField, _ := flags.GetString("sort-field")
			sortAscending, _ := flags.GetBool("sort-ascending")
			toolType, _ := flags.GetString("tool-type")
			infraType, _ := flags.GetString("infra-type")

			result, err := sess.client.ListTemplates(cmd.Context(), api.TemplateListOptions{
				ListOptions: api.ListOptions{
					Page:          page,
					Limit:         limit,
					Search:        search,
					SortField:     sortField,
					SortAscending: sortAscending,
					ToolType:      toolType,
				},
				HubIdentity: hubID(cmd),
				InfraType:   infraType,
			})
			if err != nil {
				return err
			}

			return sess.printer.Print(result, templateTable(result.Items))
		},
	}

	flags := cmd.Flags()

	registerHubFlag(cmd)
	flags.Int64("page", 0, "Zero-based page number")
	flags.Int64("limit", 0, "Results per page, up to 100 (default 15)")
	flags.String("search", "", "Filter by a substring of the name")
	flags.String("sort-field", "", "Field to sort by: name, createdAt or updatedAt")
	flags.Bool("sort-ascending", false, "Sort ascending instead of descending")
	flags.String("tool-type", "", "Filter by tool: JMeter, Locust or K6")
	flags.String("infra-type", "", "Filter by infrastructure family: kubernetes or linux")

	return cmd
}

func newTemplateGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <identity>",
		Short: "Show the latest revision of a template",
		Long: `Show a template, resolved to its latest revision.

The table view is a summary. Use --format json or --format yaml to see the
tool configuration in full, including any "<+input>" leaves a load test
created from it will have to supply.

To read a specific version, use "hc rt loadtest template get-revision".`,
		Example: `  hc rt loadtest template get standard-http --hub-id harness-hub
  hc rt loadtest template get standard-http --hub-id harness-hub --format yaml`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			template, err := sess.client.GetTemplate(cmd.Context(), hubID(cmd), args[0])
			if err != nil {
				return err
			}

			return sess.printer.Print(template, templateTable([]*api.Template{template}))
		},
	}

	registerHubFlag(cmd)

	return cmd
}

func newTemplateUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <identity>",
		Short: "Edit the latest revision of a template",
		Long: `Change the metadata of a template's latest revision, in place.

This edits the existing revision rather than publishing a new one, so anything
already pinned to it sees the change. To version a change instead, use
"hc rt loadtest template create-revision".

The script cannot be changed here; it belongs to a revision. Only the fields
you pass are changed.`,
		Example: `  hc rt loadtest template update standard-http --hub-id harness-hub \
    --description "Standard HTTP journey, updated for the new checkout"

  # Raise the default load the template suggests
  hc rt loadtest template update standard-http --hub-id harness-hub --target-users 250`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			identity := args[0]

			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			// The stored config is the base. Sending only the changed leaf
			// would drop the rest of the configuration, including the script.
			current, err := sess.client.GetTemplate(cmd.Context(), hubID(cmd), identity)
			if err != nil {
				return err
			}

			request, err := buildUpdateTemplateRequest(cmd, current)
			if err != nil {
				return err
			}

			updated, err := sess.client.UpdateTemplate(cmd.Context(), hubID(cmd), identity, *request)
			if err != nil {
				return err
			}

			return sess.printer.Print(updated, templateTable([]*api.Template{updated}))
		},
	}

	flags := cmd.Flags()

	registerConfigFlag(cmd, "Path to a JSON definition of the changes; flags override its values")
	registerHubFlag(cmd)

	flags.String("name", "", "New display name")
	flags.String("description", "", "New description")
	flags.String("infra-type", "", "Infrastructure family the template targets: kubernetes or linux")
	flags.StringSlice("tag", nil, "Replace the tags with this set")
	flags.String("infra-id", "", "Default infrastructure identifier")
	flags.String("environment-id", "", "Default environment identifier")

	registerToolConfigFlags(cmd)

	flags.StringArray("var", nil, "Set a template variable as NAME=VALUE; repeat for several")

	return cmd
}

func buildUpdateTemplateRequest(cmd *cobra.Command, current *api.Template) (*api.UpdateTemplateRequest, error) {
	request := &api.UpdateTemplateRequest{}

	configPath, _ := cmd.Flags().GetString(FlagConfig)
	if configPath != "" {
		if err := LoadConfigFile(configPath, request); err != nil {
			return nil, err
		}
	}

	set := changedFlags(cmd)
	str := func(name string) string { v, _ := cmd.Flags().GetString(name); return v }

	applyIf(set, "name", func() { request.Name = str("name") })
	applyIf(set, "description", func() { request.Description = str("description") })
	applyIf(set, "infra-type", func() { request.InfraType = str("infra-type") })
	applyIf(set, "infra-id", func() { request.InfraID = str("infra-id") })
	applyIf(set, "environment-id", func() { request.EnvID = str("environment-id") })
	applyIf(set, "tag", func() { request.Tags, _ = cmd.Flags().GetStringSlice("tag") })

	if set["var"] {
		pairs, _ := cmd.Flags().GetStringArray("var")
		parsed, err := parseVariables(pairs)
		if err != nil {
			return nil, err
		}
		request.Variables = mergeVariables(request.Variables, parsed)
	}

	if touchesToolConfig(set) || request.ToolConfig != nil {
		if request.ToolConfig == nil {
			request.ToolConfig = current.ToolConfig
		}
		if request.ToolConfig == nil {
			request.ToolConfig = map[string]any{}
		}
		writer, err := newToolWriter(request.ToolConfig, current.ToolType)
		if err != nil {
			return nil, err
		}
		if set["mode"] {
			if err := writer.setMode(api.Mode(str("mode"))); err != nil {
				return nil, err
			}
		}
		if err := applyToolConfigFlags(cmd, writer, set); err != nil {
			return nil, err
		}
	}

	return request, nil
}

func newTemplateDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <identity>",
		Short: "Delete a template and all of its revisions",
		Long: `Delete a template, including every revision of it.

This cannot be undone. Load tests imported from the template as a REFERENCE
resolve their definition from it on every read, so deleting a template they
depend on will break them; a LOCAL import holds its own copy and is
unaffected.

To remove one version and keep the rest, use
"hc rt loadtest template delete-revision".`,
		Example: `  hc rt loadtest template delete standard-http --hub-id harness-hub
  hc rt loadtest template delete standard-http --hub-id harness-hub --force`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			identity := args[0]

			force, _ := cmd.Flags().GetBool("force")
			if !force {
				confirmed, err := confirm(cmd, fmt.Sprintf("Delete template %q and every revision of it?", identity))
				if err != nil {
					return err
				}
				if !confirmed {
					fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
					return nil
				}
			}

			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			if err := sess.client.DeleteTemplate(cmd.Context(), hubID(cmd), identity); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Deleted template %q.\n", identity)
			return nil
		},
	}

	registerHubFlag(cmd)
	cmd.Flags().Bool("force", false, "Delete without asking for confirmation")

	return cmd
}

func newTemplateListRevisionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-revisions <identity>",
		Short: "List the revisions of a template",
		Long: `List every published revision of a template, newest first.

A revision name from this list is what "hc rt loadtest create-from-template"
takes as --template-revision.`,
		Example:      `  hc rt loadtest template list-revisions standard-http --hub-id harness-hub`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			revisions, err := sess.client.ListTemplateRevisions(cmd.Context(), hubID(cmd), args[0])
			if err != nil {
				return err
			}

			return sess.printer.Print(revisions, templateRevisionTable(revisions))
		},
	}

	registerHubFlag(cmd)

	return cmd
}

func newTemplateGetRevisionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get-revision <identity> <revision>",
		Short: "Show one revision of a template",
		Long: `Show a specific revision of a template.

The revision argument is a name published with
"hc rt loadtest template create-revision" and listed by
"hc rt loadtest template list-revisions", such as v2 — not a position in the
list.`,
		Example: `  hc rt loadtest template get-revision standard-http v2 --hub-id harness-hub
  hc rt loadtest template get-revision standard-http v2 --hub-id harness-hub --format yaml`,
		Args:         cobra.ExactArgs(2),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			revision, err := sess.client.GetTemplateRevision(cmd.Context(), hubID(cmd), args[0], args[1])
			if err != nil {
				return err
			}

			return sess.printer.Print(revision, templateRevisionTable([]*api.Template{revision}))
		},
	}

	registerHubFlag(cmd)

	return cmd
}

func newTemplateCreateRevisionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create-revision <identity>",
		Short: "Publish a new revision of a template",
		Long: `Publish a new version of an existing template.

Everything you do not pass is inherited from the current latest revision, so a
revision that only raises the user count need not restate the script. Existing
revisions are immutable and anything pinned to one is unaffected.

Variables are the exception to inheritance: passing --var replaces the
inherited set rather than adding to it.`,
		Example: `  # A new revision that only changes the load profile
  hc rt loadtest template create-revision standard-http --revision v2 \
    --hub-id harness-hub --target-users 500 --ramp-up 120

  # A new revision with a new script
  hc rt loadtest template create-revision standard-http --revision v3 \
    --hub-id harness-hub --script ./standard-v3.js \
    --description "Adds the new checkout journey"`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			identity := args[0]

			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			// Fetch for the tool type, which says which toolConfig block the
			// tunable flags write into, and for the config to patch.
			current, err := sess.client.GetTemplate(cmd.Context(), hubID(cmd), identity)
			if err != nil {
				return err
			}

			request, err := buildCreateRevisionRequest(cmd, current)
			if err != nil {
				return err
			}

			created, err := sess.client.CreateTemplateRevision(cmd.Context(), hubID(cmd), identity, *request)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.ErrOrStderr(), "Published %s revision %s.\n", identity, created.Revision)
			return sess.printer.Print(created, templateRevisionTable([]*api.Template{created}))
		},
	}

	flags := cmd.Flags()

	registerConfigFlag(cmd, "Path to a JSON revision definition; flags override its values")
	registerHubFlag(cmd)

	flags.String("revision", "", "Name of the revision to publish, for example v2 (required)")
	flags.String("name", "", "Display name; inherited when omitted")
	flags.String("description", "", "Description of what changed in this revision")
	flags.StringSlice("tag", nil, "Replace the tags with this set")
	flags.String("infra-id", "", "Default infrastructure identifier")
	flags.String("environment-id", "", "Default environment identifier")

	registerToolConfigFlags(cmd)

	flags.String("script", "", "Path to a new script file for this revision")
	flags.String("test-plan", "", "Name of the main .jmx inside a JMeter .zip workspace")
	flags.StringArray("var", nil, "Replace the variables with this set, as NAME=VALUE; repeat for several")

	return cmd
}

func buildCreateRevisionRequest(cmd *cobra.Command, current *api.Template) (*api.CreateRevisionRequest, error) {
	request := &api.CreateRevisionRequest{}

	configPath, _ := cmd.Flags().GetString(FlagConfig)
	if configPath != "" {
		if err := LoadConfigFile(configPath, request); err != nil {
			return nil, err
		}
	}

	set := changedFlags(cmd)
	str := func(name string) string { v, _ := cmd.Flags().GetString(name); return v }

	applyIf(set, "revision", func() { request.Revision = str("revision") })
	applyIf(set, "name", func() { request.Name = str("name") })
	applyIf(set, "description", func() { request.Description = str("description") })
	applyIf(set, "infra-id", func() { request.InfraID = str("infra-id") })
	applyIf(set, "environment-id", func() { request.EnvID = str("environment-id") })
	applyIf(set, "tag", func() { request.Tags, _ = cmd.Flags().GetStringSlice("tag") })

	if request.Revision == "" {
		return nil, fmt.Errorf("--revision is required: name the new revision, for example v2")
	}
	if request.Revision == current.Revision {
		return nil, fmt.Errorf("revision %q is already the latest revision of %q: a revision is immutable, so choose a new name or use \"hc rt loadtest template update\" to edit it in place",
			request.Revision, current.Identity)
	}

	if set["var"] {
		pairs, _ := cmd.Flags().GetStringArray("var")
		parsed, err := parseVariables(pairs)
		if err != nil {
			return nil, err
		}
		request.Variables = mergeVariables(request.Variables, parsed)
	}

	if touchesToolConfig(set) || set["script"] || set["test-plan"] || request.ToolConfig != nil {
		if request.ToolConfig == nil {
			request.ToolConfig = current.ToolConfig
		}
		if request.ToolConfig == nil {
			request.ToolConfig = map[string]any{}
		}
		writer, err := newToolWriter(request.ToolConfig, current.ToolType)
		if err != nil {
			return nil, err
		}
		if _, err := applyArtifactFlags(cmd, writer, set); err != nil {
			return nil, err
		}
		if err := applyToolConfigFlags(cmd, writer, set); err != nil {
			return nil, err
		}
	}

	return request, nil
}

func newTemplateDeleteRevisionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete-revision <identity> <revision>",
		Short: "Delete one revision of a template",
		Long: `Delete a single revision, leaving the other revisions of the template in
place.

This cannot be undone, and a load test pinned to the deleted revision as a
REFERENCE import will no longer resolve. To remove the template entirely, use
"hc rt loadtest template delete".`,
		Example: `  hc rt loadtest template delete-revision standard-http v1 --hub-id harness-hub
  hc rt loadtest template delete-revision standard-http v1 --hub-id harness-hub --force`,
		Args:         cobra.ExactArgs(2),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			identity, revision := args[0], args[1]

			force, _ := cmd.Flags().GetBool("force")
			if !force {
				confirmed, err := confirm(cmd, fmt.Sprintf("Delete revision %q of template %q?", revision, identity))
				if err != nil {
					return err
				}
				if !confirmed {
					fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
					return nil
				}
			}

			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			if err := sess.client.DeleteTemplateRevision(cmd.Context(), hubID(cmd), identity, revision); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Deleted revision %q of template %q.\n", revision, identity)
			return nil
		},
	}

	registerHubFlag(cmd)
	cmd.Flags().Bool("force", false, "Delete without asking for confirmation")

	return cmd
}

func newTemplateExportYAMLCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export-yaml <identity>",
		Short: "Export a template as YAML",
		Long: `Print a template's canonical YAML form.

This is the representation the Harness UI shows in its YAML view, and it is
returned as-is rather than re-rendered, so it round-trips. Use --output-file
to save it.

A JMeter template built from a .zip workspace masks the archive in this view,
as a placeholder rather than its contents, so the exported YAML alone is not
enough to recreate that template.`,
		Example: `  hc rt loadtest template export-yaml standard-http --hub-id harness-hub
  hc rt loadtest template export-yaml standard-http --hub-id harness-hub --output-file ./standard-http.yaml`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			document, err := sess.client.ExportTemplateYAML(cmd.Context(), hubID(cmd), args[0])
			if err != nil {
				return err
			}

			// The body is YAML, not a JSON document, so it bypasses the printer
			// and --format; re-encoding would defeat the point of an export.
			destination, _ := cmd.Flags().GetString("output-file")
			if destination == "" {
				_, err := cmd.OutOrStdout().Write(document)
				return err
			}

			if err := os.WriteFile(destination, document, 0o600); err != nil {
				return fmt.Errorf("writing %q: %w", destination, err)
			}

			fmt.Fprintf(cmd.ErrOrStderr(), "Wrote %d bytes to %s.\n", len(document), destination)
			return nil
		},
	}

	registerHubFlag(cmd)
	cmd.Flags().String("output-file", "", "Write the YAML to this path instead of stdout")

	return cmd
}

func newTemplateVariablesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "variables <identity>",
		Short: "List what a template revision needs supplied",
		Long: `List a template revision's variables and its runtime inputs.

The two are different. A variable is a named value the template declares,
overridden with "hc rt loadtest create-from-template --var NAME=VALUE". An input
is a tool configuration leaf left as "<+input>", addressed by its path and
supplied when the load test is created or run.

Without --revision the latest revision is described. The table view lists the
inputs, since those are what block a run; use --format json to see both.`,
		Example: `  hc rt loadtest template variables standard-http --hub-id harness-hub

  # What a specific revision expects
  hc rt loadtest template variables standard-http --hub-id harness-hub --revision v2 --format json`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			revision, _ := cmd.Flags().GetString("revision")
			result, err := sess.client.GetTemplateVariables(cmd.Context(), hubID(cmd), args[0], revision)
			if err != nil {
				return err
			}

			return sess.printer.Print(result, inputTable(result.Inputs))
		},
	}

	registerHubFlag(cmd)
	cmd.Flags().String("revision", "", "Revision to describe (default the latest)")

	return cmd
}
