package command

import (
	"context"
	"fmt"
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

type PipRegistryConfig struct {
	RegistryIdentifier string `json:"registryIdentifier"`
	RegistryURL        string `json:"registryUrl"`
	OrgID              string `json:"orgId,omitempty"`
	ProjectID          string `json:"projectId,omitempty"`
	PipConfBackupPath  string `json:"pipConfBackupPath,omitempty"`
	PipConfPath        string `json:"pipConfPath"`
}

func SavePipRegistryConfig(cfg PipRegistryConfig) error {
	return saveRegistryConfig(PipConfigFile, cfg)
}

func LoadPipRegistryConfig() (*PipRegistryConfig, error) {
	var cfg PipRegistryConfig
	if err := loadRegistryConfig(PipConfigFile, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// RestorePipConf restores the backed-up pip.conf file if a backup exists.
func RestorePipConf() error {
	cfg, err := LoadPipRegistryConfig()
	if err != nil {
		return nil
	}

	if cfg.PipConfBackupPath == "" || cfg.PipConfPath == "" {
		return nil
	}

	backupData, err := os.ReadFile(cfg.PipConfBackupPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return os.WriteFile(cfg.PipConfPath, backupData, 0600)
}

func NewConfigurePipCmd(f *cmdutils.Factory) *cobra.Command {
	var registryIdentifier string
	var token string
	var global bool

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
				return fmt.Errorf("auth token not configured")
			}
			progress.Success("Configuration loaded")

			progress.Start("Fetching registry details")
			org := config.Global.OrgID
			project := config.Global.ProjectID
			registryRef := client2.GetRef(accountID, org, project, registryIdentifier)
			log.Info().Str("registryRef", registryRef).Msg("Fetching registry")

			registryResp, err := f.RegistryHttpClient().GetRegistryWithResponse(context.Background(), registryRef)
			if err != nil {
				progress.Error("Failed to fetch registry details")
				return fmt.Errorf("failed to fetch registry details: %w", err)
			}
			if registryResp.StatusCode() != 200 || registryResp.JSON200 == nil {
				progress.Error(fmt.Sprintf("Registry '%s' not found (status: %d)", registryIdentifier, registryResp.StatusCode()))
				return fmt.Errorf("registry '%s' not found (status: %d)", registryIdentifier, registryResp.StatusCode())
			}

			pkgBaseURL, err := getRegistryBaseURL(f, accountID)
			if err != nil {
				progress.Error(fmt.Sprintf("Failed to get registry base URL: %s", err))
				return fmt.Errorf("failed to get registry base URL: %w", err)
			}

			registryURL := fmt.Sprintf("%s/pkg/%s/%s/pypi/simple/", pkgBaseURL, accountID, registryIdentifier)
			progress.Success(fmt.Sprintf("Registry URL: %s", registryURL))

			// Determine pip.conf path
			homeDir, err := os.UserHomeDir()
			if err != nil {
				progress.Error("Failed to get home directory")
				return fmt.Errorf("failed to get home directory: %w", err)
			}

			var pipConfPath string
			if global {
				pipConfDir := filepath.Join(homeDir, ".config", "pip")
				if err := os.MkdirAll(pipConfDir, 0755); err != nil {
					progress.Error("Failed to create pip config directory")
					return fmt.Errorf("failed to create pip config directory: %w", err)
				}
				pipConfPath = filepath.Join(pipConfDir, "pip.conf")
			} else {
				pipConfPath = "pip.conf"
			}

			// Backup existing pip.conf
			progress.Start("Backing up existing pip.conf")
			backupPath, err := BackupFile(pipConfPath, "pip-conf")
			if err != nil {
				progress.Error(fmt.Sprintf("Failed to backup pip.conf: %s", err))
				return fmt.Errorf("failed to backup pip.conf: %w", err)
			}
			if backupPath != "" {
				progress.Success(fmt.Sprintf("Backed up existing pip.conf to %s", backupPath))
			} else {
				progress.Success("No existing pip.conf to backup")
			}

			// Write new pip.conf
			progress.Start("Configuring pip")
			if err := writePipConf(pipConfPath, registryURL, authToken); err != nil {
				progress.Error("Failed to configure pip")
				// Restore backup on failure
				if backupPath != "" {
					if restoreErr := RestoreFromBackup(backupPath, pipConfPath); restoreErr != nil {
						log.Error().Err(restoreErr).Msg("Failed to restore pip.conf from backup")
					} else {
						progress.Step("Restored original pip.conf from backup")
					}
				}
				return fmt.Errorf("failed to configure pip: %w", err)
			}

			// Save registry config with backup path for future restore
			if err := SavePipRegistryConfig(PipRegistryConfig{
				RegistryIdentifier: registryIdentifier,
				RegistryURL:        registryURL,
				OrgID:              org,
				ProjectID:          project,
				PipConfBackupPath:  backupPath,
				PipConfPath:        pipConfPath,
			}); err != nil {
				log.Warn().Err(err).Msg("Failed to save pip registry config")
			}

			progress.Success(fmt.Sprintf("Successfully configured pip to use registry %s", registryURL))
			return nil
		},
	}

	cmd.Flags().StringVar(&registryIdentifier, "registry", "", "Registry identifier (required)")
	cmd.Flags().StringVar(&token, "token", "", "Auth token (optional)")
	cmd.Flags().BoolVar(&global, "global", true, "Configure globally (default: true)")

	return cmd
}

func writePipConf(pipConfPath, registryURL, authToken string) error {
	// Embed token in URL for pip authentication
	authedURL := strings.Replace(registryURL, "://", fmt.Sprintf("://harness:%s@", authToken), 1)
	content := fmt.Sprintf("[global]\nindex-url = %s\n", authedURL)

	// Atomic write
	dir := filepath.Dir(pipConfPath)
	tmpFile, err := os.CreateTemp(dir, ".pip-conf-tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write([]byte(content)); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write pip.conf content: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to set permissions: %w", err)
	}
	if err := os.Rename(tmpPath, pipConfPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write pip.conf: %w", err)
	}

	return nil
}
