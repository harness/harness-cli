package command

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	client2 "github.com/harness/harness-cli/util/client"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

type NugetRegistryConfig struct {
	RegistryIdentifier    string `json:"registryIdentifier"`
	RegistryURL           string `json:"registryUrl"`
	OrgID                 string `json:"orgId,omitempty"`
	ProjectID             string `json:"projectId,omitempty"`
	NugetConfigBackupPath string `json:"nugetConfigBackupPath,omitempty"`
	NugetConfigPath       string `json:"nugetConfigPath"`
}

func SaveNugetRegistryConfig(cfg NugetRegistryConfig) error {
	return saveRegistryConfig(NugetConfigFile, cfg)
}

func LoadNugetRegistryConfig() (*NugetRegistryConfig, error) {
	var cfg NugetRegistryConfig
	if err := loadRegistryConfig(NugetConfigFile, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// RestoreNugetConfig restores the backed-up NuGet.Config file if a backup exists.
func RestoreNugetConfig() error {
	cfg, err := LoadNugetRegistryConfig()
	if err != nil {
		return nil
	}

	if cfg.NugetConfigBackupPath == "" || cfg.NugetConfigPath == "" {
		return nil
	}

	backupData, err := os.ReadFile(cfg.NugetConfigBackupPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return os.WriteFile(cfg.NugetConfigPath, backupData, 0600)
}

func NewConfigureNugetCmd(f *cmdutils.Factory) *cobra.Command {
	var registryIdentifier string
	var token string

	cmd := &cobra.Command{
		Use:   "nuget",
		Short: "Configure NuGet client for Harness Artifact Registry",
		Long:  "Configure NuGet/dotnet client to use a Harness Artifact Registry virtual NuGet registry",
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

			registryURL := fmt.Sprintf("%s/pkg/%s/%s/nuget/v3/index.json", pkgBaseURL, accountID, registryIdentifier)
			progress.Success(fmt.Sprintf("Registry URL: %s", registryURL))

			// Determine NuGet.Config path
			homeDir, err := os.UserHomeDir()
			if err != nil {
				progress.Error("Failed to get home directory")
				return fmt.Errorf("failed to get home directory: %w", err)
			}

			nugetDir := filepath.Join(homeDir, ".nuget", "NuGet")
			if err := os.MkdirAll(nugetDir, 0755); err != nil {
				progress.Error("Failed to create NuGet config directory")
				return fmt.Errorf("failed to create NuGet config directory: %w", err)
			}
			configPath := filepath.Join(nugetDir, "NuGet.Config")

			// Backup existing NuGet.Config
			progress.Start("Backing up existing NuGet.Config")
			backupPath, err := BackupFile(configPath, "nuget-config")
			if err != nil {
				progress.Error(fmt.Sprintf("Failed to backup NuGet.Config: %s", err))
				return fmt.Errorf("failed to backup NuGet.Config: %w", err)
			}
			if backupPath != "" {
				progress.Success(fmt.Sprintf("Backed up existing NuGet.Config to %s", backupPath))
			} else {
				progress.Success("No existing NuGet.Config to backup")
			}

			// Write new NuGet.Config
			progress.Start("Configuring NuGet")
			if err := writeNugetConfig(configPath, registryIdentifier, registryURL, authToken); err != nil {
				progress.Error("Failed to configure NuGet")
				// Restore backup on failure
				if backupPath != "" {
					if restoreErr := RestoreFromBackup(backupPath, configPath); restoreErr != nil {
						log.Error().Err(restoreErr).Msg("Failed to restore NuGet.Config from backup")
					} else {
						progress.Step("Restored original NuGet.Config from backup")
					}
				}
				return fmt.Errorf("failed to configure NuGet: %w", err)
			}

			// Save registry config with backup path for future restore
			if err := SaveNugetRegistryConfig(NugetRegistryConfig{
				RegistryIdentifier:    registryIdentifier,
				RegistryURL:           registryURL,
				OrgID:                 org,
				ProjectID:             project,
				NugetConfigBackupPath: backupPath,
				NugetConfigPath:       configPath,
			}); err != nil {
				log.Warn().Err(err).Msg("Failed to save nuget registry config")
			}

			progress.Success(fmt.Sprintf("Successfully configured NuGet to use registry %s", registryURL))
			return nil
		},
	}

	cmd.Flags().StringVar(&registryIdentifier, "registry", "", "Registry identifier (required)")
	cmd.Flags().StringVar(&token, "token", "", "Auth token (optional)")

	return cmd
}

func writeNugetConfig(configPath, registryID, registryURL, authToken string) error {
	type Add struct {
		XMLName         xml.Name `xml:"add"`
		Key             string   `xml:"key,attr"`
		Value           string   `xml:"value,attr"`
		ProtocolVersion *string  `xml:"protocolVersion,attr,omitempty"`
	}
	type PackageSources struct {
		XMLName xml.Name `xml:"packageSources"`
		Clear   *struct {
			XMLName xml.Name `xml:"clear"`
		} `xml:"clear,omitempty"`
		Sources []Add `xml:"add"`
	}
	type CredentialAdd struct {
		XMLName xml.Name `xml:"add"`
		Key     string   `xml:"key,attr"`
		Value   string   `xml:"value,attr"`
	}
	type SourceCredential struct {
		XMLName xml.Name
		Adds    []CredentialAdd `xml:"add"`
	}
	type PackageSourceCredentials struct {
		XMLName xml.Name           `xml:"packageSourceCredentials"`
		Sources []SourceCredential `xml:",any"`
	}
	type Configuration struct {
		XMLName                  xml.Name                  `xml:"configuration"`
		PackageSources           PackageSources            `xml:"packageSources"`
		PackageSourceCredentials *PackageSourceCredentials `xml:"packageSourceCredentials,omitempty"`
	}

	conf := Configuration{}

	// Load existing config if present (preserve existing sources)
	if data, err := os.ReadFile(configPath); err == nil && len(data) > 0 {
		if xmlErr := xml.Unmarshal(data, &conf); xmlErr != nil {
			log.Warn().Err(xmlErr).Msg("Failed to parse existing NuGet.Config, creating new one")
			conf = Configuration{}
		}
	}

	sourceName := "harness-" + registryID
	protocolVersion := "3"

	// Update or add package source
	sourceFound := false
	for i, s := range conf.PackageSources.Sources {
		if s.Key == sourceName {
			conf.PackageSources.Sources[i].Value = registryURL
			conf.PackageSources.Sources[i].ProtocolVersion = &protocolVersion
			sourceFound = true
			break
		}
	}
	if !sourceFound {
		conf.PackageSources.Sources = append(conf.PackageSources.Sources, Add{
			Key:             sourceName,
			Value:           registryURL,
			ProtocolVersion: &protocolVersion,
		})
	}

	// Add credentials
	if conf.PackageSourceCredentials == nil {
		conf.PackageSourceCredentials = &PackageSourceCredentials{}
	}
	credFound := false
	for i, src := range conf.PackageSourceCredentials.Sources {
		if src.XMLName.Local == sourceName {
			conf.PackageSourceCredentials.Sources[i].Adds = []CredentialAdd{
				{Key: "Username", Value: "harness"},
				{Key: "ClearTextPassword", Value: authToken},
			}
			credFound = true
			break
		}
	}
	if !credFound {
		conf.PackageSourceCredentials.Sources = append(conf.PackageSourceCredentials.Sources, SourceCredential{
			XMLName: xml.Name{Local: sourceName},
			Adds: []CredentialAdd{
				{Key: "Username", Value: "harness"},
				{Key: "ClearTextPassword", Value: authToken},
			},
		})
	}

	output, err := xml.MarshalIndent(conf, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal NuGet.Config: %w", err)
	}

	content := xml.Header + string(output) + "\n"

	// Atomic write
	dir := filepath.Dir(configPath)
	tmpFile, err := os.CreateTemp(dir, ".nuget-config-tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write([]byte(content)); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write NuGet.Config content: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to set permissions: %w", err)
	}
	if err := os.Rename(tmpPath, configPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write NuGet.Config: %w", err)
	}

	return nil
}
