package command

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"unicode/utf8"
)

// readable rewrites a response for the json and yaml views. The API sends a
// script twice: base64 under toolConfig, and again inside the base64 "yaml"
// mirror of the whole object. On a typical load test those two account for
// over 90% of the payload and neither can be read. The script is decoded and
// the mirror dropped; "hc rt loadtest export-yaml" serves the mirror properly
// when it is wanted.
func readable(value any) any {
	decoded, err := throughJSON(value)
	if err != nil {
		// Printing the untouched response beats failing a command over a
		// presentation step.
		return value
	}
	return simplify(decoded)
}

// simplify walks a decoded response so it works on a single object and on a
// page of them without knowing which it was handed.
func simplify(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		if script, ok := typed["script"].(map[string]any); ok {
			if content, ok := script["content"].(string); ok {
				script["content"] = decodeScript(content)
			}
		}
		// Guarded by identity so this cannot swallow an unrelated "yaml" field
		// on some other object.
		if _, mirrored := typed["yaml"]; mirrored {
			if _, isLoadTest := typed["identity"]; isLoadTest {
				delete(typed, "yaml")
			}
		}
		for key, nested := range typed {
			typed[key] = simplify(nested)
		}
		return typed
	case []any:
		for index, nested := range typed {
			typed[index] = simplify(nested)
		}
		return typed
	default:
		return value
	}
}

// decodeScript turns a base64 script body into text. A JMeter .zip workspace is
// binary, so it is described rather than printed as mojibake, and a runtime
// input such as <+input> is not base64 at all and is left as it stands.
func decodeScript(encoded string) string {
	if encoded == "" {
		return encoded
	}

	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return encoded
	}

	if !utf8.Valid(raw) || bytes.IndexByte(raw, 0) >= 0 {
		return fmt.Sprintf("<%d bytes of binary; save it with \"hc rt loadtest script get\" and --output-file>", len(raw))
	}

	return string(raw)
}
