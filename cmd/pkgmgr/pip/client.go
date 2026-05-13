package pip

import (
	"bytes"
	"encoding/json"
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

var harURLPattern = regexp.MustCompile(`(?:https?://[^/]+)/(?:pkg/)?([^/]+)/([^/]+)/pypi/?`)

var has403Pattern = regexp.MustCompile(`(?i)(403\s*Forbidden|HTTP\s+error\s+403|status\s*code\s*403|Client Error:\s*403)`)

var lockFileSearchOrder = []string{
	"Pipfile.lock",
	"poetry.lock",
	"requirements.txt",
}

type PipClient struct{}

func NewClient() *PipClient {
	return &PipClient{}
}

func (c *PipClient) Name() string        { return pkgmgr.CommandPip }
func (c *PipClient) PackageType() string { return pkgmgr.PackageTypePyPI }

func (c *PipClient) FallbackOrgProject() (string, string) {
	savedCfg, err := regcmd.LoadPipRegistryConfig()
	if err != nil || savedCfg == nil {
		return "", ""
	}
	return savedCfg.OrgID, savedCfg.ProjectID
}

func (c *PipClient) DetectFirewallError(stderr string) bool {
	return has403Pattern.MatchString(stderr)
}

func (c *PipClient) DetectRegistry(explicitRegistry string) (*pkgmgr.RegistryInfo, error) {
	savedCfg, err := regcmd.LoadPipRegistryConfig()
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

	// Check pip.conf for HAR registry
	pipConfPaths := getPipConfPaths()
	for _, path := range pipConfPaths {
		info, err := parsePipConfForHAR(path, explicitRegistry)
		if err == nil && info != nil {
			return info, nil
		}
	}

	if explicitRegistry != "" {
		return nil, fmt.Errorf("HAR registry '%s' not found in pip configuration", explicitRegistry)
	}
	return nil, fmt.Errorf("no HAR registry found. Run 'hc registry configure pip' first")
}

func (c *PipClient) RunCommand(command string, args []string) (*pkgmgr.InstallResult, error) {
	pipBin := findPipBinary()
	cmdArgs := append([]string{command}, args...)
	cmd := exec.Command(pipBin, cmdArgs...)
	cmd.Dir = "."
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout

	var stderrBuf bytes.Buffer
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	log.Info().Strs("args", cmdArgs).Msgf("Running %s %s", pipBin, command)

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

func (c *PipClient) ResolveDependencies(progress p.Reporter) (*pkgmgr.DependencyResult, error) {
	noop := func() {}

	// Check for existing lock files first
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

	// Try pip install --dry-run --report
	progress.Step("Running pip install --dry-run --report to resolve dependencies")
	reportPath := filepath.Join(os.TempDir(), "pip-report.json")
	cleanup := func() {
		os.Remove(reportPath)
	}

	reqsArg := "-r"
	reqsFile := "requirements.txt"
	if _, err := os.Stat("requirements.txt"); err != nil {
		if _, err := os.Stat("pyproject.toml"); err == nil {
			reqsArg = "."
			reqsFile = ""
		} else {
			return nil, fmt.Errorf("no requirements.txt or pyproject.toml found")
		}
	}

	var cmdArgs []string
	if reqsFile != "" {
		cmdArgs = []string{"install", "--dry-run", "--report", reportPath, reqsArg, reqsFile}
	} else {
		cmdArgs = []string{"install", "--dry-run", "--report", reportPath, reqsArg}
	}

	cmd := exec.Command(findPipBinary(), cmdArgs...)
	cmd.Dir = "."
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log.Warn().Err(err).Str("stderr", stderr.String()).Msg("pip install --dry-run failed")
		cleanup()
		progress.Step("Dry-run failed, falling back to requirements.txt (direct deps only)")
		if _, statErr := os.Stat("requirements.txt"); statErr == nil {
			deps, parseErr := regcmd.ParseLockFile("requirements.txt")
			return &pkgmgr.DependencyResult{Dependencies: deps, Cleanup: noop}, parseErr
		}
		return nil, fmt.Errorf("pip dry-run failed and no requirements.txt found: %w", err)
	}

	deps, err := parsePipReport(reportPath)
	if err != nil {
		cleanup()
		return nil, err
	}

	return &pkgmgr.DependencyResult{Dependencies: deps, Cleanup: cleanup}, nil
}

func parsePipReport(reportPath string) ([]regcmd.Dependency, error) {
	data, err := os.ReadFile(reportPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read pip report: %w", err)
	}

	var report struct {
		Install []struct {
			Metadata struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"metadata"`
			Requested bool `json:"requested"`
		} `json:"install"`
	}

	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("failed to parse pip report: %w", err)
	}

	deps := make([]regcmd.Dependency, 0, len(report.Install))
	for _, pkg := range report.Install {
		if pkg.Metadata.Name == "" {
			continue
		}
		deps = append(deps, regcmd.Dependency{
			Name:    pkg.Metadata.Name,
			Version: pkg.Metadata.Version,
			Source:  "pip-report",
		})
	}

	return deps, nil
}

func getPipConfPaths() []string {
	paths := []string{}
	if homeDir, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(homeDir, ".config", "pip", "pip.conf"))
		paths = append(paths, filepath.Join(homeDir, ".pip", "pip.conf"))
	}
	paths = append(paths, "pip.conf")
	return paths
}

func parsePipConfForHAR(confPath, explicitRegistry string) (*pkgmgr.RegistryInfo, error) {
	data, err := os.ReadFile(confPath)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "index-url") || strings.HasPrefix(line, "extra-index-url") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			url := strings.TrimSpace(parts[1])
			if harURLPattern.MatchString(url) {
				matches := harURLPattern.FindStringSubmatch(url)
				if len(matches) < 3 {
					continue
				}
				registryIdentifier := matches[2]
				if explicitRegistry != "" && registryIdentifier != explicitRegistry {
					continue
				}
				return &pkgmgr.RegistryInfo{
					RegistryURL:        url,
					AccountID:          matches[1],
					RegistryIdentifier: registryIdentifier,
					AuthToken:          config.Global.AuthToken,
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("no HAR registry URL found in %s", confPath)
}

func findPipBinary() string {
	if _, err := exec.LookPath("pip"); err == nil {
		return "pip"
	}
	if _, err := exec.LookPath("pip3"); err == nil {
		return "pip3"
	}
	return "pip"
}
