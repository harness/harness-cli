package npm

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/harness/harness-cli/cmd/pkgmgr"
	regcmd "github.com/harness/harness-cli/cmd/registry/command"
	"github.com/harness/harness-cli/config"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/rs/zerolog/log"
)

// harURLPattern matches HAR registry URLs in both formats:
// Subdomain: https://pkg.host.io/{account_id}/{registry_name}/npm
// Path:      https://host.io/pkg/{account_id}/{registry_name}/npm
var harURLPattern = regexp.MustCompile(`(?:https?://[^/]+)/(?:pkg/)?([^/]+)/([^/]+)/npm/?$`)

// has403Pattern detects 403 Forbidden in npm stderr.
var has403Pattern = regexp.MustCompile(`(?i)403\s*[Ff]orbidden`)

// lockFileSearchOrder lists lock files to check for dependency resolution.
var lockFileSearchOrder = []string{
	"package-lock.json",
	"yarn.lock",
	"pnpm-lock.yaml",
}

// NpmClient implements pkgmgr.Client for npm.
type NpmClient struct{}

func NewClient() *NpmClient {
	return &NpmClient{}
}

func (c *NpmClient) Name() string        { return "npm" }
func (c *NpmClient) PackageType() string { return "npm" }

func (c *NpmClient) FallbackOrgProject() (string, string) {
	savedCfg, err := regcmd.LoadNpmRegistryConfig()
	if err != nil || savedCfg == nil {
		return "", ""
	}
	return savedCfg.OrgID, savedCfg.ProjectID
}

func (c *NpmClient) DetectFirewallError(stderr string) bool {
	return has403Pattern.MatchString(stderr)
}

// DetectRegistry detects the HAR registry from saved config or .npmrc files.
func (c *NpmClient) DetectRegistry(explicitRegistry string) (*pkgmgr.RegistryInfo, error) {
	savedCfg, err := regcmd.LoadNpmRegistryConfig()
	if err == nil && savedCfg != nil && savedCfg.RegistryURL != "" {
		if explicitRegistry == "" || explicitRegistry == savedCfg.RegistryIdentifier {
			return &pkgmgr.RegistryInfo{
				RegistryURL:        savedCfg.RegistryURL,
				RegistryIdentifier: savedCfg.RegistryIdentifier,
				AccountID:          config.Global.AccountID,
				AuthToken:          config.Global.AuthToken,
			}, nil
		}
	}

	npmrcPaths := []string{
		filepath.Join(".", ".npmrc"),
	}
	if homeDir, err := os.UserHomeDir(); err == nil {
		npmrcPaths = append(npmrcPaths, filepath.Join(homeDir, ".npmrc"))
	}

	for _, npmrcPath := range npmrcPaths {
		info, err := parseNpmrcForHAR(npmrcPath, explicitRegistry)
		if err == nil && info != nil {
			return info, nil
		}
	}

	if explicitRegistry != "" {
		return nil, fmt.Errorf("HAR registry '%s' not found in .npmrc files", explicitRegistry)
	}
	return nil, fmt.Errorf("no HAR registry found. Run 'hc registry configure npm' first")
}

// RunCommand executes a native npm command with live output streaming.
func (c *NpmClient) RunCommand(command string, args []string) (*pkgmgr.InstallResult, error) {
	cmdArgs := append([]string{command}, args...)
	cmd := exec.Command("npm", cmdArgs...)
	cmd.Dir = "."
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout

	var stderrBuf bytes.Buffer
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	log.Info().Strs("args", cmdArgs).Msgf("Running npm %s", command)

	err := cmd.Run()
	stderrStr := stderrBuf.String()

	if err != nil {
		return &pkgmgr.InstallResult{
			Status: "FAILURE",
			Stderr: stderrStr,
			Err:    err,
		}, nil
	}
	return &pkgmgr.InstallResult{
		Status: "SUCCESS",
		Stderr: stderrStr,
	}, nil
}

// ResolveDependencies resolves all npm dependencies from lock files.
// If no lock file exists, generates one with npm install --package-lock-only.
func (c *NpmClient) ResolveDependencies(progress p.Reporter) (*pkgmgr.DependencyResult, error) {
	noop := func() {}

	// Check for existing lock files
	for _, lockFile := range lockFileSearchOrder {
		if _, err := os.Stat(lockFile); err == nil {
			progress.Step(fmt.Sprintf("Found existing %s, parsing dependencies", lockFile))
			deps, err := regcmd.ParseLockFile(lockFile)
			if err == nil && len(deps) > 0 {
				return &pkgmgr.DependencyResult{Dependencies: deps, Cleanup: noop}, nil
			}
			log.Warn().Err(err).Str("file", lockFile).Msg("Failed to parse existing lock file, trying next")
		}
	}

	// No usable lock file — generate one
	progress.Step("No lock file found, generating package-lock.json")
	log.Info().Msg("Running npm install --package-lock-only to generate lock file")

	cmd := exec.Command("npm", "install", "--package-lock-only")
	cmd.Dir = "."
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log.Warn().Err(err).Str("stderr", stderr.String()).Msg("npm install --package-lock-only failed")
		progress.Step("Lock file generation failed, falling back to package.json (direct deps only)")
		if _, statErr := os.Stat("package.json"); statErr == nil {
			deps, parseErr := regcmd.ParseLockFile("package.json")
			return &pkgmgr.DependencyResult{Dependencies: deps, Cleanup: noop}, parseErr
		}
		return nil, fmt.Errorf("no dependency files available: %w", err)
	}

	// Parse the generated lock file and return cleanup to remove it
	if _, err := os.Stat("package-lock.json"); err == nil {
		cleanup := func() {
			if removeErr := os.Remove("package-lock.json"); removeErr != nil {
				log.Warn().Err(removeErr).Msg("Failed to clean up generated package-lock.json")
			} else {
				log.Info().Msg("Cleaned up generated package-lock.json")
			}
		}
		deps, parseErr := regcmd.ParseLockFile("package-lock.json")
		return &pkgmgr.DependencyResult{Dependencies: deps, Cleanup: cleanup}, parseErr
	}

	return nil, fmt.Errorf("package-lock.json was not generated")
}

// --- .npmrc parsing helpers ---

func parseNpmrcForHAR(npmrcPath, explicitRegistry string) (*pkgmgr.RegistryInfo, error) {
	data, err := os.ReadFile(npmrcPath)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	var registryURL, authToken string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.Contains(line, "registry=") {
			parts := strings.SplitN(line, "registry=", 2)
			if len(parts) == 2 {
				candidateURL := strings.TrimSpace(parts[1])
				if harURLPattern.MatchString(candidateURL) {
					registryURL = candidateURL
				}
			}
		}

		if strings.Contains(line, ":_authToken=") {
			parts := strings.SplitN(line, ":_authToken=", 2)
			if len(parts) == 2 {
				authToken = strings.TrimSpace(parts[1])
			}
		}
	}

	if registryURL == "" {
		return nil, fmt.Errorf("no HAR registry URL found in %s", npmrcPath)
	}

	matches := harURLPattern.FindStringSubmatch(registryURL)
	if len(matches) < 3 {
		return nil, fmt.Errorf("failed to parse HAR registry URL: %s", registryURL)
	}

	info := &pkgmgr.RegistryInfo{
		RegistryURL:        registryURL,
		AccountID:          matches[1],
		RegistryIdentifier: matches[2],
		AuthToken:          authToken,
	}

	if explicitRegistry != "" && info.RegistryIdentifier != explicitRegistry {
		return nil, fmt.Errorf("registry mismatch")
	}

	return info, nil
}
