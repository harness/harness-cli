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

// NuGetRegistryConfig stores nuget registry configuration in ~/.harness/nuget-config.json.
type NuGetRegistryConfig struct {
	RegistryIdentifier string `json:"registryIdentifier"`
	RegistryURL        string `json:"registryUrl"`
	OrgID              string `json:"orgId,omitempty"`
	ProjectID          string `json:"projectId,omitempty"`
	NuGetConfigBackup  string `json:"nugetConfigBackupPath,omitempty"`
	NuGetConfigPath    string `json:"nugetConfigPath"`
}

func NewConfigureNuGetCmd(f *cmdutils.Factory) *cobra.Command {
	var registryIdentifier string
	var token string
	var global bool
	var projectLevel bool

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

			registryURL := fmt.Sprintf("%s/pkg/%s/%s/nuget/index.json", pkgBaseURL, accountID, registryIdentifier)
			progress.Success(fmt.Sprintf("Registry URL: %s", registryURL))

			// Determine nuget.config path
			var nugetConfigPath string
			if global {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("failed to get home directory: %w", err)
				}
				nugetConfigPath = filepath.Join(homeDir, ".nuget", "NuGet", "NuGet.Config")
			} else {
				cwd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("failed to get current directory: %w", err)
				}
				// Check .csproj or .sln exists for project-level
				hasDotnet := false
				entries, _ := os.ReadDir(cwd)
				for _, e := range entries {
					if strings.HasSuffix(e.Name(), ".csproj") || strings.HasSuffix(e.Name(), ".sln") || strings.HasSuffix(e.Name(), ".fsproj") {
						hasDotnet = true
						break
					}
				}
				if !hasDotnet {
					progress.Error("No .NET project files found in current directory")
					return fmt.Errorf("no .csproj, .fsproj, or .sln found. Use --global flag to configure globally")
				}
				nugetConfigPath = filepath.Join(cwd, "nuget.config")
			}

			// Backup existing nuget.config
			progress.Start("Backing up existing nuget.config")
			backupPath, err := backupNuGetConfig(nugetConfigPath)
			if err != nil {
				progress.Error(fmt.Sprintf("Failed to backup nuget.config: %s", err))
				return fmt.Errorf("failed to backup nuget.config: %w", err)
			}
			if backupPath != "" {
				progress.Success(fmt.Sprintf("Backed up existing nuget.config to %s", backupPath))
			} else {
				progress.Success("No existing nuget.config to backup")
			}

			// Write nuget.config
			progress.Start("Configuring NuGet")
			if err := writeNuGetConfig(nugetConfigPath, registryURL, registryIdentifier, authToken); err != nil {
				progress.Error("Failed to configure NuGet")
				return fmt.Errorf("failed to configure NuGet: %w", err)
			}

			// Save config
			if err := saveNuGetRegistryConfig(NuGetRegistryConfig{
				RegistryIdentifier: registryIdentifier,
				RegistryURL:        registryURL,
				OrgID:              org,
				ProjectID:          project,
				NuGetConfigBackup:  backupPath,
				NuGetConfigPath:    nugetConfigPath,
			}); err != nil {
				log.Warn().Err(err).Msg("Failed to save nuget registry config")
			}

			if global {
				progress.Success(fmt.Sprintf("Successfully configured NuGet globally to use registry %s", registryURL))
			} else {
				progress.Success(fmt.Sprintf("Successfully configured NuGet at project level to use registry %s", registryURL))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&registryIdentifier, "registry", "", "Registry identifier (required)")
	cmd.Flags().StringVar(&token, "token", "", "Auth token (optional, uses token from login if not provided)")
	cmd.Flags().BoolVar(&global, "global", false, "Configure globally (~/.nuget/NuGet/NuGet.Config)")
	cmd.Flags().BoolVar(&projectLevel, "project-level", false, "Configure at project level (nuget.config in current directory)")

	return cmd
}

// backupNuGetConfig backs up an existing nuget.config to ~/.harness/nuget-config-backup.
func backupNuGetConfig(configPath string) (string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read existing nuget.config: %w", err)
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

	backupPath := filepath.Join(backupDir, "nuget-config-backup")
	if _, err := os.Stat(backupPath); err == nil {
		log.Info().Str("backupPath", backupPath).Msg("Backup already exists, skipping")
		return backupPath, nil
	}

	if err := os.WriteFile(backupPath, data, 0600); err != nil {
		return "", err
	}
	return backupPath, nil
}

// writeNuGetConfig writes a nuget.config with HAR registry as package source.
func writeNuGetConfig(configPath, registryURL, registryID, authToken string) error {
	type Add struct {
		XMLName xml.Name `xml:"add"`
		Key     string   `xml:"key,attr"`
		Value   string   `xml:"value,attr"`
	}

	type PackageSources struct {
		XMLName xml.Name `xml:"packageSources"`
		Clear   struct {
			XMLName xml.Name `xml:"clear"`
		}
		Add []Add
	}

	type PackageSourceCredentials struct {
		XMLName xml.Name
		Add     []Add
	}

	type Configuration struct {
		XMLName                  xml.Name `xml:"configuration"`
		PackageSources           PackageSources
		PackageSourceCredentials struct {
			XMLName xml.Name `xml:"packageSourceCredentials"`
		}
	}

	// Build the XML manually for better control over the credentials section
	// NuGet requires the source name as an XML element under packageSourceCredentials
	content := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<configuration>
  <packageSources>
    <clear />
    <add key="%s" value="%s" />
    <add key="nuget.org" value="https://api.nuget.org/v3/index.json" />
  </packageSources>
  <packageSourceCredentials>
    <%s>
      <add key="Username" value="harness" />
      <add key="ClearTextPassword" value="%s" />
    </%s>
  </packageSourceCredentials>
</configuration>
`, registryID, registryURL, registryID, authToken, registryID)

	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write nuget.config: %w", err)
	}

	return nil
}

func saveNuGetRegistryConfig(cfg NuGetRegistryConfig) error {
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
	return os.WriteFile(filepath.Join(configDir, "nuget-config.json"), data, 0600)
}

// LoadNuGetRegistryConfig loads the saved nuget registry config.
func LoadNuGetRegistryConfig() (*NuGetRegistryConfig, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(homeDir, ".harness", "nuget-config.json"))
	if err != nil {
		return nil, err
	}
	var cfg NuGetRegistryConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
