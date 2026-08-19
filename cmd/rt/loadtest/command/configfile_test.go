package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing the config file: %v", err)
	}
	return path
}

type endpointSpec struct {
	Host      string   `json:"host"`
	Endpoints []string `json:"endpoints"`
}

// The file a test was created from is the obvious one to hand to
// "update-json-spec --spec", so the keys around the section must not reject it.
func TestLoadConfigSectionIgnoresTheRestOfTheDefinition(t *testing.T) {
	path := writeConfig(t, `{
	  "identity": "api-journey",
	  "toolType": "Locust",
	  "infraIdentifier": "perf-cluster",
	  "jsonSpec": {"host": "https://api.example.com", "endpoints": ["/v1/orders"]}
	}`)

	var spec endpointSpec
	if err := LoadConfigSection(path, "jsonSpec", &spec); err != nil {
		t.Fatalf("LoadConfigSection: %v", err)
	}
	if spec.Host != "https://api.example.com" || len(spec.Endpoints) != 1 {
		t.Fatalf("read the wrong section: %+v", spec)
	}
}

// A file holding nothing but the section is the other way it is written, and
// has to keep working.
func TestLoadConfigSectionAcceptsTheBareSection(t *testing.T) {
	path := writeConfig(t, `{"host": "https://api.example.com", "endpoints": ["/v1/orders"]}`)

	var spec endpointSpec
	if err := LoadConfigSection(path, "jsonSpec", &spec); err != nil {
		t.Fatalf("LoadConfigSection: %v", err)
	}
	if spec.Host != "https://api.example.com" {
		t.Fatalf("bare section was not read: %+v", spec)
	}
}

// Ignoring the fields around the section must not become ignoring the ones
// inside it, which would upload a spec that is not the one on disk.
func TestLoadConfigSectionStillRejectsATypoInsideTheSection(t *testing.T) {
	path := writeConfig(t, `{
	  "identity": "api-journey",
	  "jsonSpec": {"hsot": "https://api.example.com"}
	}`)

	var spec endpointSpec
	err := LoadConfigSection(path, "jsonSpec", &spec)
	if err == nil {
		t.Fatal("a mistyped field inside the section was accepted")
	}
	if !strings.Contains(err.Error(), "hsot") {
		t.Fatalf("the error does not name the offending key: %v", err)
	}
	if !strings.Contains(err.Error(), "jsonSpec") {
		t.Fatalf("the error does not say which section it was in: %v", err)
	}
}

// A whole definition with no section of that name is neither the section nor a
// document holding it, and saying which key is missing beats saying nothing.
func TestLoadConfigSectionReportsADocumentThatHasNoSection(t *testing.T) {
	path := writeConfig(t, `{"identity": "api-journey", "toolType": "Locust"}`)

	var spec endpointSpec
	err := LoadConfigSection(path, "jsonSpec", &spec)
	if err == nil {
		t.Fatal("a document with no section was accepted")
	}
	if !strings.Contains(err.Error(), "identity") {
		t.Fatalf("the error does not show what was found instead: %v", err)
	}
}

// A file has to be readable as a document before a section can be looked for
// in it, and the complaint has to name the file: a hand-edited config with a
// trailing comma is the common case, and it is not obvious which one broke.
func TestLoadConfigSectionReportsAFileThatIsNotJSON(t *testing.T) {
	path := writeConfig(t, `{"jsonSpec": {"host": "https://api.example.com",}}`)

	var spec endpointSpec
	err := LoadConfigSection(path, "jsonSpec", &spec)
	if err == nil {
		t.Fatal("a malformed document was accepted")
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("the error does not name the file: %v", err)
	}
}

// decodeHint only rewrites the unknown-field message. Anything else has to
// survive untouched, or a type mismatch would lose its explanation.
func TestDecodeHintLeavesAnErrorItDoesNotRecogniseAlone(t *testing.T) {
	path := writeConfig(t, `{"host": 42}`)

	var spec endpointSpec
	err := LoadConfigFile(path, &spec)
	if err == nil {
		t.Fatal("a string field given a number was accepted")
	}
	if !strings.Contains(err.Error(), "host") {
		t.Fatalf("the type mismatch lost its detail: %v", err)
	}
}

// LoadConfigFile is the strict path, used where the file is the whole request.
func TestLoadConfigFileRejectsAnUnknownField(t *testing.T) {
	path := writeConfig(t, `{"host": "https://api.example.com", "hosts": []}`)

	var spec endpointSpec
	err := LoadConfigFile(path, &spec)
	if err == nil {
		t.Fatal("an unknown field was accepted")
	}
	if !strings.Contains(err.Error(), "hosts") {
		t.Fatalf("the error does not name the offending key: %v", err)
	}
}
