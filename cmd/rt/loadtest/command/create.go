package command

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/harness/harness-cli/cmd/rt/loadtest/api"

	"github.com/spf13/cobra"
)

func NewCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a load test",
		Long: `Create a load test from a script file or a container image.

The definition can come from flags, from a JSON file passed with --config, or
from both. When a field is set in each, the flag wins, so one config file can
be reused across environments and adjusted per invocation.

In script mode --script points at a local file, which is read and encoded for
you: a .py for Locust, a .js for K6, and a .jmx or a .zip workspace for JMeter.
A .zip also needs --test-plan to name the main plan inside it. In image mode
--image names a container that already contains the test.

Any tunable may be left for the caller of "hc rt loadtest start" to supply, by
passing <+input> in place of a value or by naming the leaf with
--runtime-input. A tunable may also reference a test variable declared with
--var, as <+variable.NAME>.

On an account with risk services enabled, --service-ref is required and names
a chaos service the test exercises; the create is rejected outright without
one. Copy the references off an existing test if you are unsure which apply.

The load test is only a definition. Use "hc rt loadtest start" to execute it.`,
		Example: `  # JMeter from a script, tuned inline
  hc rt loadtest create --identity checkout-load --name "Checkout load" \
    --tool-type JMeter --script ./checkout.jmx \
    --infra-id perf-cluster --environment-id staging \
    --service-ref checkout-api \
    --target-url https://staging.example.com \
    --target-users 200 --ramp-up 60 --duration 900

  # K6 from a container image, failing the run if p95 latency regresses
  hc rt loadtest create --identity api-smoke --name "API smoke" \
    --tool-type K6 --mode image --image ghcr.io/acme/k6-suite:1.4.0 \
    --infra-id perf-cluster --environment-id staging \
    --threshold "metric=http_req_duration,stat=p95,operator=<,value=500"

  # Leave the user count to be chosen at run time
  hc rt loadtest create --identity spike --tool-type Locust --script ./spike.py \
    --infra-id perf-cluster --environment-id staging \
    --target-users '<+input>'

  # Locust from a config file, overriding the target for this run
  hc rt loadtest create --config locust.json --target-url https://canary.example.com`,
		Args: cobra.NoArgs,
		// A failed API call or a bad value is not a usage error; printing the
		// whole flag list on top of the message buries it.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if skeleton, _ := cmd.Flags().GetBool(flagGenerateSkeleton); skeleton {
				toolType, _ := cmd.Flags().GetString("tool-type")
				mode, _ := cmd.Flags().GetString("mode")
				return printSkeleton(cmd, toolType, mode)
			}

			request, err := buildCreateRequest(cmd)
			if err != nil {
				return err
			}

			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			created, err := sess.client.Create(cmd.Context(), *request)
			if err != nil {
				return err
			}

			return sess.printer.Print(created, loadTestTable([]*api.LoadTest{created}))
		},
	}

	flags := cmd.Flags()

	registerConfigFlag(cmd, "Path to a JSON load test definition; flags override its values")

	flags.String("identity", "", "Unique identifier for the load test (required)")
	flags.String("name", "", "Display name (defaults to the identity)")
	flags.String("description", "", "Description of what the test exercises")
	flags.String("tool-type", "", "Load testing tool: JMeter, Locust or K6 (required)")
	flags.String("infra-id", "", "Identifier of the Kubernetes infrastructure to run on (required)")
	flags.String("environment-id", "", "Identifier of the environment the infrastructure belongs to (required)")
	flags.String("target-type", "", "Infrastructure family: kubernetes or linux (default kubernetes)")
	flags.StringSlice("tag", nil, "Tag to attach; repeat for several")
	flags.StringSlice("service-ref", nil, "Chaos service to link; required when risk services are enabled")
	flags.String("cleanup-policy", "", "Whether run resources are removed afterwards: delete or retain")

	registerToolConfigFlags(cmd)

	flags.String("script", "", "Path to the local script file to upload (script mode)")
	flags.String("test-plan", "", "Name of the main .jmx inside a JMeter .zip workspace")

	flags.StringArray("var", nil, "Test variable as NAME=VALUE, referenced from config as <+variable.NAME>; repeat for several")

	flags.String("cpu-request", "", "CPU requested per pod, for example 500m")
	flags.String("cpu-limit", "", "CPU limit per pod, for example 2")
	flags.String("memory-request", "", "Memory requested per pod, for example 512Mi")
	flags.String("memory-limit", "", "Memory limit per pod, for example 2Gi")

	registerSkeletonFlag(cmd)

	return cmd
}

// flagGenerateSkeleton prints a config template instead of calling the API,
// the counterpart of AWS CLI's --generate-cli-skeleton.
const flagGenerateSkeleton = "generate-config-skeleton"

func registerSkeletonFlag(cmd *cobra.Command) {
	cmd.Flags().Bool(flagGenerateSkeleton, false, "Print an example --config file for the chosen --tool-type and --mode, then exit")
}

// printSkeleton runs before validation and makes no network call, so a
// skeleton needs nothing but a tool name and no credentials.
func printSkeleton(cmd *cobra.Command, toolType, mode string) error {
	tool := api.ToolType(toolType)
	if tool == "" {
		return fmt.Errorf("--tool-type is required with --%s: supported values are %s", flagGenerateSkeleton, joinTools(api.SupportedToolTypes))
	}
	if api.ToolKey(tool) == "" {
		return fmt.Errorf("unsupported --tool-type %q: supported values are %s", toolType, joinTools(api.SupportedToolTypes))
	}

	return writeSkeleton(cmd, api.Skeleton(tool, api.Mode(mode)))
}

// writeSkeleton emits JSON, the only format --config reads. Escaping is off so
// a "<+input>" placeholder survives literally.
func writeSkeleton(cmd *cobra.Command, template map[string]any) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(template)
}

// registerToolConfigFlags is shared by create and update. Numeric tunables are
// declared as strings so a leaf can hold an expression; scalar types them.
func registerToolConfigFlags(cmd *cobra.Command) {
	flags := cmd.Flags()

	flags.String("mode", "", "How the test is defined: script or image (default script)")
	flags.String("image", "", "Container image containing the test (image mode)")
	flags.String("entrypoint", "", "Entrypoint to invoke inside the image")
	flags.String("image-pull-secret", "", "Name of the Kubernetes secret used to pull the image")
	flags.StringArray("run-arg", nil, "Argument appended to the image entrypoint; repeat for several")

	flags.String("target-url", "", "URL the test drives load against")
	flags.String("target-users", "", "Peak concurrent users or virtual users")
	flags.String("ramp-up", "", "Seconds spent ramping up to the target user count")
	flags.String("duration", "", "Seconds to hold load for")
	flags.String("worker-count", "", "Injector count; above one runs a distributed topology")
	flags.String("max-duration", "", "Hard cap in seconds on any run of this test")
	flags.String("spawn-rate", "", "Users started per second (Locust only)")
	flags.String("host-url", "", "Host forwarded to the script as HOST_URL (K6 only)")
	flags.String("iterations", "", "Iteration cap; --duration wins when both are set (K6 only)")
	flags.String("rps-limit", "", "Requests per second ceiling (K6 only)")

	flags.StringArray("env", nil, "Environment variable as KEY=VALUE; repeat for several")
	flags.StringArray("env-secret", nil, "Environment variable sourced from a secret, as KEY=SECRET_REF; repeat for several")
	flags.StringArray("property", nil, "JMeter runtime property as KEY=VALUE; repeat for several")
	flags.StringArray("metric-tag", nil, "K6 tag applied to every metric sample, as KEY=VALUE; repeat for several")
	flags.StringArray("threshold", nil, "Pass/fail rule as \"metric=...,stat=...,operator=...,value=...\"; repeat for several")

	flags.String("k6-user-agent", "", "User-Agent header k6 sends (K6 only)")
	flags.String("k6-dns-strategy", "", "DNS resolution strategy, for example round-robin (K6 only)")
	flags.Int("k6-batch-concurrent", 0, "Concurrent requests per batch call (K6 only)")
	flags.Int("k6-batch-per-host", 0, "Concurrent requests per host per batch call (K6 only)")
	flags.Bool("k6-no-connection-reuse", false, "Close connections between iterations (K6 only)")
	flags.Bool("k6-insecure-skip-tls-verify", false, "Skip TLS certificate verification (K6 only)")
	flags.Bool("k6-discard-response-bodies", false, "Drop response bodies to save memory (K6 only)")

	flags.StringArray("set", nil, "Set any tool config leaf as PATH=VALUE, for example tunables.targetUsers=500; repeat for several")
	flags.StringArray("runtime-input", nil, "Mark a tool config leaf as supplied at run time, for example tunables.targetUsers; repeat for several")
}

// buildCreateRequest layers the flags that were explicitly set over the
// optional config file, then validates the result before any network call.
func buildCreateRequest(cmd *cobra.Command) (*api.CreateRequest, error) {
	request := &api.CreateRequest{}

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
	applyIf(set, "cleanup-policy", func() { request.CleanupPolicy = api.CleanupPolicy(str("cleanup-policy")) })
	applyIf(set, "tag", func() { request.Tags, _ = cmd.Flags().GetStringSlice("tag") })
	applyIf(set, "service-ref", func() { request.ServiceReferences, _ = cmd.Flags().GetStringSlice("service-ref") })

	if request.Identity == "" {
		return nil, fmt.Errorf("--identity is required")
	}
	if request.Name == "" {
		request.Name = request.Identity
	}
	if request.ToolType == "" {
		return nil, fmt.Errorf("--tool-type is required: supported values are %s", joinTools(api.SupportedToolTypes))
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
	if request.ToolType == api.ToolK6 && request.TargetType != string(api.TargetKubernetes) {
		return nil, fmt.Errorf("K6 load tests only run on kubernetes infrastructure, not %q", request.TargetType)
	}
	if err := validateCleanupPolicy(request.CleanupPolicy); err != nil {
		return nil, err
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
	// Completeness is checked after every flag has landed, so --image set
	// alongside --mode image is seen regardless of the order they are applied.
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
	if err := applyResourceFlags(cmd, request, set); err != nil {
		return nil, err
	}

	return request, nil
}

// applyArtifactFlags settles the mode and the script or image. Create only: an
// existing test changes its script with "hc rt loadtest script update".
func applyArtifactFlags(cmd *cobra.Command, writer *toolWriter, set map[string]bool) (api.Mode, error) {
	str := func(name string) string { v, _ := cmd.Flags().GetString(name); return v }

	mode := api.Mode(str("mode"))
	if !set["mode"] {
		mode = defaultMode(writer, set["image"])
	}
	if err := writer.setMode(mode); err != nil {
		return "", err
	}

	if set["script"] {
		encoded, err := encodeScript(str("script"))
		if err != nil {
			return "", err
		}
		if err := writer.set("script.content", encoded); err != nil {
			return "", err
		}
	}
	if set["test-plan"] {
		if err := writer.set("script.testPlan", str("test-plan")); err != nil {
			return "", err
		}
	}

	// source records which of the two artifact shapes the block carries, so
	// the runner does not have to infer it from which fields are populated.
	source := api.SourceInline
	if mode == api.ModeImage {
		source = api.SourceImage
	}
	if err := writer.set("script.source", string(source)); err != nil {
		return "", err
	}

	return mode, nil
}

// applyToolConfigFlags writes every flag that lands inside toolConfig and is
// meaningful on both create and update.
func applyToolConfigFlags(cmd *cobra.Command, writer *toolWriter, set map[string]bool) error {
	flags := cmd.Flags()
	str := func(name string) string { v, _ := flags.GetString(name); return v }
	num := func(name string) int { v, _ := flags.GetInt(name); return v }
	yes := func(name string) bool { v, _ := flags.GetBool(name); return v }
	array := func(name string) []string { v, _ := flags.GetStringArray(name); return v }

	if set["image"] {
		if err := writer.set("script.image", str("image")); err != nil {
			return err
		}
	}
	if set["entrypoint"] {
		if err := writer.set("script.entrypoint", str("entrypoint")); err != nil {
			return err
		}
	}
	if set["image-pull-secret"] {
		if err := writer.set("script.imagePullSecret", str("image-pull-secret")); err != nil {
			return err
		}
	}
	if set["run-arg"] {
		args := array("run-arg")
		values := make([]any, len(args))
		for i, arg := range args {
			values[i] = arg
		}
		if err := writer.set("script.runArgs", values); err != nil {
			return err
		}
	}

	if err := writer.applyTunables(set, str); err != nil {
		return err
	}
	if err := writer.applyK6Options(set, str, num, yes); err != nil {
		return err
	}

	if set["env"] {
		if err := writer.applyEnvVars(array("env"), false); err != nil {
			return err
		}
	}
	if set["env-secret"] {
		if err := writer.applyEnvVars(array("env-secret"), true); err != nil {
			return err
		}
	}
	if set["property"] {
		if err := writer.applyProperties(array("property")); err != nil {
			return err
		}
	}
	if set["metric-tag"] {
		if err := writer.applyMetricTags(array("metric-tag")); err != nil {
			return err
		}
	}
	if set["threshold"] {
		if err := writer.applyThresholds(array("threshold")); err != nil {
			return err
		}
	}

	// --set and --runtime-input are applied last so an explicit path always
	// wins over the dedicated flag that writes the same leaf.
	if set["set"] {
		if err := writer.applySetFlags(array("set")); err != nil {
			return err
		}
	}
	if set["runtime-input"] {
		if err := writer.applyRuntimeInputs(array("runtime-input")); err != nil {
			return err
		}
	}

	return nil
}

func applyResourceFlags(cmd *cobra.Command, request *api.CreateRequest, set map[string]bool) error {
	fields := map[string]struct {
		list string
		key  string
	}{
		"cpu-request":    {"requests", "cpu"},
		"memory-request": {"requests", "memory"},
		"cpu-limit":      {"limits", "cpu"},
		"memory-limit":   {"limits", "memory"},
	}

	for flag, target := range fields {
		if !set[flag] {
			continue
		}
		value, _ := cmd.Flags().GetString(flag)
		if request.Resources == nil {
			request.Resources = &api.ResourceRequirements{}
		}
		if target.list == "requests" {
			if request.Resources.Requests == nil {
				request.Resources.Requests = api.ResourceList{}
			}
			request.Resources.Requests[target.key] = value
			continue
		}
		if request.Resources.Limits == nil {
			request.Resources.Limits = api.ResourceList{}
		}
		request.Resources.Limits[target.key] = value
	}

	return nil
}

// defaultMode prefers what the config file already says, then --image, then
// script mode.
func defaultMode(writer *toolWriter, imageSet bool) api.Mode {
	if existing := writer.currentMode(); existing != "" {
		return existing
	}
	if imageSet {
		return api.ModeImage
	}
	return api.ModeScript
}

// validateArtifact rejects a definition the API would reject, so the failure
// is immediate and names the flag to fix rather than arriving as a 400.
func validateArtifact(writer *toolWriter, mode api.Mode, scriptPath string) error {
	// A value may come from a flag or from the config file, so presence is
	// read back out of the block rather than from the flag.
	filled := func(path string) bool {
		value, found := writer.get(path)
		if !found {
			return false
		}
		text, ok := value.(string)
		return !ok || text != ""
	}

	switch mode {
	case api.ModeScript:
		if !filled("script.content") {
			return fmt.Errorf("--script is required in script mode; pass a %s file or use --mode image", scriptExtensions(writer.toolType))
		}
		if strings.HasSuffix(strings.ToLower(scriptPath), ".zip") && !filled("script.testPlan") {
			return fmt.Errorf("--test-plan is required with a .zip workspace: name the main plan inside the archive")
		}
	case api.ModeImage:
		if !filled("script.image") {
			return fmt.Errorf("--image is required in image mode")
		}
	}
	return nil
}

func scriptExtensions(toolType api.ToolType) string {
	switch toolType {
	case api.ToolJMeter:
		return ".jmx or .zip"
	case api.ToolLocust:
		return ".py"
	case api.ToolK6:
		return ".js"
	}
	return "script"
}

func encodeScript(path string) (string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading script %q: %w", path, err)
	}
	if len(contents) == 0 {
		return "", fmt.Errorf("script %q is empty", path)
	}
	return base64.StdEncoding.EncodeToString(contents), nil
}

func validateCleanupPolicy(policy api.CleanupPolicy) error {
	switch policy {
	case "", api.CleanupDelete, api.CleanupRetain:
		return nil
	}
	return fmt.Errorf("unsupported --cleanup-policy %q: supported values are delete, retain", policy)
}
