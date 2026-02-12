package command

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/harness/harness-cli/util/client/iacm"
	"github.com/stretchr/testify/assert"
)

type GetPipelineExecutionMock struct {
	Err  error
	Resp *iacm.PipelineExecutionDetail
}

func (m *GetPipelineExecutionMock) GetPipelineExecution(ctx context.Context, org, project, executionID string, stageNodeID string) (*iacm.PipelineExecutionDetail, error) {
	return m.Resp, m.Err
}

func TestValidateWorkingDirectory(t *testing.T) {
	t.Run("working directory does not contain repository path", func(t *testing.T) {
		ws := &iacm.Workspace{
			RepositoryPath: "tf/aws",
		}
		actualRepoRoot := t.TempDir()
		workingDirectory := actualRepoRoot
		repoRoot, warning, err := getRepoRootFromWorkingDirectory(workingDirectory, ws)
		assert.Empty(t, repoRoot)
		assert.Empty(t, warning)
		assert.Equal(t, fmt.Sprintf(folderPathNotFoundErr, ws.RepositoryPath), err.Error())
	})
	t.Run("working directory contains the repository path", func(t *testing.T) {
		ws := &iacm.Workspace{
			RepositoryPath: "tf/a3ws",
		}
		actualRepoRoot := t.TempDir()
		workingDirectory := actualRepoRoot
		err := os.MkdirAll(filepath.Join(actualRepoRoot, ws.RepositoryPath), 0700)
		assert.Nil(t, err)
		repoRoot, warning, err := getRepoRootFromWorkingDirectory(workingDirectory, ws)
		assert.Equal(t, repoRoot, actualRepoRoot)
		assert.Equal(t, fmt.Sprintf(folderPathWarningMsg, ws.RepositoryPath, actualRepoRoot), warning)
		assert.Nil(t, err)
	})
	t.Run("working directory is the same as the repository path", func(t *testing.T) {
		ws := &iacm.Workspace{
			RepositoryPath: "tf/aws",
		}
		actualRepoRoot := t.TempDir()
		workingDirectory := filepath.Join(actualRepoRoot, ws.RepositoryPath)
		err := os.MkdirAll(workingDirectory, 0700)
		assert.Nil(t, err)
		repoRoot, warning, err := getRepoRootFromWorkingDirectory(workingDirectory, ws)
		assert.Equal(t, repoRoot, actualRepoRoot)
		assert.Equal(t, fmt.Sprintf(folderPathWarningMsg, ws.RepositoryPath, actualRepoRoot), warning)
		assert.Nil(t, err)
	})
}

func TestGetDefaultPipeline(t *testing.T) {
	errMsg := "The workspace has no configured default pipeline"
	projectPipeline := "projectPipeline"
	workspacePipeline := "workspacePipeline"

	t.Run("No plan pipeline set for workspace", func(t *testing.T) {
		defaultPipelines := make(map[string]*iacm.DefaultPipelineOverride)
		_, err := getDefaultPipeline(defaultPipelines)
		assert.EqualError(t, err, errMsg)
	})

	t.Run("Plan pipeline set at workspace level", func(t *testing.T) {
		defaultPipelines := make(map[string]*iacm.DefaultPipelineOverride)

		defaultPipelines["plan"] = &iacm.DefaultPipelineOverride{
			ProjectPipeline:   &projectPipeline,
			WorkspacePipeline: &workspacePipeline,
		}
		pipeline, err := getDefaultPipeline(defaultPipelines)
		assert.Nil(t, err)
		assert.Equal(t, "workspacePipeline", pipeline)
	})

	t.Run("Plan pipeline set at project level", func(t *testing.T) {
		defaultPipelines := make(map[string]*iacm.DefaultPipelineOverride)

		defaultPipelines["plan"] = &iacm.DefaultPipelineOverride{
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
		layoutNodeMap       map[string]iacm.GraphLayoutNode
	}{
		"finds the next active stage": {
			expectedActiveStage: "node2",
			startingNodeID:      "node1",
			layoutNodeMap: map[string]iacm.GraphLayoutNode{
				"node1": {
					NodeUuid: "node1",
					EdgeLayoutList: &iacm.EdgeLayoutList{
						NextIds: []string{"node2"},
					},
					Status: "Success",
				},
				"node2": {
					NodeUuid: "node2",
					EdgeLayoutList: &iacm.EdgeLayoutList{
						NextIds: []string{"node3"},
					},
					Status: "Running",
				},
			},
		},
		"there is no next active stage": {
			expectedActiveStage: "",
			startingNodeID:      "node1",
			layoutNodeMap: map[string]iacm.GraphLayoutNode{
				"node1": {
					NodeUuid: "node1",
					EdgeLayoutList: &iacm.EdgeLayoutList{
						NextIds: []string{"node2"},
					},
					Status: "Success",
				},
				"node2": {
					NodeUuid: "node2",
					EdgeLayoutList: &iacm.EdgeLayoutList{
						NextIds: []string{"node3"},
					},
					Status: "Success",
				},
				"node3": {
					NodeUuid: "node3",
					EdgeLayoutList: &iacm.EdgeLayoutList{
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
		startingNodeID       string
		layoutNodeMap       map[string]iacm.GraphLayoutNode
	}{
		"finds the next active stage": {
			expectedActiveStage: "node2",
			startingNodeID:      "node1",
			layoutNodeMap: map[string]iacm.GraphLayoutNode{
				"node1": {
					NodeUuid: "node1",
					EdgeLayoutList: &iacm.EdgeLayoutList{
						NextIds: []string{"node2"},
					},
					Status: "Success",
				},
				"node2": {
					NodeUuid: "node2",
					EdgeLayoutList: &iacm.EdgeLayoutList{
						NextIds: []string{"node3"},
					},
					Status: "Running",
				},
			},
		},
		"there is no next active stage": {
			expectedActiveStage: "",
			startingNodeID:      "node1",
			layoutNodeMap: map[string]iacm.GraphLayoutNode{
				"node1": {
					NodeUuid: "node1",
					EdgeLayoutList: &iacm.EdgeLayoutList{
						NextIds: []string{"node2"},
					},
					Status: "Success",
				},
				"node2": {
					NodeUuid: "node2",
					EdgeLayoutList: &iacm.EdgeLayoutList{
						NextIds: []string{"node3"},
					},
					Status: "Success",
				},
				"node3": {
					NodeUuid: "node3",
					EdgeLayoutList: &iacm.EdgeLayoutList{
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
		mockClient := &GetPipelineExecutionMock{
			Resp: &iacm.PipelineExecutionDetail{
				PipelineExecutionSummary: &iacm.PipelineExecutionSummary{
					StartingNodeId: expectedStartingNodeID,
					LayoutNodeMap: map[string]iacm.GraphLayoutNode{
						expectedStartingNodeID: {
							Status: "Running",
						},
					},
				},
			},
			Err: nil,
		}
		ctx := context.Background()
		actualStartingNodeID, actualErr := getStartingNodeID(ctx, mockClient, "", "", "")
		assert.Equal(t, expectedStartingNodeID, actualStartingNodeID)
		assert.Equal(t, expectedErr, actualErr)
	})
	t.Run("returns an error retrieving starting node", func(t *testing.T) {
		expectedStartingNodeID := ""
		expectedErr := errors.New("error")
		mockClient := &GetPipelineExecutionMock{
			Resp: nil,
			Err:  expectedErr,
		}
		ctx := context.Background()
		actualStartingNodeID, actualErr := getStartingNodeID(ctx, mockClient, "", "", "")
		assert.Equal(t, expectedStartingNodeID, actualStartingNodeID)
		assert.Equal(t, expectedErr, actualErr)
	})
	t.Run("starting node never enters running state", func(t *testing.T) {
		expectedStartingNodeID := ""
		expectedErr := errors.New("The pipeline execution was not started after 5 seconds")
		mockClient := &GetPipelineExecutionMock{
			Resp: &iacm.PipelineExecutionDetail{
				PipelineExecutionSummary: &iacm.PipelineExecutionSummary{
					StartingNodeId: "StartingNodeId",
					LayoutNodeMap: map[string]iacm.GraphLayoutNode{
						"StartingNodeId": {
							Status: "NotStarted",
						},
					},
				},
			},
			Err: nil,
		}
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		actualStartingNodeID, actualErr := getStartingNodeID(ctx, mockClient, "", "", "")
		assert.Equal(t, expectedStartingNodeID, actualStartingNodeID)
		assert.Equal(t, expectedErr.Error(), actualErr.Error())
	})
}
