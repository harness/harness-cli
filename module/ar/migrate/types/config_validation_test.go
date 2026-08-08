package types

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// baseValidConfig returns a minimal Config that passes validateConfig, so tests
// can tweak a single field to exercise one rule at a time.
func baseValidConfig() *Config {
	cred := CredentialsConfig{Username: "user", Password: "pass"}
	return &Config{
		Concurrency: 1,
		Source:      RegistryConfig{Endpoint: "https://src.example", Type: JFROG, Credentials: cred},
		Dest:        RegistryConfig{Endpoint: "https://dst.example", Type: HAR, Credentials: cred},
		Mappings: []RegistryMapping{
			{
				ArtifactType:        MAVEN,
				SourceRegistry:      "src",
				DestinationRegistry: "dst",
			},
		},
	}
}

func TestValidateConfig_MavenWithDateFilterWarnsButPasses(t *testing.T) {
	config := baseValidConfig()
	after := time.Unix(0, 0)
	config.Mappings[0].DateFilter = &DateFilter{
		Match:        DateFilterMatchAny,
		CreatedAfter: &after,
	}

	if err := validateConfig(config); err != nil {
		t.Fatalf("expected MAVEN mapping with date filter to pass with a warning, got: %v", err)
	}
}

func TestValidateConfig_MavenWithoutDateFilterOK(t *testing.T) {
	config := baseValidConfig()

	if err := validateConfig(config); err != nil {
		t.Fatalf("expected MAVEN mapping without date filter to pass, got: %v", err)
	}
}

func TestValidateConfig_NonMavenWithDateFilterOK(t *testing.T) {
	config := baseValidConfig()
	config.Mappings[0].ArtifactType = PYTHON
	after := time.Unix(0, 0)
	config.Mappings[0].DateFilter = &DateFilter{
		Match:        DateFilterMatchAny,
		CreatedAfter: &after,
	}

	if err := validateConfig(config); err != nil {
		t.Fatalf("expected PYTHON mapping with date filter to pass, got: %v", err)
	}
}

func TestValidateConfig_UnknownArtifactTypeRejected(t *testing.T) {
	config := baseValidConfig()
	config.Mappings[0].ArtifactType = ArtifactType("NOTAREALTYPE")

	err := validateConfig(config)
	if err == nil {
		t.Fatal("expected error for unknown artifactType, got nil")
	}
}

func TestValidateConfig_AllKnownArtifactTypesAccepted(t *testing.T) {
	known := []ArtifactType{
		DOCKER, HELM, HELM_LEGACY, HELM_HTTP, GENERIC, PYTHON, MAVEN,
		NPM, NUGET, RPM, DEBIAN, GO, CONDA, COMPOSER, DART, RAW,
		SWIFT, PUPPET, CONAN, TERRAFORM,
	}
	for _, at := range known {
		config := baseValidConfig()
		config.Mappings[0].ArtifactType = at
		if err := validateConfig(config); err != nil {
			t.Errorf("expected known artifactType %q to pass, got: %v", at, err)
		}
	}
}

func TestValidateConfig_EmptyArtifactTypeRejected(t *testing.T) {
	config := baseValidConfig()
	config.Mappings[0].ArtifactType = ArtifactType("")

	err := validateConfig(config)
	if err == nil {
		t.Fatal("expected error for empty artifactType, got nil")
	}
}

// TestLoadConfig_UnknownArtifactTypeFailsAtLoad verifies §1-W3: a typo'd
// artifactType fails at config LOAD time — i.e. before NewMigrationService and
// therefore before any source API call — and the error lists the valid values
// (including TERRAFORM).
func TestLoadConfig_UnknownArtifactTypeFailsAtLoad(t *testing.T) {
	yaml := `
version: 1.0.0
concurrency: 1
source:
  endpoint: https://src.example
  type: JFROG
  credentials: {username: u, password: p}
destination:
  endpoint: https://dst.example
  type: HAR
  credentials: {username: u, password: p}
mappings:
  - artifactType: NOTAREALTYPE
    sourceRegistry: src
    destinationRegistry: dst
`
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected LoadConfig to reject unknown artifactType, got nil")
	}
	if !strings.Contains(err.Error(), "NOTAREALTYPE") {
		t.Errorf("error should name the offending type, got: %v", err)
	}
	if !strings.Contains(err.Error(), "TERRAFORM") {
		t.Errorf("error should list valid values including TERRAFORM, got: %v", err)
	}
}

// TestValidateConfig_PatternsRejectedForNonFilterableTypes verifies §6-W1:
// include/exclude patterns on types with no pattern support (DEBIAN,
// TERRAFORM, PUPPET) are a config error, not a silent no-op.
func TestValidateConfig_PatternsRejectedForNonFilterableTypes(t *testing.T) {
	for _, at := range []ArtifactType{DEBIAN, TERRAFORM, PUPPET} {
		config := baseValidConfig()
		config.Mappings[0].ArtifactType = at
		config.Mappings[0].IncludePatterns = []string{"foo/**"}

		err := validateConfig(config)
		if err == nil {
			t.Errorf("expected error for includePatterns on %s, got nil", at)
			continue
		}
		if !strings.Contains(err.Error(), "not supported for artifact type") {
			t.Errorf("%s: error should call out unsupported patterns, got: %v", at, err)
		}

		config = baseValidConfig()
		config.Mappings[0].ArtifactType = at
		config.Mappings[0].ExcludePatterns = []string{"foo/**"}
		if err := validateConfig(config); err == nil {
			t.Errorf("expected error for excludePatterns on %s, got nil", at)
		}
	}
}

// TestValidateConfig_PatternsAcceptedForFilterableTypes verifies §6-W1 does
// not over-reject: file-level (GENERIC) and package-level (CONDA) filterable
// types still accept patterns.
func TestValidateConfig_PatternsAcceptedForFilterableTypes(t *testing.T) {
	for _, at := range []ArtifactType{GENERIC, RAW, PYTHON, MAVEN, NUGET, NPM, DART, GO,
		DOCKER, HELM, HELM_LEGACY, HELM_HTTP, RPM, CONDA, COMPOSER, SWIFT, CONAN} {
		config := baseValidConfig()
		config.Mappings[0].ArtifactType = at
		config.Mappings[0].IncludePatterns = []string{"foo/**"}
		if err := validateConfig(config); err != nil {
			t.Errorf("expected patterns on %s to pass, got: %v", at, err)
		}
	}
}

// TestValidateConfig_IndexSeededDateFilterWarnsButPasses verifies §7-W1:
// date filters on index-seeded types (PYTHON, CONDA, RPM) pass validation
// (with a printed warning), mirroring the existing MAVEN behavior.
func TestValidateConfig_IndexSeededDateFilterWarnsButPasses(t *testing.T) {
	after := time.Unix(0, 0)
	for _, at := range []ArtifactType{PYTHON, CONDA, RPM} {
		config := baseValidConfig()
		config.Mappings[0].ArtifactType = at
		config.Mappings[0].DateFilter = &DateFilter{Match: DateFilterMatchAny, CreatedAfter: &after}
		if err := validateConfig(config); err != nil {
			t.Errorf("expected date filter on %s to pass with warning, got: %v", at, err)
		}
	}
}

// TestKnownArtifactTypesSingleSource verifies the exported list and the lookup
// map stay in sync and both include TERRAFORM (§1-W1).
func TestKnownArtifactTypesSingleSource(t *testing.T) {
	list := KnownArtifactTypes()
	if len(list) != len(knownArtifactTypes) {
		t.Fatalf("KnownArtifactTypes() len %d != map len %d", len(list), len(knownArtifactTypes))
	}
	for _, at := range list {
		if !IsKnownArtifactType(at) {
			t.Errorf("KnownArtifactTypes() contains %q but IsKnownArtifactType says false", at)
		}
	}
	if !strings.Contains(KnownArtifactTypesString(), "TERRAFORM") {
		t.Errorf("KnownArtifactTypesString() must include TERRAFORM, got: %s", KnownArtifactTypesString())
	}
}
