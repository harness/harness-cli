package npm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNpmClientName(t *testing.T) {
	c := NewClient()
	assert.Equal(t, "npm", c.Name())
	assert.Equal(t, "npm", c.PackageType())
}

func TestDetectFirewallError(t *testing.T) {
	c := NewClient()

	tests := []struct {
		name   string
		stderr string
		want   bool
	}{
		{"403 Forbidden", "npm ERR! 403 Forbidden - GET https://pkg.harness.io/...", true},
		{"E403", "npm ERR! code E403", true},
		{"status 403", "npm ERR! status 403 while fetching", true},
		{"case insensitive", "npm ERR! 403 forbidden", true},
		{"no firewall error", "npm ERR! 404 Not Found", false},
		{"empty stderr", "", false},
		{"general error", "npm ERR! code ERESOLVE", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, c.DetectFirewallError(tt.stderr))
		})
	}
}

func TestHarURLPattern(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		matches bool
	}{
		{"subdomain format", "https://pkg.qa.harness.io/acct123/my-registry/npm", true},
		{"path format with pkg", "https://app.harness.io/pkg/acct123/my-registry/npm", true},
		{"trailing slash", "https://pkg.harness.io/acct123/my-registry/npm/", true},
		{"http", "http://pkg.harness.io/acct123/my-registry/npm", true},
		{"not npm", "https://pkg.harness.io/acct123/my-registry/maven", false},
		{"too few segments", "https://pkg.harness.io/npm", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.matches, harURLPattern.MatchString(tt.url))
		})
	}
}

func TestParseNpmrcForHAR(t *testing.T) {
	t.Run("valid npmrc with HAR registry", func(t *testing.T) {
		tmpDir := t.TempDir()
		npmrcPath := filepath.Join(tmpDir, ".npmrc")
		content := `@myorg:registry=https://pkg.harness.io/acct123/my-npm-reg/npm/
//pkg.harness.io/acct123/my-npm-reg/npm/:_authToken=pat.abc123
always-auth=true
`
		require.NoError(t, os.WriteFile(npmrcPath, []byte(content), 0600))

		info, err := parseNpmrcForHAR(npmrcPath, "")
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "my-npm-reg", info.RegistryIdentifier)
		assert.Equal(t, "acct123", info.AccountID)
		assert.Equal(t, "pat.abc123", info.AuthToken)
		assert.Contains(t, info.RegistryURL, "pkg.harness.io")
	})

	t.Run("valid npmrc with path-style URL", func(t *testing.T) {
		tmpDir := t.TempDir()
		npmrcPath := filepath.Join(tmpDir, ".npmrc")
		content := `registry=https://app.harness.io/pkg/iWnhltqOT7GFt7R/test-reg/npm/
//app.harness.io/pkg/iWnhltqOT7GFt7R/test-reg/npm/:_authToken=mytoken
`
		require.NoError(t, os.WriteFile(npmrcPath, []byte(content), 0600))

		info, err := parseNpmrcForHAR(npmrcPath, "")
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "test-reg", info.RegistryIdentifier)
		assert.Equal(t, "iWnhltqOT7GFt7R", info.AccountID)
	})

	t.Run("explicit registry match", func(t *testing.T) {
		tmpDir := t.TempDir()
		npmrcPath := filepath.Join(tmpDir, ".npmrc")
		content := `registry=https://pkg.harness.io/acct/target-reg/npm/
//pkg.harness.io/acct/target-reg/npm/:_authToken=tok
`
		require.NoError(t, os.WriteFile(npmrcPath, []byte(content), 0600))

		info, err := parseNpmrcForHAR(npmrcPath, "target-reg")
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "target-reg", info.RegistryIdentifier)
	})

	t.Run("explicit registry mismatch", func(t *testing.T) {
		tmpDir := t.TempDir()
		npmrcPath := filepath.Join(tmpDir, ".npmrc")
		content := `registry=https://pkg.harness.io/acct/other-reg/npm/
//pkg.harness.io/acct/other-reg/npm/:_authToken=tok
`
		require.NoError(t, os.WriteFile(npmrcPath, []byte(content), 0600))

		info, err := parseNpmrcForHAR(npmrcPath, "wanted-reg")
		assert.Error(t, err)
		assert.Nil(t, info)
		assert.Contains(t, err.Error(), "mismatch")
	})

	t.Run("npmrc without HAR URL", func(t *testing.T) {
		tmpDir := t.TempDir()
		npmrcPath := filepath.Join(tmpDir, ".npmrc")
		content := `registry=https://registry.npmjs.org/
`
		require.NoError(t, os.WriteFile(npmrcPath, []byte(content), 0600))

		info, err := parseNpmrcForHAR(npmrcPath, "")
		assert.Error(t, err)
		assert.Nil(t, info)
	})

	t.Run("missing file", func(t *testing.T) {
		info, err := parseNpmrcForHAR("/nonexistent/.npmrc", "")
		assert.Error(t, err)
		assert.Nil(t, info)
	})

	t.Run("skips comments and empty lines", func(t *testing.T) {
		tmpDir := t.TempDir()
		npmrcPath := filepath.Join(tmpDir, ".npmrc")
		content := `# This is a comment

# Another comment
registry=https://pkg.harness.io/acct/my-reg/npm/
//pkg.harness.io/acct/my-reg/npm/:_authToken=token123
`
		require.NoError(t, os.WriteFile(npmrcPath, []byte(content), 0600))

		info, err := parseNpmrcForHAR(npmrcPath, "")
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "my-reg", info.RegistryIdentifier)
		assert.Equal(t, "token123", info.AuthToken)
	})
}

func TestDetectRegistry(t *testing.T) {
	t.Run("no saved config and no npmrc returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(tmpDir))
		defer os.Chdir(origDir)

		c := NewClient()
		_, err := c.DetectRegistry("")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no HAR registry found")
	})

	t.Run("explicit registry not found returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(tmpDir))
		defer os.Chdir(origDir)

		c := NewClient()
		_, err := c.DetectRegistry("nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("detects from local npmrc", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(tmpDir))
		defer os.Chdir(origDir)

		npmrcContent := `registry=https://pkg.harness.io/acct123/local-reg/npm/
//pkg.harness.io/acct123/local-reg/npm/:_authToken=mytoken
`
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".npmrc"), []byte(npmrcContent), 0600))

		c := NewClient()
		info, err := c.DetectRegistry("")
		require.NoError(t, err)
		assert.Equal(t, "local-reg", info.RegistryIdentifier)
		assert.Equal(t, "acct123", info.AccountID)
	})
}

func TestFallbackOrgProject(t *testing.T) {
	t.Run("no saved config returns empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		c := NewClient()
		org, project := c.FallbackOrgProject()
		assert.Empty(t, org)
		assert.Empty(t, project)
	})
}
