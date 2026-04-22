package python

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"

	"github.com/harness/harness-cli/cmd/pkgmgr"
	regcmd "github.com/harness/harness-cli/cmd/registry/command"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/rs/zerolog/log"
)

// has403Pattern detects 403 Forbidden in pip stderr.
// pip outputs: "ERROR: Could not install packages due to an OSError: 403 Client Error: Forbidden"
var has403Pattern = regexp.MustCompile(`(?i)403|[Ff]orbidden`)

// lockFileSearchOrder lists files to check for python dependency resolution.
var lockFileSearchOrder = []string{
	"Pipfile.lock",
	"poetry.lock",
	"requirements.txt",
	"pyproject.toml",
}

// PythonClient implements pkgmgr.Client for Python/pip.
type PythonClient struct{}

func NewClient() *PythonClient {
	return &PythonClient{}
}

func (c *PythonClient) Name() string        { return "pip" }
func (c *PythonClient) PackageType() string { return "pypi" }

func (c *PythonClient) DetectFirewallError(stderr string) bool {
	return has403Pattern.MatchString(stderr)
}

func (c *PythonClient) FallbackOrgProject() (string, string) {
	cfg, err := regcmd.LoadPipRegistryConfig()
	if err != nil {
		return "", ""
	}
	return cfg.OrgID, cfg.ProjectID
}

// DetectRegistry detects the HAR registry from saved pip config or explicit flag.
func (c *PythonClient) DetectRegistry(explicitRegistry string) (*pkgmgr.RegistryInfo, error) {
	if explicitRegistry != "" {
		return &pkgmgr.RegistryInfo{
			RegistryIdentifier: explicitRegistry,
		}, nil
	}

	// Try loading from saved configure config
	cfg, err := regcmd.LoadPipRegistryConfig()
	if err == nil && cfg.RegistryIdentifier != "" {
		log.Info().Str("registry", cfg.RegistryIdentifier).Msg("Using saved pip registry config")
		return &pkgmgr.RegistryInfo{
			RegistryIdentifier: cfg.RegistryIdentifier,
		}, nil
	}

	return nil, fmt.Errorf("no registry configured. Run 'hc registry configure pip --registry <name>' first, or use --registry flag")
}

// RunCommand executes a native pip command with live output streaming.
func (c *PythonClient) RunCommand(command string, args []string) (*pkgmgr.InstallResult, error) {
	pipPath, err := exec.LookPath("pip")
	if err != nil {
		// Try pip3 as fallback
		pipPath, err = exec.LookPath("pip3")
		if err != nil {
			return nil, fmt.Errorf("pip/pip3 not found in PATH: %w", err)
		}
	}

	cmdArgs := append([]string{command}, args...)
	cmd := exec.Command(pipPath, cmdArgs...)
	cmd.Dir = "."
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout

	var stderrBuf bytes.Buffer
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	log.Info().Strs("args", cmdArgs).Msgf("Running pip %s", command)

	err = cmd.Run()
	stderrStr := stderrBuf.String()

	if err != nil {
		return &pkgmgr.InstallResult{Status: "FAILURE", Stderr: stderrStr, Err: err}, nil
	}
	return &pkgmgr.InstallResult{Status: "SUCCESS", Stderr: stderrStr}, nil
}

// ResolveDependencies resolves python dependencies.
// Primary: check for existing lock files (Pipfile.lock, poetry.lock).
// Secondary: run `pip install --dry-run --report` for transitive resolution.
// Fallback: parse requirements.txt / pyproject.toml (direct deps only).
func (c *PythonClient) ResolveDependencies(progress p.Reporter) (*pkgmgr.DependencyResult, error) {
	noop := func() {}

	// Check for existing lock files first (these have transitive deps)
	for _, lockFile := range lockFileSearchOrder {
		if _, err := os.Stat(lockFile); err == nil {
			progress.Step(fmt.Sprintf("Found existing %s, parsing dependencies", lockFile))
			deps, err := regcmd.ParseLockFile(lockFile)
			if err == nil && len(deps) > 0 {
				return &pkgmgr.DependencyResult{Dependencies: deps, Cleanup: noop}, nil
			}
			log.Warn().Err(err).Str("file", lockFile).Msg("Failed to parse lock file, trying next")
		}
	}

	// Try pip install --dry-run --report for transitive resolution
	deps, cleanup, err := runPipDryRunReport()
	if err == nil && len(deps) > 0 {
		progress.Step(fmt.Sprintf("Resolved %d dependencies via pip --dry-run --report", len(deps)))
		return &pkgmgr.DependencyResult{Dependencies: deps, Cleanup: cleanup}, nil
	}
	log.Warn().Err(err).Msg("pip --dry-run --report failed")

	return nil, fmt.Errorf("no python dependencies found. Ensure requirements.txt, Pipfile.lock, or pyproject.toml exists")
}

// pipReportInstall represents the JSON output of `pip install --dry-run --report`.
type pipReportInstall struct {
	Install []struct {
		Metadata struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"metadata"`
		IsDirectURL bool `json:"is_direct"`
	} `json:"install"`
}

// runPipDryRunReport runs `pip install --dry-run --report report.json -r requirements.txt`
// and parses the output for transitive dependency resolution.
func runPipDryRunReport() ([]regcmd.Dependency, func(), error) {
	noop := func() {}

	// Find the requirements source
	reqFile := ""
	for _, candidate := range []string{"requirements.txt", "pyproject.toml"} {
		if _, err := os.Stat(candidate); err == nil {
			reqFile = candidate
			break
		}
	}
	if reqFile == "" {
		return nil, noop, fmt.Errorf("no requirements file found")
	}

	pipPath, err := exec.LookPath("pip")
	if err != nil {
		pipPath, err = exec.LookPath("pip3")
		if err != nil {
			return nil, noop, fmt.Errorf("pip/pip3 not found: %w", err)
		}
	}

	reportFile := ".harness-pip-report.json"
	var cmdArgs []string
	if reqFile == "pyproject.toml" {
		cmdArgs = []string{"install", "--dry-run", "--report", reportFile, "."}
	} else {
		cmdArgs = []string{"install", "--dry-run", "--report", reportFile, "-r", reqFile}
	}

	cmd := exec.Command(pipPath, cmdArgs...)
	cmd.Dir = "."
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	log.Info().Strs("args", cmdArgs).Msg("Running pip install --dry-run --report")

	if err := cmd.Run(); err != nil {
		os.Remove(reportFile)
		return nil, noop, fmt.Errorf("pip --dry-run failed: %w (stderr: %s)", err, stderr.String())
	}

	cleanup := func() {
		if removeErr := os.Remove(reportFile); removeErr != nil {
			log.Warn().Err(removeErr).Msg("Failed to clean up pip report file")
		}
	}

	data, err := os.ReadFile(reportFile)
	if err != nil {
		cleanup()
		return nil, noop, fmt.Errorf("failed to read pip report: %w", err)
	}

	var report pipReportInstall
	if err := json.Unmarshal(data, &report); err != nil {
		cleanup()
		return nil, noop, fmt.Errorf("failed to parse pip report: %w", err)
	}

	var deps []regcmd.Dependency
	seen := make(map[string]bool)
	for _, item := range report.Install {
		name := item.Metadata.Name
		version := item.Metadata.Version
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		deps = append(deps, regcmd.Dependency{
			Name:    name,
			Version: version,
			Source:  "pip-dry-run-report",
		})
	}

	return deps, cleanup, nil
}
