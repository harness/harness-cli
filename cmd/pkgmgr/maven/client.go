package maven

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/harness/harness-cli/cmd/pkgmgr"
	regcmd "github.com/harness/harness-cli/cmd/registry/command"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/rs/zerolog/log"
)

// has403Pattern detects 403 Forbidden in maven stderr/stdout.
// Maven outputs errors like: "Could not transfer artifact ... from/to ...: status code: 403"
var has403Pattern = regexp.MustCompile(`(?i)403|[Ff]orbidden`)

// mavenDepTreeLine matches lines from `mvn dependency:tree -DoutputType=text` output.
// Example: "[INFO]    +- org.apache.commons:commons-lang3:jar:3.12.0:compile"
var mavenDepTreeLine = regexp.MustCompile(`[|+\-\s]+(\S+):(\S+):(\S+):(\S+):(\S+)`)

// lockFileSearchOrder lists files to check for maven dependency resolution.
var lockFileSearchOrder = []string{
	"pom.xml",
	"build.gradle",
	"build.gradle.kts",
}

// MavenClient implements pkgmgr.Client for Maven.
type MavenClient struct{}

func NewClient() *MavenClient {
	return &MavenClient{}
}

func (c *MavenClient) Name() string        { return "mvn" }
func (c *MavenClient) PackageType() string { return "maven" }

func (c *MavenClient) DetectFirewallError(stderr string) bool {
	return has403Pattern.MatchString(stderr)
}

func (c *MavenClient) FallbackOrgProject() (string, string) {
	cfg, err := regcmd.LoadMavenRegistryConfig()
	if err != nil {
		return "", ""
	}
	return cfg.OrgID, cfg.ProjectID
}

// DetectRegistry detects the HAR registry from saved maven config or explicit flag.
func (c *MavenClient) DetectRegistry(explicitRegistry string) (*pkgmgr.RegistryInfo, error) {
	if explicitRegistry != "" {
		return &pkgmgr.RegistryInfo{
			RegistryIdentifier: explicitRegistry,
		}, nil
	}

	// Try loading from saved configure config
	cfg, err := regcmd.LoadMavenRegistryConfig()
	if err == nil && cfg.RegistryIdentifier != "" {
		log.Info().Str("registry", cfg.RegistryIdentifier).Msg("Using saved maven registry config")
		return &pkgmgr.RegistryInfo{
			RegistryIdentifier: cfg.RegistryIdentifier,
		}, nil
	}

	return nil, fmt.Errorf("no registry configured. Run 'hc registry configure maven --registry <name>' first, or use --registry flag")
}

// RunCommand executes a native maven command with live output streaming.
// If a project-level .mvn/settings.xml exists and no -s flag is provided, it adds -s .mvn/settings.xml.
func (c *MavenClient) RunCommand(command string, args []string) (*pkgmgr.InstallResult, error) {
	mvnPath, err := exec.LookPath("mvn")
	if err != nil {
		return nil, fmt.Errorf("mvn not found in PATH: %w", err)
	}

	cmdArgs := append([]string{command}, args...)

	// Auto-inject project-level settings.xml if it exists and user didn't specify -s
	if !containsSettingsFlag(args) {
		if _, err := os.Stat(".mvn/settings.xml"); err == nil {
			log.Info().Msg("Using project-level .mvn/settings.xml")
			cmdArgs = append(cmdArgs, "-s", ".mvn/settings.xml")
		}
	}

	cmd := exec.Command(mvnPath, cmdArgs...)
	cmd.Dir = "."
	cmd.Stdin = os.Stdin

	// Maven writes [ERROR] lines (including 403) to stdout, so capture both streams
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	log.Info().Strs("args", cmdArgs).Msgf("Running mvn %s", command)

	err = cmd.Run()
	combinedOutput := stdoutBuf.String() + "\n" + stderrBuf.String()

	if err != nil {
		return &pkgmgr.InstallResult{Status: "FAILURE", Stderr: combinedOutput, Err: err}, nil
	}
	return &pkgmgr.InstallResult{Status: "SUCCESS", Stderr: combinedOutput}, nil
}

// ResolveDependencies resolves maven dependencies.
// Primary: run `mvn dependency:tree` and parse the output.
// Fallback: parse pom.xml / build.gradle (direct deps only).
func (c *MavenClient) ResolveDependencies(progress p.Reporter) (*pkgmgr.DependencyResult, error) {
	noop := func() {}

	// Try mvn dependency:tree for transitive deps
	deps, err := runMvnDependencyTree()
	if err == nil && len(deps) > 0 {
		progress.Step(fmt.Sprintf("Resolved %d dependencies via mvn dependency:tree", len(deps)))
		return &pkgmgr.DependencyResult{Dependencies: deps, Cleanup: noop}, nil
	}
	log.Warn().Err(err).Msg("mvn dependency:tree failed, falling back to build file parsing")

	// Fallback: parse build files
	for _, buildFile := range lockFileSearchOrder {
		if _, statErr := os.Stat(buildFile); statErr == nil {
			progress.Step(fmt.Sprintf("Parsing %s for dependencies", buildFile))
			deps, parseErr := regcmd.ParseLockFile(buildFile)
			if parseErr == nil && len(deps) > 0 {
				return &pkgmgr.DependencyResult{Dependencies: deps, Cleanup: noop}, nil
			}
			log.Warn().Err(parseErr).Str("file", buildFile).Msg("Failed to parse build file")
		}
	}

	return nil, fmt.Errorf("no maven dependencies found. Ensure pom.xml or build.gradle exists")
}

// runMvnDependencyTree runs `mvn dependency:tree` and parses the output.
func runMvnDependencyTree() ([]regcmd.Dependency, error) {
	mvnPath, err := exec.LookPath("mvn")
	if err != nil {
		return nil, fmt.Errorf("mvn not found: %w", err)
	}

	cmdArgs := []string{"dependency:tree", "-DoutputType=text", "-q"}

	// Use project-level settings.xml if it exists
	if _, err := os.Stat(".mvn/settings.xml"); err == nil {
		cmdArgs = append(cmdArgs, "-s", ".mvn/settings.xml")
	}

	cmd := exec.Command(mvnPath, cmdArgs...)
	cmd.Dir = "."

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	log.Info().Msg("Running mvn dependency:tree")

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("mvn dependency:tree failed: %w (stderr: %s)", err, stderr.String())
	}

	return parseMvnDependencyTree(stdout.String()), nil
}

// parseMvnDependencyTree parses the output of `mvn dependency:tree -DoutputType=text`.
// Lines look like: "[INFO]    +- org.apache.commons:commons-lang3:jar:3.12.0:compile"
func parseMvnDependencyTree(output string) []regcmd.Dependency {
	var deps []regcmd.Dependency
	seen := make(map[string]bool)

	for _, line := range strings.Split(output, "\n") {
		// Strip [INFO] prefix if present
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[INFO]") {
			line = strings.TrimSpace(line[6:])
		}

		matches := mavenDepTreeLine.FindStringSubmatch(line)
		if len(matches) < 5 {
			continue
		}

		groupId := matches[1]
		artifactId := matches[2]
		// matches[3] is packaging (jar, war, etc.)
		version := matches[4]
		name := groupId + ":" + artifactId

		if seen[name] {
			continue
		}
		seen[name] = true

		deps = append(deps, regcmd.Dependency{
			Name:    name,
			Version: version,
			Source:  "mvn-dependency-tree",
		})
	}

	return deps
}

// containsSettingsFlag checks if args already contain -s or --settings.
func containsSettingsFlag(args []string) bool {
	for _, a := range args {
		if a == "-s" || a == "--settings" || strings.HasPrefix(a, "-s=") || strings.HasPrefix(a, "--settings=") {
			return true
		}
	}
	return false
}
