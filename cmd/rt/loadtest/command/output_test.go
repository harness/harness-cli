package command

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseFormatAcceptsTheDocumentedValues(t *testing.T) {
	for _, format := range validFormats {
		parsed, err := parseFormat(format)
		if err != nil {
			t.Errorf("parseFormat(%q) rejected a value that --help advertises: %v", format, err)
			continue
		}
		if parsed != format {
			t.Errorf("parseFormat(%q) returned %q", format, parsed)
		}
	}
}

func TestParseFormatNormalises(t *testing.T) {
	for _, input := range []string{"JSON", " json", "json ", "Json"} {
		parsed, err := parseFormat(input)
		if err != nil {
			t.Errorf("parseFormat(%q): %v", input, err)
			continue
		}
		if parsed != formatJSON {
			t.Errorf("parseFormat(%q) returned %q, want %q", input, parsed, formatJSON)
		}
	}
}

// An empty value is a flag left at its zero value, not a bad format.
func TestParseFormatTreatsEmptyAsTheDefault(t *testing.T) {
	parsed, err := parseFormat("")
	if err != nil {
		t.Fatalf("parseFormat(\"\"): %v", err)
	}
	if parsed != formatTable {
		t.Errorf("parseFormat(\"\") returned %q, want %q", parsed, formatTable)
	}
}

func TestParseFormatRejectsAnythingElse(t *testing.T) {
	for _, input := range []string{"xml", "csv", "yml", "tabl"} {
		if _, err := parseFormat(input); err == nil {
			t.Errorf("parseFormat(%q) was accepted", input)
		}
	}
}

// A format missing from the switch in Print quietly renders something else, so
// check each one reaches the writer and renders distinctly.
func TestEveryValidFormatIsHandledExplicitly(t *testing.T) {
	value := []struct {
		Name  string `json:"name" yaml:"name"`
		Count int    `json:"count" yaml:"count"`
	}{{Name: "checkout", Count: 3}}
	table := &Table{Headers: []string{"NAME", "COUNT"}, Rows: [][]string{{"checkout", "3"}}}

	rendered := map[string]string{}
	for _, format := range validFormats {
		buffer := &bytes.Buffer{}
		if err := (&resultPrinter{format: format, out: buffer}).Print(value, table); err != nil {
			t.Fatalf("printing as %s: %v", format, err)
		}

		output := buffer.String()
		if strings.TrimSpace(output) == "" {
			t.Errorf("%s wrote nothing to the supplied writer", format)
			continue
		}
		if previous, seen := rendered[output]; seen {
			t.Errorf("%s renders identically to %s", format, previous)
		}
		rendered[output] = format
	}
}

// A detail view has no sensible single-row projection, so it falls through to
// YAML rather than one very wide row.
func TestTableFormatFallsBackToYamlWithoutAProjection(t *testing.T) {
	buffer := &bytes.Buffer{}
	value := map[string]string{"identity": "checkout-load"}

	if err := (&resultPrinter{format: formatTable, out: buffer}).Print(value, nil); err != nil {
		t.Fatalf("Print: %v", err)
	}
	if !strings.Contains(buffer.String(), "identity: checkout-load") {
		t.Errorf("expected YAML, got %q", buffer.String())
	}
}

func TestProjectedTableHonoursTheWriter(t *testing.T) {
	buffer := &bytes.Buffer{}
	table := Table{Headers: []string{"NAME"}, Rows: [][]string{{"checkout"}}}

	if err := renderTable(table, buffer); err != nil {
		t.Fatalf("renderTable: %v", err)
	}
	if !strings.Contains(buffer.String(), "checkout") {
		t.Errorf("the projected table did not reach the supplied writer; got %q", buffer.String())
	}
}

// An empty listing must say so rather than draw an empty box.
func TestProjectedTableReportsNoResults(t *testing.T) {
	buffer := &bytes.Buffer{}

	if err := renderTable(Table{Headers: []string{"NAME"}}, buffer); err != nil {
		t.Fatalf("renderTable: %v", err)
	}
	if !strings.Contains(buffer.String(), "No results.") {
		t.Errorf("an empty table rendered %q", buffer.String())
	}
}

// The YAML encoder names keys after Go fields unless it is fed a value that has
// already been through the json tags. It printed "tooltype" for a whole release.
func TestYamlKeysUseTheApiFieldNames(t *testing.T) {
	definition := struct {
		ToolType   string `json:"toolType"`
		TargetUser string `json:"targetUsers"`
	}{ToolType: "K6", TargetUser: "50"}

	buffer := &bytes.Buffer{}
	if err := renderYAML(definition, buffer); err != nil {
		t.Fatalf("renderYAML: %v", err)
	}

	for _, key := range []string{"toolType:", "targetUsers:"} {
		if !strings.Contains(buffer.String(), key) {
			t.Errorf("YAML is missing %q; got:\n%s", key, buffer.String())
		}
	}
	if strings.Contains(buffer.String(), "tooltype:") {
		t.Errorf("YAML fell back to the Go field name; got:\n%s", buffer.String())
	}
}

// Routing through JSON must not turn counts into floats: a plain any decode
// makes every number a float64, and the encoder then writes 1e+06.
func TestYamlKeepsLargeCountsReadable(t *testing.T) {
	buffer := &bytes.Buffer{}
	if err := renderYAML(map[string]any{"totalRequests": 1000000}, buffer); err != nil {
		t.Fatalf("renderYAML: %v", err)
	}

	if !strings.Contains(buffer.String(), "totalRequests: 1000000") {
		t.Errorf("a large count did not survive the round trip; got %q", buffer.String())
	}
}
