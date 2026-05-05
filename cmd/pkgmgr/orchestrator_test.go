package pkgmgr

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	regcmd "github.com/harness/harness-cli/cmd/registry/command"
	"github.com/harness/harness-cli/config"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveOrgProject(t *testing.T) {
	t.Run("from global config", func(t *testing.T) {
		config.Global = config.GlobalFlags{
			OrgID:     "config-org",
			ProjectID: "config-proj",
		}
		t.Setenv("ORG_IDENTIFIER", "")
		t.Setenv("PROJECT_IDENTIFIER", "")

		client := &mockClient{orgProject: [2]string{"", ""}}
		org, project := resolveOrgProject(client)
		assert.Equal(t, "config-org", org)
		assert.Equal(t, "config-proj", project)
	})

	t.Run("env vars override global config", func(t *testing.T) {
		config.Global = config.GlobalFlags{
			OrgID:     "config-org",
			ProjectID: "config-proj",
		}
		t.Setenv("ORG_IDENTIFIER", "env-org")
		t.Setenv("PROJECT_IDENTIFIER", "env-proj")

		client := &mockClient{orgProject: [2]string{"", ""}}
		org, project := resolveOrgProject(client)
		assert.Equal(t, "env-org", org)
		assert.Equal(t, "env-proj", project)
	})

	t.Run("fallback to client when empty", func(t *testing.T) {
		config.Global = config.GlobalFlags{
			OrgID:     "",
			ProjectID: "",
		}
		t.Setenv("ORG_IDENTIFIER", "")
		t.Setenv("PROJECT_IDENTIFIER", "")

		client := &mockClient{orgProject: [2]string{"fallback-org", "fallback-proj"}}
		org, project := resolveOrgProject(client)
		assert.Equal(t, "fallback-org", org)
		assert.Equal(t, "fallback-proj", project)
	})

	t.Run("partial fallback", func(t *testing.T) {
		config.Global = config.GlobalFlags{
			OrgID:     "config-org",
			ProjectID: "",
		}
		t.Setenv("ORG_IDENTIFIER", "")
		t.Setenv("PROJECT_IDENTIFIER", "")

		client := &mockClient{orgProject: [2]string{"fb-org", "fb-proj"}}
		org, project := resolveOrgProject(client)
		assert.Equal(t, "config-org", org)
		assert.Equal(t, "fb-proj", project)
	})
}

func TestSaveBuildInfo(t *testing.T) {
	t.Run("writes build info file", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(tmpDir))
		defer os.Chdir(origDir)

		deps := []regcmd.Dependency{
			{Name: "lodash", Version: "4.17.21"},
			{Name: "express", Version: "4.18.0"},
		}

		saveBuildInfo("npm", "install", "my-registry", deps)

		data, err := os.ReadFile(filepath.Join(tmpDir, ".harness", "build-info.json"))
		require.NoError(t, err)

		var info buildInfo
		require.NoError(t, json.Unmarshal(data, &info))

		assert.Equal(t, "npm", info.Client)
		assert.Equal(t, "install", info.Command)
		assert.Equal(t, "my-registry", info.Registry)
		assert.Len(t, info.Dependencies, 2)
		assert.Equal(t, "lodash", info.Dependencies[0].Name)
		assert.Equal(t, "4.17.21", info.Dependencies[0].Version)
		assert.NotEmpty(t, info.Timestamp)
	})

	t.Run("creates .harness directory if missing", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(tmpDir))
		defer os.Chdir(origDir)

		saveBuildInfo("npm", "ci", "reg", nil)

		_, err := os.Stat(filepath.Join(tmpDir, ".harness", "build-info.json"))
		assert.NoError(t, err)
	})
}

// mockClient implements Client interface for testing
type mockClient struct {
	name       string
	pkgType    string
	orgProject [2]string
}

func (m *mockClient) Name() string        { return m.name }
func (m *mockClient) PackageType() string { return m.pkgType }
func (m *mockClient) DetectRegistry(explicit string) (*RegistryInfo, error) {
	return nil, nil
}
func (m *mockClient) RunCommand(command string, args []string) (*InstallResult, error) {
	return nil, nil
}
func (m *mockClient) ResolveDependencies(progress p.Reporter) (*DependencyResult, error) {
	return nil, nil
}
func (m *mockClient) DetectFirewallError(stderr string) bool { return false }
func (m *mockClient) FallbackOrgProject() (string, string) {
	return m.orgProject[0], m.orgProject[1]
}
