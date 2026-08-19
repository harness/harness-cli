package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// FlagConfig is the per-command flag that points at a JSON definition file.
const FlagConfig = "config"

// LoadConfigFile reads a JSON config file into out. Unknown fields are
// rejected so a mistyped key fails loudly instead of being silently dropped.
func LoadConfigFile(path string, out any) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading config file %q: %w", path, err)
	}

	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("parsing config file %q: %w", path, decodeHint(err))
	}

	return nil
}

// LoadConfigSection strictly decodes one top-level key, so a whole definition
// can be passed to a command reading part of it. No key means a bare section.
func LoadConfigSection(path, key string, out any) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading config file %q: %w", path, err)
	}

	var document map[string]json.RawMessage
	if err := json.Unmarshal(contents, &document); err != nil {
		return fmt.Errorf("parsing config file %q: %w", path, err)
	}

	section, wrapped := document[key]
	if !wrapped {
		section = contents
	}

	decoder := json.NewDecoder(bytes.NewReader(section))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		if wrapped {
			return fmt.Errorf("parsing %q of config file %q: %w", key, path, decodeHint(err))
		}
		return fmt.Errorf("parsing config file %q: %w", path, decodeHint(err))
	}

	return nil
}

// decodeHint rewrites encoding/json's unknown-field message into something
// that names the offending key plainly.
func decodeHint(err error) error {
	const prefix = "json: unknown field "
	message := err.Error()
	if !strings.HasPrefix(message, prefix) {
		return err
	}

	field := strings.Trim(strings.TrimPrefix(message, prefix), `"`)
	return fmt.Errorf("unknown field %q (check spelling and capitalization; field names are case sensitive)", field)
}
