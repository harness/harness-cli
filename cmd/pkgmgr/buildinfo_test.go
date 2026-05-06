package pkgmgr

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPackageTypeToAPI(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"npm", "NPM"},
		{"maven", "MAVEN"},
		{"pypi", "PYTHON"},
		{"pip", "PYTHON"},
		{"nuget", "NUGET"},
		{"cargo", "cargo"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := packageTypeToAPI(tt.input)
			assert.Equal(t, tt.expected, string(result))
		})
	}
}

func TestDetectRootPackage(t *testing.T) {
	t.Run("npm with valid package.json", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(tmpDir))
		defer os.Chdir(origDir)

		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "package.json"),
			[]byte(`{"name": "@myorg/myapp", "version": "1.2.3"}`), 0644))

		client := &mockClient{pkgType: "npm"}
		info := detectRootPackage(client)
		assert.Equal(t, "@myorg/myapp", info.Name)
		assert.Equal(t, "1.2.3", info.Version)
	})

	t.Run("npm with missing package.json", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(tmpDir))
		defer os.Chdir(origDir)

		client := &mockClient{pkgType: "npm"}
		info := detectRootPackage(client)
		assert.Equal(t, "unknown", info.Name)
		assert.Equal(t, "0.0.0", info.Version)
	})

	t.Run("npm with empty name and version", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(tmpDir))
		defer os.Chdir(origDir)

		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "package.json"),
			[]byte(`{"description": "test"}`), 0644))

		client := &mockClient{pkgType: "npm"}
		info := detectRootPackage(client)
		assert.Equal(t, "unknown", info.Name)
		assert.Equal(t, "0.0.0", info.Version)
	})

	t.Run("npm with invalid JSON", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(tmpDir))
		defer os.Chdir(origDir)

		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "package.json"),
			[]byte(`{invalid json}`), 0644))

		client := &mockClient{pkgType: "npm"}
		info := detectRootPackage(client)
		assert.Equal(t, "unknown", info.Name)
		assert.Equal(t, "0.0.0", info.Version)
	})

	t.Run("unknown package type", func(t *testing.T) {
		client := &mockClient{pkgType: "cargo"}
		info := detectRootPackage(client)
		assert.Equal(t, "unknown", info.Name)
		assert.Equal(t, "0.0.0", info.Version)
	})
}

func TestBuildPipelineContext(t *testing.T) {
	t.Run("returns nil when pipeline env vars missing", func(t *testing.T) {
		t.Setenv("HARNESS_PIPELINE_ID", "")
		t.Setenv("HARNESS_EXECUTION_ID", "")

		ctx := buildPipelineContext("org", "project")
		assert.Nil(t, ctx)
	})

	t.Run("returns nil when only pipeline ID set", func(t *testing.T) {
		t.Setenv("HARNESS_PIPELINE_ID", "pipe-1")
		t.Setenv("HARNESS_EXECUTION_ID", "")

		ctx := buildPipelineContext("org", "project")
		assert.Nil(t, ctx)
	})

	t.Run("returns context with defaults", func(t *testing.T) {
		t.Setenv("HARNESS_PIPELINE_ID", "pipe-1")
		t.Setenv("HARNESS_EXECUTION_ID", "build-123")
		t.Setenv("HARNESS_STAGE_ID", "")
		t.Setenv("HARNESS_STEP_ID", "")

		ctx := buildPipelineContext("org1", "proj1")
		require.NotNil(t, ctx)
		assert.Equal(t, "pipe-1", ctx.PipelineId)
		assert.Equal(t, "build-123", ctx.ExecutionId)
		assert.Equal(t, "default", ctx.StageId)
		assert.Equal(t, "org1", ctx.OrgId)
		assert.Equal(t, "proj1", ctx.ProjectId)
		assert.Nil(t, ctx.StepId)
	})

	t.Run("includes stage and step when set", func(t *testing.T) {
		t.Setenv("HARNESS_PIPELINE_ID", "pipe-1")
		t.Setenv("HARNESS_EXECUTION_ID", "build-123")
		t.Setenv("HARNESS_STAGE_ID", "stage-1")
		t.Setenv("HARNESS_STEP_ID", "step-1")

		ctx := buildPipelineContext("org1", "proj1")
		require.NotNil(t, ctx)
		assert.Equal(t, "stage-1", ctx.StageId)
		require.NotNil(t, ctx.StepId)
		assert.Equal(t, "step-1", *ctx.StepId)
	})
}
