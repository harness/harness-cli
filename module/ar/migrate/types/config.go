package types

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"strings"
)

type RegistryType string

var (
	HAR   RegistryType = "HAR"
	JFROG RegistryType = "JFROG"
)

type ArtifactType string

var (
	DOCKER  ArtifactType = "DOCKER"
	HELM    ArtifactType = "HELM"
	GENERIC ArtifactType = "GENERIC"
	PYTHON  ArtifactType = "PYTHON"
	MAVEN   ArtifactType = "MAVEN"
	NPM     ArtifactType = "NPM"
	NUGET   ArtifactType = "NUGET"
	RPM     ArtifactType = "RPM"
)

// Config represents the top-level configuration structure
type Config struct {
	Version   string             `yaml:"version"`
	Migration MigrationConfig    `yaml:"migration"`
	Source    RegistryConfig     `yaml:"source"`
	Dest      RegistryConfig     `yaml:"destination"`
	Mappings  []RegistryOverride `yaml:"overrides"`
	Filters   FiltersConfig      `yaml:"filters"`
}

// MigrationConfig contains settings for a migration process
type MigrationConfig struct {
	DryRun      bool   `yaml:"dryRun"`
	Concurrency int    `yaml:"concurrency"`
	FailureMode string `yaml:"failureMode"`
}

// RegistryConfig defines the source ar configuration
type RegistryConfig struct {
	Endpoint    string            `yaml:"endpoint"`
	Type        RegistryType      `yaml:"type"`
	Credentials CredentialsConfig `yaml:"credentials"`
}

// FiltersConfig defines filters for source artifacts. This will be an intersection of all the properties
type FiltersConfig struct {
	Registries   []string     `yaml:"registries"`
	ArtifactType ArtifactType `yaml:"artifactType"`
}

// RegistryOverride defines the mapping between source and destination registries
// Slashes are used to defined the scope. The format would be
// - "registry": Create registry at Account level
// - "org/registry": Create registry at Org level
// - "org/project/registry": Create registry at Project level
type RegistryOverride struct {
	SourceRegistry       string `yaml:"sourceRegistry"`
	DestinationRegistry  string `yaml:"destinationRegistry"`
	ArtifactNamePatterns struct {
		Include []string `yaml:"include"`
		Exclude []string `yaml:"exclude"`
	} `yaml:"artifactNamePatterns"`
}

// CredentialsConfig defines the credential configuration
type CredentialsConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password,omitempty"`
	Token    string `yaml:"token,omitempty"`
}

// LoadConfig loads the configuration from a file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	// Expand environment variables in the file
	expandedData := expandEnvInYaml(string(data))

	var config Config
	if err := yaml.Unmarshal([]byte(expandedData), &config); err != nil {
		return nil, fmt.Errorf("error parsing config file: %w", err)
	}

	// Validate the configuration
	if err := validateConfig(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// expandEnvInYaml expands environment variables in YAML content
func expandEnvInYaml(content string) string {
	// Process ${VAR} style environment variables
	result := os.Expand(content, func(key string) string {
		return os.Getenv(key)
	})

	return result
}

// validateConfig performs basic validation on the configuration
func validateConfig(config *Config) error {
	// Check migration configuration
	if config.Migration.Concurrency <= 0 {
		return fmt.Errorf("concurrency must be greater than 0")
	}

	switch strings.ToLower(config.Migration.FailureMode) {
	case "continue", "stop":
		// Valid values
	default:
		return fmt.Errorf("invalid failure mode: %s, must be 'continue' or 'stop'", config.Migration.FailureMode)
	}

	// Validate source and destination registry configurations
	if err := validateCredentials(config.Source); err != nil {
		return fmt.Errorf("invalid source credentials block provided in config: %w", err)
	}

	if err := validateCredentials(config.Dest); err != nil {
		return fmt.Errorf("invalid destination credentials block provided in config: %w", err)
	}

	// Validate registry mappings
	if len(config.Mappings) == 0 {
		return fmt.Errorf("at least one registry mapping must be defined")
	}

	// Validate each mapping
	for i, mapping := range config.Mappings {
		if mapping.SourceRegistry == "" {
			return fmt.Errorf("mapping %d: source registry cannot be empty", i)
		}
		if mapping.DestinationRegistry == "" {
			return fmt.Errorf("mapping %d: destination registry cannot be empty", i)
		}
	}

	// Validate filters if specified
	if len(config.Filters.Registries) > 0 {
		for i, registry := range config.Filters.Registries {
			if registry == "" {
				return fmt.Errorf("filter registry %d cannot be empty", i)
			}
		}
	}

	if len(config.Filters.ArtifactType) == 0 {
		return fmt.Errorf("filter artifact type cannot be empty")
	}

	return nil
}

func validateCredentials(registry RegistryConfig) error {
	// Check that the endpoint is not empty
	if registry.Endpoint == "" {
		return fmt.Errorf("registry endpoint cannot be empty")
	}

	// Validate registry type
	if registry.Type == "" {
		return fmt.Errorf("registry type cannot be empty")
	}

	// Check supported registry types
	switch registry.Type {
	case HAR, JFROG:
		// These are supported
	default:
		return fmt.Errorf("unsupported registry type: %s", registry.Type)
	}

	// Validate credentials
	// Authentication must be provided via either token or username
	hasToken := registry.Credentials.Token != ""
	hasUsername := registry.Credentials.Username != ""

	if !hasToken && !hasUsername {
		return fmt.Errorf("either token or username must be provided for authentication")
	}

	// If using username auth, password should also be provided
	if hasUsername && registry.Credentials.Password == "" {
		return fmt.Errorf("password must be provided when using username authentication")
	}

	return nil
}
