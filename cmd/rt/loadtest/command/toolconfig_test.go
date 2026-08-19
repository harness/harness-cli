package command

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/harness/harness-cli/cmd/rt/loadtest/api"
)

func writerFor(t *testing.T, tool api.ToolType, root map[string]any) *toolWriter {
	t.Helper()

	if root == nil {
		root = map[string]any{}
	}
	writer, err := newToolWriter(root, tool)
	if err != nil {
		t.Fatalf("newToolWriter: %v", err)
	}

	return writer
}

func TestNewToolWriterRejectsUnknownTool(t *testing.T) {
	_, err := newToolWriter(map[string]any{}, "Gatling")
	if err == nil {
		t.Fatal("expected an error for an unsupported tool")
	}
	if !strings.Contains(err.Error(), "JMeter") {
		t.Errorf("error should list the supported tools, got: %v", err)
	}
}

// Editing a config file from one engine to another leaves the old block behind,
// and a load test runs exactly one engine.
func TestNewToolWriterDropsOtherToolBlocks(t *testing.T) {
	root := map[string]any{
		"jmeter": map[string]any{"mode": "script"},
		"locust": map[string]any{"mode": "script"},
	}

	writerFor(t, api.ToolK6, root)

	if _, present := root["jmeter"]; present {
		t.Error("jmeter block survived a switch to K6")
	}
	if _, present := root["locust"]; present {
		t.Error("locust block survived a switch to K6")
	}
	if _, present := root["k6"]; !present {
		t.Error("k6 block was not created")
	}
}

func TestSetModeRejectsRetiredUIMode(t *testing.T) {
	err := writerFor(t, api.ToolK6, nil).setMode("ui")
	if err == nil {
		t.Fatal("expected ui mode to be rejected")
	}
	if !strings.Contains(err.Error(), "no longer supported") {
		t.Errorf("error should explain that ui was retired, got: %v", err)
	}
}

func TestScalarTyping(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		kind    scalarKind
		want    any
		wantErr bool
	}{
		{name: "int", raw: "200", kind: kindInt, want: 200},
		{name: "float", raw: "5.5", kind: kindFloat, want: 5.5},
		{name: "string", raw: "https://a", kind: kindString, want: "https://a"},
		{name: "runtime input at an int leaf", raw: api.RuntimeInput, kind: kindInt, want: api.RuntimeInput},
		{name: "variable at a float leaf", raw: "<+variable.rate>", kind: kindFloat, want: "<+variable.rate>"},
		{name: "words at an int leaf", raw: "twenty", kind: kindInt, wantErr: true},
		{name: "float at an int leaf", raw: "1.5", kind: kindInt, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := scalar("target-users", tc.raw, tc.kind)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %q", tc.raw)
				}
				if !strings.Contains(err.Error(), api.RuntimeInput) {
					t.Errorf("error should mention the runtime input escape hatch, got: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestApplyTunablesRejectsWrongEngine(t *testing.T) {
	cases := []struct {
		tool api.ToolType
		flag string
	}{
		{api.ToolK6, "spawn-rate"},
		{api.ToolJMeter, "spawn-rate"},
		{api.ToolJMeter, "host-url"},
		{api.ToolLocust, "iterations"},
		{api.ToolLocust, "rps-limit"},
	}

	for _, tc := range cases {
		t.Run(string(tc.tool)+"/"+tc.flag, func(t *testing.T) {
			writer := writerFor(t, tc.tool, nil)

			err := writer.applyTunables(map[string]bool{tc.flag: true}, func(string) string { return "1" })
			if err == nil {
				t.Fatalf("--%s should not be accepted for %s", tc.flag, tc.tool)
			}
			if !strings.Contains(err.Error(), string(tc.tool)) {
				t.Errorf("error should name the engine, got: %v", err)
			}
		})
	}
}

func TestApplyTunablesWritesUnderTheToolKey(t *testing.T) {
	root := map[string]any{}
	writer := writerFor(t, api.ToolLocust, root)

	values := map[string]string{
		"target-url":   "https://a",
		"target-users": "200",
		"duration":     "900",
		"spawn-rate":   "5.5",
		"max-duration": "3600",
	}
	set := map[string]bool{}
	for flag := range values {
		set[flag] = true
	}

	if err := writer.applyTunables(set, func(name string) string { return values[name] }); err != nil {
		t.Fatalf("applyTunables: %v", err)
	}

	want := map[string]any{
		"locust.tunables.targetUrl":       "https://a",
		"locust.tunables.targetUsers":     200,
		"locust.tunables.durationSeconds": 900,
		"locust.tunables.spawnRate":       5.5,
		"locust.tunables.maxDurationSec":  3600,
	}
	for path, expected := range want {
		got, found := api.GetPath(root, path)
		if !found {
			t.Errorf("%s: not written", path)
			continue
		}
		if got != expected {
			t.Errorf("%s: got %#v, want %#v", path, got, expected)
		}
	}
}

// TestEnvVarsUpsert covers the override rule: repeating a key replaces it
// rather than sending two entries for the same variable.
func TestEnvVarsUpsert(t *testing.T) {
	root := map[string]any{
		"jmeter": map[string]any{
			"envVars": []any{map[string]any{"key": "REGION", "value": "us-east-1"}},
		},
	}
	writer := writerFor(t, api.ToolJMeter, root)

	if err := writer.applyEnvVars([]string{"REGION=eu-west-1", "DEBUG=true"}, false); err != nil {
		t.Fatalf("applyEnvVars: %v", err)
	}
	if err := writer.applyEnvVars([]string{"TOKEN=account.perfToken"}, true); err != nil {
		t.Fatalf("applyEnvVars secret: %v", err)
	}

	got := writer.list("envVars")
	want := []any{
		map[string]any{"key": "REGION", "value": "eu-west-1"},
		map[string]any{"key": "DEBUG", "value": "true"},
		map[string]any{"key": "TOKEN", "value": "account.perfToken", "secret": true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %#v\nwant %#v", got, want)
	}
}

func TestEnvVarsRejectMalformedPairs(t *testing.T) {
	err := writerFor(t, api.ToolK6, nil).applyEnvVars([]string{"NOEQUALS"}, false)
	if err == nil || !strings.Contains(err.Error(), "KEY=VALUE") {
		t.Errorf("expected a KEY=VALUE hint, got: %v", err)
	}
}

func TestThresholdsKeyOnMetricAndStat(t *testing.T) {
	writer := writerFor(t, api.ToolK6, nil)

	specs := []string{
		"metric=http_req_duration,stat=p95,operator=<,value=500",
		"metric=http_req_duration,stat=p99,operator=<,value=900",
		// Same metric and stat as the first: an override, not a third rule.
		"metric=http_req_duration,stat=p95,operator=<,value=400",
	}
	if err := writer.applyThresholds(specs); err != nil {
		t.Fatalf("applyThresholds: %v", err)
	}

	got := writer.list("thresholds")
	if len(got) != 2 {
		t.Fatalf("got %d thresholds, want 2: %#v", len(got), got)
	}
	if value := got[0].(map[string]any)["value"]; value != 400 {
		t.Errorf("p95 threshold: got value %v, want it overridden to 400", value)
	}
}

func TestThresholdParsingErrors(t *testing.T) {
	cases := map[string]string{
		"metric=m,value=1":                              "operator is required",
		"metric=m,operator=~,value=1":                   "unsupported operator",
		"operator=<,value=1":                            "metric is required",
		"metric=m,operator=<":                           "value is required",
		"metric=m,operator=<,value=1,warn=yes":          "unknown field",
		"metric=m,operator=<,value=1,abortOnFail=maybe": "must be true or false",
		"garbage": "name=value pairs",
	}

	for spec, wantErr := range cases {
		t.Run(spec, func(t *testing.T) {
			_, _, err := parseThreshold(spec)
			if err == nil {
				t.Fatalf("expected an error containing %q", wantErr)
			}
			if !strings.Contains(err.Error(), wantErr) {
				t.Errorf("got %q, want it to contain %q", err, wantErr)
			}
		})
	}
}

func TestThresholdsRejectedForLocust(t *testing.T) {
	err := writerFor(t, api.ToolLocust, nil).applyThresholds([]string{"metric=m,operator=<,value=1"})
	if err == nil || !strings.Contains(err.Error(), "Locust") {
		t.Errorf("expected thresholds to be rejected for Locust, got: %v", err)
	}
}

func TestLiteralTyping(t *testing.T) {
	cases := map[string]any{
		"200":            200,
		"0.01":           0.01,
		"true":           true,
		"false":          false,
		"hello":          "hello",
		api.RuntimeInput: api.RuntimeInput,
		"<+variable.x>":  "<+variable.x>",
	}

	for raw, want := range cases {
		if got := literal(raw); got != want {
			t.Errorf("literal(%q): got %#v, want %#v", raw, got, want)
		}
	}
}

func TestApplySetFlagsWritesArbitraryPaths(t *testing.T) {
	root := map[string]any{}
	writer := writerFor(t, api.ToolK6, root)

	if err := writer.applySetFlags([]string{"options.dnsStrategy=round-robin", "tunables.targetUsers=500"}); err != nil {
		t.Fatalf("applySetFlags: %v", err)
	}

	if got, _ := api.GetPath(root, "k6.options.dnsStrategy"); got != "round-robin" {
		t.Errorf("dnsStrategy: got %#v", got)
	}
	if got, _ := api.GetPath(root, "k6.tunables.targetUsers"); got != 500 {
		t.Errorf("targetUsers: got %#v, want the number 500", got)
	}
}

func TestApplyRuntimeInputs(t *testing.T) {
	root := map[string]any{}
	writer := writerFor(t, api.ToolJMeter, root)

	if err := writer.applyRuntimeInputs([]string{"tunables.targetUsers"}); err != nil {
		t.Fatalf("applyRuntimeInputs: %v", err)
	}
	if got, _ := api.GetPath(root, "jmeter.tunables.targetUsers"); got != api.RuntimeInput {
		t.Errorf("got %#v, want %q", got, api.RuntimeInput)
	}

	if err := writer.applyRuntimeInputs([]string{"tunables.targetUsers=500"}); err == nil {
		t.Error("a path with a value should be rejected; that is what --set is for")
	}
}

func TestMergeVariablesOverridesByName(t *testing.T) {
	existing := []api.Variable{
		{Name: "buildId", Value: "1000"},
		{Name: "region", Value: "us-east-1"},
	}
	incoming := []api.Variable{
		{Name: "buildId", Value: "2000"},
		{Name: "tier", Value: "gold"},
	}

	got := mergeVariables(existing, incoming)
	want := []api.Variable{
		{Name: "buildId", Value: "2000"},
		{Name: "region", Value: "us-east-1"},
		{Name: "tier", Value: "gold"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %#v\nwant %#v", got, want)
	}
}

func TestMetricTagsMergeAndRestrictToK6(t *testing.T) {
	root := map[string]any{}
	writer := writerFor(t, api.ToolK6, root)

	if err := writer.applyMetricTags([]string{"build=1", "region=us"}); err != nil {
		t.Fatalf("applyMetricTags: %v", err)
	}
	if err := writer.applyMetricTags([]string{"build=2"}); err != nil {
		t.Fatalf("applyMetricTags second call: %v", err)
	}

	got, _ := api.GetPath(root, "k6.metricTags")
	want := map[string]any{"build": "2", "region": "us"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}

	if err := writerFor(t, api.ToolJMeter, nil).applyMetricTags([]string{"a=b"}); err == nil {
		t.Error("metric tags should be rejected for JMeter")
	}
}

func TestPropertiesRestrictedToJMeter(t *testing.T) {
	root := map[string]any{}
	writer := writerFor(t, api.ToolJMeter, root)

	if err := writer.applyProperties([]string{"threads=200", "loops=<+input>"}); err != nil {
		t.Fatalf("applyProperties: %v", err)
	}

	got := writer.list("properties")
	want := []any{
		map[string]any{"key": "threads", "value": 200},
		map[string]any{"key": "loops", "value": api.RuntimeInput},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %#v\nwant %#v", got, want)
	}

	if err := writerFor(t, api.ToolK6, nil).applyProperties([]string{"a=b"}); err == nil {
		t.Error("properties should be rejected for K6")
	}
}

// Some records come back with the tool block's fields at the root. Without the
// lift the patch lands in a sibling block and the update changes nothing.
func TestFlattenedToolConfigIsLifted(t *testing.T) {
	// Shape captured from GET /load-tests/testk6image97x on qa.harness.io.
	root := map[string]any{
		"mode":    "image",
		"envVars": []any{},
		"options": map[string]any{},
		"script": map[string]any{
			"image":           api.RuntimeInput,
			"imagePullSecret": "test",
		},
		"tunables": map[string]any{
			"targetUsers": api.RuntimeInput,
			"workerCount": 1,
		},
	}

	writer := writerFor(t, api.ToolK6, root)
	if err := writer.set("tunables.targetUsers", 500); err != nil {
		t.Fatalf("set: %v", err)
	}

	if len(root) != 1 {
		t.Fatalf("toolConfig should hold only the tool block, got keys %v", keysOf(root))
	}
	block, ok := root["k6"].(map[string]any)
	if !ok {
		t.Fatalf("expected the block under \"k6\", got %#v", root)
	}
	if got := block["mode"]; got != "image" {
		t.Errorf("mode should survive the lift, got %v", got)
	}
	if got := block["imagePullSecret"]; got != nil {
		t.Errorf("script fields should stay nested, not be hoisted: %v", got)
	}
	tunables, _ := block["tunables"].(map[string]any)
	if got := tunables["targetUsers"]; got != 500 {
		t.Errorf("targetUsers = %v, want the patched 500", got)
	}
	if got := tunables["workerCount"]; got != 1 {
		t.Errorf("workerCount = %v, want the inherited 1", got)
	}
}

// TestNestedToolConfigIsLeftAlone is the other half: an already-nested block
// must not be wrapped a second time.
func TestNestedToolConfigIsLeftAlone(t *testing.T) {
	root := map[string]any{
		"k6": map[string]any{"mode": "script", "tunables": map[string]any{"targetUsers": 10}},
	}
	writer := writerFor(t, api.ToolK6, root)
	if err := writer.set("tunables.targetUsers", 20); err != nil {
		t.Fatalf("set: %v", err)
	}
	block, ok := root["k6"].(map[string]any)
	if !ok {
		t.Fatalf("expected the block under \"k6\", got %#v", root)
	}
	if _, doubled := block["k6"]; doubled {
		t.Error("an already-nested block was wrapped again")
	}
	tunables, _ := block["tunables"].(map[string]any)
	if got := tunables["targetUsers"]; got != 20 {
		t.Errorf("targetUsers = %v, want 20", got)
	}
}

func keysOf(m map[string]any) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
