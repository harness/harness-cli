package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/harness/harness-cli/util/common/printer"

	"github.com/pterm/pterm"
	"gopkg.in/yaml.v3"
)

// Accepted values for --format.
const (
	formatTable = "table"
	formatJSON  = "json"
	formatYAML  = "yaml"
)

var validFormats = []string{formatTable, formatJSON, formatYAML}

// Table is a listing the caller has already projected, for cells that are
// computed rather than read straight off the response.
type Table struct {
	Headers []string
	Rows    [][]string
}

// parseFormat normalises a --format value and rejects anything unrecognised.
// Empty means the flag was left at its default.
func parseFormat(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", formatTable:
		return formatTable, nil
	case formatJSON:
		return formatJSON, nil
	case formatYAML:
		return formatYAML, nil
	default:
		return "", fmt.Errorf("invalid --format %q: expected one of %s",
			value, strings.Join(validFormats, ", "))
	}
}

// renderTable draws a projected listing in the same pterm style the shared
// printer uses, so a hand-built table looks no different from a derived one.
func renderTable(table Table, writer io.Writer) error {
	if writer == nil {
		writer = os.Stdout
	}
	if len(table.Rows) == 0 {
		_, err := fmt.Fprintln(writer, "No results.")
		return err
	}

	data := make([][]string, 0, len(table.Rows)+1)
	renderer := pterm.DefaultTable.WithBoxed(true)
	if len(table.Headers) > 0 {
		data = append(data, table.Headers)
		renderer = renderer.WithHasHeader()
	}
	data = append(data, table.Rows...)

	rendered, err := renderer.WithData(data).Srender()
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(writer, strings.TrimRight(rendered, "\n"))
	return err
}

// renderYAML is the readable rendering for a single deeply nested object that
// a table would flatten away.
func renderYAML(res any, writer io.Writer) error {
	if writer == nil {
		writer = os.Stdout
	}

	// Marshal through JSON so the json tags decide the key names. Handing the
	// struct straight to the YAML encoder emits lowercased Go field names --
	// "tooltype", not "toolType" -- which matches neither the API nor a config
	// file, so the output cannot be fed back in.
	normalised, err := throughJSON(res)
	if err != nil {
		return err
	}

	encoded, err := yaml.Marshal(normalised)
	if err != nil {
		return fmt.Errorf("failed to encode YAML: %w", err)
	}

	_, err = writer.Write(encoded)
	return err
}

// throughJSON re-reads a value as generic JSON, preserving number precision.
func throughJSON(res any) (any, error) {
	encoded, err := json.Marshal(res)
	if err != nil {
		return nil, fmt.Errorf("failed to encode YAML: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()

	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("failed to encode YAML: %w", err)
	}

	return demoteNumbers(decoded), nil
}

// demoteNumbers replaces json.Number with a real numeric type. Left alone it is
// a string, so a count would print quoted; decoded as plain any it would be a
// float64, so a large count would print as 1e+06.
func demoteNumbers(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			typed[key] = demoteNumbers(nested)
		}
		return typed
	case []any:
		for index, nested := range typed {
			typed[index] = demoteNumbers(nested)
		}
		return typed
	case json.Number:
		if integer, err := typed.Int64(); err == nil {
			return integer
		}
		if float, err := typed.Float64(); err == nil {
			return float
		}
		return typed.String()
	default:
		return value
	}
}

// renderJSON defers to the shared printer, which already honours the writer.
func renderJSON(res any, writer io.Writer) error {
	options := printer.DefaultJsonOptions()
	options.Writer = writer
	return printer.PrintJsonWithOptions(res, options)
}
