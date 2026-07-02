package pkgmgr

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/harness/harness-cli/cmd/cmdutils"
	regcmd "github.com/harness/harness-cli/cmd/registry/command"
	"github.com/harness/harness-cli/config"
	ar_v3 "github.com/harness/harness-cli/internal/api/ar_v3"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/rs/zerolog/log"
)

func uploadBuildInfo(
	f *cmdutils.Factory,
	client Client,
	registryUUID uuid.UUID,
	deps []regcmd.Dependency,
	progress p.Reporter,
) {
	pipelineCtx := buildPipelineContext()
	if pipelineCtx == nil {
		progress.Step("Skipping build info upload: required HARNESS_* pipeline env vars are missing")
		return
	}

	progress.Step("Uploading build info")

	rootPkg := detectRootPackage(client)
	rootNodeKey := rootPkg.Name + "@" + rootPkg.Version

	nodes := make([]ar_v3.BuildInfoNode, 0, len(deps))
	for _, dep := range deps {
		nodeKey := dep.Name + "@" + dep.Version
		node := ar_v3.BuildInfoNode{
			NodeKey: nodeKey,
		}
		if dep.Parent != "" {
			parentKey := dep.Parent
			node.ParentNodeKey = &parentKey
		} else {
			node.ParentNodeKey = &rootNodeKey
		}
		nodes = append(nodes, node)
	}

	status := ar_v3.BuildInfoRequestInputStatusFAILURE

	body := ar_v3.BuildInfoRequestInput{
		RegistryId:  openapi_types.UUID(registryUUID),
		PackageType: packageTypeToAPI(client.PackageType()),
		RootPackage: ar_v3.RootPackage{
			Name:    rootPkg.Name,
			Version: rootPkg.Version,
		},
		Status:          status,
		Metadata:        nodes,
		PipelineContext: pipelineCtx,
	}

	params := &ar_v3.AddBuildInfoParams{
		AccountIdentifier: config.Global.AccountID,
	}

	v3Client := f.RegistryV3HttpClient()
	if overrideURL := os.Getenv("BUILD_INFO_URL"); overrideURL != "" {
		log.Info().Str("url", overrideURL).Msg("Using BUILD_INFO_URL override for build info upload")
		override, oErr := f.NewRegistryV3HttpClientWithURL(overrideURL)
		if oErr == nil {
			v3Client = override
		}
		if overrideAccount := os.Getenv("BUILD_INFO_ACCOUNT"); overrideAccount != "" {
			params.AccountIdentifier = overrideAccount
		}
	}

	resp, err := v3Client.AddBuildInfoWithResponse(context.Background(), params, body)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to upload build info")
		progress.Step("Build info upload failed (non-fatal)")
		return
	}

	if resp.StatusCode() == 201 {
		progress.Success(fmt.Sprintf("Build info uploaded (%d nodes)", len(nodes)))
	} else {
		log.Warn().Int("status", resp.StatusCode()).Msg("Build info upload returned unexpected status")
		progress.Step(fmt.Sprintf("Build info upload returned status %d (non-fatal)", resp.StatusCode()))
	}
}

type rootPackageInfo struct {
	Name    string
	Version string
}

func detectRootPackage(client Client) rootPackageInfo {
	switch client.PackageType() {
	case PackageTypeNpm:
		return detectNpmRootPackage()
	case PackageTypeMaven:
		return detectMavenRootPackage()
	case PackageTypePyPI:
		return detectPythonRootPackage()
	case PackageTypeNuGet:
		return detectNugetRootPackage()
	default:
		return rootPackageInfo{Name: "unknown", Version: "0.0.0"}
	}
}

func detectNpmRootPackage() rootPackageInfo {
	data, err := os.ReadFile("package.json")
	if err != nil {
		return rootPackageInfo{Name: "unknown", Version: "0.0.0"}
	}

	var pkg struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return rootPackageInfo{Name: "unknown", Version: "0.0.0"}
	}

	if pkg.Name == "" {
		pkg.Name = "unknown"
	}
	if pkg.Version == "" {
		pkg.Version = "0.0.0"
	}

	return rootPackageInfo{Name: pkg.Name, Version: pkg.Version}
}

func detectMavenRootPackage() rootPackageInfo {
	data, err := os.ReadFile("pom.xml")
	if err != nil {
		return rootPackageInfo{Name: "unknown", Version: "0.0.0"}
	}

	var pom struct {
		XMLName    xml.Name `xml:"project"`
		GroupID    string   `xml:"groupId"`
		ArtifactID string   `xml:"artifactId"`
		Version    string   `xml:"version"`
		Parent     struct {
			GroupID string `xml:"groupId"`
			Version string `xml:"version"`
		} `xml:"parent"`
	}
	if err := xml.Unmarshal(data, &pom); err != nil {
		return rootPackageInfo{Name: "unknown", Version: "0.0.0"}
	}

	groupID := pom.GroupID
	if groupID == "" {
		groupID = pom.Parent.GroupID
	}
	version := pom.Version
	if version == "" {
		version = pom.Parent.Version
	}
	if groupID == "" || pom.ArtifactID == "" {
		return rootPackageInfo{Name: "unknown", Version: "0.0.0"}
	}
	if version == "" {
		version = "0.0.0"
	}

	return rootPackageInfo{Name: groupID + ":" + pom.ArtifactID, Version: version}
}

func detectPythonRootPackage() rootPackageInfo {
	// Try pyproject.toml first
	if data, err := os.ReadFile("pyproject.toml"); err == nil {
		// Simple TOML parsing for name and version
		var name, version string
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "name") && strings.Contains(line, "=") {
				parts := strings.SplitN(line, "=", 2)
				name = strings.Trim(strings.TrimSpace(parts[1]), "\"'")
			}
			if strings.HasPrefix(line, "version") && strings.Contains(line, "=") {
				parts := strings.SplitN(line, "=", 2)
				version = strings.Trim(strings.TrimSpace(parts[1]), "\"'")
			}
		}
		if name != "" {
			if version == "" {
				version = "0.0.0"
			}
			return rootPackageInfo{Name: name, Version: version}
		}
	}

	// Try setup.py / setup.cfg
	if data, err := os.ReadFile("setup.cfg"); err == nil {
		var name, version string
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "name") && strings.Contains(line, "=") {
				parts := strings.SplitN(line, "=", 2)
				name = strings.TrimSpace(parts[1])
			}
			if strings.HasPrefix(line, "version") && strings.Contains(line, "=") {
				parts := strings.SplitN(line, "=", 2)
				version = strings.TrimSpace(parts[1])
			}
		}
		if name != "" {
			if version == "" {
				version = "0.0.0"
			}
			return rootPackageInfo{Name: name, Version: version}
		}
	}

	return rootPackageInfo{Name: "unknown", Version: "0.0.0"}
}

func detectNugetRootPackage() rootPackageInfo {
	csprojFiles, _ := filepath.Glob("*.csproj")
	if len(csprojFiles) == 0 {
		return rootPackageInfo{Name: "unknown", Version: "0.0.0"}
	}

	data, err := os.ReadFile(csprojFiles[0])
	if err != nil {
		return rootPackageInfo{Name: "unknown", Version: "0.0.0"}
	}

	type PropertyGroup struct {
		AssemblyName string `xml:"AssemblyName"`
		Version      string `xml:"Version"`
		PackageId    string `xml:"PackageId"`
	}
	type Project struct {
		PropertyGroups []PropertyGroup `xml:"PropertyGroup"`
	}

	var proj Project
	if err := xml.Unmarshal(data, &proj); err != nil {
		// Use filename as fallback
		name := strings.TrimSuffix(filepath.Base(csprojFiles[0]), ".csproj")
		return rootPackageInfo{Name: name, Version: "0.0.0"}
	}

	var name, version string
	for _, pg := range proj.PropertyGroups {
		if pg.PackageId != "" {
			name = pg.PackageId
		} else if pg.AssemblyName != "" && name == "" {
			name = pg.AssemblyName
		}
		if pg.Version != "" {
			version = pg.Version
		}
	}

	if name == "" {
		name = strings.TrimSuffix(filepath.Base(csprojFiles[0]), ".csproj")
	}
	if version == "" {
		version = "0.0.0"
	}

	return rootPackageInfo{Name: name, Version: version}
}

func packageTypeToAPI(pkgType string) ar_v3.PackageType {
	switch pkgType {
	case PackageTypeNpm:
		return "NPM"
	case PackageTypeMaven:
		return "MAVEN"
	case PackageTypePyPI:
		return "PYTHON"
	case PackageTypeNuGet:
		return "NUGET"
	default:
		return ar_v3.PackageType(pkgType)
	}
}

func buildPipelineContext() *ar_v3.PipelineContext {
	pipelineID := os.Getenv("HARNESS_PIPELINE_ID")
	executionID := os.Getenv("HARNESS_EXECUTION_ID")
	orgID := os.Getenv("HARNESS_ORG_ID")
	projectID := os.Getenv("HARNESS_PROJECT_ID")
	stageID := os.Getenv("HARNESS_STAGE_ID")

	var missing []string
	if pipelineID == "" {
		missing = append(missing, "HARNESS_PIPELINE_ID")
	}
	if executionID == "" {
		missing = append(missing, "HARNESS_EXECUTION_ID")
	}
	if orgID == "" {
		missing = append(missing, "HARNESS_ORG_ID")
	}
	if projectID == "" {
		missing = append(missing, "HARNESS_PROJECT_ID")
	}
	if len(missing) > 0 {
		log.Info().Strs("missing", missing).Msg("Skipping build info upload: pipeline env vars missing")
		return nil
	}

	if stageID == "" {
		stageID = "default"
	}

	ctx := &ar_v3.PipelineContext{
		PipelineId:  pipelineID,
		ExecutionId: executionID,
		OrgId:       orgID,
		ProjectId:   projectID,
		StageId:     stageID,
	}

	if stepID := os.Getenv("HARNESS_STEP_ID"); stepID != "" {
		ctx.StepId = &stepID
	}

	return ctx
}
