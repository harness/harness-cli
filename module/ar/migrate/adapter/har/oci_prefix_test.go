package har

import (
	"encoding/json"
	"testing"

	"github.com/harness/harness-cli/internal/api/ar"
)

// buildDetails constructs a ClientSetupDetails whose single INLINE section
// contains one step with the given command values.
func buildDetails(t *testing.T, commands ...string) ar.ClientSetupDetails {
	t.Helper()

	cmds := make([]ar.ClientSetupStepCommand, len(commands))
	for i, v := range commands {
		v := v
		cmds[i] = ar.ClientSetupStepCommand{Value: &v}
	}
	step := ar.ClientSetupStep{Commands: &cmds}
	cfg := ar.ClientSetupStepConfig{Steps: &[]ar.ClientSetupStep{step}}

	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal ClientSetupStepConfig: %v", err)
	}

	section := ar.ClientSetupSection{}
	if err := section.FromClientSetupStepConfig(cfg); err != nil {
		t.Fatalf("FromClientSetupStepConfig: %v", err)
	}
	_ = raw

	return ar.ClientSetupDetails{Sections: []ar.ClientSetupSection{section}}
}

// --- ociPrefixFromCommand ---

func TestOCIPrefixFromCommand_VanityURL(t *testing.T) {
	cmd := "helm push mychart-0.1.0.tgz oci://har-automation.harness.io/oci/helm-oci"
	prefix, ok := ociPrefixFromCommand(cmd, "helm-oci")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if prefix != "oci" {
		t.Fatalf("expected prefix 'oci', got %q", prefix)
	}
}

func TestOCIPrefixFromCommand_AccountIDURL(t *testing.T) {
	cmd := "helm push mychart-0.1.0.tgz oci://har-automation.harness.io/mty0othiyzytnzc4mi00n2/helm-oci"
	prefix, ok := ociPrefixFromCommand(cmd, "helm-oci")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if prefix != "mty0othiyzytnzc4mi00n2" {
		t.Fatalf("expected accountID prefix, got %q", prefix)
	}
}

func TestOCIPrefixFromCommand_NoOCIScheme(t *testing.T) {
	cmd := "helm pull https://charts.example.com/nginx-1.0.0.tgz"
	_, ok := ociPrefixFromCommand(cmd, "helm-oci")
	if ok {
		t.Fatal("expected ok=false for non-oci command")
	}
}

func TestOCIPrefixFromCommand_WrongRegistry(t *testing.T) {
	cmd := "helm push mychart.tgz oci://har-automation.harness.io/oci/other-registry"
	_, ok := ociPrefixFromCommand(cmd, "helm-oci")
	if ok {
		t.Fatal("expected ok=false when registry name does not match")
	}
}

func TestOCIPrefixFromCommand_StripsTrailingArgs(t *testing.T) {
	// Some command strings include the chart name after the registry path.
	cmd := "helm push mychart-0.1.0.tgz oci://har-automation.harness.io/oci/helm-oci --insecure-skip-tls-verify"
	prefix, ok := ociPrefixFromCommand(cmd, "helm-oci")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if prefix != "oci" {
		t.Fatalf("expected 'oci', got %q", prefix)
	}
}

// --- extractOCIPrefix ---

func TestExtractOCIPrefix_VanityURL(t *testing.T) {
	details := buildDetails(t,
		"helm registry login har-automation.harness.io --username <USERNAME> --password <PASSWORD>",
		"helm push mychart-0.1.0.tgz oci://har-automation.harness.io/oci/helm-oci",
	)
	prefix, err := extractOCIPrefix(details, "MTY0OThiYzYtNzc4Mi00N2/helm-oci")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prefix != "oci" {
		t.Fatalf("expected 'oci', got %q", prefix)
	}
}

func TestExtractOCIPrefix_AccountIDURL(t *testing.T) {
	details := buildDetails(t,
		"helm push mychart-0.1.0.tgz oci://har-automation.harness.io/mty0othiyzytnzc4mi00n2/helm-oci",
	)
	prefix, err := extractOCIPrefix(details, "MTY0OThiYzYtNzc4Mi00N2/helm-oci")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prefix != "mty0othiyzytnzc4mi00n2" {
		t.Fatalf("expected accountID, got %q", prefix)
	}
}

func TestExtractOCIPrefix_NoMatchReturnsError(t *testing.T) {
	details := buildDetails(t,
		"helm registry login har-automation.harness.io --username u --password p",
	)
	_, err := extractOCIPrefix(details, "acct/helm-oci")
	if err == nil {
		t.Fatal("expected error when no oci:// command present")
	}
}

func TestExtractOCIPrefix_EmptySections(t *testing.T) {
	details := ar.ClientSetupDetails{}
	_, err := extractOCIPrefix(details, "acct/helm-oci")
	if err == nil {
		t.Fatal("expected error for empty sections")
	}
}
