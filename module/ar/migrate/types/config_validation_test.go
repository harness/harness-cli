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
		SWIFT, PUPPET, RUBY, CONAN, TERRAFORM,
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
