package pkgmgr

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/harness/harness-cli/util/common/progress"

	"github.com/google/uuid"
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
		{"pip", "pip"},
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

func TestDetectMavenRootPackage(t *testing.T) {
	t.Run("valid pom.xml", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(tmpDir))
		defer os.Chdir(origDir)

		pom := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <groupId>com.example</groupId>
  <artifactId>my-app</artifactId>
  <version>2.0.1</version>
</project>`
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pom), 0644))

		client := &mockClient{pkgType: "maven"}
		info := detectRootPackage(client)
		assert.Equal(t, "com.example:my-app", info.Name)
		assert.Equal(t, "2.0.1", info.Version)
	})

	t.Run("pom.xml with parent groupId", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(tmpDir))
		defer os.Chdir(origDir)

		pom := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <parent>
    <groupId>com.parent</groupId>
    <version>1.0.0</version>
  </parent>
  <artifactId>child-app</artifactId>
</project>`
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pom), 0644))

		client := &mockClient{pkgType: "maven"}
		info := detectRootPackage(client)
		assert.Equal(t, "com.parent:child-app", info.Name)
		assert.Equal(t, "1.0.0", info.Version)
	})

	t.Run("pom.xml missing groupId and artifactId", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(tmpDir))
		defer os.Chdir(origDir)

		pom := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <version>1.0.0</version>
</project>`
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pom), 0644))

		client := &mockClient{pkgType: "maven"}
		info := detectRootPackage(client)
		assert.Equal(t, "unknown", info.Name)
		assert.Equal(t, "0.0.0", info.Version)
	})

	t.Run("missing pom.xml", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(tmpDir))
		defer os.Chdir(origDir)

		client := &mockClient{pkgType: "maven"}
		info := detectRootPackage(client)
		assert.Equal(t, "unknown", info.Name)
		assert.Equal(t, "0.0.0", info.Version)
	})

	t.Run("invalid pom.xml XML", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(tmpDir))
		defer os.Chdir(origDir)

		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(`not xml`), 0644))

		client := &mockClient{pkgType: "maven"}
		info := detectRootPackage(client)
		assert.Equal(t, "unknown", info.Name)
		assert.Equal(t, "0.0.0", info.Version)
	})
}

func TestDetectPythonRootPackage(t *testing.T) {
	t.Run("valid pyproject.toml", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(tmpDir))
		defer os.Chdir(origDir)

		content := `[project]
name = "my-python-pkg"
version = "3.1.4"
`
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "pyproject.toml"), []byte(content), 0644))

		client := &mockClient{pkgType: "pypi"}
		info := detectRootPackage(client)
		assert.Equal(t, "my-python-pkg", info.Name)
		assert.Equal(t, "3.1.4", info.Version)
	})

	t.Run("pyproject.toml without version", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(tmpDir))
		defer os.Chdir(origDir)

		content := `[project]
name = "my-pkg"
`
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "pyproject.toml"), []byte(content), 0644))

		client := &mockClient{pkgType: "pypi"}
		info := detectRootPackage(client)
		assert.Equal(t, "my-pkg", info.Name)
		assert.Equal(t, "0.0.0", info.Version)
	})

	t.Run("setup.cfg fallback", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(tmpDir))
		defer os.Chdir(origDir)

		content := `[metadata]
name = setup-pkg
version = 1.2.0
`
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "setup.cfg"), []byte(content), 0644))

		client := &mockClient{pkgType: "pypi"}
		info := detectRootPackage(client)
		assert.Equal(t, "setup-pkg", info.Name)
		assert.Equal(t, "1.2.0", info.Version)
	})

	t.Run("no python config files", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(tmpDir))
		defer os.Chdir(origDir)

		client := &mockClient{pkgType: "pypi"}
		info := detectRootPackage(client)
		assert.Equal(t, "unknown", info.Name)
		assert.Equal(t, "0.0.0", info.Version)
	})
}

func TestDetectNugetRootPackage(t *testing.T) {
	t.Run("valid csproj", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(tmpDir))
		defer os.Chdir(origDir)

		csproj := `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <PackageId>MyCompany.MyLib</PackageId>
    <Version>4.2.0</Version>
  </PropertyGroup>
</Project>`
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "MyLib.csproj"), []byte(csproj), 0644))

		client := &mockClient{pkgType: "nuget"}
		info := detectRootPackage(client)
		assert.Equal(t, "MyCompany.MyLib", info.Name)
		assert.Equal(t, "4.2.0", info.Version)
	})

	t.Run("csproj with AssemblyName only", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(tmpDir))
		defer os.Chdir(origDir)

		csproj := `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <AssemblyName>MyAssembly</AssemblyName>
    <Version>1.0.0</Version>
  </PropertyGroup>
</Project>`
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "App.csproj"), []byte(csproj), 0644))

		client := &mockClient{pkgType: "nuget"}
		info := detectRootPackage(client)
		assert.Equal(t, "MyAssembly", info.Name)
		assert.Equal(t, "1.0.0", info.Version)
	})

	t.Run("csproj with no metadata uses filename", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(tmpDir))
		defer os.Chdir(origDir)

		csproj := `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>net6.0</TargetFramework>
  </PropertyGroup>
</Project>`
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "FallbackApp.csproj"), []byte(csproj), 0644))

		client := &mockClient{pkgType: "nuget"}
		info := detectRootPackage(client)
		assert.Equal(t, "FallbackApp", info.Name)
		assert.Equal(t, "0.0.0", info.Version)
	})

	t.Run("no csproj files", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(tmpDir))
		defer os.Chdir(origDir)

		client := &mockClient{pkgType: "nuget"}
		info := detectRootPackage(client)
		assert.Equal(t, "unknown", info.Name)
		assert.Equal(t, "0.0.0", info.Version)
	})

	t.Run("invalid csproj XML uses filename", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(tmpDir))
		defer os.Chdir(origDir)

		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "Bad.csproj"), []byte(`not xml`), 0644))

		client := &mockClient{pkgType: "nuget"}
		info := detectRootPackage(client)
		assert.Equal(t, "Bad", info.Name)
		assert.Equal(t, "0.0.0", info.Version)
	})
}

func TestBuildPipelineContext(t *testing.T) {
	t.Run("returns nil when all pipeline env vars missing", func(t *testing.T) {
		t.Setenv("HARNESS_PIPELINE_ID", "")
		t.Setenv("HARNESS_EXECUTION_ID", "")
		t.Setenv("HARNESS_ORG_ID", "")
		t.Setenv("HARNESS_PROJECT_ID", "")

		ctx := buildPipelineContext()
		assert.Nil(t, ctx)
	})

	t.Run("returns nil when only pipeline ID set", func(t *testing.T) {
		t.Setenv("HARNESS_PIPELINE_ID", "pipe-1")
		t.Setenv("HARNESS_EXECUTION_ID", "")
		t.Setenv("HARNESS_ORG_ID", "")
		t.Setenv("HARNESS_PROJECT_ID", "")

		ctx := buildPipelineContext()
		assert.Nil(t, ctx)
	})

	t.Run("returns nil when execution ID missing", func(t *testing.T) {
		t.Setenv("HARNESS_PIPELINE_ID", "pipe-1")
		t.Setenv("HARNESS_EXECUTION_ID", "")
		t.Setenv("HARNESS_ORG_ID", "org1")
		t.Setenv("HARNESS_PROJECT_ID", "proj1")

		ctx := buildPipelineContext()
		assert.Nil(t, ctx)
	})

	t.Run("returns nil when pipeline ID missing but others set", func(t *testing.T) {
		t.Setenv("HARNESS_PIPELINE_ID", "")
		t.Setenv("HARNESS_EXECUTION_ID", "build-123")
		t.Setenv("HARNESS_ORG_ID", "org1")
		t.Setenv("HARNESS_PROJECT_ID", "proj1")

		ctx := buildPipelineContext()
		assert.Nil(t, ctx)
	})

	t.Run("returns nil when org missing", func(t *testing.T) {
		t.Setenv("HARNESS_PIPELINE_ID", "pipe-1")
		t.Setenv("HARNESS_EXECUTION_ID", "build-123")
		t.Setenv("HARNESS_ORG_ID", "")
		t.Setenv("HARNESS_PROJECT_ID", "proj1")

		ctx := buildPipelineContext()
		assert.Nil(t, ctx)
	})

	t.Run("returns nil when project missing", func(t *testing.T) {
		t.Setenv("HARNESS_PIPELINE_ID", "pipe-1")
		t.Setenv("HARNESS_EXECUTION_ID", "build-123")
		t.Setenv("HARNESS_ORG_ID", "org1")
		t.Setenv("HARNESS_PROJECT_ID", "")

		ctx := buildPipelineContext()
		assert.Nil(t, ctx)
	})

	t.Run("returns context with defaults", func(t *testing.T) {
		t.Setenv("HARNESS_PIPELINE_ID", "pipe-1")
		t.Setenv("HARNESS_EXECUTION_ID", "build-123")
		t.Setenv("HARNESS_ORG_ID", "org1")
		t.Setenv("HARNESS_PROJECT_ID", "proj1")
		t.Setenv("HARNESS_STAGE_ID", "")
		t.Setenv("HARNESS_STEP_ID", "")

		ctx := buildPipelineContext()
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
		t.Setenv("HARNESS_ORG_ID", "org1")
		t.Setenv("HARNESS_PROJECT_ID", "proj1")
		t.Setenv("HARNESS_STAGE_ID", "stage-1")
		t.Setenv("HARNESS_STEP_ID", "step-1")

		ctx := buildPipelineContext()
		require.NotNil(t, ctx)
		assert.Equal(t, "stage-1", ctx.StageId)
		require.NotNil(t, ctx.StepId)
		assert.Equal(t, "step-1", *ctx.StepId)
	})

	t.Run("uses HARNESS_ORG_ID and HARNESS_PROJECT_ID env vars, not args", func(t *testing.T) {
		t.Setenv("HARNESS_PIPELINE_ID", "pipe-1")
		t.Setenv("HARNESS_EXECUTION_ID", "build-123")
		t.Setenv("HARNESS_ORG_ID", "env-org")
		t.Setenv("HARNESS_PROJECT_ID", "env-proj")
		t.Setenv("HARNESS_STAGE_ID", "")
		t.Setenv("HARNESS_STEP_ID", "")

		ctx := buildPipelineContext()
		require.NotNil(t, ctx)
		assert.Equal(t, "env-org", ctx.OrgId)
		assert.Equal(t, "env-proj", ctx.ProjectId)
	})
}

// TestUploadBuildInfoSkipsWhenEnvMissing verifies that uploadBuildInfo returns
// early — before touching Factory/HTTP — when required pipeline env vars are
// missing. Passing nil Factory would panic otherwise; this asserts the guard
// short-circuits before the factory is used.
func TestUploadBuildInfoSkipsWhenEnvMissing(t *testing.T) {
	t.Run("no panic when factory nil and env vars missing", func(t *testing.T) {
		t.Setenv("HARNESS_PIPELINE_ID", "")
		t.Setenv("HARNESS_EXECUTION_ID", "")
		t.Setenv("HARNESS_ORG_ID", "")
		t.Setenv("HARNESS_PROJECT_ID", "")

		reporter := progress.NewNopReporter()
		client := &mockClient{name: "npm", pkgType: PackageTypeNpm}

		assert.NotPanics(t, func() {
			uploadBuildInfo(nil, client, uuid.Nil, nil, reporter)
		})
	})

	t.Run("no panic when only some env vars set", func(t *testing.T) {
		t.Setenv("HARNESS_PIPELINE_ID", "pipe-1")
		t.Setenv("HARNESS_EXECUTION_ID", "build-1")
		t.Setenv("HARNESS_ORG_ID", "")
		t.Setenv("HARNESS_PROJECT_ID", "")

		reporter := progress.NewNopReporter()
		client := &mockClient{name: "npm", pkgType: PackageTypeNpm}

		assert.NotPanics(t, func() {
			uploadBuildInfo(nil, client, uuid.Nil, nil, reporter)
		})
	})
}
