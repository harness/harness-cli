package command

import (
	"context"
	"encoding/xml"
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

// MavenRegistryConfig stores maven registry configuration in ~/.harness/maven-config.json.
type MavenRegistryConfig struct {
	RegistryIdentifier string `json:"registryIdentifier"`
	RegistryURL        string `json:"registryUrl"`
	OrgID              string `json:"orgId,omitempty"`
	ProjectID          string `json:"projectId,omitempty"`
	SettingsBackupPath string `json:"settingsBackupPath,omitempty"`
	SettingsPath       string `json:"settingsPath"`
}

func SaveMavenRegistryConfig(cfg MavenRegistryConfig) error {
	return saveRegistryConfig(MavenConfigFile, cfg)
}

func LoadMavenRegistryConfig() (*MavenRegistryConfig, error) {
	var cfg MavenRegistryConfig
	if err := loadRegistryConfig(MavenConfigFile, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// RestoreMavenSettings restores the backed-up settings.xml file if a backup exists.
func RestoreMavenSettings() error {
	cfg, err := LoadMavenRegistryConfig()
	if err != nil {
		return nil
	}

	if cfg.SettingsBackupPath == "" || cfg.SettingsPath == "" {
		return nil
	}

	backupData, err := os.ReadFile(cfg.SettingsBackupPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return os.WriteFile(cfg.SettingsPath, backupData, 0600)
}

func NewConfigureMavenCmd(f *cmdutils.Factory) *cobra.Command {
	var registryIdentifier string
	var token string

	cmd := &cobra.Command{
		Use:   "maven",
		Short: "Configure Maven client for Harness Artifact Registry",
		Long:  "Configure Maven client to use a Harness Artifact Registry virtual Maven registry by updating ~/.m2/settings.xml",
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
				return fmt.Errorf("auth token not configured, please run 'hc auth login' first or provide --token flag")
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

			registryURL := fmt.Sprintf("%s/pkg/%s/%s/maven", pkgBaseURL, accountID, registryIdentifier)
			progress.Success(fmt.Sprintf("Registry URL: %s", registryURL))

			homeDir, err := os.UserHomeDir()
			if err != nil {
				progress.Error("Failed to get home directory")
				return fmt.Errorf("failed to get home directory: %w", err)
			}

			m2Dir := filepath.Join(homeDir, ".m2")
			if err := os.MkdirAll(m2Dir, 0755); err != nil {
				progress.Error("Failed to create .m2 directory")
				return fmt.Errorf("failed to create .m2 directory: %w", err)
			}
			settingsPath := filepath.Join(m2Dir, "settings.xml")

			// Backup existing settings.xml
			progress.Start("Backing up existing settings.xml")
			backupPath, err := BackupFile(settingsPath, "settings-xml")
			if err != nil {
				progress.Error(fmt.Sprintf("Failed to backup settings.xml: %s", err))
				return fmt.Errorf("failed to backup settings.xml: %w", err)
			}
			if backupPath != "" {
				progress.Success(fmt.Sprintf("Backed up existing settings.xml to %s", backupPath))
			} else {
				progress.Success("No existing settings.xml to backup")
			}

			// Write new settings.xml
			progress.Start("Configuring Maven")
			if err := writeMavenSettings(settingsPath, registryIdentifier, registryURL, authToken); err != nil {
				progress.Error("Failed to configure Maven")
				// Restore backup on failure
				if backupPath != "" {
					if restoreErr := RestoreFromBackup(backupPath, settingsPath); restoreErr != nil {
						log.Error().Err(restoreErr).Msg("Failed to restore settings.xml from backup")
					} else {
						progress.Step("Restored original settings.xml from backup")
					}
				}
				return fmt.Errorf("failed to configure Maven: %w", err)
			}

			// Save registry config with backup path for future restore
			if err := SaveMavenRegistryConfig(MavenRegistryConfig{
				RegistryIdentifier: registryIdentifier,
				RegistryURL:        registryURL,
				OrgID:              org,
				ProjectID:          project,
				SettingsBackupPath: backupPath,
				SettingsPath:       settingsPath,
			}); err != nil {
				log.Warn().Err(err).Msg("Failed to save maven registry config")
			}

			progress.Success(fmt.Sprintf("Successfully configured Maven to use registry %s", registryURL))
			return nil
		},
	}

	cmd.Flags().StringVar(&registryIdentifier, "registry", "", "Registry identifier (required)")
	cmd.Flags().StringVar(&token, "token", "", "Auth token (optional, uses token from login if not provided)")

	return cmd
}

func writeMavenSettings(settingsPath, registryID, registryURL, authToken string) error {
	type Server struct {
		XMLName  xml.Name `xml:"server"`
		ID       string   `xml:"id"`
		Username string   `xml:"username"`
		Password string   `xml:"password"`
	}
	type Mirror struct {
		XMLName  xml.Name `xml:"mirror"`
		ID       string   `xml:"id"`
		Name     string   `xml:"name"`
		URL      string   `xml:"url"`
		MirrorOf string   `xml:"mirrorOf"`
	}
	type Settings struct {
		XMLName xml.Name `xml:"settings"`
		Xmlns   string   `xml:"xmlns,attr"`
		Servers struct {
			XMLName xml.Name `xml:"servers"`
			Server  []Server `xml:"server"`
		}
		Mirrors struct {
			XMLName xml.Name `xml:"mirrors"`
			Mirror  []Mirror `xml:"mirror"`
		}
	}

	settings := Settings{
		Xmlns: "http://maven.apache.org/SETTINGS/1.0.0",
	}

	// Load existing settings if present (preserve existing config)
	if data, err := os.ReadFile(settingsPath); err == nil && len(data) > 0 {
		if xmlErr := xml.Unmarshal(data, &settings); xmlErr != nil {
			log.Warn().Err(xmlErr).Msg("Failed to parse existing settings.xml, creating new one")
			settings = Settings{Xmlns: "http://maven.apache.org/SETTINGS/1.0.0"}
		}
	}

	serverID := "harness-" + registryID

	// Remove any existing harness-* mirrors with mirrorOf=* to avoid conflicts,
	// then add/update the current one.
	var filteredMirrors []Mirror
	for _, m := range settings.Mirrors.Mirror {
		if m.ID == serverID {
			continue // will be re-added below
		}
		if strings.HasPrefix(m.ID, "harness-") && m.MirrorOf == "*" {
			// Remove stale harness mirrors that mirror everything
			continue
		}
		filteredMirrors = append(filteredMirrors, m)
	}
	filteredMirrors = append(filteredMirrors, Mirror{
		ID:       serverID,
		Name:     "Harness " + registryID,
		URL:      registryURL,
		MirrorOf: "*",
	})
	settings.Mirrors.Mirror = filteredMirrors

	// Also clean up stale harness-* servers
	var filteredServers []Server
	for _, s := range settings.Servers.Server {
		if s.ID == serverID {
			continue // will be re-added below
		}
		if strings.HasPrefix(s.ID, "harness-") {
			continue
		}
		filteredServers = append(filteredServers, s)
	}
	filteredServers = append(filteredServers, Server{
		ID:       serverID,
		Username: "harness",
		Password: authToken,
	})
	settings.Servers.Server = filteredServers

	output, err := xml.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings.xml: %w", err)
	}

	content := xml.Header + string(output) + "\n"

	// Atomic write
	dir := filepath.Dir(settingsPath)
	tmpFile, err := os.CreateTemp(dir, ".settings-xml-tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write([]byte(content)); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write settings.xml content: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to set permissions: %w", err)
	}
	if err := os.Rename(tmpPath, settingsPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write settings.xml: %w", err)
	}

	return nil
}
