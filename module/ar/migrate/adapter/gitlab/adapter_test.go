package gitlab

import (
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/types"
	"github.com/stretchr/testify/assert"
)

func TestMapArtifactTypeToGitLab(t *testing.T) {
	tests := []struct {
		name         string
		artifactType types.ArtifactType
		expected     string
	}{
		{"Maven", types.MAVEN, "maven"},
		{"NPM", types.NPM, "npm"},
		{"Python", types.PYTHON, "pypi"},
		{"NuGet", types.NUGET, "nuget"},
		{"Composer", types.COMPOSER, "composer"},
		{"Conan", types.CONAN, "conan"},
		{"Helm", types.HELM, "helm"},
		{"Debian", types.DEBIAN, "debian"},
		{"Go", types.GO, "golang"},
		{"RubyGems", types.RUBYGEMS, "rubygems"},
		{"Swift", types.SWIFT, "generic"},
		{"Generic", types.GENERIC, "generic"},
		{"Unknown", types.ArtifactType("unknown"), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapArtifactTypeToGitLab(tt.artifactType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMapGitLabPackageType(t *testing.T) {
	tests := []struct {
		name        string
		gitlabType  string
		expected    types.ArtifactType
	}{
		{"Maven", "maven", types.MAVEN},
		{"NPM", "npm", types.NPM},
		{"PyPI", "pypi", types.PYTHON},
		{"NuGet", "nuget", types.NUGET},
		{"Composer", "composer", types.COMPOSER},
		{"Helm", "helm", types.HELM},
		{"Debian", "debian", types.DEBIAN},
		{"Golang", "golang", types.GO},
		{"Generic", "generic", types.GENERIC},
		{"Unknown", "unknown", types.GENERIC},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapGitLabPackageType(tt.gitlabType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewAdapter(t *testing.T) {
	config := types.RegistryConfig{
		Endpoint: "https://gitlab.com",
		Type:     types.GITLAB,
		Credentials: types.CredentialsConfig{
			Username: "test-user",
			Password: "test-token",
		},
	}

	adp, err := newAdapter(config)
	assert.NoError(t, err)
	assert.NotNil(t, adp)

	gitlabAdapter, ok := adp.(*adapter)
	assert.True(t, ok)
	assert.NotNil(t, gitlabAdapter.client)
	assert.Equal(t, config, gitlabAdapter.reg)
}

func TestGetConfig(t *testing.T) {
	config := types.RegistryConfig{
		Endpoint: "https://gitlab.com",
		Type:     types.GITLAB,
		Credentials: types.CredentialsConfig{
			Username: "test-user",
			Password: "test-token",
		},
	}

	adp, _ := newAdapter(config)
	result := adp.GetConfig()
	assert.Equal(t, config, result)
}

func TestGetKeyChain(t *testing.T) {
	config := types.RegistryConfig{
		Endpoint: "https://gitlab.com",
		Type:     types.GITLAB,
		Credentials: types.CredentialsConfig{
			Username: "test-user",
			Password: "test-token",
		},
	}

	adp, _ := newAdapter(config)
	keychain, err := adp.GetKeyChain("")
	assert.NoError(t, err)
	assert.NotNil(t, keychain)
}

func TestGetOCIImagePath(t *testing.T) {
	config := types.RegistryConfig{
		Endpoint: "https://gitlab.com",
		Type:     types.GITLAB,
	}

	adp, _ := newAdapter(config)
	gitlabAdapter := adp.(*adapter)

	tests := []struct {
		name            string
		registry        string
		packageHostname string
		image           string
		expected        string
	}{
		{
			"With custom hostname",
			"mygroup/myproject",
			"registry.gitlab.com",
			"myimage:latest",
			"registry.gitlab.com/mygroup/myproject/myimage:latest",
		},
		{
			"Without custom hostname",
			"mygroup/myproject",
			"",
			"myimage:latest",
			"registry.gitlab.com/mygroup/myproject/myimage:latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := gitlabAdapter.GetOCIImagePath(tt.registry, tt.packageHostname, tt.image)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
