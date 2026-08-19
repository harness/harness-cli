package command

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/harness/harness-cli/cmd/rt/loadtest/api"
)

// escapedAngleBracket is what Go's JSON encoder writes for "<" unless escaping
// is turned off. Built rather than written out so this file does not itself
// contain the sequence it is asserting the absence of.
var escapedAngleBracket = `\` + "u003c"

// created drives "create" against a stub and returns the body it sent, since
// what reaches the wire is the whole job of this command.
func created(t *testing.T, args ...string) api.CreateRequest {
	t.Helper()

	stub := serveLTM(t, map[call]any{
		POST("/load-tests"): aLoadTest("checkout-load"),
	})

	expectSuccess(t, NewCreateCmd(), args...)

	var body api.CreateRequest
	stub.only().decode(t, &body)
	return body
}

// scriptMode is the flag set every script-mode create needs, so a test states
// only what it is varying.
func scriptMode(t *testing.T, tool, extension string, extra ...string) []string {
	t.Helper()
	return append([]string{
		"--identity", "checkout-load", "--name", "Checkout load",
		"--tool-type", tool,
		"--script", scriptFile(t, "checkout"+extension, "the script body"),
		"--infra-id", "perf-cluster", "--environment-id", "staging",
	}, extra...)
}

func TestCreateUploadsTheScriptAndFillsInTheDefaults(t *testing.T) {
	body := created(t, scriptMode(t, "K6", ".js",
		"--target-users", "200", "--duration", "900", "--ramp-up", "60")...)

	if body.Identity != "checkout-load" || body.Name != "Checkout load" {
		t.Errorf("identity/name = %q/%q", body.Identity, body.Name)
	}
	if body.TargetType != string(api.TargetKubernetes) {
		t.Errorf("targetType = %q, want kubernetes by default", body.TargetType)
	}

	k6 := toolBlock(t, body.ToolConfig, "k6")
	script := nested(t, k6, "script")

	decoded, err := base64.StdEncoding.DecodeString(script["content"].(string))
	if err != nil {
		t.Fatalf("the script was not uploaded as base64: %v", err)
	}
	if string(decoded) != "the script body" {
		t.Errorf("uploaded %q, want the file contents", decoded)
	}
	// source records which artifact shape the block carries, so the runner
	// does not have to infer it from which fields are populated.
	if script["source"] != string(api.SourceInline) {
		t.Errorf("source = %v, want inline for a script", script["source"])
	}
}

func TestCreateFallsBackToTheIdentityForTheName(t *testing.T) {
	body := created(t,
		"--identity", "checkout-load", "--tool-type", "K6",
		"--script", scriptFile(t, "checkout.js", "body"),
		"--infra-id", "perf-cluster", "--environment-id", "staging")

	if body.Name != "checkout-load" {
		t.Errorf("name = %q, want it to fall back to the identity", body.Name)
	}
}

// Naming an image is enough to mean image mode; making the user say both would
// be a second chance to get them out of step.
func TestCreateInfersImageModeFromTheImageFlag(t *testing.T) {
	body := created(t,
		"--identity", "api-smoke", "--tool-type", "K6",
		"--image", "ghcr.io/acme/k6-suite:1.4.0",
		"--entrypoint", "/bin/run.sh", "--run-arg", "--vus=50",
		"--infra-id", "perf-cluster", "--environment-id", "staging")

	script := nested(t, toolBlock(t, body.ToolConfig, "k6"), "script")

	if script["image"] != "ghcr.io/acme/k6-suite:1.4.0" {
		t.Errorf("image = %v, want the one passed", script["image"])
	}
	if script["source"] != string(api.SourceImage) {
		t.Errorf("source = %v, want image", script["source"])
	}
}

func TestCreateInsistsOnWhatTheServiceRequires(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		wants []string
	}{
		{
			name: "no identity",
			args: []string{"--tool-type", "K6", "--infra-id", "perf-cluster",
				"--environment-id", "staging"},
			wants: []string{"--identity is required"},
		},
		{
			name: "no tool type",
			args: []string{"--identity", "checkout-load", "--infra-id", "perf-cluster",
				"--environment-id", "staging"},
			wants: []string{"--tool-type is required", "JMeter", "Locust", "K6"},
		},
		{
			name: "no infrastructure",
			args: []string{"--identity", "checkout-load", "--tool-type", "K6",
				"--environment-id", "staging"},
			wants: []string{"--infra-id is required"},
		},
		{
			name: "no environment",
			args: []string{"--identity", "checkout-load", "--tool-type", "K6",
				"--infra-id", "perf-cluster"},
			wants: []string{"--environment-id is required"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			serveLTM(t, map[call]any{})
			mustContain(t, "missing flag error", expectFailure(t, NewCreateCmd(), tc.args...), tc.wants...)
		})
	}
}

// Script mode with no script is the mistake this catches. Naming the extensions
// the tool takes saves a second round trip to the documentation.
func TestCreateRefusesScriptModeWithNoScript(t *testing.T) {
	for tool, extensions := range map[string]string{
		"JMeter": ".jmx or .zip",
		"Locust": ".py",
		"K6":     ".js",
	} {
		t.Run(tool, func(t *testing.T) {
			serveLTM(t, map[call]any{})

			message := expectFailure(t, NewCreateCmd(),
				"--identity", "checkout-load", "--tool-type", tool,
				"--infra-id", "perf-cluster", "--environment-id", "staging")

			mustContain(t, "missing script error", message, "--script is required", extensions)
		})
	}
}

func TestCreateRefusesImageModeWithNoImage(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewCreateCmd(),
		"--identity", "checkout-load", "--tool-type", "K6", "--mode", "image",
		"--infra-id", "perf-cluster", "--environment-id", "staging")

	mustContain(t, "missing image error", message, "--image is required")
}

// A zip holds several plans and the runner has to be told which one to run.
func TestCreateInsistsOnATestPlanForAZipWorkspace(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewCreateCmd(),
		"--identity", "checkout-load", "--tool-type", "JMeter",
		"--script", scriptFile(t, "workspace.zip", "PK\x03\x04"),
		"--infra-id", "perf-cluster", "--environment-id", "staging")

	mustContain(t, "missing test plan error", message, "--test-plan is required", ".zip")
}

func TestCreateAcceptsAZipWorkspaceWithItsMainPlan(t *testing.T) {
	body := created(t,
		"--identity", "checkout-load", "--tool-type", "JMeter",
		"--script", scriptFile(t, "workspace.zip", "PK\x03\x04"),
		"--test-plan", "checkout.jmx",
		"--infra-id", "perf-cluster", "--environment-id", "staging")

	script := nested(t, toolBlock(t, body.ToolConfig, "jmeter"), "script")
	if script["testPlan"] != "checkout.jmx" {
		t.Errorf("testPlan = %v, want the named plan", script["testPlan"])
	}
}

func TestCreateReportsAScriptItCannotRead(t *testing.T) {
	serveLTM(t, map[call]any{})

	missing := filepath.Join(t.TempDir(), "absent.js")
	message := expectFailure(t, NewCreateCmd(),
		"--identity", "checkout-load", "--tool-type", "K6", "--script", missing,
		"--infra-id", "perf-cluster", "--environment-id", "staging")

	mustContain(t, "unreadable script error", message, "reading script", "absent.js")
}

// An empty file uploads as an empty script and the run fails with nothing to
// explain it, so it is caught here instead.
func TestCreateRefusesAnEmptyScript(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewCreateCmd(),
		"--identity", "checkout-load", "--tool-type", "K6",
		"--script", scriptFile(t, "empty.js", ""),
		"--infra-id", "perf-cluster", "--environment-id", "staging")

	mustContain(t, "empty script error", message, "is empty")
}

func TestCreateRejectsAToolItDoesNotSupport(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewCreateCmd(),
		"--identity", "checkout-load", "--tool-type", "Gatling",
		"--script", scriptFile(t, "checkout.scala", "body"),
		"--infra-id", "perf-cluster", "--environment-id", "staging")

	mustContain(t, "unsupported tool error", message, "Gatling")
}

// K6 has no linux runner. Saying so names the flag; the service's rejection
// would not.
func TestCreateRefusesK6OnLinuxInfrastructure(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewCreateCmd(), scriptMode(t, "K6", ".js",
		"--target-type", "linux")...)

	mustContain(t, "k6 on linux error", message, "K6", "kubernetes", "linux")
}

func TestCreateRejectsACleanupPolicyThatIsNotOne(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewCreateCmd(), scriptMode(t, "K6", ".js",
		"--cleanup-policy", "keep")...)

	mustContain(t, "cleanup policy error", message, "keep", "delete", "retain")
}

func TestCreateCarriesResourcesTagsAndServiceReferences(t *testing.T) {
	body := created(t, scriptMode(t, "K6", ".js",
		"--tag", "nightly,eu", "--service-ref", "checkout-api",
		"--cpu-request", "500m", "--cpu-limit", "2",
		"--memory-request", "512Mi", "--memory-limit", "2Gi")...)

	if len(body.Tags) != 2 {
		t.Errorf("tags = %v, want both", body.Tags)
	}
	if len(body.ServiceReferences) != 1 || body.ServiceReferences[0] != "checkout-api" {
		t.Errorf("serviceReferences = %v, want the chaos service", body.ServiceReferences)
	}
	if body.Resources == nil {
		t.Fatal("the resource flags produced no resources block")
	}
	if body.Resources.Requests["cpu"] != "500m" || body.Resources.Limits["memory"] != "2Gi" {
		t.Errorf("resources = %+v, want requests and limits kept apart", body.Resources)
	}
}

func TestCreateDeclaresVariablesWithTheTypeItInfers(t *testing.T) {
	body := created(t, scriptMode(t, "K6", ".js",
		"--var", "REGION=eu-west-1", "--var", "TIMEOUT=30", "--var", "DEBUG=true")...)

	byName := map[string]api.Variable{}
	for _, variable := range body.Variables {
		byName[variable.Name] = variable
	}

	for name, want := range map[string]string{
		"REGION": "String", "TIMEOUT": "Integer", "DEBUG": "Boolean",
	} {
		if got := byName[name].Type; got != want {
			t.Errorf("%s type = %q, want %q inferred from the value", name, got, want)
		}
	}
}

func TestCreateRejectsAVariableWithoutAValue(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewCreateCmd(), scriptMode(t, "K6", ".js", "--var", "REGION")...)

	mustContain(t, "variable error", message, "REGION", "NAME=VALUE")
}

// A tunable left as the sentinel is what makes one definition serve several
// volumes, so it has to survive to the request literally rather than be
// coerced into a number.
func TestCreateKeepsARuntimeInputSentinelLiteral(t *testing.T) {
	body := created(t, scriptMode(t, "K6", ".js", "--target-users", "<+input>")...)

	tunables := nested(t, toolBlock(t, body.ToolConfig, "k6"), "tunables")
	if tunables["targetUsers"] != "<+input>" {
		t.Errorf("targetUsers = %v, want the sentinel preserved", tunables["targetUsers"])
	}
}

func TestCreateMarksALeafAsARuntimeInput(t *testing.T) {
	body := created(t, scriptMode(t, "K6", ".js",
		"--target-users", "200", "--runtime-input", "tunables.durationSeconds")...)

	tunables := nested(t, toolBlock(t, body.ToolConfig, "k6"), "tunables")
	if tunables["durationSeconds"] != "<+input>" {
		t.Errorf("durationSeconds = %v, want it marked as a runtime input", tunables["durationSeconds"])
	}
}

// --set reaches leaves that have no flag of their own, so a new field in the
// schema is usable before the CLI has caught up with it.
func TestCreateSetsAnArbitraryLeaf(t *testing.T) {
	body := created(t, scriptMode(t, "K6", ".js", "--set", "tunables.targetUsers=500")...)

	tunables := nested(t, toolBlock(t, body.ToolConfig, "k6"), "tunables")
	if jsonNumber(tunables["targetUsers"]) != 500 {
		t.Errorf("targetUsers = %v, want the value --set gave", tunables["targetUsers"])
	}
}

// A config file is the base and flags win, so one file serves several
// environments without being edited between them.
// Every tool-config flag has a leaf it is supposed to write, and a flag that
// parses but lands nowhere is indistinguishable from one that works. This
// drives the whole k6 surface in one create and reads each leaf back.
func TestCreateCarriesEveryK6ToolConfigFlagToItsLeaf(t *testing.T) {
	body := created(t, scriptMode(t, "K6", ".js",
		"--description", "Checkout path under peak load",
		"--env", "REGION=us-east-1", "--env", "TIER=premium",
		"--env-secret", "API_TOKEN=account.checkoutToken",
		"--metric-tag", "service=checkout", "--metric-tag", "build=4821",
		"--threshold", "metric=http_req_duration,stat=p95,operator=<,value=500",
		"--threshold", "metric=http_req_failed,operator=<,value=0.01,abortOnFail=true",
		"--k6-user-agent", "hc-loadtest/1.0",
		"--k6-batch-per-host", "12",
		"--k6-no-connection-reuse",
		"--rps-limit", "300",
		"--set", "options.dnsStrategy=round-robin",
		"--runtime-input", "tunables.targetUsers")...)

	if body.Description != "Checkout path under peak load" {
		t.Errorf("description = %q, want the one passed", body.Description)
	}

	k6 := toolBlock(t, body.ToolConfig, "k6")

	// envVars is a list of {key, value}, and a secret is marked rather than
	// inlined so the value is resolved at dispatch.
	envByKey := map[string]map[string]any{}
	for _, entry := range k6["envVars"].([]any) {
		env := entry.(map[string]any)
		envByKey[env["key"].(string)] = env
	}
	if got := envByKey["REGION"]["value"]; got != "us-east-1" {
		t.Errorf("REGION = %v", got)
	}
	if got := envByKey["API_TOKEN"]["secret"]; got != true {
		t.Errorf("API_TOKEN is not marked secret: %v", envByKey["API_TOKEN"])
	}
	if _, marked := envByKey["TIER"]["secret"]; marked {
		t.Errorf("a plain --env was marked secret: %v", envByKey["TIER"])
	}

	tags := nested(t, k6, "metricTags")
	if tags["service"] != "checkout" || tags["build"] != "4821" {
		t.Errorf("metricTags = %v, want both", tags)
	}

	thresholds := k6["thresholds"].([]any)
	if len(thresholds) != 2 {
		t.Fatalf("got %d thresholds, want 2: %v", len(thresholds), thresholds)
	}
	if got := thresholds[1].(map[string]any)["abortOnFail"]; got != true {
		t.Errorf("abortOnFail = %#v, want the bool", got)
	}

	options := nested(t, k6, "options")
	if options["userAgent"] != "hc-loadtest/1.0" {
		t.Errorf("userAgent = %v", options["userAgent"])
	}
	if jsonNumber(options["batchPerHost"]) != 12 {
		t.Errorf("batchPerHost = %#v, want the number 12", options["batchPerHost"])
	}
	if options["noConnectionReuse"] != true {
		t.Errorf("noConnectionReuse = %#v, want the bool", options["noConnectionReuse"])
	}
	// --set is applied after the dedicated flags, so it can reach a leaf that
	// has none.
	if options["dnsStrategy"] != "round-robin" {
		t.Errorf("dnsStrategy = %v, want the --set value", options["dnsStrategy"])
	}

	tunables := nested(t, k6, "tunables")
	if jsonNumber(tunables["rpsLimit"]) != 300 {
		t.Errorf("rpsLimit = %#v", tunables["rpsLimit"])
	}
	if tunables["targetUsers"] != api.RuntimeInput {
		t.Errorf("targetUsers = %#v, want it left for run time", tunables["targetUsers"])
	}
}

// --property is JMeter's, and the container flags only mean anything in image
// mode, so they need a create of their own rather than a k6 script.
func TestCreateCarriesTheJMeterAndContainerFlags(t *testing.T) {
	body := created(t,
		"--identity", "checkout-load", "--tool-type", "JMeter",
		"--infra-id", "perf-cluster", "--environment-id", "staging",
		"--mode", "image",
		"--image", "ghcr.io/example/jmeter-suite:1.0.0",
		"--entrypoint", "/bin/run-suite.sh",
		"--image-pull-secret", "ghcr-credentials",
		"--run-arg", "--loglevel=INFO", "--run-arg", "--nongui",
		"--property", "threads=200", "--property", "rampUp=60")

	jmeter := toolBlock(t, body.ToolConfig, "jmeter")
	script := nested(t, jmeter, "script")

	if script["image"] != "ghcr.io/example/jmeter-suite:1.0.0" {
		t.Errorf("image = %v", script["image"])
	}
	if script["entrypoint"] != "/bin/run-suite.sh" {
		t.Errorf("entrypoint = %v", script["entrypoint"])
	}
	if script["imagePullSecret"] != "ghcr-credentials" {
		t.Errorf("imagePullSecret = %v", script["imagePullSecret"])
	}
	if got := script["runArgs"].([]any); len(got) != 2 || got[0] != "--loglevel=INFO" {
		t.Errorf("runArgs = %v, want both in order", got)
	}
	if script["source"] != string(api.SourceImage) {
		t.Errorf("source = %v, want image", script["source"])
	}

	properties := jmeter["properties"].([]any)
	if len(properties) != 2 {
		t.Fatalf("got %d properties, want 2: %v", len(properties), properties)
	}
	// A property value is typed, since JMeter reads a thread count as a number.
	if got := properties[0].(map[string]any)["value"]; jsonNumber(got) != 200 {
		t.Errorf("threads = %#v, want the number 200", got)
	}
}

// A flag naming the wrong engine's setting has to be refused rather than
// written into a block the runner will ignore.
func TestCreateRefusesAFlagBelongingToAnotherEngine(t *testing.T) {
	cases := map[string][]string{
		"--metric-tag":    {"--metric-tag", "service=checkout"},
		"--property":      {"--property", "threads=200"},
		"--k6-user-agent": {"--k6-user-agent", "hc/1.0"},
		"--spawn-rate":    {"--spawn-rate", "2.5"},
	}

	for flag, extra := range cases {
		t.Run(flag, func(t *testing.T) {
			stub := serveLTM(t, map[call]any{})

			// Locust accepts none of these: metric tags and the k6 options are
			// K6's, properties are JMeter's, and spawn-rate is Locust's own,
			// so it is aimed at JMeter instead.
			tool := "Locust"
			extension := ".py"
			if flag == "--spawn-rate" {
				tool, extension = "JMeter", ".jmx"
			}

			message := expectFailure(t, NewCreateCmd(), scriptMode(t, tool, extension, extra...)...)

			mustContain(t, "wrong engine", message, flag)
			if len(stub.requests()) != 0 {
				t.Errorf("made %v, want the create abandoned before it was sent", summarise(stub.requests()))
			}
		})
	}
}

// A config file that states the mode settles it, so an edit that does not
// mention --mode does not silently fall back to script.
func TestCreateKeepsTheModeTheConfigFileAlreadyStates(t *testing.T) {
	config := configFile(t, map[string]any{
		"toolConfig": map[string]any{
			"k6": map[string]any{
				"mode":   "image",
				"script": map[string]any{"image": "ghcr.io/example/k6-suite:1.0.0"},
			},
		},
	})

	body := created(t, "--config", config,
		"--identity", "checkout-load", "--tool-type", "K6",
		"--infra-id", "perf-cluster", "--environment-id", "staging")

	k6 := toolBlock(t, body.ToolConfig, "k6")
	if k6["mode"] != "image" {
		t.Errorf("mode = %v, want the config file's image mode kept", k6["mode"])
	}
	if nested(t, k6, "script")["source"] != string(api.SourceImage) {
		t.Errorf("source does not match the mode: %v", k6["script"])
	}
}

// The message names the extensions the engine accepts, so it has to have an
// answer for each one rather than a shared default.
func TestScriptExtensionsAreNamedPerEngine(t *testing.T) {
	for tool, want := range map[api.ToolType]string{
		api.ToolJMeter: ".jmx",
		api.ToolLocust: ".py",
		api.ToolK6:     ".js",
	} {
		if got := scriptExtensions(tool); !strings.Contains(got, want) {
			t.Errorf("%s accepts %q, which does not mention %q", tool, got, want)
		}
	}

	// newToolWriter rejects an unknown engine before this is reached, so the
	// fallback only has to be harmless rather than specific.
	if got := scriptExtensions("Gatling"); got == "" {
		t.Error("an unknown engine left the message with a blank where the extension goes")
	}
}

func TestCreateLetsFlagsOverrideTheConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "create.json")
	contents, err := json.Marshal(map[string]any{
		"identity":              "checkout-load",
		"name":                  "From the file",
		"toolType":              "K6",
		"infraIdentifier":       "file-cluster",
		"environmentIdentifier": "staging",
		"toolConfig": map[string]any{
			"k6": map[string]any{
				"script":   map[string]any{"content": "ZXhwb3J0IGRlZmF1bHQ=", "source": "inline"},
				"tunables": map[string]any{"targetUsers": 100, "durationSeconds": 600},
			},
		},
	})
	if err != nil {
		t.Fatalf("building the config file: %v", err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("writing the config file: %v", err)
	}

	body := created(t, "--config", path, "--infra-id", "prod-cluster", "--target-users", "500")

	if body.InfraIdentifier != "prod-cluster" {
		t.Errorf("infraIdentifier = %q, want the flag to win", body.InfraIdentifier)
	}
	if body.Name != "From the file" {
		t.Errorf("name = %q, want the file value where no flag was passed", body.Name)
	}

	tunables := nested(t, toolBlock(t, body.ToolConfig, "k6"), "tunables")
	if jsonNumber(tunables["targetUsers"]) != 500 {
		t.Errorf("targetUsers = %v, want the flag to win", tunables["targetUsers"])
	}
	if jsonNumber(tunables["durationSeconds"]) != 600 {
		t.Errorf("durationSeconds = %v, want the file value kept", tunables["durationSeconds"])
	}
}

// A tool config is opaque, so a file can put a scalar where the flags expect a
// block. Writing through it has to be refused and the path named, rather than
// the flag being dropped or the leaf replacing what was there.
func TestCreateReportsAConfigFileThatBlocksTheFlagsFromWriting(t *testing.T) {
	stub := serveLTM(t, map[call]any{})

	path := configFile(t, map[string]any{
		"identity":              "checkout-load",
		"toolType":              "K6",
		"infraIdentifier":       "perf-cluster",
		"environmentIdentifier": "staging",
		"toolConfig": map[string]any{
			"k6": map[string]any{"script": "export default function () {}"},
		},
	})

	message := expectFailure(t, NewCreateCmd(), "--config", path,
		"--mode", "image", "--image", "ghcr.io/acme/k6:1")

	mustContain(t, "blocked path error", message, "script")
	if got := len(stub.requests()); got != 0 {
		t.Errorf("a load test was created from a config the flags could not patch: %v", summarise(stub.requests()))
	}
}

func TestCreateReportsAConfigFileThatIsNotThere(t *testing.T) {
	serveLTM(t, map[call]any{})

	missing := filepath.Join(t.TempDir(), "absent.json")
	message := expectFailure(t, NewCreateCmd(), "--config", missing)

	mustContain(t, "missing config error", message, "absent.json")
}

// A skeleton needs no credentials and makes no call, so it works before an
// account is set up at all.
func TestCreateEmitsASkeletonForEveryToolAndMode(t *testing.T) {
	for _, tool := range []string{"JMeter", "Locust", "K6"} {
		for _, mode := range []string{"script", "image"} {
			t.Run(tool+"/"+mode, func(t *testing.T) {
				serveLTM(t, map[call]any{})

				out := expectSuccess(t, NewCreateCmd(),
					"--generate-config-skeleton", "--tool-type", tool, "--mode", mode)

				var skeleton map[string]any
				if err := json.Unmarshal([]byte(out.stdout), &skeleton); err != nil {
					t.Fatalf("the skeleton is not JSON: %v\n%s", err, out.stdout)
				}
				if skeleton["toolType"] != tool {
					t.Errorf("toolType = %v, want %s", skeleton["toolType"], tool)
				}
			})
		}
	}
}

// Go's JSON encoder escapes angle brackets by default, which would render the
// placeholder as an escape sequence. Still valid JSON, but nothing a user would
// recognise as the thing they were told to fill in.
func TestTheSkeletonKeepsTheInputPlaceholderReadable(t *testing.T) {
	serveLTM(t, map[call]any{})

	out := expectSuccess(t, NewCreateCmd(),
		"--generate-config-skeleton", "--tool-type", "K6", "--mode", "script")

	mustContain(t, "skeleton", out.stdout, "<+input>", "<+variable.baseUrl>")
	mustNotContain(t, "skeleton", out.stdout, escapedAngleBracket)
}

func TestTheSkeletonNeedsAToolToBeFor(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewCreateCmd(), "--generate-config-skeleton")

	mustContain(t, "skeleton error", message, "--tool-type is required")
}

func TestTheSkeletonRejectsAToolThatIsNotSupported(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewCreateCmd(),
		"--generate-config-skeleton", "--tool-type", "Gatling")

	mustContain(t, "skeleton error", message, "unsupported --tool-type", "Gatling")
}

func TestCreateSurfacesARejectionFromTheService(t *testing.T) {
	serveLTM(t, map[call]any{
		POST("/load-tests"): http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"message":"serviceReferences is required when risk services are enabled"}`, http.StatusBadRequest)
		}),
	})

	message := expectFailure(t, NewCreateCmd(), scriptMode(t, "K6", ".js")...)

	mustContain(t, "service rejection", message, "risk services")
}

// toolBlock reads the per-tool block out of a request that has been through
// JSON, where every nested object is a map[string]any.
func toolBlock(t *testing.T, config map[string]any, key string) map[string]any {
	t.Helper()
	block, ok := config[key].(map[string]any)
	if !ok {
		t.Fatalf("toolConfig has no %q block: %v", key, config)
	}
	return block
}

func nested(t *testing.T, block map[string]any, key string) map[string]any {
	t.Helper()
	inner, ok := block[key].(map[string]any)
	if !ok {
		t.Fatalf("block has no %q: %v", key, block)
	}
	return inner
}
