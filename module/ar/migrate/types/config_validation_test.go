package types

import (
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

func TestValidateConfig_MavenWithDateFilterFails(t *testing.T) {
	config := baseValidConfig()
	after := time.Unix(0, 0)
	config.Mappings[0].DateFilter = &DateFilter{
		Match:        DateFilterMatchAny,
		CreatedAfter: &after,
	}

	err := validateConfig(config)
	if err == nil {
		t.Fatal("expected error for MAVEN mapping with date filter, got nil")
	}
	if !strings.Contains(err.Error(), "date filter is not supported") {
		t.Fatalf("expected 'date filter is not supported' error, got: %v", err)
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
