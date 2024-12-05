package main

import (
	"fmt"
	"harness/client"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateWorkingDirectory(t *testing.T) {
	t.Run("working directory does not contain folder path", func(*testing.T) {
		ws := &client.Workspace{
			RepositoryPath: "tf/aws",
		}
		wd := t.TempDir()
		warning, err := validateWorkdingDirectory(wd, ws)
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
		warning, err := validateWorkdingDirectory(wd, ws)
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
		warning, err := validateWorkdingDirectory(wd, ws)
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
