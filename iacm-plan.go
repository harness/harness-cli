package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"harness/client"
	"harness/utils"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/hashicorp/go-slug"
	"gopkg.in/yaml.v3"

	"github.com/urfave/cli/v2"
)

const (
	orgIdFlag       = "org-id"
	projectIdFlag   = "project-id"
	workspaceIdFlag = "workspace-id"
	targetFlag      = "target"
	replaceFlag     = "replace"

	folderPathWarningMsg   = "The workspace is configured with the folder path %s,\nHarness will upload the following directory and its contents: \n%s"
	noFolderPathWarningMsg = "The workspace has no configured folder path,\nHarness will upload the following directory and its contents \n%s"
	folderPathNotFoundErr  = "The folder path configured in the workspace %s does not exist in the current directory"
	folderPathErr          = "An error occurred when trying to find the repo root from the current current directory: %v"

	workspaceInfoCliArgErr = "When supplying workspace info via CLI arguments the org-id, project-id and workspace-id must all be present"
	workspaceInfoFileErr   = "No workspace.yaml file present in the .harness folder in the current directory, consider creating one or supplying workspace info via cli argument"

	startingStepMsg  = "========================== Starting step %s ==========================\n"
	startingStageMsg = "========================== Starting stage %s ==========================\n"
)

type IacmClient interface {
	GetWorkspace(ctx context.Context, org string, project string, workspace string) (*client.Workspace, error)
	CreateRemoteExecution(ctx context.Context, org, project, workspace string, customArguments map[string][]string) (*client.RemoteExecution, error)
	UploadRemoteExecution(ctx context.Context, org, project, workspace, id string, file []byte) (*client.RemoteExecution, error)
	ExecuteRemoteExecution(ctx context.Context, org, project, workspace, id string) (*client.RemoteExecution, error)
	GetPipelineExecution(ctx context.Context, org, project, executionID string, stageNodeId string) (*client.PipelineExecutionDetail, error)
	GetLogToken(ctx context.Context) (string, error)
}

type LogClient interface {
	Tail(ctx context.Context, key string) error
	Blob(ctx context.Context, key string) (int, error)
}

type WorkspaceInfo struct {
	Org       string `yaml:"org"`
	Project   string `yaml:"project"`
	Workspace string `yaml:"workspace"`
}

type IacmCommand struct {
	client       IacmClient
	logClient    LogClient
	account      string
	org          string
	project      string
	workspace    string
	debug        bool
	targets      []string
	replacements []string
}

func NewIacmCommand(account string, client IacmClient, logClient LogClient) IacmCommand {
	return IacmCommand{
		account:   account,
		client:    client,
		logClient: logClient,
	}
}

func (c IacmCommand) executePlan(ctx context.Context) error {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Println(utils.GetColoredText(err.Error(), color.FgRed))
		return err
	}

	workspaceInfo, err := getWorkspaceInfo(c.org, c.project, c.workspace, wd)
	if err != nil {
		fmt.Println(utils.GetColoredText(err.Error(), color.FgRed))
		return err
	}

	fmt.Printf("Fetching workspace %s... \n", workspaceInfo.Workspace)
	ws, err := c.client.GetWorkspace(ctx, workspaceInfo.Org, workspaceInfo.Project, workspaceInfo.Workspace)
	if err != nil {
		fmt.Printf("An error occurred when fetching the workspace: %v \n", err)
		return err
	}

	defaultPipeline, err := getDefaultPipeline(ws.DefaultPipelines)
	if err != nil {
		fmt.Println(utils.GetColoredText(err.Error(), color.FgRed))
		return err
	}
	fmt.Printf(
		"The plan will execute with the default pipeline: %s... \n",
		utils.GetColoredText(defaultPipeline, color.FgCyan),
	)

	repoRoot, warning, err := getRepoRootFromWorkingDirectory(wd, ws)
	if err != nil {
		fmt.Println(utils.GetColoredText(err.Error(), color.FgRed))
		return err
	}

	fmt.Println(warning)

	confirm := utils.ConfirmInput("Do you want to continue?")
	if !confirm {
		return errors.New("some error")
	}

	packer, err := slug.NewPacker(slug.ApplyTerraformIgnore())
	if err != nil {
		return err
	}

	archive := bytes.NewBuffer([]byte{})
	_, err = packer.Pack(repoRoot, archive)
	if err != nil {
		return err
	}

	customArguments := map[string][]string{}
	if len(c.replacements) > 0 {
		customArguments["replace"] = c.replacements
	}
	if len(c.targets) > 0 {
		customArguments["target"] = c.targets
	}
	plan, err := c.client.CreateRemoteExecution(ctx, ws.Org, ws.Project, ws.Identifier, customArguments)
	if err != nil {
		fmt.Println(utils.GetColoredText(fmt.Sprintf("An error occurred creating the remote execution: %v", err.Error()), color.FgRed))
		return err
	}

	plan, err = c.client.UploadRemoteExecution(ctx, ws.Org, ws.Project, ws.Identifier, plan.ID, archive.Bytes())
	if err != nil {
		fmt.Println(utils.GetColoredText(fmt.Sprintf("An error occurred uploading the source code: %v", err.Error()), color.FgRed))
		return err
	}

	plan, err = c.client.ExecuteRemoteExecution(ctx, ws.Org, ws.Project, ws.Identifier, plan.ID)
	if err != nil {
		fmt.Println(utils.GetColoredText(fmt.Sprintf("An error occurred executing the pipeline: %v", err.Error()), color.FgRed))
		return err
	}
	fmt.Printf("Pipeline execution: %s\n", utils.GetColoredText(plan.PipelineExecutionURL, color.FgCyan))

	startingNodeID, err := c.getStartingNodeID(ctx, ws.Org, ws.Project, plan.PipelineExecutionID)
	if err != nil {
		fmt.Println(utils.GetColoredText(fmt.Sprintf("An error occurred fetching starting node id: %v", err.Error()), color.FgRed))
		return err
	}

	err = c.walkExecutionGraph(ctx, plan.PipelineExecutionID, startingNodeID)
	if err != nil {
		fmt.Println(utils.GetColoredText(fmt.Sprintf("An error occurred: %v", err.Error()), color.FgRed))
		return err
	}
	return nil
}

func (c IacmCommand) ExecutePlan(ctx *cli.Context) error {
	org := ctx.String(orgIdFlag)
	project := ctx.String(projectIdFlag)
	workspace := ctx.String(workspaceIdFlag)
	target := ctx.StringSlice(targetFlag)
	replacements := ctx.StringSlice(replaceFlag)
	c.org = org
	c.project = project
	c.workspace = workspace
	c.targets = target
	c.replacements = replacements
	err := c.executePlan(gracefulShutdown(ctx.Context))
	if err != nil && c.debug {
		return err
	}
	return nil
}

func gracefulShutdown(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		select {
		case <-c:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx
}

func getDefaultPipeline(defaultPipelines map[string]*client.DefaultPipelineOverride) (string, error) {
	err := errors.New("The workspace has no configured default pipeline")
	dp, ok := defaultPipelines["plan"]
	if !ok {
		return "", err
	}
	if dp.WorkspacePipeline != nil {
		return *dp.WorkspacePipeline, nil
	}
	if dp.ProjectPipeline != nil {
		return *dp.ProjectPipeline, nil
	}
	return "", err
}

func getRepoRootFromWorkingDirectory(workingDirectory string, workspace *client.Workspace) (string, string, error) {
	if workspace.RepositoryPath == "" {
		return workingDirectory, fmt.Sprintf(noFolderPathWarningMsg, utils.GetColoredText(workingDirectory, color.FgCyan)), nil
	}

	workingDirectory = filepath.Clean(workingDirectory)
	repositoryPath := filepath.Clean(workspace.RepositoryPath)
	// if the working directory is the same as the configured workspace repository
	// path we trim the repository path from the working directory to find the repo
	// root
	if strings.HasSuffix(workingDirectory, repositoryPath) {
		repoRoot := strings.TrimSuffix(workingDirectory, workspace.RepositoryPath)
		repoRoot = filepath.Clean(repoRoot)
		_, err := os.Stat(repoRoot)
		if err != nil {
			return "", "", fmt.Errorf(folderPathErr, err)
		}
		return repoRoot,
			fmt.Sprintf(
				folderPathWarningMsg,
				utils.GetColoredText(repositoryPath, color.FgCyan),
				utils.GetColoredText(repoRoot, color.FgCyan),
			), nil
	}

	// the working directory is not the repository path so we try and find
	// the repository path within the working directory and if found use the
	// working directory as the repo root
	_, err := os.Stat(filepath.Join(workingDirectory, repositoryPath))
	if os.IsNotExist(err) {
		return "", "", fmt.Errorf(folderPathNotFoundErr, repositoryPath)
	}

	return workingDirectory,
		fmt.Sprintf(
			folderPathWarningMsg,
			utils.GetColoredText(repositoryPath, color.FgCyan),
			utils.GetColoredText(workingDirectory, color.FgCyan),
		), nil
}

// getWorkspaceInfo returns the values supplied directly to the cli if they are present
// and falls back to a config file in .harness/workspace.yaml if not
func getWorkspaceInfo(org, project, workspace, workingDirectory string) (*WorkspaceInfo, error) {
	if org != "" && project != "" && workspace != "" {
		return &WorkspaceInfo{
			Org:       org,
			Project:   project,
			Workspace: workspace,
		}, nil
	}
	if org == "" && (project != "" || workspace != "") {
		return nil, errors.New(workspaceInfoCliArgErr)
	}
	if project == "" && (org != "" || workspace != "") {
		return nil, errors.New(workspaceInfoCliArgErr)
	}
	if workspace == "" && (org != "" || project != "") {
		return nil, errors.New(workspaceInfoCliArgErr)
	}

	file, err := os.ReadFile(filepath.Join(workingDirectory, ".harness/workspace.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New(workspaceInfoFileErr)
		}
	}
	workspaceInfo := &WorkspaceInfo{}
	err = yaml.Unmarshal(file, workspaceInfo)
	if err != nil {
		return nil, err
	}
	return workspaceInfo, nil
}

func (c *IacmCommand) getStartingNodeID(ctx context.Context, org, project, executionID string) (string, error) {
	ticker := time.NewTicker(500 * time.Millisecond)
	timer := time.After(5 * time.Second)
	for {
		select {
		case <-ctx.Done():
		case <-ticker.C:
			execution, err := c.client.GetPipelineExecution(ctx, org, project, executionID, "")
			if err != nil {
				return "", err
			}
			startingNodeID := execution.PipelineExecutionSummary.StartingNodeId
			startingNodeStatus := execution.PipelineExecutionSummary.LayoutNodeMap[startingNodeID].Status
			if startingNodeID != "" && startingNodeStatus == "Running" {
				return startingNodeID, nil
			}
		case <-timer:
			return "", errors.New("The pipeline execution was not started after 5 seconds")
		}
	}
}

func (c *IacmCommand) walkExecutionGraph(ctx context.Context, executionID string, startingNodeID string) error {
	ticker := time.NewTicker(3 * time.Second)
	stageNodeID := startingNodeID
	visited := map[string]struct{}{}
	for {
		select {
		case <-ctx.Done():
			return errors.New("context cancelled")
		case <-ticker.C:
			execution, err := c.client.GetPipelineExecution(ctx, c.org, c.project, executionID, stageNodeID)
			if err != nil {
				return err
			}
			stageNode := getNextActiveStage(ctx, execution.PipelineExecutionSummary.LayoutNodeMap, stageNodeID)
			if stageNode.NodeUuid == "" {
				return nil
			}
			_, ok := visited[stageNode.NodeUuid]
			if ok {
				continue
			}
			visited[stageNode.NodeUuid] = struct{}{}
			stageNodeID = stageNode.NodeUuid
			fmt.Printf(startingStageMsg, stageNode.Name)
			err = c.walkStage(ctx, executionID, stageNodeID, execution.ExecutionGraph.RootNodeId)
			if err != nil {
				return err
			}
		}
	}
}

func (c *IacmCommand) walkStage(ctx context.Context, executionID string, stageNodeID string, rootNodeID string) error {
	execution, err := c.client.GetPipelineExecution(ctx, c.org, c.project, executionID, stageNodeID)
	if err != nil {
		return err
	}
	rootNodeID = execution.ExecutionGraph.RootNodeId

	ticker := time.NewTicker(1 * time.Second)
	lastStepNodeID := rootNodeID
	visited := map[string]struct{}{}
	for {
		select {
		case <-ctx.Done():
			return errors.New("context cancelled")
		case <-ticker.C:
			execution, err := c.client.GetPipelineExecution(ctx, c.org, c.project, executionID, stageNodeID)
			if err != nil {
				return err
			}
			if lastStepNodeID == "" {
				lastStepNodeID = rootNodeID
			}
			stageNode := execution.PipelineExecutionSummary.LayoutNodeMap[stageNodeID]
			var stepNode client.ExecutionNode
			switch {
			case isActiveStageNode(stageNode.Status):
				stepNode = getNextActiveStep(*execution.ExecutionGraph, lastStepNodeID)
				lastStepNodeID = stepNode.Uuid
				_, ok := visited[lastStepNodeID]
				if lastStepNodeID != "" && !ok {
					go func() {
						fmt.Printf(startingStepMsg, stepNode.Name)
						// some steps finish so quickly that the stream is closed by the time we try and
						// tail it. To mitigate this we try and use the blob endpoint and fall back to tail
						// if it throws an error
						lineCount, err := c.logClient.Blob(ctx, getLogKeyFromStepNode(stepNode))
						if err != nil || lineCount < 1 {
							err := c.logClient.Tail(ctx, getLogKeyFromStepNode(stepNode))
							if err != nil {
								fmt.Println(err)
							}
						}
					}()
					visited[lastStepNodeID] = struct{}{}
				}
			case isInactiveStageNode(stageNode.Status):
				stepNode = getNextInactiveStep(*execution.ExecutionGraph, lastStepNodeID)
				if stepNode.Uuid == "" {
					return nil
				}
				lastStepNodeID = stepNode.Uuid
				_, ok := visited[lastStepNodeID]
				if lastStepNodeID != "" && !ok {
					go func() {
						fmt.Printf(startingStepMsg, stepNode.Name)
						_, err := c.logClient.Blob(ctx, getLogKeyFromStepNode(stepNode))
						if err != nil {
							fmt.Println(err)
						}
					}()
					visited[lastStepNodeID] = struct{}{}
				}
			}
		}
	}
}

func getNextActiveStage(ctx context.Context, layoutNodeMap map[string]client.GraphLayoutNode, stageNodeID string) client.GraphLayoutNode {
	node := layoutNodeMap[stageNodeID]
	if isActiveStageNode(node.Status) && !shouldIgnoreStepType(node.NodeType) {
		return node
	}
	if len(node.EdgeLayoutList.NextIds) < 1 {
		return client.GraphLayoutNode{}
	}
	return getNextActiveStage(ctx, layoutNodeMap, node.EdgeLayoutList.NextIds[0])
}

func getNextActiveStep(executionGraph client.ExecutionGraph, rootNodeID string) client.ExecutionNode {
	node := executionGraph.NodeMap[rootNodeID]
	if isActiveStepNode(node.Status) && !shouldIgnoreStepType(node.StepType) {
		return node
	}
	children := []string{}
	children = append(children, executionGraph.NodeAdjacencyListMap[rootNodeID].Children...)
	children = append(children, executionGraph.NodeAdjacencyListMap[rootNodeID].NextIds...)
	for _, child := range children {
		node = getNextActiveStep(executionGraph, child)
		if node.Uuid != "" {
			return node
		}
	}
	return client.ExecutionNode{}
}

func getNextInactiveStep(executionGraph client.ExecutionGraph, rootNodeID string) client.ExecutionNode {
	node := executionGraph.NodeMap[rootNodeID]
	children := []string{}
	children = append(children, executionGraph.NodeAdjacencyListMap[rootNodeID].Children...)
	children = append(children, executionGraph.NodeAdjacencyListMap[rootNodeID].NextIds...)
	for _, child := range children {
		node = getNextInactiveStep(executionGraph, child)
		if node.Uuid != "" {
			return node
		}
	}
	return client.ExecutionNode{}
}

func isActiveStepNode(status string) bool {
	if status == "Running" || status == "Queued" || status == "AsyncWaiting" || status == "NotStarted" {
		return true
	}
	return false
}

func isActiveStageNode(status string) bool {
	if status == "Running" || status == "Queued" || status == "AsyncWaiting" || status == "NotStarted" {
		return true
	}
	return false
}

func isInactiveStageNode(status string) bool {
	if status == "Success" || status == "Failed" || status == "IgnoreFailed" {
		return true
	}
	return false
}

func shouldIgnoreStepType(stepType string) bool {
	if stepType == "IACMIntegrationStageStepPMS" || stepType == "IntegrationStageStepPMS" || stepType == "NG_EXECUTION" || stepType == "IACMPrepareExecution" {
		return true
	}
	return false
}

func getLogKeyFromStepNode(stepNode client.ExecutionNode) string {
	if len(stepNode.ExecutableResponses) >= 1 {
		if len(stepNode.ExecutableResponses[0].Async.LogKeys) >= 1 {
			return stepNode.ExecutableResponses[0].Async.LogKeys[0]
		}
	}
	return stepNode.LogBaseKey
}
