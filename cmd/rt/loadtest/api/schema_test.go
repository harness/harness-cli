package api

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSetPathCreatesIntermediateBlocks(t *testing.T) {
	root := map[string]any{}
	if err := SetPath(root, "k6.tunables.targetUsers", 200); err != nil {
		t.Fatalf("SetPath: %v", err)
	}

	got, found := GetPath(root, "k6.tunables.targetUsers")
	if !found {
		t.Fatal("GetPath did not find the value just written")
	}
	if got != 200 {
		t.Fatalf("got %v, want 200", got)
	}
}

func TestSetPathPreservesSiblings(t *testing.T) {
	// A tunable flag patches one leaf of a config loaded from a file. Losing
	// a sibling would silently drop the script from the request.
	root := map[string]any{
		"jmeter": map[string]any{
			"script":   map[string]any{"content": "abc", "testPlan": "main.jmx"},
			"tunables": map[string]any{"targetUsers": 100, "durationSeconds": 900},
		},
	}

	if err := SetPath(root, "jmeter.tunables.targetUsers", 500); err != nil {
		t.Fatalf("SetPath: %v", err)
	}

	for path, want := range map[string]any{
		"jmeter.tunables.targetUsers":     500,
		"jmeter.tunables.durationSeconds": 900,
		"jmeter.script.content":           "abc",
		"jmeter.script.testPlan":          "main.jmx",
	} {
		got, found := GetPath(root, path)
		if !found {
			t.Errorf("%s: missing after write", path)
			continue
		}
		if got != want {
			t.Errorf("%s: got %v, want %v", path, got, want)
		}
	}
}

func TestSetPathRejectsWritingThroughAScalar(t *testing.T) {
	root := map[string]any{"k6": map[string]any{"mode": "script"}}

	err := SetPath(root, "k6.mode.nested", "x")
	if err == nil {
		t.Fatal("expected an error when a path segment is a value, not a block")
	}
	if !strings.Contains(err.Error(), "k6.mode") {
		t.Errorf("error should name the offending segment, got: %v", err)
	}
}

func TestGetPathMissingSegment(t *testing.T) {
	root := map[string]any{"k6": map[string]any{}}

	if _, found := GetPath(root, "k6.tunables.targetUsers"); found {
		t.Error("GetPath reported a value that was never written")
	}
	if _, found := GetPath(root, "locust"); found {
		t.Error("GetPath reported a tool block that was never written")
	}
}

// What --generate-config-skeleton prints must be accepted by --config
// unchanged, and the loader rejects unknown fields.
func TestSkeletonRoundTrip(t *testing.T) {
	for _, tool := range SupportedToolTypes {
		for _, mode := range []Mode{ModeScript, ModeImage} {
			t.Run(string(tool)+"/"+string(mode), func(t *testing.T) {
				encoded := encodeSkeleton(t, Skeleton(tool, mode))

				decoder := json.NewDecoder(bytes.NewReader(encoded))
				decoder.DisallowUnknownFields()

				request := &CreateRequest{}
				if err := decoder.Decode(request); err != nil {
					t.Fatalf("skeleton is not a valid CreateRequest: %v", err)
				}
				if request.ToolType != tool {
					t.Errorf("toolType: got %q, want %q", request.ToolType, tool)
				}
				if _, found := GetPath(request.ToolConfig, ToolKey(tool)+".tunables.targetUsers"); !found {
					t.Error("skeleton has no targetUsers tunable")
				}
			})
		}
	}
}

func TestJSONSpecSkeletonRoundTrip(t *testing.T) {
	encoded := encodeSkeleton(t, JSONSpecSkeleton())

	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()

	request := &CreateFromJSONRequest{}
	if err := decoder.Decode(request); err != nil {
		t.Fatalf("skeleton is not a valid CreateFromJSONRequest: %v", err)
	}
	if err := request.JSONSpec.Validate(); err != nil {
		t.Fatalf("skeleton spec does not validate: %v", err)
	}
}

// Go's JSON encoder escapes < and > by default, which turns <+input> into an
// unreadable sequence in a file users are meant to edit.
func TestSkeletonKeepsExpressionsLiteral(t *testing.T) {
	encoded := encodeSkeleton(t, Skeleton(ToolK6, ModeScript))

	if !bytes.Contains(encoded, []byte(RuntimeInput)) {
		t.Errorf("skeleton should contain a literal %s, got:\n%s", RuntimeInput, encoded)
	}
	// json.Marshal escapes by default, so its output is exactly the form the
	// skeleton writer must avoid producing.
	escaped, err := json.Marshal(RuntimeInput)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, escaped) {
		t.Errorf("skeleton contains HTML-escaped expressions (%s):\n%s", escaped, encoded)
	}
}

// TestSkeletonOmitsInapplicableTunables checks the template does not suggest a
// setting the chosen engine ignores.
func TestSkeletonOmitsInapplicableTunables(t *testing.T) {
	cases := []struct {
		tool    ToolType
		absent  []string
		present []string
	}{
		{tool: ToolJMeter, absent: []string{"spawnRate", "hostUrl", "iterations", "rpsLimit"}},
		{tool: ToolLocust, absent: []string{"hostUrl", "iterations", "rpsLimit"}, present: []string{"spawnRate"}},
		{tool: ToolK6, absent: []string{"spawnRate"}, present: []string{"hostUrl", "iterations", "rpsLimit"}},
	}

	for _, tc := range cases {
		t.Run(string(tc.tool), func(t *testing.T) {
			skeleton := Skeleton(tc.tool, ModeScript)
			base := "toolConfig." + ToolKey(tc.tool) + ".tunables."

			for _, field := range tc.absent {
				if _, found := GetPath(skeleton, base+field); found {
					t.Errorf("%s is not a %s tunable but appears in its skeleton", field, tc.tool)
				}
			}
			for _, field := range tc.present {
				if _, found := GetPath(skeleton, base+field); !found {
					t.Errorf("%s is a %s tunable but is missing from its skeleton", field, tc.tool)
				}
			}
		})
	}
}

func TestSkeletonThresholdsOnlyWhereSupported(t *testing.T) {
	if _, found := GetPath(Skeleton(ToolLocust, ModeScript), "toolConfig.locust.thresholds"); found {
		t.Error("Locust does not support thresholds but its skeleton declares them")
	}
	for _, tool := range []ToolType{ToolJMeter, ToolK6} {
		if _, found := GetPath(Skeleton(tool, ModeScript), "toolConfig."+ToolKey(tool)+".thresholds"); !found {
			t.Errorf("%s supports thresholds but its skeleton omits them", tool)
		}
	}
}

func TestToolKey(t *testing.T) {
	want := map[ToolType]string{ToolJMeter: "jmeter", ToolLocust: "locust", ToolK6: "k6", "Gatling": ""}
	for tool, key := range want {
		if got := ToolKey(tool); got != key {
			t.Errorf("ToolKey(%q): got %q, want %q", tool, got, key)
		}
	}
}

func TestJSONSpecValidate(t *testing.T) {
	valid := func() JSONSpec {
		return JSONSpec{
			Config:    JSONSpecConfig{Host: "https://api.example.com"},
			Endpoints: []JSONSpecEndpoint{{Name: "login", Method: "POST", Path: "/login"}},
		}
	}

	cases := []struct {
		name    string
		mutate  func(*JSONSpec)
		wantErr string
	}{
		{name: "valid"},
		{
			name:    "no host",
			mutate:  func(s *JSONSpec) { s.Config.Host = "" },
			wantErr: "config.host is required",
		},
		{
			name:    "no endpoints",
			mutate:  func(s *JSONSpec) { s.Endpoints = nil },
			wantErr: "endpoints is required",
		},
		{
			name:    "unsupported method",
			mutate:  func(s *JSONSpec) { s.Endpoints[0].Method = "FETCH" },
			wantErr: `unsupported method "FETCH"`,
		},
		{
			name:    "missing path",
			mutate:  func(s *JSONSpec) { s.Endpoints[0].Path = "" },
			wantErr: "path is required",
		},
		{
			name: "duplicate name",
			mutate: func(s *JSONSpec) {
				s.Endpoints = append(s.Endpoints, JSONSpecEndpoint{Name: "login", Method: "GET", Path: "/other"})
			},
			wantErr: "duplicate name",
		},
		{
			name: "inverted wait window",
			mutate: func(s *JSONSpec) {
				min, max := 9, 2
				s.Config.MinWait, s.Config.MaxWait = &min, &max
			},
			wantErr: "cannot be greater than",
		},
		{
			name: "half an extractor",
			mutate: func(s *JSONSpec) {
				s.Endpoints[0].Extract = &JSONSpecExtract{VariableName: "token"}
			},
			wantErr: "extract needs both",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := valid()
			if tc.mutate != nil {
				tc.mutate(&spec)
			}

			err := spec.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("got %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

// toolConfig is a map, not a typed struct, because a numeric leaf may hold
// "<+input>" and no int field survives that.
func TestCreateRequestCarriesToolConfigOpaquely(t *testing.T) {
	const body = `{
      "identity": "x",
      "toolType": "K6",
      "toolConfig": {
        "k6": {
          "tunables": { "targetUsers": "<+input>", "durationSeconds": 900 },
          "unknownFutureField": { "nested": true }
        }
      }
    }`

	request := &CreateRequest{}
	if err := json.Unmarshal([]byte(body), request); err != nil {
		t.Fatalf("decode: %v", err)
	}

	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	var original, roundTripped map[string]any
	if err := json.Unmarshal([]byte(body), &original); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(original["toolConfig"], roundTripped["toolConfig"]) {
		t.Errorf("toolConfig changed in transit:\n got %v\nwant %v", roundTripped["toolConfig"], original["toolConfig"])
	}
}

// Skeletons are generated from the structs and cannot drift; the examples are
// hand-written, so a renamed field ships a file that --config rejects.
func TestCommittedExamplesDecode(t *testing.T) {
	dir := filepath.Join("..", "examples")

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading examples: %v", err)
	}

	found := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		found++

		t.Run(entry.Name(), func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				t.Fatalf("reading: %v", err)
			}

			// A bare endpoint spec is passed with --spec rather than --config,
			// so it decodes into the spec type instead of a whole definition.
			if bare := (&JSONSpec{}); json.Unmarshal(raw, bare) == nil && bare.Config.Host != "" && !bytes.Contains(raw, []byte(`"identity"`)) {
				decodeStrict(t, raw, bare)
				if err := bare.Validate(); err != nil {
					t.Fatalf("spec does not validate: %v", err)
				}
				return
			}

			if bytes.Contains(raw, []byte(`"jsonSpec"`)) {
				request := &CreateFromJSONRequest{}
				decodeStrict(t, raw, request)
				if err := request.JSONSpec.Validate(); err != nil {
					t.Fatalf("spec does not validate: %v", err)
				}
				return
			}

			request := &CreateRequest{}
			decodeStrict(t, raw, request)
			if ToolKey(request.ToolType) == "" {
				t.Fatalf("toolType %q is not a supported tool", request.ToolType)
			}
			// An example that writes its config under the wrong tool key would
			// decode cleanly and then send an empty engine block.
			if _, found := GetPath(request.ToolConfig, ToolKey(request.ToolType)); !found {
				t.Errorf("toolConfig has no %q block for toolType %q", ToolKey(request.ToolType), request.ToolType)
			}
		})
	}

	if found == 0 {
		t.Fatalf("no example configs found in %s", dir)
	}
}

func decodeStrict(t *testing.T, raw []byte, into any) {
	t.Helper()

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		t.Fatalf("example is not accepted by --config: %v", err)
	}
}

func encodeSkeleton(t *testing.T, skeleton map[string]any) []byte {
	t.Helper()

	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(skeleton); err != nil {
		t.Fatalf("encoding skeleton: %v", err)
	}

	return buffer.Bytes()
}
