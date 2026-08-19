package command

import (
	"fmt"

	"github.com/harness/harness-cli/cmd/rt/loadtest/api"

	"github.com/spf13/cobra"
)

func NewCreateFromJSONCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create-from-json",
		Short: "Create a load test from a declarative endpoint spec",
		Long: `Create a load test by declaring the endpoints to exercise, instead of
writing a script.

The spec names a base host and a list of requests, each with optional headers,
a body, query parameters and assertions. The server compiles it into a
runnable test, so no local tooling is needed. An endpoint may also extract a
value from its response into a variable that later endpoints reference, which
is how a login token is carried through a multi-step flow.

Only Locust supports this authoring mode. Use "hc rt loadtest create" for a test
defined by a script or a container image.

Pass the spec with --spec as a file holding the jsonSpec object, or put it
under a "jsonSpec" key in the --config file along with the rest of the
definition.`,
		Example: `  # Spec in its own file, everything else on the command line
  hc rt loadtest create-from-json --identity api-journey --name "API journey" \
    --spec ./journey.json --tool-type Locust \
    --infra-id perf-cluster --environment-id staging \
    --target-users 100 --duration 600

  # Whole definition in one file
  hc rt loadtest create-from-json --config ./api-journey.json`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if skeleton, _ := cmd.Flags().GetBool(flagGenerateSkeleton); skeleton {
				return writeSkeleton(cmd, api.JSONSpecSkeleton())
			}

			request, err := buildCreateFromJSONRequest(cmd)
			if err != nil {
				return err
			}

			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			created, err := sess.client.CreateFromJSON(cmd.Context(), *request)
			if err != nil {
				return err
			}

			return sess.printer.Print(created, loadTestTable([]*api.LoadTest{created}))
		},
	}

	flags := cmd.Flags()

	registerConfigFlag(cmd, "Path to a JSON load test definition; flags override its values")

	flags.String("spec", "", "Path to a JSON file holding the endpoint spec")
	flags.String("identity", "", "Unique identifier for the load test (required)")
	flags.String("name", "", "Display name (defaults to the identity)")
	flags.String("description", "", "Description of what the test exercises")
	flags.String("tool-type", string(api.ToolLocust), "Load testing tool; only Locust supports a JSON spec")
	flags.String("infra-id", "", "Identifier of the Kubernetes infrastructure to run on (required)")
	flags.String("environment-id", "", "Identifier of the environment the infrastructure belongs to (required)")
	flags.String("target-type", "", "Infrastructure family: kubernetes or linux (default kubernetes)")
	flags.StringSlice("tag", nil, "Tag to attach; repeat for several")
	flags.StringSlice("service-ref", nil, "Chaos service to link; required when risk services are enabled")

	flags.String("target-url", "", "URL the test drives load against")
	flags.String("target-users", "", "Peak concurrent users (required)")
	flags.String("ramp-up", "", "Seconds spent ramping up to the target user count")
	flags.String("duration", "", "Seconds to hold load for (required)")
	flags.String("worker-count", "", "Injector count; above one runs a distributed topology")
	flags.String("max-duration", "", "Hard cap in seconds on any run of this test")
	flags.String("spawn-rate", "", "Users started per second")

	flags.StringArray("env", nil, "Environment variable as KEY=VALUE; repeat for several")
	flags.StringArray("env-secret", nil, "Environment variable sourced from a secret, as KEY=SECRET_REF; repeat for several")
	flags.StringArray("var", nil, "Test variable as NAME=VALUE; repeat for several")
	flags.StringArray("set", nil, "Set any tool config leaf as PATH=VALUE; repeat for several")
	flags.StringArray("runtime-input", nil, "Mark a tool config leaf as supplied at run time; repeat for several")

	registerSkeletonFlag(cmd)

	return cmd
}

func buildCreateFromJSONRequest(cmd *cobra.Command) (*api.CreateFromJSONRequest, error) {
	request := &api.CreateFromJSONRequest{}

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
	applyIf(set, "tool-type", func() { request.ToolType = api.ToolType(str("tool-type")) })
	applyIf(set, "infra-id", func() { request.InfraIdentifier = str("infra-id") })
	applyIf(set, "environment-id", func() { request.EnvironmentIdentifier = str("environment-id") })
	applyIf(set, "target-type", func() { request.TargetType = str("target-type") })
	applyIf(set, "tag", func() { request.Tags, _ = cmd.Flags().GetStringSlice("tag") })
	applyIf(set, "service-ref", func() { request.ServiceReferences, _ = cmd.Flags().GetStringSlice("service-ref") })

	if set["spec"] {
		spec, err := loadJSONSpec(str("spec"))
		if err != nil {
			return nil, err
		}
		request.JSONSpec = *spec
	}

	if request.Identity == "" {
		return nil, fmt.Errorf("--identity is required")
	}
	if request.Name == "" {
		request.Name = request.Identity
	}
	if request.ToolType == "" {
		request.ToolType = api.ToolLocust
	}
	if request.ToolType != api.ToolLocust {
		return nil, fmt.Errorf("a JSON spec is only supported for Locust, not %s: use \"hc rt loadtest create\" with --script or --image", request.ToolType)
	}
	if request.InfraIdentifier == "" {
		return nil, fmt.Errorf("--infra-id is required; list available infrastructures in the Harness UI")
	}
	if request.EnvironmentIdentifier == "" {
		return nil, fmt.Errorf("--environment-id is required")
	}
	if request.TargetType == "" {
		request.TargetType = string(api.TargetKubernetes)
	}
	if len(request.JSONSpec.Endpoints) == 0 && request.JSONSpec.Config.Host == "" {
		return nil, fmt.Errorf("--spec is required: pass a JSON file declaring config.host and the endpoints to exercise")
	}
	if err := request.JSONSpec.Validate(); err != nil {
		return nil, fmt.Errorf("invalid endpoint spec: %w", err)
	}

	if request.ToolConfig == nil {
		request.ToolConfig = map[string]any{}
	}
	writer, err := newToolWriter(request.ToolConfig, request.ToolType)
	if err != nil {
		return nil, err
	}
	if err := applyToolConfigFlags(cmd, writer, set); err != nil {
		return nil, err
	}

	// The API requires both here, so name the flag to pass rather than let the
	// request come back rejected.
	for _, required := range []struct{ path, flag string }{
		{"tunables.targetUsers", "target-users"},
		{"tunables.durationSeconds", "duration"},
	} {
		if _, found := writer.get(required.path); !found {
			return nil, fmt.Errorf("--%s is required for a JSON spec load test", required.flag)
		}
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

func NewUpdateJSONSpecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-json-spec <identity>",
		Short: "Replace the endpoint spec of a JSON-defined load test",
		Long: `Upload a new endpoint spec for a load test created with
"hc rt loadtest create-from-json", creating a new script revision.

The previous spec is kept as a revision, so a bad upload can be inspected with
"hc rt loadtest script list-revisions" and recovered from. Runs already in flight
keep the spec they started with.

This only applies to tests defined by a JSON spec. For a test defined by a
script file, use "hc rt loadtest script update".`,
		Example: `  hc rt loadtest update-json-spec api-journey --spec ./journey-v2.json \
    --description "Add the checkout step"`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, _ := cmd.Flags().GetString("spec")
			if path == "" {
				return fmt.Errorf("--spec is required: pass the path to the new endpoint spec")
			}

			spec, err := loadJSONSpec(path)
			if err != nil {
				return err
			}

			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			description, _ := cmd.Flags().GetString("description")
			revision, err := sess.client.UpdateJSONSpec(cmd.Context(), args[0], api.UpdateJSONSpecRequest{
				JSONSpec:    *spec,
				Description: description,
			})
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.ErrOrStderr(), "Uploaded %s as revision %d.\n", path, revision.RevisionNumber)
			return sess.printer.Print(revision, revisionTable([]*api.ScriptRevision{revision}))
		},
	}

	cmd.Flags().String("spec", "", "Path to a JSON file holding the endpoint spec (required)")
	cmd.Flags().String("description", "", "What changed in this revision")

	return cmd
}

// loadJSONSpec accepts either the bare spec object or a whole definition
// carrying it under "jsonSpec", so a create file can be handed back to --spec.
func loadJSONSpec(path string) (*api.JSONSpec, error) {
	spec := &api.JSONSpec{}
	if err := LoadConfigSection(path, "jsonSpec", spec); err != nil {
		return nil, err
	}

	if err := spec.Validate(); err != nil {
		return nil, fmt.Errorf("invalid endpoint spec %q: %w", path, err)
	}

	return spec, nil
}
