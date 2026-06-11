package command

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	HarnessConfigDir = ".harness"
	MavenConfigFile  = "maven-config.json"
	PipConfigFile    = "pip-config.json"
	NugetConfigFile  = "nuget-config.json"
)

func harnessConfigPath(filename string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, HarnessConfigDir, filename), nil
}

func ensureHarnessConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	configDir := filepath.Join(homeDir, HarnessConfigDir)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", err
	}
	return configDir, nil
}

func saveRegistryConfig(filename string, cfg interface{}) error {
	configDir, err := ensureHarnessConfigDir()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(configDir, filename), data, 0600)
}

func loadRegistryConfig(filename string, cfg interface{}) error {
	path, err := harnessConfigPath(filename)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, cfg)
}

// BackupFile backs up a file to ~/.harness/<prefix>-backup-<timestamp>.
// Returns empty string if the file doesn't exist or is empty.
func BackupFile(filePath, prefix string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read existing file: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return "", nil
	}

	backupDir, err := ensureHarnessConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

	backupPath := filepath.Join(backupDir, fmt.Sprintf("%s-backup-%s", prefix, time.Now().Format("20060102-150405")))
	if err := os.WriteFile(backupPath, data, 0600); err != nil {
		return "", fmt.Errorf("failed to write backup: %w", err)
	}

	return backupPath, nil
}

// RestoreFromBackup copies backup file contents back to the target path.
func RestoreFromBackup(backupPath, targetPath string) error {
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("failed to read backup: %w", err)
	}
	return os.WriteFile(targetPath, data, 0600)
}
