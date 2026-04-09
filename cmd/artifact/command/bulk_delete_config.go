package command

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type BulkDeleteConfig struct {
	Version     string                         `yaml:"version"`
	Concurrency int                            `yaml:"concurrency"`
	OrgID       string                         `yaml:"org_id"`
	ProjectID   string                         `yaml:"project_id"`
	Registries  map[string]map[string][]string `yaml:"registries"`
}

func LoadBulkDeleteConfig(path string) (*BulkDeleteConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	expandedData := os.Expand(string(data), func(key string) string {
		return os.Getenv(key)
	})

	var config BulkDeleteConfig
	if err := yaml.Unmarshal([]byte(expandedData), &config); err != nil {
		return nil, fmt.Errorf("error parsing config file: %w", err)
	}

	if err := validateBulkDeleteConfig(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

func validateBulkDeleteConfig(config *BulkDeleteConfig) error {
	if config.Concurrency <= 0 {
		return fmt.Errorf("concurrency must be greater than 0")
	}

	if len(config.Registries) == 0 {
		return fmt.Errorf("at least one registry must be defined")
	}

	return nil
}
