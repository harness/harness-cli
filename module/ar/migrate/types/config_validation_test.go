package types

import (
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
