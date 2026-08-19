package command

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/harness/harness-cli/cmd/rt/loadtest/api"
)

// script wraps a body the way a response nests it, so a test reads as the
// shape it is asserting on.
func script(content string) map[string]any {
	return map[string]any{
		"identity": "checkout-load",
		"toolConfig": map[string]any{
			"locust": map[string]any{
				"script": map[string]any{"content": content},
			},
		},
	}
}

func contentOf(t *testing.T, value any) string {
	t.Helper()
	root, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected an object, got %T", value)
	}
	tool := root["toolConfig"].(map[string]any)["locust"].(map[string]any)
	return tool["script"].(map[string]any)["content"].(string)
}

// A base64 script is unreadable in a diff and in a terminal, and it is the bulk
// of a response; the whole point of the json and yaml views is to see it.
func TestScriptIsDecodedForDisplay(t *testing.T) {
	body := "from locust import HttpUser\nimport uuid\n"
	got := contentOf(t, readable(script(base64.StdEncoding.EncodeToString([]byte(body)))))

	if got != body {
		t.Errorf("script was not decoded:\n got %q\nwant %q", got, body)
	}
}

// A tunable deferred to run time is a literal, not base64, and must survive
// untouched or the config stops round-tripping.
func TestRuntimeInputIsLeftAlone(t *testing.T) {
	if got := contentOf(t, readable(script("<+input>"))); got != "<+input>" {
		t.Errorf("runtime input was rewritten to %q", got)
	}
}

// A JMeter .zip workspace is an archive. Decoding it to a string would spray
// mojibake across the terminal, so it is described instead.
func TestBinaryScriptIsDescribedNotPrinted(t *testing.T) {
	archive := []byte{0x50, 0x4b, 0x03, 0x04, 0x00, 0x01, 0x02}
	got := contentOf(t, readable(script(base64.StdEncoding.EncodeToString(archive))))

	if !strings.Contains(got, "binary") {
		t.Errorf("binary script was not described: %q", got)
	}
	if strings.ContainsRune(got, 0) {
		t.Error("raw archive bytes reached the output")
	}
}

// The yaml field is a base64 copy of the whole object, script included, so it
// roughly doubles a response and repeats what is already on screen.
func TestYamlMirrorIsDropped(t *testing.T) {
	test := script("")
	test["yaml"] = base64.StdEncoding.EncodeToString([]byte("kind: LoadTest"))

	root := readable(test).(map[string]any)
	if _, present := root["yaml"]; present {
		t.Error("the yaml mirror was kept")
	}
}

// Dropping the mirror keys off identity, so an unrelated object that happens to
// carry a yaml field must not lose it.
func TestYamlIsKeptOnSomethingThatIsNotALoadTest(t *testing.T) {
	root := readable(map[string]any{"name": "a-hub", "yaml": "kind: Hub"}).(map[string]any)

	if _, present := root["yaml"]; !present {
		t.Error("yaml was dropped from an object that is not a load test")
	}
}

// A page of tests is the case where the saving matters most, and it exercises
// the array branch of the walk.
func TestScriptsAreDecodedThroughAList(t *testing.T) {
	body := base64.StdEncoding.EncodeToString([]byte("print(1)"))
	page := map[string]any{"items": []any{script(body), script(body)}}

	items := readable(page).(map[string]any)["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	for index, item := range items {
		if got := contentOf(t, item); got != "print(1)" {
			t.Errorf("item %d was not decoded: %q", index, got)
		}
	}
}

// The top-level targetUsers and durationSeconds are projections kept for
// backward compatibility and are due to be removed. Reading them would leave
// the USERS and DURATION columns blank the day they go, so the tunables win.
func TestTunableIsPreferredOverTheLegacyProjection(t *testing.T) {
	test := &api.LoadTest{
		ToolType:    api.ToolLocust,
		TargetUsers: "stale",
		ToolConfig: map[string]any{
			"locust": map[string]any{
				"tunables": map[string]any{"targetUsers": float64(250)},
			},
		},
	}

	if got := tunable(test, "targetUsers", test.TargetUsers); got != "250" {
		t.Errorf("read the projection instead of the tunable: got %q, want \"250\"", got)
	}
}

// Until the projection is actually removed it is the only source for a test
// whose tool config did not come back.
func TestProjectionIsUsedWhenTheTunableIsAbsent(t *testing.T) {
	test := &api.LoadTest{ToolType: api.ToolLocust, TargetUsers: "100"}

	if got := tunable(test, "targetUsers", test.TargetUsers); got != "100" {
		t.Errorf("did not fall back to the projection: got %q, want \"100\"", got)
	}
}

// JSON decodes every number into a float64, and %v renders a large one in
// scientific notation, so a duration would print as 1e+06.
func TestLargeTunableStaysAWholeNumber(t *testing.T) {
	test := &api.LoadTest{
		ToolType: api.ToolLocust,
		ToolConfig: map[string]any{
			"locust": map[string]any{
				"tunables": map[string]any{"durationSeconds": float64(1000000)},
			},
		},
	}

	if got := tunable(test, "durationSeconds", ""); got != "1000000" {
		t.Errorf("got %q, want \"1000000\"", got)
	}
}
