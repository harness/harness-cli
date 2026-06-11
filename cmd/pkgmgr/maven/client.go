package maven

import (
	"bytes"
	"encoding/xml"
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

var harURLPattern = regexp.MustCompile(`(?:https?://[^/]+)/(?:pkg/)?([^/]+)/([^/]+)/maven/?`)

var has403Pattern = regexp.MustCompile(`(?i)(403\s*Forbidden|status\s*code:\s*403|Return code is:\s*403|HTTP/\S+\s+403)`)

type MavenClient struct{}

func NewClient() *MavenClient {
	return &MavenClient{}
}

func (c *MavenClient) Name() string        { return pkgmgr.CommandMvn }
func (c *MavenClient) PackageType() string { return pkgmgr.PackageTypeMaven }

func (c *MavenClient) FallbackOrgProject() (string, string) {
	savedCfg, err := regcmd.LoadMavenRegistryConfig()
	if err != nil || savedCfg == nil {
		return "", ""
	}
	return savedCfg.OrgID, savedCfg.ProjectID
}

func (c *MavenClient) DetectFirewallError(stderr string) bool {
	return has403Pattern.MatchString(stderr)
}

func (c *MavenClient) DetectRegistry(explicitRegistry string) (*pkgmgr.RegistryInfo, error) {
	savedCfg, err := regcmd.LoadMavenRegistryConfig()
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

	settingsPaths := []string{}
	if homeDir, err := os.UserHomeDir(); err == nil {
		settingsPaths = append(settingsPaths, filepath.Join(homeDir, ".m2", "settings.xml"))
	}

	for _, path := range settingsPaths {
		info, err := parseSettingsXMLForHAR(path, explicitRegistry)
		if err == nil && info != nil {
			return info, nil
		}
	}

	if explicitRegistry != "" {
		return nil, fmt.Errorf("HAR registry '%s' not found in Maven settings.xml", explicitRegistry)
	}
	return nil, fmt.Errorf("no HAR registry found. Run 'hc registry configure maven' first")
}

func (c *MavenClient) RunCommand(command string, args []string) (*pkgmgr.InstallResult, error) {
	cmdArgs := append([]string{command}, args...)
	cmd := exec.Command("mvn", cmdArgs...)
	cmd.Dir = "."
	cmd.Stdin = os.Stdin

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	log.Info().Strs("args", cmdArgs).Msgf("Running mvn %s", command)

	err := cmd.Run()
	combinedOutput := stdoutBuf.String() + "\n" + stderrBuf.String()

	if err != nil {
		return &pkgmgr.InstallResult{
			Status: "FAILURE",
			Stderr: combinedOutput,
			Err:    err,
		}, nil
	}
	return &pkgmgr.InstallResult{
		Status: "SUCCESS",
		Stderr: combinedOutput,
	}, nil
}

func (c *MavenClient) ResolveDependencies(progress p.Reporter) (*pkgmgr.DependencyResult, error) {
	noop := func() {}

	progress.Step("Running mvn dependency:tree to resolve transitive dependencies")
	log.Info().Msg("Running mvn dependency:tree -DoutputType=text")

	cmd := exec.Command("mvn", "dependency:tree", "-DoutputType=text")
	cmd.Dir = "."
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log.Warn().Err(err).Str("stderr", stderr.String()).Msg("mvn dependency:tree failed")
		progress.Step("dependency:tree failed, falling back to pom.xml (direct deps only)")
		if _, statErr := os.Stat("pom.xml"); statErr == nil {
			deps, parseErr := parsePomForDeps("pom.xml")
			return &pkgmgr.DependencyResult{Dependencies: deps, Cleanup: noop}, parseErr
		}
		return nil, fmt.Errorf("mvn dependency:tree failed and no pom.xml found: %w", err)
	}

	deps := parseDependencyTree(stdout.String())
	if len(deps) == 0 {
		progress.Step("No dependencies parsed from dependency:tree, falling back to pom.xml")
		if _, statErr := os.Stat("pom.xml"); statErr == nil {
			pomDeps, parseErr := parsePomForDeps("pom.xml")
			return &pkgmgr.DependencyResult{Dependencies: pomDeps, Cleanup: noop}, parseErr
		}
		return nil, fmt.Errorf("no dependencies resolved from mvn dependency:tree")
	}

	return &pkgmgr.DependencyResult{Dependencies: deps, Cleanup: noop}, nil
}

// parseDependencyTree parses the output of `mvn dependency:tree -DoutputType=text`.
// Each line looks like:
// [INFO] +- com.google.guava:guava:jar:31.1-jre:compile
// [INFO] |  \- com.google.guava:failureaccess:jar:1.0.1:compile
func parseDependencyTree(output string) []regcmd.Dependency {
	deps := make([]regcmd.Dependency, 0)
	seen := make(map[string]bool)

	// Pattern: groupId:artifactId:packaging:version:scope
	depPattern := regexp.MustCompile(`[|+\\\- ]+\s*(\S+):(\S+):(\S+):(\S+):(\S+)`)

	// Track parent by depth level
	var parentStack []string

	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, "[INFO]") {
			continue
		}
		line = strings.TrimPrefix(line, "[INFO] ")

		matches := depPattern.FindStringSubmatch(line)
		if len(matches) < 6 {
			continue
		}

		groupID := matches[1]
		artifactID := matches[2]
		version := matches[4]
		name := groupID + ":" + artifactID

		if seen[name+"@"+version] {
			continue
		}
		seen[name+"@"+version] = true

		// Calculate depth from tree indentation
		depth := calculateDepth(line)

		// Trim parent stack to current depth
		if depth < len(parentStack) {
			parentStack = parentStack[:depth]
		}

		parent := ""
		if len(parentStack) > 0 {
			parent = parentStack[len(parentStack)-1]
		}

		deps = append(deps, regcmd.Dependency{
			Name:    name,
			Version: version,
			Source:  "mvn-dependency-tree",
			Parent:  parent,
		})

		// Push this dep as potential parent for deeper deps
		parentStack = append(parentStack, name+"@"+version)
	}

	return deps
}

func calculateDepth(line string) int {
	// Each level of indentation in Maven tree is 3 characters ("+- ", "|  ", "\- ")
	trimmed := strings.TrimPrefix(line, "[INFO] ")
	if trimmed == "" {
		return 0
	}
	// Count sets of 3 tree characters
	depth := 0
	for i := 0; i < len(trimmed); i += 3 {
		if i+2 < len(trimmed) {
			chunk := trimmed[i : i+3]
			if chunk == "+- " || chunk == "\\- " || chunk == "|  " || chunk == "   " {
				depth++
			} else {
				break
			}
		}
	}
	return depth
}

// parsePomForDeps is a fallback that parses pom.xml for direct dependencies only.
func parsePomForDeps(pomPath string) ([]regcmd.Dependency, error) {
	data, err := os.ReadFile(pomPath)
	if err != nil {
		return nil, err
	}

	type PomDep struct {
		GroupId    string `xml:"groupId"`
		ArtifactId string `xml:"artifactId"`
		Version    string `xml:"version"`
	}

	type PomProject struct {
		XMLName      xml.Name `xml:"project"`
		Dependencies struct {
			Dependency []PomDep `xml:"dependency"`
		} `xml:"dependencies"`
		DependencyManagement struct {
			Dependencies struct {
				Dependency []PomDep `xml:"dependency"`
			} `xml:"dependencies"`
		} `xml:"dependencyManagement"`
	}

	var pom PomProject
	if err := xml.Unmarshal(data, &pom); err != nil {
		return nil, fmt.Errorf("failed to parse pom.xml: %w", err)
	}

	deps := make([]regcmd.Dependency, 0)
	seen := make(map[string]bool)

	allDeps := append(pom.Dependencies.Dependency, pom.DependencyManagement.Dependencies.Dependency...)
	for _, dep := range allDeps {
		if dep.GroupId == "" || dep.ArtifactId == "" {
			continue
		}
		name := dep.GroupId + ":" + dep.ArtifactId
		if seen[name] {
			continue
		}
		seen[name] = true

		version := dep.Version
		if version == "" {
			version = "latest"
		}

		deps = append(deps, regcmd.Dependency{
			Name:    name,
			Version: version,
			Source:  "pom.xml",
		})
	}

	return deps, nil
}

// --- settings.xml parsing ---

func parseSettingsXMLForHAR(settingsPath, explicitRegistry string) (*pkgmgr.RegistryInfo, error) {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil, err
	}

	type Mirror struct {
		ID  string `xml:"id"`
		URL string `xml:"url"`
	}
	type Server struct {
		ID       string `xml:"id"`
		Username string `xml:"username"`
		Password string `xml:"password"`
	}
	type Settings struct {
		XMLName xml.Name `xml:"settings"`
		Mirrors struct {
			Mirror []Mirror `xml:"mirror"`
		} `xml:"mirrors"`
		Servers struct {
			Server []Server `xml:"server"`
		} `xml:"servers"`
	}

	var settings Settings
	if err := xml.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("failed to parse settings.xml: %w", err)
	}

	// Find a mirror that matches HAR URL pattern
	for _, mirror := range settings.Mirrors.Mirror {
		if !harURLPattern.MatchString(mirror.URL) {
			continue
		}

		matches := harURLPattern.FindStringSubmatch(mirror.URL)
		if len(matches) < 3 {
			continue
		}

		registryIdentifier := matches[2]
		if explicitRegistry != "" && registryIdentifier != explicitRegistry {
			continue
		}

		// Find corresponding server credentials
		var authToken string
		for _, server := range settings.Servers.Server {
			if server.ID == mirror.ID {
				authToken = server.Password
				break
			}
		}

		return &pkgmgr.RegistryInfo{
			RegistryURL:        mirror.URL,
			AccountID:          matches[1],
			RegistryIdentifier: registryIdentifier,
			AuthToken:          authToken,
		}, nil
	}

	return nil, fmt.Errorf("no HAR registry URL found in %s", settingsPath)
}
