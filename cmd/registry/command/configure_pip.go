package command

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	client2 "github.com/harness/harness-cli/util/client"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// PipRegistryConfig stores pip registry configuration in ~/.harness/pip-config.json.
type PipRegistryConfig struct {
	RegistryIdentifier string `json:"registryIdentifier"`
	RegistryURL        string `json:"registryUrl"`
	OrgID              string `json:"orgId,omitempty"`
	ProjectID          string `json:"projectId,omitempty"`
	PipConfBackupPath  string `json:"pipConfBackupPath,omitempty"`
	PipConfPath        string `json:"pipConfPath"`
}

func NewConfigurePipCmd(f *cmdutils.Factory) *cobra.Command {
	var registryIdentifier string
	var token string
	var global bool
	var projectLevel bool

	cmd := &cobra.Command{
		Use:   "pip",
		Short: "Configure pip client for Harness Artifact Registry",
		Long:  "Configure pip client to use a Harness Artifact Registry virtual PyPI registry",
		RunE: func(cmd *cobra.Command, args []string) error {
			progress := p.NewConsoleReporter()

			progress.Start("Validating input parameters")
			if registryIdentifier == "" {
				progress.Error("Registry identifier is required")
				return fmt.Errorf("--registry flag is required")
			}

			if global && projectLevel {
				progress.Error("Cannot use both --global and --project-level flags")
				return fmt.Errorf("cannot use both --global and --project-level flags")
			}
			if !global && !projectLevel {
				projectLevel = true
			}
			progress.Success("Input parameters validated")

			progress.Start("Loading configuration")
			accountID := config.Global.AccountID
			authToken := token
			if authToken == "" {
				authToken = config.Global.AuthToken
			}
			if accountID == "" {
				progress.Error("Account ID not configured")
				return fmt.Errorf("account ID not configured, please run 'hc auth login' first")
			}
			if authToken == "" {
				progress.Error("Auth token not configured")
				return fmt.Errorf("auth token not configured, please run 'hc auth login' first or provide --token flag")
			}
			progress.Success("Configuration loaded")

			// Fetch registry details
			progress.Start("Fetching registry details")
			org := config.Global.OrgID
			project := config.Global.ProjectID
			registryRef := client2.GetRef(accountID, org, project) + "/" + registryIdentifier

			registryResp, err := f.RegistryHttpClient().GetRegistryWithResponse(context.Background(), registryRef)
			if err != nil {
				progress.Error("Failed to fetch registry details")
				return fmt.Errorf("failed to fetch registry details: %w", err)
			}
			if registryResp.StatusCode() != 200 || registryResp.JSON200 == nil {
				progress.Error(fmt.Sprintf("Registry '%s' not found (status: %d)", registryIdentifier, registryResp.StatusCode()))
				return fmt.Errorf("registry '%s' not found (status: %d)", registryIdentifier, registryResp.StatusCode())
			}

			pkgBaseURL, err := getRegistryBaseURL(accountID, authToken)
			if err != nil {
				progress.Error(fmt.Sprintf("Failed to get registry base URL: %s", err))
				return fmt.Errorf("failed to get registry base URL: %w", err)
			}

			registryURL := fmt.Sprintf("%s/pkg/%s/%s/pypi/simple", pkgBaseURL, accountID, registryIdentifier)
			progress.Success(fmt.Sprintf("Registry URL: %s", registryURL))

			// Determine pip.conf path
			var pipConfPath string
			if global {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("failed to get home directory: %w", err)
				}
				pipConfPath = filepath.Join(homeDir, ".pip", "pip.conf")
			} else {
				cwd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("failed to get current directory: %w", err)
				}
				// Check requirements file exists for project-level
				hasReqs := false
				for _, f := range []string{"requirements.txt", "pyproject.toml", "setup.py", "Pipfile"} {
					if _, err := os.Stat(filepath.Join(cwd, f)); err == nil {
						hasReqs = true
						break
					}
				}
				if !hasReqs {
					progress.Error("No Python project files found in current directory")
					return fmt.Errorf("no requirements.txt, pyproject.toml, setup.py, or Pipfile found. Use --global flag to configure globally")
				}
				pipConfPath = filepath.Join(cwd, "pip.conf")
			}

			// Backup existing pip.conf
			progress.Start("Backing up existing pip.conf")
			backupPath, err := backupPipConf(pipConfPath)
			if err != nil {
				progress.Error(fmt.Sprintf("Failed to backup pip.conf: %s", err))
				return fmt.Errorf("failed to backup pip.conf: %w", err)
			}
			if backupPath != "" {
				progress.Success(fmt.Sprintf("Backed up existing pip.conf to %s", backupPath))
			} else {
				progress.Success("No existing pip.conf to backup")
			}

			// Write pip.conf
			progress.Start("Configuring pip")
			if err := writePipConf(pipConfPath, registryURL, authToken); err != nil {
				progress.Error("Failed to configure pip")
				return fmt.Errorf("failed to configure pip: %w", err)
			}

			// Save config
			if err := savePipRegistryConfig(PipRegistryConfig{
				RegistryIdentifier: registryIdentifier,
				RegistryURL:        registryURL,
				OrgID:              org,
				ProjectID:          project,
				PipConfBackupPath:  backupPath,
				PipConfPath:        pipConfPath,
			}); err != nil {
				log.Warn().Err(err).Msg("Failed to save pip registry config")
			}

			if global {
				progress.Success(fmt.Sprintf("Successfully configured pip globally to use registry %s", registryURL))
			} else {
				progress.Success(fmt.Sprintf("Successfully configured pip at project level to use registry %s", registryURL))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&registryIdentifier, "registry", "", "Registry identifier (required)")
	cmd.Flags().StringVar(&token, "token", "", "Auth token (optional, uses token from login if not provided)")
	cmd.Flags().BoolVar(&global, "global", false, "Configure globally (~/.pip/pip.conf)")
	cmd.Flags().BoolVar(&projectLevel, "project-level", false, "Configure at project level (pip.conf in current directory)")

	return cmd
}

// backupPipConf backs up an existing pip.conf to ~/.harness/pip-conf-backup.
func backupPipConf(pipConfPath string) (string, error) {
	data, err := os.ReadFile(pipConfPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read existing pip.conf: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return "", nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	backupDir := filepath.Join(homeDir, ".harness")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", err
	}

	backupPath := filepath.Join(backupDir, "pip-conf-backup")
	if _, err := os.Stat(backupPath); err == nil {
		log.Info().Str("backupPath", backupPath).Msg("Backup already exists, skipping")
		return backupPath, nil
	}

	if err := os.WriteFile(backupPath, data, 0600); err != nil {
		return "", err
	}
	return backupPath, nil
}

// writePipConf writes a pip.conf with HAR registry as index-url with embedded auth.
func writePipConf(pipConfPath, registryURL, authToken string) error {
	// pip supports auth in the URL: https://user:token@host/path
	parsedURL, err := url.Parse(registryURL)
	if err != nil {
		return fmt.Errorf("invalid registry URL: %w", err)
	}
	parsedURL.User = url.UserPassword("harness", authToken)
	authedURL := parsedURL.String()

	content := fmt.Sprintf("[global]\nindex-url = %s\ntrusted-host = %s\n",
		authedURL, parsedURL.Hostname())

	dir := filepath.Dir(pipConfPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	if err := os.WriteFile(pipConfPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write pip.conf: %w", err)
	}

	return nil
}

func savePipRegistryConfig(cfg PipRegistryConfig) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	configDir := filepath.Join(homeDir, ".harness")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(configDir, "pip-config.json"), data, 0600)
}

// LoadPipRegistryConfig loads the saved pip registry config.
func LoadPipRegistryConfig() (*PipRegistryConfig, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(homeDir, ".harness", "pip-config.json"))
	if err != nil {
		return nil, err
	}
	var cfg PipRegistryConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
