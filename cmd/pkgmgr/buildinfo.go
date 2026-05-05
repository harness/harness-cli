package pkgmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

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
	org, project string,
	progress p.Reporter,
) {
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
		Status:   status,
		Metadata: nodes,
	}

	pipelineCtx := buildPipelineContext(org, project)
	if pipelineCtx != nil {
		body.PipelineContext = pipelineCtx
	}

	params := &ar_v3.AddBuildInfoParams{
		AccountIdentifier: config.Global.AccountID,
	}

	v3Client := f.RegistryV3HttpClient()
	if overrideURL := os.Getenv("BUILD_INFO_URL"); overrideURL != "" {
		log.Info().Str("url", overrideURL).Msg("Using BUILD_INFO_URL override for build info upload")
		override, oErr := ar_v3.NewClientWithResponses(overrideURL)
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
	case "npm":
		return detectNpmRootPackage()
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

func packageTypeToAPI(pkgType string) ar_v3.PackageType {
	switch pkgType {
	case "npm":
		return "NPM"
	case "maven":
		return "MAVEN"
	case "pypi", "pip":
		return "PYTHON"
	case "nuget":
		return "NUGET"
	default:
		return ar_v3.PackageType(pkgType)
	}
}

func buildPipelineContext(org, project string) *ar_v3.PipelineContext {
	pipelineID := os.Getenv("HARNESS_PIPELINE_ID")
	executionID := os.Getenv("HARNESS_EXECUTION_ID")
	stageID := os.Getenv("HARNESS_STAGE_ID")

	if pipelineID == "" || executionID == "" {
		return nil
	}

	if stageID == "" {
		stageID = "default"
	}

	ctx := &ar_v3.PipelineContext{
		PipelineId:  pipelineID,
		ExecutionId: executionID,
		OrgId:       org,
		ProjectId:   project,
		StageId:     stageID,
	}

	if stepID := os.Getenv("HARNESS_STEP_ID"); stepID != "" {
		ctx.StepId = &stepID
	}

	return ctx
}
