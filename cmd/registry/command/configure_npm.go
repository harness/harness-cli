package command

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar_v3"
	"github.com/harness/harness-cli/util"
	client2 "github.com/harness/harness-cli/util/client"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// NpmRegistryConfig stores npm registry configuration in ~/.harness/npm-config.json.
// Used by the install wrapper to detect the HAR registry without re-parsing .npmrc.
type NpmRegistryConfig struct {
	RegistryIdentifier string `json:"registryIdentifier"`
	RegistryURL        string `json:"registryUrl"`
	Scope              string `json:"scope,omitempty"`
	OrgID              string `json:"orgId,omitempty"`
	ProjectID          string `json:"projectId,omitempty"`
	NpmrcBackupPath    string `json:"npmrcBackupPath,omitempty"`
	NpmrcPath          string `json:"npmrcPath"`
}

func NewConfigureNpmCmd(f *cmdutils.Factory) *cobra.Command {
	var registryIdentifier string
	var scope string
	var token string
	var pkgURL string
	var global bool
	var projectLevel bool

	cmd := &cobra.Command{
		Use:   "npm",
		Short: "Configure npm client for Harness Artifact Registry",
		Long:  "Configure npm client to use a Harness Artifact Registry virtual npm registry",
		PreRun: func(cmd *cobra.Command, args []string) {
			if pkgURL != "" {
				config.Global.Registry.PkgURL = util.GetPkgUrl(pkgURL)
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			progress := p.NewConsoleReporter()

			progress.Start("Validating input parameters")
			if registryIdentifier == "" {
				progress.Error("Registry identifier is required")
				return fmt.Errorf("--registry flag is required")
			}

			if scope != "" && !strings.HasPrefix(scope, "@") {
				scope = "@" + scope
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

			// Fetch registry URL from GetRegistry API
			progress.Start("Fetching registry details")
			org := config.Global.OrgID
			project := config.Global.ProjectID
			registryRef := client2.GetRef(accountID, org, project, registryIdentifier)
			log.Info().Str("registryRef", registryRef).Str("org", org).Str("project", project).Str("account", accountID).Msg("Fetching registry")

			registryResp, err := f.RegistryHttpClient().GetRegistryWithResponse(context.Background(), registryRef)
			if err != nil {
				progress.Error("Failed to fetch registry details")
				return fmt.Errorf("failed to fetch registry details: %w", err)
			}

			if registryResp.StatusCode() != 200 || registryResp.JSON200 == nil {
				log.Error().Int("statusCode", registryResp.StatusCode()).Str("registryRef", registryRef).Msg("Registry lookup failed")
				progress.Error(fmt.Sprintf("Registry '%s' not found (status: %d)", registryIdentifier, registryResp.StatusCode()))
				return fmt.Errorf("registry '%s' not found (status: %d)", registryIdentifier, registryResp.StatusCode())
			}

			// Get the package registry base URL from system info API
			pkgBaseURL, err := getRegistryBaseURL(f, accountID)
			if err != nil {
				progress.Error(fmt.Sprintf("Failed to get registry base URL: %s", err))
				return fmt.Errorf("failed to get registry base URL: %w", err)
			}

			registryURL := fmt.Sprintf("%s/pkg/%s/%s/npm", pkgBaseURL, accountID, registryIdentifier)
			progress.Success(fmt.Sprintf("Registry URL: %s", registryURL))

			// Determine target .npmrc path
			var npmrcPath string
			if global {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					progress.Error("Failed to get home directory")
					return fmt.Errorf("failed to get home directory: %w", err)
				}
				npmrcPath = filepath.Join(homeDir, ".npmrc")
			} else {
				if _, err := os.Stat("package.json"); os.IsNotExist(err) {
					progress.Error("package.json not found in current directory")
					return fmt.Errorf("package.json not found in current directory. Please run this command from your npm project root directory where package.json exists, or use --global flag to configure globally")
				}
				cwd, err := os.Getwd()
				if err != nil {
					progress.Error("Failed to get current directory")
					return fmt.Errorf("failed to get current directory: %w", err)
				}
				npmrcPath = filepath.Join(cwd, ".npmrc")
			}

			// Backup existing .npmrc if it exists
			progress.Start("Backing up existing .npmrc")
			backupPath, err := backupNpmrc(npmrcPath)
			if err != nil {
				progress.Error(fmt.Sprintf("Failed to backup .npmrc: %s", err))
				return fmt.Errorf("failed to backup .npmrc: %w", err)
			}
			if backupPath != "" {
				progress.Success(fmt.Sprintf("Backed up existing .npmrc to %s", backupPath))
			} else {
				progress.Success("No existing .npmrc to backup")
			}

			// Write new .npmrc with Harness config
			progress.Start("Configuring npm")
			if err := configureNpm(registryURL, scope, authToken, global, projectLevel); err != nil {
				progress.Error("Failed to configure npm")
				return fmt.Errorf("failed to configure npm: %w", err)
			}

			// Save registry config to ~/.harness/npm-config.json
			if err := saveNpmRegistryConfig(NpmRegistryConfig{
				RegistryIdentifier: registryIdentifier,
				RegistryURL:        registryURL,
				Scope:              scope,
				OrgID:              org,
				ProjectID:          project,
				NpmrcBackupPath:    backupPath,
				NpmrcPath:          npmrcPath,
			}); err != nil {
				log.Warn().Err(err).Msg("Failed to save npm registry config to harness config dir")
			}

			if global {
				if scope != "" {
					progress.Success(fmt.Sprintf("Successfully configured npm globally for scope %s to use registry %s", scope, registryURL))
				} else {
					progress.Success(fmt.Sprintf("Successfully configured npm globally to use registry %s", registryURL))
				}
			} else {
				if scope != "" {
					progress.Success(fmt.Sprintf("Successfully configured npm at project level for scope %s to use registry %s", scope, registryURL))
				} else {
					progress.Success(fmt.Sprintf("Successfully configured npm at project level to use registry %s", registryURL))
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&registryIdentifier, "registry", "", "Registry identifier (required)")
	cmd.Flags().StringVar(&scope, "scope", "", "NPM scope (e.g., @myorg) (optional, configures default registry if not provided)")
	cmd.Flags().StringVar(&token, "token", "", "Auth token (optional, uses token from login if not provided)")
	cmd.Flags().StringVar(&pkgURL, "pkg-url", "", "Package registry base URL (optional)")
	cmd.Flags().BoolVar(&global, "global", false, "Configure globally for the user")
	cmd.Flags().BoolVar(&projectLevel, "project-level", false, "Configure at project level (.npmrc in current directory)")

	return cmd
}

// backupNpmrc backs up the existing .npmrc file to ~/.harness/npmrc-backup.
// Returns the backup path if a backup was created, or empty string if no .npmrc existed.
func backupNpmrc(npmrcPath string) (string, error) {
	data, err := os.ReadFile(npmrcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read existing .npmrc: %w", err)
	}

	if len(strings.TrimSpace(string(data))) == 0 {
		return "", nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	backupDir := filepath.Join(homeDir, ".harness")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

	backupPath := filepath.Join(backupDir, fmt.Sprintf("npmrc-backup-%s", time.Now().Format("20060102-150405")))

	if err := os.WriteFile(backupPath, data, 0600); err != nil {
		return "", fmt.Errorf("failed to write backup: %w", err)
	}

	return backupPath, nil
}

// saveNpmRegistryConfig persists the npm registry config to ~/.harness/npm-config.json.
func saveNpmRegistryConfig(cfg NpmRegistryConfig) error {
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

	return os.WriteFile(filepath.Join(configDir, "npm-config.json"), data, 0600)
}

// LoadNpmRegistryConfig loads the saved npm registry config from ~/.harness/npm-config.json.
func LoadNpmRegistryConfig() (*NpmRegistryConfig, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(filepath.Join(homeDir, ".harness", "npm-config.json"))
	if err != nil {
		return nil, err
	}

	var cfg NpmRegistryConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// getRegistryBaseURL calls GET /system/info to get the package registry base URL.
// Returns e.g. "https://pkg.qa.harness.io"
func getRegistryBaseURL(f *cmdutils.Factory, accountID string) (string, error) {
	params := &ar_v3.GetSystemInfoParams{
		AccountIdentifier: accountID,
	}

	resp, err := f.RegistryV3HttpClient().GetSystemInfoWithResponse(context.Background(), params)
	if err != nil {
		return "", fmt.Errorf("failed to call /system/info: %w", err)
	}

	if resp.JSON200 != nil {
		if data, ok := (*resp.JSON200)["data"].(map[string]interface{}); ok {
			if registryURL, ok := data["registryUrl"].(string); ok && registryURL != "" {
				log.Debug().Str("registryUrl", registryURL).Msg("System info response")
				return strings.TrimSuffix(registryURL, "/"), nil
			}
		}
	}

	return "", fmt.Errorf("failed to extract registryUrl from /system/info response (status: %d)", resp.StatusCode())
}

// RestoreNpmrc restores the backed-up .npmrc file if a backup exists.
func RestoreNpmrc() error {
	cfg, err := LoadNpmRegistryConfig()
	if err != nil {
		return nil // no config, nothing to restore
	}

	if cfg.NpmrcBackupPath == "" || cfg.NpmrcPath == "" {
		return nil
	}

	backupData, err := os.ReadFile(cfg.NpmrcBackupPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return os.WriteFile(cfg.NpmrcPath, backupData, 0600)
}

func configureNpm(registryURL, scope, authToken string, global, projectLevel bool) error {
	parsedURL, err := url.Parse(registryURL)
	if err != nil {
		return fmt.Errorf("invalid registry URL: %w", err)
	}

	registryHost := parsedURL.Host + parsedURL.Path

	if global {
		return configureNpmGlobal(registryURL, scope, authToken, registryHost)
	} else if projectLevel {
		return configureNpmProject(registryURL, scope, authToken, registryHost)
	}

	return nil
}

func configureNpmGlobal(registryURL, scope, authToken, registryHost string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	npmrcPath := filepath.Join(homeDir, ".npmrc")
	return writeNpmrcConfig(npmrcPath, registryURL, scope, authToken, registryHost)
}

func configureNpmProject(registryURL, scope, authToken, registryHost string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}
	npmrcPath := filepath.Join(cwd, ".npmrc")
	return writeNpmrcConfig(npmrcPath, registryURL, scope, authToken, registryHost)
}

func writeNpmrcConfig(npmrcPath, registryURL, scope, authToken, registryHost string) error {
	var existingContent []byte
	hasExistingContent := false

	if _, err := os.Stat(npmrcPath); err == nil {
		var readErr error
		existingContent, readErr = os.ReadFile(npmrcPath)
		if readErr != nil {
			return fmt.Errorf("failed to read existing .npmrc: %w", readErr)
		}
		if len(strings.TrimSpace(string(existingContent))) > 0 {
			hasExistingContent = true
		}
	}

	var scopeRegistryLine string
	if scope != "" {
		scopeRegistryLine = fmt.Sprintf("%s:registry=%s/", scope, registryURL)
	} else {
		scopeRegistryLine = fmt.Sprintf("registry=%s/", registryURL)
	}
	authTokenLine := fmt.Sprintf("//%s/:_authToken=%s", registryHost, authToken)
	alwaysAuthLine := "always-auth=true"

	scopeFound := false
	authFound := false
	var newLines []string

	if hasExistingContent {
		existingLines := strings.Split(string(existingContent), "\n")

		for _, line := range existingLines {
			trimmedLine := strings.TrimSpace(line)
			if scope != "" && strings.HasPrefix(trimmedLine, scope+":registry=") {
				newLines = append(newLines, scopeRegistryLine)
				scopeFound = true
			} else if scope == "" && strings.HasPrefix(trimmedLine, "registry=") && !strings.Contains(trimmedLine, ":") {
				newLines = append(newLines, scopeRegistryLine)
				scopeFound = true
			} else if strings.HasPrefix(trimmedLine, "//"+registryHost+"/:_authToken=") {
				newLines = append(newLines, authTokenLine)
				authFound = true
			} else if trimmedLine != "" {
				newLines = append(newLines, line)
			}
		}
	}

	if !scopeFound || !authFound {
		if hasExistingContent && len(newLines) > 0 {
			newLines = append(newLines, "")
		}
		if !scopeFound {
			newLines = append(newLines, scopeRegistryLine)
		}
		if !authFound {
			newLines = append(newLines, authTokenLine)
		}
		newLines = append(newLines, alwaysAuthLine)
	}

	content := strings.Join(newLines, "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	// Atomic write: write to temp file in the same directory, then rename.
	// This ensures the original .npmrc is never left in a corrupted state.
	dir := filepath.Dir(npmrcPath)
	tmpFile, err := os.CreateTemp(dir, ".npmrc-tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file for .npmrc: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write([]byte(content)); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write .npmrc content: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp .npmrc: %w", err)
	}

	if err := os.Chmod(tmpPath, 0600); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to set .npmrc permissions: %w", err)
	}

	if err := os.Rename(tmpPath, npmrcPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write .npmrc: %w", err)
	}

	return nil
}
