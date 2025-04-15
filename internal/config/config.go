package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the top-level configuration structure
type Config struct {
	Version   string           `yaml:"version"`
	Migration MigrationConfig  `yaml:"migration"`
	Source    SourceConfig     `yaml:"source"`
	Dest      DestinationConfig `yaml:"destination"`
}

// MigrationConfig contains settings for migration process
type MigrationConfig struct {
	DryRun       bool   `yaml:"dryRun"`
	Concurrency  int    `yaml:"concurrency"`
	FailureMode  string `yaml:"failureMode"`
}

// SourceConfig defines the source registry configuration
type SourceConfig struct {
	Type        string              `yaml:"type"`
	Endpoint    string              `yaml:"endpoint"`
	Credentials CredentialsConfig   `yaml:"credentials"`
	Filters     SourceFiltersConfig `yaml:"filters"`
}

// SourceFiltersConfig defines filters for source artifacts
type SourceFiltersConfig struct {
	Registries           []string `yaml:"registries"`
	ArtifactNamePatterns struct {
		Include []string `yaml:"include"`
		Exclude []string `yaml:"exclude"`
	} `yaml:"artifactNamePatterns"`
	ArtifactTypes []string `yaml:"artifactTypes"`
}

// DestinationConfig defines the destination registry configuration
type DestinationConfig struct {
	Type             string             `yaml:"type"`
	AccountIdentifier string            `yaml:"accountIdentifier"`
	RegistryEndpoint string            `yaml:"registryEndpoint"`
	Credentials      CredentialsConfig  `yaml:"credentials"`
	Mappings         []RegistryMapping  `yaml:"mappings"`
}

// RegistryMapping defines the mapping between source and destination registries
type RegistryMapping struct {
	SourceRegistry      string `yaml:"sourceRegistry"`
	DestinationRegistry string `yaml:"destinationRegistry"`
}

// CredentialsConfig defines the credentials configuration
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

	// Check source configuration
	if config.Source.Endpoint == "" {
		return fmt.Errorf("source endpoint must be specified")
	}

	// Check destination configuration
	if config.Dest.RegistryEndpoint == "" {
		return fmt.Errorf("destination registry endpoint must be specified")
	}

	// Additional validation can be added here as needed

	return nil
}
