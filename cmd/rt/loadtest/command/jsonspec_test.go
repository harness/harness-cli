package command

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/harness/harness-cli/cmd/rt/loadtest/api"
)

// aSpec is a valid endpoint spec: a base host and one request. Tests that care
// about a particular field edit the map rather than restate the whole thing.
func aSpec() map[string]any {
	return map[string]any{
		"config": map[string]any{
			"host":     "https://api.example.com",
			"min_wait": 1,
			"max_wait": 3,
		},
		"endpoints": []any{
			map[string]any{
				"name":   "login",
				"method": "POST",
				"path":   "/v1/login",
				"body":   map[string]any{"user": "qa"},
				"extract": map[string]any{
					"variable_name": "token",
					"json_path":     "$.accessToken",
				},
			},
			map[string]any{
				"name":       "list orders",
				"method":     "GET",
				"path":       "/v1/orders",
				"headers":    map[string]any{"Authorization": "Bearer {{token}}"},
				"assertions": map[string]any{"status_code": 200, "max_response_time_ms": 800},
			},
		},
	}
}

// specFile writes a spec to disk and returns its path, since both commands
// take one as a file rather than inline.
func specFile(t *testing.T, spec any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "journey.json")
	contents, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("building the spec file: %v", err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("writing the spec file: %v", err)
	}
	return path
}

// createFromJSON is the flag set every spec test needs, so a test can say only
// what it is varying.
func createFromJSON(specPath string, extra ...string) []string {
	return append([]string{
		"--identity", "api-journey", "--name", "API journey",
		"--spec", specPath,
		"--infra-id", "perf-cluster", "--environment-id", "staging",
		"--target-users", "100", "--duration", "600",
	}, extra...)
}

func TestCreateFromJSONSendsTheWholeSpec(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		POST("/load-tests/from-json"): aLoadTest("api-journey"),
	})

	expectSuccess(t, NewCreateFromJSONCmd(), createFromJSON(specFile(t, aSpec()))...)

	var body api.CreateFromJSONRequest
	stub.only().decode(t, &body)

	if body.JSONSpec.Config.Host != "https://api.example.com" {
		t.Errorf("host = %q, want the one in the spec", body.JSONSpec.Config.Host)
	}
	if len(body.JSONSpec.Endpoints) != 2 {
		t.Fatalf("sent %d endpoints, want both", len(body.JSONSpec.Endpoints))
	}
	// The extraction is what carries a login token into later requests, so it
	// has to survive the round trip through the file.
	extract := body.JSONSpec.Endpoints[0].Extract
	if extract == nil || extract.VariableName != "token" || extract.JSONPath != "$.accessToken" {
		t.Errorf("extract = %+v, want the token extraction preserved", extract)
	}
	if body.JSONSpec.Endpoints[1].Assertions == nil {
		t.Error("the assertions were dropped")
	}
}

// Only Locust compiles a spec. Defaulting saves saying so on every invocation.
func TestCreateFromJSONDefaultsToLocust(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		POST("/load-tests/from-json"): aLoadTest("api-journey"),
	})

	expectSuccess(t, NewCreateFromJSONCmd(), createFromJSON(specFile(t, aSpec()))...)

	var body api.CreateFromJSONRequest
	stub.only().decode(t, &body)

	if body.ToolType != api.ToolLocust {
		t.Errorf("toolType = %q, want Locust", body.ToolType)
	}
	if body.TargetType != string(api.TargetKubernetes) {
		t.Errorf("targetType = %q, want kubernetes by default", body.TargetType)
	}
}

// The other tools have no compiler for a spec. Saying so here is better than
// letting the request come back rejected with a message about the wrong thing.
func TestCreateFromJSONRefusesTheToolsThatCannotCompileASpec(t *testing.T) {
	for _, tool := range []string{"K6", "JMeter"} {
		t.Run(tool, func(t *testing.T) {
			serveLTM(t, map[call]any{})

			message := expectFailure(t, NewCreateFromJSONCmd(),
				createFromJSON(specFile(t, aSpec()), "--tool-type", tool)...)

			mustContain(t, "wrong tool error", message, "only supported for Locust", tool, "--script")
		})
	}
}

func TestCreateFromJSONNamesTheTestAfterItsIdentityByDefault(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		POST("/load-tests/from-json"): aLoadTest("api-journey"),
	})

	expectSuccess(t, NewCreateFromJSONCmd(),
		"--identity", "api-journey", "--spec", specFile(t, aSpec()),
		"--infra-id", "perf-cluster", "--environment-id", "staging",
		"--target-users", "100", "--duration", "600")

	var body api.CreateFromJSONRequest
	stub.only().decode(t, &body)

	if body.Name != "api-journey" {
		t.Errorf("name = %q, want it to fall back to the identity", body.Name)
	}
}

func TestCreateFromJSONInsistsOnWhatTheServiceRequires(t *testing.T) {
	for _, tc := range []struct {
		missing string
		omit    string
		wants   []string
	}{
		{"identity", "--identity", []string{"--identity is required"}},
		{"infra", "--infra-id", []string{"--infra-id is required"}},
		{"environment", "--environment-id", []string{"--environment-id is required"}},
		{"target users", "--target-users", []string{"--target-users is required"}},
		{"duration", "--duration", []string{"--duration is required"}},
	} {
		t.Run(tc.missing, func(t *testing.T) {
			serveLTM(t, map[call]any{})

			full := createFromJSON(specFile(t, aSpec()))
			args := make([]string, 0, len(full))
			for i := 0; i < len(full); i++ {
				if full[i] == tc.omit {
					i++ // skip its value too
					continue
				}
				args = append(args, full[i])
			}

			mustContain(t, "missing flag error", expectFailure(t, NewCreateFromJSONCmd(), args...), tc.wants...)
		})
	}
}

func TestCreateFromJSONInsistsOnASpec(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewCreateFromJSONCmd(),
		"--identity", "api-journey",
		"--infra-id", "perf-cluster", "--environment-id", "staging",
		"--target-users", "100", "--duration", "600")

	mustContain(t, "missing spec error", message, "--spec is required", "config.host")
}

// A malformed spec is worth catching locally: the message can name the
// offending endpoint, which a rejection from the service would not.
func TestCreateFromJSONValidatesTheSpecBeforeSending(t *testing.T) {
	for _, tc := range []struct {
		name  string
		spec  func(map[string]any)
		wants []string
	}{
		{
			name:  "no host",
			spec:  func(s map[string]any) { s["config"] = map[string]any{} },
			wants: []string{"config.host is required"},
		},
		{
			name:  "no endpoints",
			spec:  func(s map[string]any) { s["endpoints"] = []any{} },
			wants: []string{"endpoints is required"},
		},
		{
			name: "waits the wrong way round",
			spec: func(s map[string]any) {
				s["config"] = map[string]any{"host": "https://api.example.com", "min_wait": 9, "max_wait": 2}
			},
			wants: []string{"min_wait", "max_wait"},
		},
		{
			name: "endpoint without a name",
			spec: func(s map[string]any) {
				s["endpoints"] = []any{map[string]any{"method": "GET", "path": "/health"}}
			},
			wants: []string{"endpoints[0]", "name is required"},
		},
		{
			name: "two endpoints with the same name",
			spec: func(s map[string]any) {
				s["endpoints"] = []any{
					map[string]any{"name": "health", "method": "GET", "path": "/health"},
					map[string]any{"name": "health", "method": "GET", "path": "/healthz"},
				}
			},
			wants: []string{"endpoints[1]", "duplicate name"},
		},
		{
			name: "endpoint without a path",
			spec: func(s map[string]any) {
				s["endpoints"] = []any{map[string]any{"name": "health", "method": "GET"}}
			},
			wants: []string{"path is required"},
		},
		{
			name: "endpoint without a method",
			spec: func(s map[string]any) {
				s["endpoints"] = []any{map[string]any{"name": "health", "path": "/health"}}
			},
			wants: []string{"method is required", "GET"},
		},
		{
			name: "a method that is not one",
			spec: func(s map[string]any) {
				s["endpoints"] = []any{map[string]any{"name": "health", "method": "FETCH", "path": "/health"}}
			},
			wants: []string{"unsupported method", "FETCH"},
		},
		{
			name: "half an extraction",
			spec: func(s map[string]any) {
				s["endpoints"] = []any{map[string]any{
					"name": "login", "method": "POST", "path": "/login",
					"extract": map[string]any{"variable_name": "token"},
				}}
			},
			wants: []string{"extract needs both", "json_path"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			serveLTM(t, map[call]any{})

			spec := aSpec()
			tc.spec(spec)

			message := expectFailure(t, NewCreateFromJSONCmd(), createFromJSON(specFile(t, spec))...)
			mustContain(t, "spec validation error", message, tc.wants...)
		})
	}
}

// A spec passed with --spec is validated as it is read, so this is the only
// route by which an invalid one reaches the request: stated inside a --config
// file, where nothing has looked at it yet.
func TestCreateFromJSONValidatesASpecThatCameFromAConfigFile(t *testing.T) {
	stub := serveLTM(t, map[call]any{})

	spec := aSpec()
	spec["endpoints"] = []any{map[string]any{"name": "health", "method": "FETCH", "path": "/health"}}

	path := configFile(t, map[string]any{
		"identity":              "api-journey",
		"infraIdentifier":       "perf-cluster",
		"environmentIdentifier": "staging",
		"jsonSpec":              spec,
	})

	message := expectFailure(t, NewCreateFromJSONCmd(), "--config", path,
		"--target-users", "100", "--duration", "600")

	mustContain(t, "spec validation error", message, "invalid endpoint spec", "FETCH")
	if got := len(stub.requests()); got != 0 {
		t.Errorf("an invalid spec was sent anyway: %v", summarise(stub.requests()))
	}
}

// The optional metadata is read flag by flag, so a reader left behind by a
// rename would drop the field silently rather than fail.
func TestCreateFromJSONCarriesEveryOptionalFlagIntoTheRequest(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		POST("/load-tests/from-json"): aLoadTest("api-journey"),
	})

	expectSuccess(t, NewCreateFromJSONCmd(), createFromJSON(specFile(t, aSpec()),
		"--description", "Drives the public API journey",
		"--target-type", "kubernetes",
		"--tag", "team:perf", "--tag", "tier:gold",
		"--service-ref", "checkout", "--service-ref", "orders",
		"--var", "REGION=eu-west-1",
		"--env", "BASE_URL=https://api.example.com")...)

	var body api.CreateFromJSONRequest
	stub.only().decode(t, &body)

	if body.Description != "Drives the public API journey" {
		t.Errorf("description = %q, want the flag's text", body.Description)
	}
	if body.TargetType != "kubernetes" {
		t.Errorf("targetType = %q, want kubernetes", body.TargetType)
	}
	if got := strings.Join(body.Tags, ","); got != "team:perf,tier:gold" {
		t.Errorf("tags = %q, want both of them", got)
	}
	if got := strings.Join(body.ServiceReferences, ","); got != "checkout,orders" {
		t.Errorf("serviceRefs = %q, want both of them", got)
	}
	if len(body.Variables) != 1 || body.Variables[0].Name != "REGION" {
		t.Errorf("variables = %v, want REGION", body.Variables)
	}
	// A spec test still writes into the Locust block, so the tunable flags have
	// to land there alongside the compiled spec.
	vars, _ := dig(body.ToolConfig, "locust", "envVars").([]any)
	if len(vars) != 1 {
		t.Fatalf("the environment variable never reached the tool config: %v", body.ToolConfig)
	}
	entry, _ := vars[0].(map[string]any)
	if entry["key"] != "BASE_URL" || entry["value"] != "https://api.example.com" {
		t.Errorf("envVars[0] = %v, want BASE_URL set from the flag", entry)
	}
}

func TestCreateFromJSONReportsAMalformedVariable(t *testing.T) {
	stub := serveLTM(t, map[call]any{})

	message := expectFailure(t, NewCreateFromJSONCmd(),
		createFromJSON(specFile(t, aSpec()), "--var", "REGION")...)

	mustContain(t, "variable error", message, "REGION", "NAME=VALUE")
	if got := len(stub.requests()); got != 0 {
		t.Errorf("a load test was created despite the bad variable: %v", summarise(stub.requests()))
	}
}

func TestCreateFromJSONReportsAConfigFileThatIsNotThere(t *testing.T) {
	serveLTM(t, map[call]any{})

	missing := filepath.Join(t.TempDir(), "absent-config.json")
	message := expectFailure(t, NewCreateFromJSONCmd(), "--config", missing)

	mustContain(t, "missing config file error", message, "absent-config.json")
}

func TestCreateFromJSONReportsASpecFileThatIsNotThere(t *testing.T) {
	serveLTM(t, map[call]any{})

	missing := filepath.Join(t.TempDir(), "absent.json")
	message := expectFailure(t, NewCreateFromJSONCmd(), createFromJSON(missing)...)

	mustContain(t, "missing spec file error", message, "absent.json")
}

// A whole definition in one file has to work as well as a spec on its own,
// since that is what the committed examples use.
func TestCreateFromJSONTakesTheWholeDefinitionFromAConfigFile(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		POST("/load-tests/from-json"): aLoadTest("api-journey"),
	})

	path := filepath.Join(t.TempDir(), "definition.json")
	contents, err := json.Marshal(map[string]any{
		"identity":              "api-journey",
		"name":                  "API journey",
		"toolType":              "Locust",
		"infraIdentifier":       "perf-cluster",
		"environmentIdentifier": "staging",
		"jsonSpec":              aSpec(),
		"toolConfig": map[string]any{
			"locust": map[string]any{"tunables": map[string]any{"targetUsers": 100, "durationSeconds": 600}},
		},
	})
	if err != nil {
		t.Fatalf("building the definition: %v", err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("writing the definition: %v", err)
	}

	expectSuccess(t, NewCreateFromJSONCmd(), "--config", path)

	var body api.CreateFromJSONRequest
	stub.only().decode(t, &body)

	if body.Identity != "api-journey" || len(body.JSONSpec.Endpoints) != 2 {
		t.Errorf("the definition did not survive the file: %+v", body)
	}
}

func TestCreateFromJSONEmitsASkeletonWithoutCallingTheService(t *testing.T) {
	serveLTM(t, map[call]any{})

	out := expectSuccess(t, NewCreateFromJSONCmd(), "--generate-config-skeleton")

	var skeleton map[string]any
	if err := json.Unmarshal([]byte(out.stdout), &skeleton); err != nil {
		t.Fatalf("the skeleton is not JSON: %v\n%s", err, out.stdout)
	}
	if _, found := skeleton["jsonSpec"]; !found {
		t.Errorf("the skeleton has no jsonSpec to fill in: %v", skeleton)
	}
}

func TestUpdateJSONSpecUploadsANewSpecAsARevision(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		PUT("/load-tests/api-journey/json-script"): aScriptRevision(2, "compiled"),
	})

	out := expectSuccess(t, NewUpdateJSONSpecCmd(), "api-journey",
		"--spec", specFile(t, aSpec()), "--description", "Add the checkout step")

	var body api.UpdateJSONSpecRequest
	stub.only().decode(t, &body)

	if body.JSONSpec.Config.Host != "https://api.example.com" {
		t.Errorf("host = %q, want the new spec", body.JSONSpec.Config.Host)
	}
	if body.Description != "Add the checkout step" {
		t.Errorf("description = %q, want the one passed", body.Description)
	}
	mustContain(t, "upload confirmation", out.stderr, "revision 2")
}

func TestUpdateJSONSpecInsistsOnASpec(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewUpdateJSONSpecCmd(), "api-journey")

	mustContain(t, "missing spec error", message, "--spec is required")
}

// The spec is validated before the upload, so a bad one does not become a
// revision that then has to be rolled back.
func TestUpdateJSONSpecValidatesBeforeUploading(t *testing.T) {
	stub := serveLTM(t, map[call]any{})

	broken := aSpec()
	broken["endpoints"] = []any{map[string]any{"name": "health", "method": "FETCH", "path": "/health"}}

	message := expectFailure(t, NewUpdateJSONSpecCmd(), "api-journey", "--spec", specFile(t, broken))

	mustContain(t, "spec validation error", message, "unsupported method")
	if len(stub.requests()) != 0 {
		t.Errorf("made %v, want nothing sent for an invalid spec", summarise(stub.requests()))
	}
}

// The create file carries the spec under "jsonSpec"; --spec also takes the bare
// object. Accepting both means a definition can be handed straight back.
func TestUpdateJSONSpecAcceptsAWholeDefinitionAsWellAsABareSpec(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		PUT("/load-tests/api-journey/json-script"): aScriptRevision(2, "compiled"),
	})

	wrapped := specFile(t, map[string]any{
		"identity": "api-journey",
		"jsonSpec": aSpec(),
	})

	expectSuccess(t, NewUpdateJSONSpecCmd(), "api-journey", "--spec", wrapped)

	var body api.UpdateJSONSpecRequest
	stub.only().decode(t, &body)

	if body.JSONSpec.Config.Host != "https://api.example.com" {
		t.Errorf("host = %q, want the spec unwrapped from the definition", body.JSONSpec.Config.Host)
	}
}
