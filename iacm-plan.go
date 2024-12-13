package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"harness/client"
	"harness/utils"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/hashicorp/go-slug"

	"github.com/urfave/cli/v2"
)

const (
	orgIdFlag       = "org-id"
	projectIdFlag   = "project-id"
	workspaceIdFlag = "workspace-id"

	folderPathWarningMsg   = "The workspace is configured with the folder path %s,\nHarness will upload the following directory and its contents: \n%s"
	noFolderPathWarningMsg = "The workspace has no configured folder path,\nHarness will upload the following directory and its contents \n%s"
	folderPathNotFoundErr  = "The folder path configured in the workspace %s does not exist in the current directory"
	folderPathErr          = "An error occurred when trying to find the folder path in the current directory: %v"
)

type Client interface {
	GetWorkspace(ctx context.Context, org string, project string, workspace string) (*client.Workspace, error)
	CreateRemoteExecution(ctx context.Context, org, project, workspace string) (*client.RemoteExecution, error)
	UploadRemoteExecution(ctx context.Context, org, project, workspace, id string, file []byte) (*client.RemoteExecution, error)
	ExecuteRemoteExecution(ctx context.Context, org, project, workspace, id string) (*client.RemoteExecution, error)
}

type IacmCommand struct {
	client  Client
	account string
	debug   bool
}

func NewIacmCommand(account string, client Client) IacmCommand {
	return IacmCommand{
		account: account,
		client:  client,
	}
}

func (c IacmCommand) executePlan(ctx context.Context, org string, project string, workspace string) error {
	fmt.Printf("Fetching workspace %s...", workspace)
	ws, err := c.client.GetWorkspace(ctx, org, project, workspace)
	if err != nil {
		fmt.Println(utils.GetColoredText(fmt.Sprintf("An error occurred when fetching the workspace: %v", err.Error()), color.FgRed))
		return err
	}

	defaultPipeline, err := getDefaultPipeline(ws.DefaultPipelines)
	if err != nil {
		fmt.Println(utils.GetColoredText(err.Error(), color.FgRed))
	}
	fmt.Printf("Condigured default pipeline: %s...", defaultPipeline)

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	warning, err := validateWorkdingDirectory(wd, ws)
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
	_, err = packer.Pack(wd, archive)
	if err != nil {
		return err
	}

	plan, err := c.client.CreateRemoteExecution(ctx, org, project, workspace)
	if err != nil {
		fmt.Println(utils.GetColoredText(fmt.Sprintf("An error occured creating the remote execution: %v", err.Error()), color.FgRed))
		return err
	}

	plan, err = c.client.UploadRemoteExecution(ctx, org, project, workspace, plan.ID, archive.Bytes())
	if err != nil {
		fmt.Println(utils.GetColoredText(fmt.Sprintf("An error occured uplaoading the source code: %v", err.Error()), color.FgRed))
		return err
	}

	plan, err = c.client.ExecuteRemoteExecution(ctx, org, project, workspace, plan.ID)
	if err != nil {
		fmt.Println(utils.GetColoredText(fmt.Sprintf("An error occured exeucting the pipeline: %v", err.Error()), color.FgRed))
		return err
	}
	fmt.Printf("Pipeline execution: %s", plan.PipelineExecutionURL)
	return nil
}

func (c IacmCommand) ExecutePlan(ctx *cli.Context) error {
	org := ctx.String(orgIdFlag)
	project := ctx.String(projectIdFlag)
	workspace := ctx.String(workspaceIdFlag)

	err := c.executePlan(ctx.Context, org, project, workspace)
	if err != nil && c.debug {
		return err
	}

	return nil
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

func validateWorkdingDirectory(workingDirectory string, workspace *client.Workspace) (string, error) {
	if workspace.RepositoryPath != "" {
		_, err := os.Stat(filepath.Join(workingDirectory, workspace.RepositoryPath))
		if err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf(folderPathNotFoundErr, workspace.RepositoryPath)
			}
			return "", fmt.Errorf(folderPathErr, err)
		}
		return fmt.Sprintf(
			folderPathWarningMsg,
			utils.GetColoredText(workspace.RepositoryPath, color.FgCyan),
			utils.GetColoredText(workingDirectory, color.FgCyan),
		), nil
	}
	return fmt.Sprintf(noFolderPathWarningMsg, utils.GetColoredText(workingDirectory, color.FgCyan)), nil
}
