package main

import (
	"context"
	"errors"
	"fmt"
	"harness/client"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

type GetPipelineExecutionMock struct {
	Err  error
	Resp *client.PipelineExecutionDetail
}

func (m *GetPipelineExecutionMock) GetPipelineExecution(ctx context.Context, org, project, executionID string, stageNodeID string) (*client.PipelineExecutionDetail, error) {
	return m.Resp, m.Err
}
func (m *GetPipelineExecutionMock) GetWorkspace(ctx context.Context, org string, project string, workspace string) (*client.Workspace, error) {
	return nil, nil
}
func (m *GetPipelineExecutionMock) CreateRemoteExecution(ctx context.Context, org, project, workspace string) (*client.RemoteExecution, error) {
	return nil, nil
}
func (m *GetPipelineExecutionMock) UploadRemoteExecution(ctx context.Context, org, project, workspace, id string, file []byte) (*client.RemoteExecution, error) {
	return nil, nil
}
func (m *GetPipelineExecutionMock) ExecuteRemoteExecution(ctx context.Context, org, project, workspace, id string) (*client.RemoteExecution, error) {
	return nil, nil
}
func (m *GetPipelineExecutionMock) GetLogToken(ctx context.Context) (string, error) {
	return "", nil
}

func TestValidateWorkingDirectory(t *testing.T) {
	t.Run("working directory does not contain folder path", func(*testing.T) {
		ws := &client.Workspace{
			RepositoryPath: "tf/aws",
		}
		wd := t.TempDir()
		warning, err := validateWorkingDirectory(wd, ws)
		assert.Empty(t, warning)
		assert.Equal(t, fmt.Sprintf(folderPathNotFoundErr, ws.RepositoryPath), err.Error())
	})
	t.Run("working directory contains folder path", func(*testing.T) {
		ws := &client.Workspace{
			RepositoryPath: "tf/aws",
		}
		wd := t.TempDir()
		err := os.MkdirAll(filepath.Join(wd, ws.RepositoryPath), 0700)
		assert.Nil(t, err)
		warning, err := validateWorkingDirectory(wd, ws)
		assert.Equal(t, fmt.Sprintf(folderPathWarningMsg, ws.RepositoryPath, wd), warning)
		assert.Nil(t, err)
	})
	t.Run("no folder path", func(*testing.T) {
		ws := &client.Workspace{
			RepositoryPath: "",
		}
		wd := t.TempDir()
		err := os.MkdirAll(filepath.Join(wd, ws.RepositoryPath), 0700)
		assert.Nil(t, err)
		warning, err := validateWorkingDirectory(wd, ws)
		assert.Equal(t, fmt.Sprintf(noFolderPathWarningMsg, wd), warning)
		assert.Nil(t, err)
	})
}

func TestGetDefaultPipeline(t *testing.T) {
	errMsg := "The workspace has no configured default pipeline"
	projectPipeline := "projectPipeline"
	workspacePipeline := "workspacePipeline"

	t.Run("No plan pipeline set for workspace", func(t *testing.T) {
		defaultPipelines := make(map[string]*client.DefaultPipelineOverride)
		_, err := getDefaultPipeline(defaultPipelines)
		assert.EqualError(t, err, errMsg)
	})

	t.Run("Plan pipeline set at workspace level", func(t *testing.T) {
		defaultPipelines := make(map[string]*client.DefaultPipelineOverride)

		defaultPipelines["plan"] = &client.DefaultPipelineOverride{
			ProjectPipeline:   &projectPipeline,
			WorkspacePipeline: &workspacePipeline,
		}
		pipeline, err := getDefaultPipeline(defaultPipelines)
		assert.Nil(t, err)
		assert.Equal(t, "workspacePipeline", pipeline)
	})

	t.Run("Plan pipeline set at project level", func(t *testing.T) {
		defaultPipelines := make(map[string]*client.DefaultPipelineOverride)

		defaultPipelines["plan"] = &client.DefaultPipelineOverride{
			ProjectPipeline: &projectPipeline,
		}
		pipeline, err := getDefaultPipeline(defaultPipelines)
		assert.Nil(t, err)
		assert.Equal(t, "projectPipeline", pipeline)
	})
}

func TestGetNextActiveStage(t *testing.T) {
	tt := map[string]struct {
		expectedActiveStage string
		startingNodeID      string
		layoutNodeMap       map[string]client.GraphLayoutNode
	}{
		"finds the next active stage": {
			expectedActiveStage: "node2",
			startingNodeID:      "node1",
			layoutNodeMap: map[string]client.GraphLayoutNode{
				"node1": {
					NodeUuid: "node1",
					EdgeLayoutList: &client.EdgeLayoutList{
						NextIds: []string{"node2"},
					},
					Status: "Success",
				},
				"node2": {
					NodeUuid: "node2",
					EdgeLayoutList: &client.EdgeLayoutList{
						NextIds: []string{"node3"},
					},
					Status: "Running",
				},
			},
		},
		"there is no next active stage": {
			expectedActiveStage: "",
			startingNodeID:      "node1",
			layoutNodeMap: map[string]client.GraphLayoutNode{
				"node1": {
					NodeUuid: "node1",
					EdgeLayoutList: &client.EdgeLayoutList{
						NextIds: []string{"node2"},
					},
					Status: "Success",
				},
				"node2": {
					NodeUuid: "node2",
					EdgeLayoutList: &client.EdgeLayoutList{
						NextIds: []string{"node3"},
					},
					Status: "Success",
				},
				"node3": {
					NodeUuid: "node3",
					EdgeLayoutList: &client.EdgeLayoutList{
						NextIds: []string{},
					},
					Status: "Success",
				},
			},
		},
	}
	for name, test := range tt {
		t.Run(name, func(t *testing.T) {
			actualNextActiveStage := getNextActiveStage(context.Background(), test.layoutNodeMap, test.startingNodeID)
			assert.Equal(t, test.expectedActiveStage, actualNextActiveStage.NodeUuid)
		})
	}
}

func TestGetNextActiveStep(t *testing.T) {
	tt := map[string]struct {
		expectedActiveStage string
		startingNodeID      string
		layoutNodeMap       map[string]client.GraphLayoutNode
	}{
		"finds the next active stage": {
			expectedActiveStage: "node2",
			startingNodeID:      "node1",
			layoutNodeMap: map[string]client.GraphLayoutNode{
				"node1": {
					NodeUuid: "node1",
					EdgeLayoutList: &client.EdgeLayoutList{
						NextIds: []string{"node2"},
					},
					Status: "Success",
				},
				"node2": {
					NodeUuid: "node2",
					EdgeLayoutList: &client.EdgeLayoutList{
						NextIds: []string{"node3"},
					},
					Status: "Running",
				},
			},
		},
		"there is no next active stage": {
			expectedActiveStage: "",
			startingNodeID:      "node1",
			layoutNodeMap: map[string]client.GraphLayoutNode{
				"node1": {
					NodeUuid: "node1",
					EdgeLayoutList: &client.EdgeLayoutList{
						NextIds: []string{"node2"},
					},
					Status: "Success",
				},
				"node2": {
					NodeUuid: "node2",
					EdgeLayoutList: &client.EdgeLayoutList{
						NextIds: []string{"node3"},
					},
					Status: "Success",
				},
				"node3": {
					NodeUuid: "node3",
					EdgeLayoutList: &client.EdgeLayoutList{
						NextIds: []string{},
					},
					Status: "Success",
				},
			},
		},
	}
	for name, test := range tt {
		t.Run(name, func(t *testing.T) {
			actualNextActiveStage := getNextActiveStage(context.Background(), test.layoutNodeMap, test.startingNodeID)
			assert.Equal(t, test.expectedActiveStage, actualNextActiveStage.NodeUuid)
		})
	}
}

func TestGetStartingNodeID(t *testing.T) {
	t.Run("successfully retrieve starting node", func(t *testing.T) {
		expectedStartingNodeID := "StartingNodeId"
		var expectedErr error
		testIacmClient := GetPipelineExecutionMock{
			Resp: &client.PipelineExecutionDetail{
				PipelineExecutionSummary: &client.PipelineExecutionSummary{
					StartingNodeId: expectedStartingNodeID,
					LayoutNodeMap: map[string]client.GraphLayoutNode{
						expectedStartingNodeID: {
							Status: "Running",
						},
					},
				},
			},
			Err: nil,
		}
		cmd := &IacmCommand{
			client: &testIacmClient,
		}
		actualStartingNodeID, actualErr := cmd.getStartingNodeID(context.Background(), "", "", "")
		assert.Equal(t, expectedStartingNodeID, actualStartingNodeID)
		assert.Equal(t, expectedErr, actualErr)
	})
	t.Run("returns an error retrieving starting node", func(t *testing.T) {
		expectedStartingNodeID := ""
		expectedErr := errors.New("error")
		testIacmClient := GetPipelineExecutionMock{
			Resp: nil,
			Err:  expectedErr,
		}
		cmd := &IacmCommand{
			client: &testIacmClient,
		}
		actualStartingNodeID, actualErr := cmd.getStartingNodeID(context.Background(), "", "", "")
		assert.Equal(t, expectedStartingNodeID, actualStartingNodeID)
		assert.Equal(t, expectedErr, actualErr)
	})
	t.Run("starting node never enters running state", func(t *testing.T) {
		expectedStartingNodeID := ""
		expectedErr := errors.New("The pipeline execution was not started after 5 seconds")
		testIacmClient := GetPipelineExecutionMock{
			Resp: &client.PipelineExecutionDetail{
				PipelineExecutionSummary: &client.PipelineExecutionSummary{
					StartingNodeId: expectedStartingNodeID,
					LayoutNodeMap: map[string]client.GraphLayoutNode{
						expectedStartingNodeID: {
							Status: "NotStarted",
						},
					},
				},
			},
			Err: nil,
		}
		cmd := &IacmCommand{
			client: &testIacmClient,
		}
		actualStartingNodeID, actualErr := cmd.getStartingNodeID(context.Background(), "", "", "")
		assert.Equal(t, expectedStartingNodeID, actualStartingNodeID)
		assert.Equal(t, expectedErr, actualErr)
	})
}
