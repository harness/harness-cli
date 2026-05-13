package nuget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNugetClientNameAndType(t *testing.T) {
	c := NewClient()
	assert.Equal(t, "dotnet", c.Name())
	assert.Equal(t, "nuget", c.PackageType())
}

func TestHarURLPatternNuget(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		matches bool
	}{
		{"subdomain format", "https://pkg.harness.io/acct123/my-registry/nuget", true},
		{"path format with pkg", "https://app.harness.io/pkg/acct123/my-registry/nuget", true},
		{"trailing slash", "https://pkg.harness.io/acct123/my-registry/nuget/", true},
		{"http", "http://pkg.harness.io/acct123/my-registry/nuget", true},
		{"not nuget", "https://pkg.harness.io/acct123/my-registry/npm", false},
		{"too few segments", "https://pkg.harness.io/nuget", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.matches, harURLPattern.MatchString(tt.url))
		})
	}
}

func TestParseNugetConfigForHAR(t *testing.T) {
	t.Run("valid NuGet.Config with HAR source", func(t *testing.T) {
		tmpDir := t.TempDir()
		confPath := filepath.Join(tmpDir, "NuGet.Config")
		content := `<?xml version="1.0" encoding="utf-8"?>
<configuration>
  <packageSources>
    <add key="harness" value="https://pkg.harness.io/acct123/my-nuget-reg/nuget/v3/index.json" />
  </packageSources>
</configuration>`
		require.NoError(t, os.WriteFile(confPath, []byte(content), 0644))

		info, err := parseNugetConfigForHAR(confPath, "")
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "my-nuget-reg", info.RegistryIdentifier)
		assert.Equal(t, "acct123", info.AccountID)
	})

	t.Run("explicit registry match", func(t *testing.T) {
		tmpDir := t.TempDir()
		confPath := filepath.Join(tmpDir, "NuGet.Config")
		content := `<?xml version="1.0" encoding="utf-8"?>
<configuration>
  <packageSources>
    <add key="har" value="https://pkg.harness.io/acct/target-reg/nuget/v3/index.json" />
  </packageSources>
</configuration>`
		require.NoError(t, os.WriteFile(confPath, []byte(content), 0644))

		info, err := parseNugetConfigForHAR(confPath, "target-reg")
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "target-reg", info.RegistryIdentifier)
	})

	t.Run("explicit registry mismatch", func(t *testing.T) {
		tmpDir := t.TempDir()
		confPath := filepath.Join(tmpDir, "NuGet.Config")
		content := `<?xml version="1.0" encoding="utf-8"?>
<configuration>
  <packageSources>
    <add key="har" value="https://pkg.harness.io/acct/other-reg/nuget/v3/index.json" />
  </packageSources>
</configuration>`
		require.NoError(t, os.WriteFile(confPath, []byte(content), 0644))

		info, err := parseNugetConfigForHAR(confPath, "wanted-reg")
		assert.Error(t, err)
		assert.Nil(t, info)
	})

	t.Run("no HAR URL in config", func(t *testing.T) {
		tmpDir := t.TempDir()
		confPath := filepath.Join(tmpDir, "NuGet.Config")
		content := `<?xml version="1.0" encoding="utf-8"?>
<configuration>
  <packageSources>
    <add key="nuget.org" value="https://api.nuget.org/v3/index.json" />
  </packageSources>
</configuration>`
		require.NoError(t, os.WriteFile(confPath, []byte(content), 0644))

		info, err := parseNugetConfigForHAR(confPath, "")
		assert.Error(t, err)
		assert.Nil(t, info)
	})

	t.Run("missing file", func(t *testing.T) {
		info, err := parseNugetConfigForHAR("/nonexistent/NuGet.Config", "")
		assert.Error(t, err)
		assert.Nil(t, info)
	})

	t.Run("invalid XML", func(t *testing.T) {
		tmpDir := t.TempDir()
		confPath := filepath.Join(tmpDir, "NuGet.Config")
		require.NoError(t, os.WriteFile(confPath, []byte(`not xml`), 0644))

		info, err := parseNugetConfigForHAR(confPath, "")
		assert.Error(t, err)
		assert.Nil(t, info)
	})
}

func TestParseProjectAssetsJson(t *testing.T) {
	t.Run("valid project.assets.json", func(t *testing.T) {
		tmpDir := t.TempDir()
		assetsPath := filepath.Join(tmpDir, "project.assets.json")
		content := `{
  "libraries": {
    "Newtonsoft.Json/13.0.1": {"type": "package"},
    "System.Memory/4.5.4": {"type": "package"},
    "Microsoft.NETCore.App/6.0.0": {"type": "project"}
  }
}`
		require.NoError(t, os.WriteFile(assetsPath, []byte(content), 0644))

		deps, err := parseProjectAssetsJson(assetsPath)
		require.NoError(t, err)
		assert.Len(t, deps, 2)

		names := map[string]string{}
		for _, dep := range deps {
			names[dep.Name] = dep.Version
		}
		assert.Equal(t, "13.0.1", names["Newtonsoft.Json"])
		assert.Equal(t, "4.5.4", names["System.Memory"])
	})

	t.Run("skips non-package type", func(t *testing.T) {
		tmpDir := t.TempDir()
		assetsPath := filepath.Join(tmpDir, "project.assets.json")
		content := `{"libraries": {"MyProj/1.0.0": {"type": "project"}}}`
		require.NoError(t, os.WriteFile(assetsPath, []byte(content), 0644))

		deps, err := parseProjectAssetsJson(assetsPath)
		require.NoError(t, err)
		assert.Empty(t, deps)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		tmpDir := t.TempDir()
		assetsPath := filepath.Join(tmpDir, "project.assets.json")
		require.NoError(t, os.WriteFile(assetsPath, []byte(`{not json}`), 0644))

		_, err := parseProjectAssetsJson(assetsPath)
		assert.Error(t, err)
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := parseProjectAssetsJson("/nonexistent/project.assets.json")
		assert.Error(t, err)
	})
}

func TestParseDotnetListOutput(t *testing.T) {
	t.Run("valid JSON output", func(t *testing.T) {
		output := `{
  "projects": [{
    "frameworks": [{
      "topLevelPackages": [
        {"ID": "Newtonsoft.Json", "Version": "13.0.1"}
      ],
      "transitivePackages": [
        {"ID": "System.Memory", "Version": "4.5.4"}
      ]
    }]
  }]
}`
		deps, err := parseDotnetListOutput(output)
		require.NoError(t, err)
		assert.Len(t, deps, 2)

		names := map[string]string{}
		for _, dep := range deps {
			names[dep.Name] = dep.Version
		}
		assert.Equal(t, "13.0.1", names["Newtonsoft.Json"])
		assert.Equal(t, "4.5.4", names["System.Memory"])
	})

	t.Run("deduplicates across frameworks", func(t *testing.T) {
		output := `{
  "projects": [{
    "frameworks": [
      {"topLevelPackages": [{"ID": "Pkg", "Version": "1.0"}], "transitivePackages": []},
      {"topLevelPackages": [{"ID": "Pkg", "Version": "1.0"}], "transitivePackages": []}
    ]
  }]
}`
		deps, err := parseDotnetListOutput(output)
		require.NoError(t, err)
		assert.Len(t, deps, 1)
	})

	t.Run("text format fallback", func(t *testing.T) {
		output := `Project 'MyApp' has the following package references
   [net6.0]:
   Top-level Package      Requested   Resolved
   > Newtonsoft.Json      13.0.*      13.0.1
   > Serilog              2.12.*      2.12.0`

		deps, err := parseDotnetListOutput(output)
		require.NoError(t, err)
		assert.Len(t, deps, 2)
	})

	t.Run("empty output", func(t *testing.T) {
		deps, err := parseDotnetListOutput("")
		require.NoError(t, err)
		assert.Empty(t, deps)
	})
}

func TestGetNugetConfigPaths(t *testing.T) {
	paths := getNugetConfigPaths()
	assert.NotEmpty(t, paths)
	assert.Contains(t, paths[0], "nuget.config")
}

func TestParsePackagesLockJsonEdgeCases(t *testing.T) {
	t.Run("multiple frameworks deduplicates", func(t *testing.T) {
		tmpDir := t.TempDir()
		lockPath := filepath.Join(tmpDir, "packages.lock.json")
		content := `{
  "version": 1,
  "dependencies": {
    "net6.0": {
      "Pkg": {"type": "Direct", "resolved": "1.0.0"}
    },
    "net7.0": {
      "Pkg": {"type": "Direct", "resolved": "1.0.0"}
    }
  }
}`
		require.NoError(t, os.WriteFile(lockPath, []byte(content), 0644))

		deps, err := parsePackagesLockJson(lockPath)
		require.NoError(t, err)
		assert.Len(t, deps, 1)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		tmpDir := t.TempDir()
		lockPath := filepath.Join(tmpDir, "packages.lock.json")
		require.NoError(t, os.WriteFile(lockPath, []byte(`{broken`), 0644))

		_, err := parsePackagesLockJson(lockPath)
		assert.Error(t, err)
	})
}

func TestFindCsprojRecursive(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(tmpDir))
	defer os.Chdir(origDir)

	// Create nested csproj
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "src", "MyApp"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "src", "MyApp", "MyApp.csproj"), []byte("<Project/>"), 0644))

	files := findCsprojRecursive()
	assert.Len(t, files, 1)
	assert.Contains(t, files[0], "MyApp.csproj")
}
