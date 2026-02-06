package command

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/spf13/cobra"
)

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
		RunE: func(cmd *cobra.Command, args []string) error {
			progress := p.NewConsoleReporter()

			progress.Start("Validating input parameters")
			if registryIdentifier == "" {
				progress.Error("Registry identifier is required")
				return fmt.Errorf("--registry flag is required")
			}

			if pkgURL == "" {
				progress.Error("Package URL is required")
				return fmt.Errorf("--pkg-url flag is required")
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
			baseURL := pkgURL
			accountID := config.Global.AccountID
			authToken := token
			if authToken == "" {
				authToken = config.Global.AuthToken
			}

			if accountID == "" {
				progress.Error("Account ID not configured")
				return fmt.Errorf("account ID not configured, please run 'hc login' first")
			}

			if authToken == "" {
				progress.Error("Auth token not configured")
				return fmt.Errorf("auth token not configured, please run 'hc login' first or provide --token flag")
			}
			progress.Success("Configuration loaded")

			registryURL := fmt.Sprintf("%s/pkg/%s/%s/npm", baseURL, accountID, registryIdentifier)

			progress.Start("Configuring npm")
			if projectLevel {
				if _, err := os.Stat("package.json"); os.IsNotExist(err) {
					progress.Error("package.json not found in current directory")
					return fmt.Errorf("package.json not found in current directory. Please run this command from your npm project root directory where package.json exists, or use --global flag to configure globally")
				}
			}
			if err := configureNpm(registryURL, scope, authToken, global, projectLevel); err != nil {
				progress.Error("Failed to configure npm")
				return fmt.Errorf("failed to configure npm: %w", err)
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
	cmd.Flags().StringVar(&pkgURL, "pkg-url", "", "Package registry base URL (required)")
	cmd.Flags().BoolVar(&global, "global", false, "Configure globally for the user")
	cmd.Flags().BoolVar(&projectLevel, "project-level", false, "Configure at project level (.npmrc in current directory)")

	return cmd
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

	if err := os.WriteFile(npmrcPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write .npmrc: %w", err)
	}

	return nil
}
