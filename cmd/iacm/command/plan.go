package command

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/util/client/iacm"
	"github.com/harness/harness-cli/util/common/progress"
	"github.com/hashicorp/go-slug"
	"github.com/spf13/cobra"
)

const (
	folderPathWarningMsg   = "The workspace is configured with the folder path %s,\nHarness will upload the following directory and its contents: \n%s"
	noFolderPathWarningMsg = "The workspace has no configured folder path,\nHarness will upload the following directory and its contents \n%s"
	folderPathNotFoundErr  = "The folder path configured in the workspace %s does not exist in the current directory"
	folderPathErr          = "An error occurred when trying to find the repo root from the current current directory: %v"

	startingStepMsg  = "========================== Starting step %s ==========================\n"
	startingStageMsg = "========================== Starting stage %s ==========================\n"
)

func NewPlanCmd() *cobra.Command {
	var (
		workspaceID string
		orgID       string
		projectID   string
		targets     []string
		replacements []string
	)

	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Execute a Terraform plan remotely",
		Long: `Execute a Terraform plan on Harness servers using remote execution.
This command uploads your local Terraform code and executes it remotely,
streaming logs back to your terminal.`,
		Example: `  # Basic plan execution
  hc iacm plan --workspace-id my-workspace

  # Plan with specific targets
  hc iacm plan --workspace-id my-workspace --target resource1 --target resource2

  # Plan with variable replacements
  hc iacm plan --workspace-id my-workspace --replace key1=value1 --replace key2=value2`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if orgID == "" {
				orgID = config.Global.OrgID
			}
			if projectID == "" {
				projectID = config.Global.ProjectID
			}
			if workspaceID == "" {
				return errors.New("workspace-id is required")
			}

			verbose, _ := cmd.Flags().GetBool("verbose")

			iacmClient := iacm.NewIacmClient(verbose)
			logClient := iacm.NewLogClient()
			
			p := progress.NewConsoleReporter()

			return executePlan(cmd.Context(), iacmClient, logClient, p, orgID, projectID, workspaceID, targets, replacements)
		},
	}

	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "Workspace identifier (required)")
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization identifier (defaults to global config)")
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project identifier (defaults to global config)")
	cmd.Flags().StringArrayVar(&targets, "target", []string{}, "Resource targets to plan (can be specified multiple times)")
	cmd.Flags().StringArrayVar(&replacements, "replace", []string{}, "Variable replacements in key=value format (can be specified multiple times)")

	cmd.MarkFlagRequired("workspace-id")

	return cmd
}

func executePlan(
	ctx context.Context,
	iacmClient *iacm.IacmClient,
	logClient *iacm.LogClient,
	p *progress.ConsoleReporter,
	orgID, projectID, workspaceID string,
	targets, replacements []string,
) error {

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n\nInterrupted. Cleaning up...")
		cancel()
	}()

	p.Start("Fetching workspace information")
	workspace, err := iacmClient.GetWorkspace(ctx, orgID, projectID, workspaceID)
	if err != nil {
		p.Error("Failed to fetch workspace")
		return fmt.Errorf("failed to get workspace: %w", err)
	}
	p.Success("Workspace found")

	defaultPipeline, err := getDefaultPipeline(workspace.DefaultPipelines)
	if err != nil {
		p.Error("Failed to get default pipeline")
		return fmt.Errorf("failed to get default pipeline: %w", err)
	}
	fmt.Printf("The plan will execute with the default pipeline: %s... \n", defaultPipeline)

	wd, err := os.Getwd()
	if err != nil {
		p.Error("Failed to get current directory")
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	repoRoot, warning, err := getRepoRootFromWorkingDirectory(wd, workspace)
	if err != nil {
		p.Error("Repository path validation failed")
		return fmt.Errorf("repository path validation failed: %w", err)
	}
	fmt.Println(warning)

	confirm := confirmInput("Do you want to continue?")
	if !confirm {
		return errors.New("user cancelled")
	}

	p.Step("Zipping source code")
	zipData, err := zipSourceCode(repoRoot)
	if err != nil {
		p.Error("Failed to zip source code")
		return fmt.Errorf("failed to zip source code: %w", err)
	}
	p.Success(fmt.Sprintf("Source code zipped (%d bytes)", len(zipData)))

	customArgs := make(map[string][]string)
	if len(replacements) > 0 {
		customArgs["replace"] = replacements
	}
	if len(targets) > 0 {
		customArgs["target"] = targets
	}

	p.Step("Creating remote execution")
	execResp, err := iacmClient.CreateRemoteExecution(ctx, orgID, projectID, workspaceID, customArgs)
	if err != nil {
		p.Error("Failed to create remote execution")
		return fmt.Errorf("failed to create remote execution: %w", err)
	}
	p.Success("Remote execution created")

	p.Step("Uploading source code")
	execResp, err = iacmClient.UploadRemoteExecution(ctx, orgID, projectID, workspaceID, execResp.ID, zipData)
	if err != nil {
		p.Error("Failed to upload source code")
		return fmt.Errorf("failed to upload source code: %w", err)
	}
	p.Success("Source code uploaded")

	p.Step("Triggering pipeline execution")
	execResp, err = iacmClient.ExecuteRemoteExecution(ctx, orgID, projectID, workspaceID, execResp.ID)
	if err != nil {
		p.Error("Failed to trigger execution")
		return fmt.Errorf("failed to trigger execution: %w", err)
	}
	p.Success("Pipeline execution triggered")
	fmt.Printf("Pipeline execution: %s\n", execResp.PipelineExecutionURL)

	p.Step("Getting log token")
	logToken, err := iacmClient.GetLogToken(ctx)
	if err != nil {
		p.Error("Failed to get log token")
		return fmt.Errorf("failed to get log token: %w", err)
	}
	logClient.SetToken(logToken)

	startingNodeID, err := getStartingNodeID(ctx, iacmClient, orgID, projectID, execResp.PipelineExecutionID)
	if err != nil {
		p.Error("Failed to get starting node ID")
		return fmt.Errorf("failed to get starting node ID: %w", err)
	}

	p.Step("Streaming logs")
	fmt.Println("\n=== Pipeline Execution Logs ===")
	err = walkExecutionGraph(ctx, iacmClient, logClient, orgID, projectID, execResp.PipelineExecutionID, startingNodeID)
	if err != nil {
		p.Error("Log streaming failed")
		return fmt.Errorf("log streaming failed: %w", err)
	}

	p.Success("Plan execution completed")
	return nil
}

func getDefaultPipeline(defaultPipelines map[string]*iacm.DefaultPipelineOverride) (string, error) {
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

func getRepoRootFromWorkingDirectory(workingDirectory string, workspace *iacm.Workspace) (string, string, error) {
	if workspace.RepositoryPath == "" {
		return workingDirectory, fmt.Sprintf(noFolderPathWarningMsg, workingDirectory), nil
	}

	workingDirectory = filepath.Clean(workingDirectory)
	repositoryPath := filepath.Clean(workspace.RepositoryPath)
	
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
				repositoryPath,
				repoRoot,
			), nil
	}

	_, err := os.Stat(filepath.Join(workingDirectory, repositoryPath))
	if os.IsNotExist(err) {
		return "", "", fmt.Errorf(folderPathNotFoundErr, repositoryPath)
	}

	return workingDirectory,
		fmt.Sprintf(
			folderPathWarningMsg,
			repositoryPath,
			workingDirectory,
		), nil
}

func confirmInput(prompt string) bool {
	fmt.Print(prompt + " (y/N): ")
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}

type pipelineExecutionGetter interface {
	GetPipelineExecution(ctx context.Context, org, project, executionID string, stageNodeID string) (*iacm.PipelineExecutionDetail, error)
}

func getStartingNodeID(ctx context.Context, iacmClient pipelineExecutionGetter, org, project, executionID string) (string, error) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	timer := time.After(5 * time.Second)
	
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			execution, err := iacmClient.GetPipelineExecution(ctx, org, project, executionID, "")
			if err != nil {
				return "", err
			}
			if execution.PipelineExecutionSummary == nil {
				continue
			}
			startingNodeID := execution.PipelineExecutionSummary.StartingNodeId
			if startingNodeID == "" {
				continue
			}
			layoutNodeMap := execution.PipelineExecutionSummary.LayoutNodeMap
			if layoutNodeMap == nil {
				continue
			}
			startingNode, exists := layoutNodeMap[startingNodeID]
			if !exists {
				continue
			}
			startingNodeStatus := startingNode.Status
			if startingNodeStatus == "Running" {
				return startingNodeID, nil
			}
		case <-timer:
			return "", errors.New("The pipeline execution was not started after 5 seconds")
		}
	}
}

func walkExecutionGraph(ctx context.Context, iacmClient *iacm.IacmClient, logClient *iacm.LogClient, org, project, executionID, startingNodeID string) error {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	stageNodeID := startingNodeID
	visited := map[string]struct{}{}
	
	for {
		select {
		case <-ctx.Done():
			return errors.New("context cancelled")
		case <-ticker.C:
			execution, err := iacmClient.GetPipelineExecution(ctx, org, project, executionID, stageNodeID)
			if err != nil {
				return err
			}
			if execution.PipelineExecutionSummary == nil || execution.ExecutionGraph == nil {
				continue
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
			
			err = walkStage(ctx, iacmClient, logClient, org, project, executionID, stageNodeID, execution.ExecutionGraph.RootNodeId)
			if err != nil {
				return err
			}
		}
	}
}

func walkStage(ctx context.Context, iacmClient *iacm.IacmClient, logClient *iacm.LogClient, org, project, executionID, stageNodeID, rootNodeID string) error {
	execution, err := iacmClient.GetPipelineExecution(ctx, org, project, executionID, stageNodeID)
	if err != nil {
		return err
	}
	if execution.ExecutionGraph == nil {
		return errors.New("execution graph not available")
	}
	rootNodeID = execution.ExecutionGraph.RootNodeId

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	lastStepNodeID := rootNodeID
	visited := map[string]struct{}{}
	
	for {
		select {
		case <-ctx.Done():
			return errors.New("context cancelled")
		case <-ticker.C:
			execution, err := iacmClient.GetPipelineExecution(ctx, org, project, executionID, stageNodeID)
			if err != nil {
				return err
			}
			if execution.PipelineExecutionSummary == nil || execution.ExecutionGraph == nil {
				continue
			}
			
			if lastStepNodeID == "" {
				lastStepNodeID = rootNodeID
			}
			
			stageNode := execution.PipelineExecutionSummary.LayoutNodeMap[stageNodeID]
			var stepNode iacm.ExecutionNode
			
			switch {
			case isActiveStageNode(stageNode.Status):
				stepNode = getNextActiveStep(*execution.ExecutionGraph, lastStepNodeID)
				lastStepNodeID = stepNode.Uuid
				_, ok := visited[lastStepNodeID]
				if lastStepNodeID != "" && !ok {
					visited[lastStepNodeID] = struct{}{}
					go func() {
						fmt.Printf(startingStepMsg, stepNode.Name)
						logKey := getLogKeyFromStepNode(stepNode)
						lineCount, err := logClient.Blob(ctx, logKey)
						if err != nil || lineCount < 1 {
							err := logClient.Tail(ctx, logKey)
							if err != nil {
								fmt.Println(err)
							}
						}
					}()
				}
			case isInactiveStageNode(stageNode.Status):
				stepNode = getNextInactiveStep(*execution.ExecutionGraph, lastStepNodeID)
				if stepNode.Uuid == "" {
					return nil
				}
				lastStepNodeID = stepNode.Uuid
				_, ok := visited[lastStepNodeID]
				if lastStepNodeID != "" && !ok {
					visited[lastStepNodeID] = struct{}{}
					go func() {
						fmt.Printf(startingStepMsg, stepNode.Name)
						logKey := getLogKeyFromStepNode(stepNode)
						_, err := logClient.Blob(ctx, logKey)
						if err != nil {
							fmt.Println(err)
						}
					}()
				}
			}
		}
	}
}

func getNextActiveStage(ctx context.Context, layoutNodeMap map[string]iacm.GraphLayoutNode, stageNodeID string) iacm.GraphLayoutNode {
	node, exists := layoutNodeMap[stageNodeID]
	if !exists {
		return iacm.GraphLayoutNode{}
	}
	if isActiveStageNode(node.Status) && !shouldIgnoreStepType(node.NodeType) {
		return node
	}
	if node.EdgeLayoutList == nil || len(node.EdgeLayoutList.NextIds) < 1 {
		return iacm.GraphLayoutNode{}
	}
	return getNextActiveStage(ctx, layoutNodeMap, node.EdgeLayoutList.NextIds[0])
}

func getNextActiveStep(executionGraph iacm.ExecutionGraph, rootNodeID string) iacm.ExecutionNode {
	node, exists := executionGraph.NodeMap[rootNodeID]
	if !exists {
		return iacm.ExecutionNode{}
	}
	if isActiveStepNode(node.Status) && !shouldIgnoreStepType(node.StepType) {
		return node
	}
	children := []string{}
	adjacencyList, exists := executionGraph.NodeAdjacencyListMap[rootNodeID]
	if exists {
		children = append(children, adjacencyList.Children...)
		children = append(children, adjacencyList.NextIds...)
	}
	for _, child := range children {
		node = getNextActiveStep(executionGraph, child)
		if node.Uuid != "" {
			return node
		}
	}
	return iacm.ExecutionNode{}
}

func getNextInactiveStep(executionGraph iacm.ExecutionGraph, rootNodeID string) iacm.ExecutionNode {
	node, exists := executionGraph.NodeMap[rootNodeID]
	if !exists {
		return iacm.ExecutionNode{}
	}
	adjacencyList, exists := executionGraph.NodeAdjacencyListMap[rootNodeID]
	children := []string{}
	if exists {
		children = append(children, adjacencyList.Children...)
		children = append(children, adjacencyList.NextIds...)
	}
	for _, child := range children {
		node = getNextInactiveStep(executionGraph, child)
		if node.Uuid != "" {
			return node
		}
	}
	return iacm.ExecutionNode{}
}

func isActiveStepNode(status string) bool {
	return status == "Running" || status == "Queued" || status == "AsyncWaiting" || status == "NotStarted"
}

func isActiveStageNode(status string) bool {
	return status == "Running" || status == "Queued" || status == "AsyncWaiting" || status == "NotStarted"
}

func isInactiveStageNode(status string) bool {
	return status == "Success" || status == "Failed" || status == "IgnoreFailed"
}

func shouldIgnoreStepType(stepType string) bool {
	return stepType == "IACMIntegrationStageStepPMS" || 
		   stepType == "IntegrationStageStepPMS" || 
		   stepType == "NG_EXECUTION" || 
		   stepType == "IACMPrepareExecution"
}

func getLogKeyFromStepNode(stepNode iacm.ExecutionNode) string {
	if len(stepNode.ExecutableResponses) >= 1 {
		if len(stepNode.ExecutableResponses[0].Async.LogKeys) >= 1 {
			return stepNode.ExecutableResponses[0].Async.LogKeys[0]
		}
	}
	return stepNode.LogBaseKey
}

func zipSourceCode(repoRoot string) ([]byte, error) {
	var buf bytes.Buffer
	meta, err := slug.Pack(repoRoot, &buf, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create zip: %w", err)
	}
	_ = meta
	return buf.Bytes(), nil
}
