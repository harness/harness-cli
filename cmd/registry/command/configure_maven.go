package command

import (
	"context"
	"encoding/json"
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

func NewConfigureMavenCmd(f *cmdutils.Factory) *cobra.Command {
	var registryIdentifier string
	var token string
	var global bool
	var projectLevel bool

	cmd := &cobra.Command{
		Use:   "maven",
		Short: "Configure Maven client for Harness Artifact Registry",
		Long:  "Configure Maven client to use a Harness Artifact Registry virtual maven registry",
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

			registryURL := fmt.Sprintf("%s/pkg/%s/%s/maven", pkgBaseURL, accountID, registryIdentifier)
			progress.Success(fmt.Sprintf("Registry URL: %s", registryURL))

			// Determine settings.xml path
			var settingsPath string
			if global {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("failed to get home directory: %w", err)
				}
				settingsPath = filepath.Join(homeDir, ".m2", "settings.xml")
			} else {
				cwd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("failed to get current directory: %w", err)
				}
				// Check pom.xml exists for project-level
				if _, err := os.Stat(filepath.Join(cwd, "pom.xml")); os.IsNotExist(err) {
					progress.Error("pom.xml not found in current directory")
					return fmt.Errorf("pom.xml not found in current directory. Use --global flag to configure globally")
				}
				settingsPath = filepath.Join(cwd, ".mvn", "settings.xml")
			}

			// Backup existing settings.xml
			progress.Start("Backing up existing settings.xml")
			backupPath, err := backupSettings(settingsPath)
			if err != nil {
				progress.Error(fmt.Sprintf("Failed to backup settings.xml: %s", err))
				return fmt.Errorf("failed to backup settings.xml: %w", err)
			}
			if backupPath != "" {
				progress.Success(fmt.Sprintf("Backed up existing settings.xml to %s", backupPath))
			} else {
				progress.Success("No existing settings.xml to backup")
			}

			// Write settings.xml
			progress.Start("Configuring Maven")
			if err := writeMavenSettings(settingsPath, registryURL, registryIdentifier, authToken); err != nil {
				progress.Error("Failed to configure Maven")
				return fmt.Errorf("failed to configure Maven: %w", err)
			}

			// Save config
			if err := saveMavenRegistryConfig(MavenRegistryConfig{
				RegistryIdentifier: registryIdentifier,
				RegistryURL:        registryURL,
				OrgID:              org,
				ProjectID:          project,
				SettingsBackupPath: backupPath,
				SettingsPath:       settingsPath,
			}); err != nil {
				log.Warn().Err(err).Msg("Failed to save maven registry config")
			}

			if global {
				progress.Success(fmt.Sprintf("Successfully configured Maven globally to use registry %s", registryURL))
			} else {
				progress.Success(fmt.Sprintf("Successfully configured Maven at project level to use registry %s", registryURL))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&registryIdentifier, "registry", "", "Registry identifier (required)")
	cmd.Flags().StringVar(&token, "token", "", "Auth token (optional, uses token from login if not provided)")
	cmd.Flags().BoolVar(&global, "global", false, "Configure globally (~/.m2/settings.xml)")
	cmd.Flags().BoolVar(&projectLevel, "project-level", false, "Configure at project level (.mvn/settings.xml)")

	return cmd
}

// backupSettings backs up an existing settings.xml to ~/.harness/settings-xml-backup.
func backupSettings(settingsPath string) (string, error) {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read existing settings.xml: %w", err)
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

	backupPath := filepath.Join(backupDir, "settings-xml-backup")
	if _, err := os.Stat(backupPath); err == nil {
		log.Info().Str("backupPath", backupPath).Msg("Backup already exists, skipping")
		return backupPath, nil
	}

	if err := os.WriteFile(backupPath, data, 0600); err != nil {
		return "", err
	}
	return backupPath, nil
}

// writeMavenSettings writes a settings.xml with HAR registry as mirror + server auth.
func writeMavenSettings(settingsPath, registryURL, registryID, authToken string) error {
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
			Server []Server `xml:"server"`
		} `xml:"servers"`
		Mirrors struct {
			Mirror []Mirror `xml:"mirror"`
		} `xml:"mirrors"`
	}

	settings := Settings{
		Xmlns: "http://maven.apache.org/SETTINGS/1.2.0",
	}
	settings.Servers.Server = []Server{
		{
			ID:       registryID,
			Username: "harness",
			Password: authToken,
		},
	}
	settings.Mirrors.Mirror = []Mirror{
		{
			ID:       registryID,
			Name:     "Harness Artifact Registry",
			URL:      registryURL + "/",
			MirrorOf: "*",
		},
	}

	data, err := xml.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to generate settings.xml: %w", err)
	}

	content := xml.Header + string(data) + "\n"

	dir := filepath.Dir(settingsPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	if err := os.WriteFile(settingsPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write settings.xml: %w", err)
	}

	return nil
}

func saveMavenRegistryConfig(cfg MavenRegistryConfig) error {
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
	return os.WriteFile(filepath.Join(configDir, "maven-config.json"), data, 0600)
}

// LoadMavenRegistryConfig loads the saved maven registry config.
func LoadMavenRegistryConfig() (*MavenRegistryConfig, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(homeDir, ".harness", "maven-config.json"))
	if err != nil {
		return nil, err
	}
	var cfg MavenRegistryConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
