package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// AuthConfig represents authentication configuration
type AuthConfig struct {
	BaseURL   string `json:"base_url"`
	Token     string `json:"token"`
	AccountID string `json:"account_id"`
	OrgID     string `json:"org_id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

// getAuthConfigPath returns the path to the auth config file
func getAuthConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Error getting home directory: %v\n", err)
		os.Exit(1)
	}

	configDir := filepath.Join(homeDir, ".harness")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		fmt.Printf("Error creating config directory: %v\n", err)
		os.Exit(1)
	}

	return filepath.Join(configDir, "auth.json")
}

func loadAuthConfig() (*AuthConfig, error) {
	configPath := getAuthConfigPath()

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var config AuthConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("error unmarshaling auth config: %w", err)
	}

	return &config, nil
}
