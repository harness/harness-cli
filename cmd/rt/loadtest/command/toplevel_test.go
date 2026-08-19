package command

import (
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/harness/harness-cli/cmd/rt/loadtest/api"
)

func TestListPrintsTheTestsInScope(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-tests"): map[string]any{
			"items": []any{aLoadTest("checkout-load"), aLoadTest("search-load")},
			"total": 2,
		},
	})

	globals().Format = formatTable
	out := expectSuccess(t, NewListCmd())

	mustContain(t, "list table", out.stdout,
		"IDENTITY", "checkout-load", "search-load", "perf-cluster", "staging", "K6")
}

func TestListSendsEveryFilterItOffers(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/load-tests"): map[string]any{"items": []any{}},
	})

	expectSuccess(t, NewListCmd(),
		"--page", "2", "--limit", "50", "--search", "checkout",
		"--sort-field", "createdAt", "--sort-ascending",
		"--tool-type", "JMeter", "--environment-id", "staging",
		"--tag", "nightly", "--tag", "eu")

	query := stub.only().Query
	for key, want := range map[string]string{
		"page":                  "2",
		"limit":                 "50",
		"search":                "checkout",
		"sortField":             "createdAt",
		"toolType":              "JMeter",
		"environmentIdentifier": "staging",
	} {
		if got := query.Get(key); got != want {
			t.Errorf("query %s = %q, want %q (full query: %v)", key, got, want, query)
		}
	}
	if len(query["tags"]) == 0 && query.Get("tags") == "" {
		t.Errorf("neither tag reached the query: %v", query)
	}
}

// Nothing is sent for a filter left alone, so a plain listing is not silently
// narrowed by a default the user never asked for.
func TestListSendsNoFilterItWasNotGiven(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/load-tests"): map[string]any{"items": []any{}},
	})

	expectSuccess(t, NewListCmd())

	query := stub.only().Query
	for _, key := range []string{"search", "toolType", "environmentIdentifier", "tags", "sortField"} {
		if query.Has(key) {
			t.Errorf("%s = %q was sent although no flag set it", key, query.Get(key))
		}
	}
}

func TestListSurvivesAProjectWithNoTests(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-tests"): map[string]any{"items": []any{}, "total": 0},
	})

	globals().Format = formatTable
	expectSuccess(t, NewListCmd())
}

func TestGetPrintsOneTest(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load"): aLoadTest("checkout-load"),
	})

	globals().Format = formatTable
	out := expectSuccess(t, NewGetCmd(), "checkout-load")

	if got := stub.only().Path; got != "/load-tests/checkout-load" {
		t.Errorf("called %s, want the test fetched by identity", got)
	}
	mustContain(t, "get table", out.stdout, "checkout-load", "Checkout load", "K6")
}

// The table is a summary; the tool configuration is the reason to ask for JSON,
// so it has to be there in full.
func TestGetShowsTheToolConfigurationInJSON(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load"): aLoadTest("checkout-load"),
	})

	out := expectSuccess(t, NewGetCmd(), "checkout-load")

	mustContain(t, "get json", out.stdout, "toolConfig", "tunables", "targetUsers", "REGION")
}

func TestGetSurfacesATestThatIsNotThere(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-tests/ghost"): http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"message":"load test not found in the given scope"}`, http.StatusNotFound)
		}),
	})

	message := expectFailure(t, NewGetCmd(), "ghost")

	mustContain(t, "not found error", message, "not found", "scope")
}

func TestVariablesListsVariablesAndInputsTogether(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load/variables"): map[string]any{
			"loadTestIdentity": "checkout-load",
			"variables": []any{
				map[string]any{"name": "REGION", "value": "eu-west-1", "type": "String", "required": true},
			},
			"inputs": []any{
				map[string]any{
					"path": "k6.tunables.targetUsers", "type": "Number",
					"required": true, "description": "Peak concurrent users",
				},
			},
		},
	})

	globals().Format = formatTable
	out := expectSuccess(t, NewVariablesCmd(), "checkout-load")

	// KIND is what tells the two apart, and they are set by different flags.
	mustContain(t, "variables table", out.stdout,
		"KIND", "variable", "REGION", "input", "k6.tunables.targetUsers")
}

// A test built around a container image often has inputs and no variables at
// all, so an empty variables list must not read as an empty command.
func TestVariablesHandlesATestWithInputsOnly(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-tests/image-load/variables"): map[string]any{
			"loadTestIdentity": "image-load",
			"inputs": []any{
				map[string]any{"path": "k6.image.tag", "type": "String", "required": true},
			},
		},
	})

	globals().Format = formatTable
	out := expectSuccess(t, NewVariablesCmd(), "image-load")

	mustContain(t, "variables table", out.stdout, "input", "k6.image.tag")
	mustNotContain(t, "variables table", out.stdout, "variable\t")
}

func TestDeleteRemovesTheTestWhenForced(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		DELETE("/load-tests/checkout-load"): "",
	})

	out := expectSuccess(t, NewDeleteCmd(), "checkout-load", "--force")

	if got := stub.only().Method; got != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", got)
	}
	mustContain(t, "delete confirmation", out.stdout, "Deleted", "checkout-load")
}

// Without a terminal there is nobody to answer the prompt. Proceeding anyway
// would make a pipeline delete without confirmation; refusing names the way out.
func TestDeleteRefusesToGuessWhenItCannotAsk(t *testing.T) {
	stub := serveLTM(t, map[call]any{})

	message := expectFailure(t, NewDeleteCmd(), "checkout-load")

	mustContain(t, "confirmation refusal", message, "--force", "checkout-load")
	if len(stub.requests()) != 0 {
		t.Errorf("made %v, want nothing sent without a confirmation", summarise(stub.requests()))
	}
}

func TestDeleteSurfacesARefusalFromTheService(t *testing.T) {
	serveLTM(t, map[call]any{
		DELETE("/load-tests/checkout-load"): http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"message":"load test has a run in progress"}`, http.StatusConflict)
		}),
	})

	message := expectFailure(t, NewDeleteCmd(), "checkout-load", "--force")

	mustContain(t, "delete refusal", message, "run in progress")
}

func TestExportYAMLDecodesWhatTheServiceEncoded(t *testing.T) {
	test := aLoadTest("checkout-load")
	test["yaml"] = base64.StdEncoding.EncodeToString([]byte("identity: checkout-load\ntoolType: K6\n"))

	serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load"): test,
	})

	out := expectSuccess(t, NewExportYAMLCmd(), "checkout-load")

	if out.stdout != "identity: checkout-load\ntoolType: K6\n" {
		t.Errorf("stdout = %q, want the decoded YAML", out.stdout)
	}
}

// The field is base64 today. Treating a failed decode as "already plain" is
// what keeps this working if that stops being true.
func TestExportYAMLPassesThroughPlainYAML(t *testing.T) {
	test := aLoadTest("checkout-load")
	test["yaml"] = "identity: checkout-load\n"

	serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load"): test,
	})

	out := expectSuccess(t, NewExportYAMLCmd(), "checkout-load")

	mustContain(t, "exported yaml", out.stdout, "identity: checkout-load")
}

// The export bypasses the printer: re-encoding YAML as JSON would defeat the
// point, so --format must not touch it.
func TestExportYAMLIgnoresTheOutputFormat(t *testing.T) {
	test := aLoadTest("checkout-load")
	test["yaml"] = "identity: checkout-load\n"

	serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load"): test,
	})

	globals().Format = formatJSON
	out := expectSuccess(t, NewExportYAMLCmd(), "checkout-load")

	if out.stdout != "identity: checkout-load\n" {
		t.Errorf("stdout = %q, want the YAML unchanged by --format json", out.stdout)
	}
}

func TestExportYAMLSavesToAFile(t *testing.T) {
	test := aLoadTest("checkout-load")
	test["yaml"] = "identity: checkout-load\n"

	serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load"): test,
	})

	destination := filepath.Join(t.TempDir(), "checkout.yaml")
	out := expectSuccess(t, NewExportYAMLCmd(), "checkout-load", "--output-file", destination)

	saved, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("reading the exported file: %v", err)
	}
	if string(saved) != "identity: checkout-load\n" {
		t.Errorf("saved %q, want the YAML", saved)
	}
	mustContain(t, "export confirmation", out.stderr, destination, "bytes")
}

func TestExportYAMLReportsATestWithNoYAMLForm(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load"): aLoadTest("checkout-load"),
	})

	message := expectFailure(t, NewExportYAMLCmd(), "checkout-load")

	mustContain(t, "missing yaml error", message, "without a YAML form")
}

func TestExportYAMLReportsAPathItCannotWrite(t *testing.T) {
	test := aLoadTest("checkout-load")
	test["yaml"] = "identity: checkout-load\n"

	serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load"): test,
	})

	inaccessible := filepath.Join(t.TempDir(), "no-such-directory", "out.yaml")
	message := expectFailure(t, NewExportYAMLCmd(), "checkout-load", "--output-file", inaccessible)

	mustContain(t, "export write error", message, "writing", inaccessible)
}

// The top-level targetUsers and durationSeconds are display projections due to
// be removed, so the tunables win and the projection is only a fallback.
func TestTheTableReadsTunablesRatherThanTheProjection(t *testing.T) {
	test := &api.LoadTest{
		Identity:        "checkout-load",
		ToolType:        api.ToolK6,
		TargetUsers:     "100",
		DurationSeconds: "600",
		ToolConfig: map[string]any{
			"k6": map[string]any{
				"tunables": map[string]any{"targetUsers": float64(500), "durationSeconds": float64(1800)},
			},
		},
	}

	row := loadTestTable([]*api.LoadTest{test}).Rows[0]

	if row[5] != "500" || row[6] != "1800" {
		t.Errorf("users/duration = %q/%q, want the tunables to win over the projection", row[5], row[6])
	}
}

func TestTheTableFallsBackToTheProjection(t *testing.T) {
	for _, tc := range []struct {
		name string
		test *api.LoadTest
	}{
		{
			name: "no tool type",
			test: &api.LoadTest{TargetUsers: "100", DurationSeconds: "600"},
		},
		{
			name: "no block for the tool",
			test: &api.LoadTest{
				ToolType: api.ToolK6, TargetUsers: "100", DurationSeconds: "600",
				ToolConfig: map[string]any{"locust": map[string]any{}},
			},
		},
		{
			name: "no tunables in the block",
			test: &api.LoadTest{
				ToolType: api.ToolK6, TargetUsers: "100", DurationSeconds: "600",
				ToolConfig: map[string]any{"k6": map[string]any{"script": map[string]any{}}},
			},
		},
		{
			name: "the tunable is null",
			test: &api.LoadTest{
				ToolType: api.ToolK6, TargetUsers: "100", DurationSeconds: "600",
				ToolConfig: map[string]any{
					"k6": map[string]any{"tunables": map[string]any{"targetUsers": nil, "durationSeconds": nil}},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := loadTestTable([]*api.LoadTest{tc.test}).Rows[0]
			if row[5] != "100" || row[6] != "600" {
				t.Errorf("users/duration = %q/%q, want the projection as the fallback", row[5], row[6])
			}
		})
	}
}

// JSON has one number type, so a duration arrives as a float64. Rendering it
// with %v would print 1000000 as 1e+06, which is not a number of seconds
// anyone can act on.
func TestNumbersRenderAsNumbersNotScientificNotation(t *testing.T) {
	for _, tc := range []struct {
		value any
		want  string
	}{
		{float64(1000000), "1000000"},
		{float64(500), "500"},
		{float64(0), "0"},
		{2.5, "2.5"},
		{"already text", "already text"},
		{true, "true"},
	} {
		if got := formatNumber(tc.value); got != tc.want {
			t.Errorf("formatNumber(%v) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

// A dash reads as "not set". An empty cell reads as a rendering fault.
func TestAbsentValuesRenderAsADash(t *testing.T) {
	if got := formatDefault(nil); got != "-" {
		t.Errorf("formatDefault(nil) = %q, want a dash", got)
	}
	if got := optionalSeconds(nil); got != "-" {
		t.Errorf("optionalSeconds(nil) = %q, want a dash", got)
	}
	if got := optionalString(nil); got != "-" {
		t.Errorf("optionalString(nil) = %q, want a dash", got)
	}

	blank := ""
	if got := optionalString(&blank); got != "-" {
		t.Errorf("optionalString(\"\") = %q, want a dash, since an empty string is also not set", got)
	}

	seconds := 600
	if got := optionalSeconds(&seconds); got != "600s" {
		t.Errorf("optionalSeconds(600) = %q, want the unit", got)
	}
	value := "2026-08-01T09:00:00Z"
	if got := optionalString(&value); got != value {
		t.Errorf("optionalString = %q, want the value", got)
	}
}

// An input has a path and often no name, since it is a config leaf rather than
// a declared value. The path is what --runtime-value takes, so it leads.
func TestTheInputTableLeadsWithThePath(t *testing.T) {
	table := inputTable([]api.Input{
		{Path: "k6.tunables.targetUsers", Type: "Number", Required: true, Default: float64(100)},
		{Name: "fallback-name", Type: "String"},
	})

	if table.Headers[0] != "PATH" {
		t.Errorf("first column is %q, want PATH", table.Headers[0])
	}
	if table.Rows[0][0] != "k6.tunables.targetUsers" {
		t.Errorf("path = %q, want the config path", table.Rows[0][0])
	}
	if table.Rows[1][0] != "fallback-name" {
		t.Errorf("path = %q, want the name when there is no path", table.Rows[1][0])
	}
	if table.Rows[1][3] != "-" {
		t.Errorf("default = %q, want a dash when there is none", table.Rows[1][3])
	}
}

func TestTheVariableTableRendersEveryValueType(t *testing.T) {
	table := variableTable([]api.Variable{
		{Name: "REGION", Value: "eu-west-1", Type: "String", Required: true},
		{Name: "TIMEOUT", Value: 30, Type: "Integer"},
		{Name: "DEBUG", Value: true, Type: "Boolean", Description: "Verbose logging"},
	})

	if table.Rows[1][1] != "30" {
		t.Errorf("integer rendered as %q", table.Rows[1][1])
	}
	if table.Rows[2][1] != "true" {
		t.Errorf("boolean rendered as %q", table.Rows[2][1])
	}
	if table.Rows[0][3] != "true" || table.Rows[1][3] != "false" {
		t.Errorf("required column = %q/%q, want true then false", table.Rows[0][3], table.Rows[1][3])
	}
}
