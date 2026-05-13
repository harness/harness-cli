package pip

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPipClientNameAndType(t *testing.T) {
	c := NewClient()
	assert.Equal(t, "pip", c.Name())
	assert.Equal(t, "pypi", c.PackageType())
}

func TestHarURLPatternPip(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		matches bool
	}{
		{"subdomain format", "https://pkg.harness.io/acct123/my-registry/pypi", true},
		{"path format with pkg", "https://app.harness.io/pkg/acct123/my-registry/pypi", true},
		{"trailing slash", "https://pkg.harness.io/acct123/my-registry/pypi/", true},
		{"http", "http://pkg.harness.io/acct123/my-registry/pypi", true},
		{"not pypi", "https://pkg.harness.io/acct123/my-registry/npm", false},
		{"too few segments", "https://pkg.harness.io/pypi", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.matches, harURLPattern.MatchString(tt.url))
		})
	}
}

func TestParsePipConfForHAR(t *testing.T) {
	t.Run("valid pip.conf with index-url", func(t *testing.T) {
		tmpDir := t.TempDir()
		confPath := filepath.Join(tmpDir, "pip.conf")
		content := `[global]
index-url = https://pkg.harness.io/acct123/my-pip-reg/pypi/simple/
trusted-host = pkg.harness.io
`
		require.NoError(t, os.WriteFile(confPath, []byte(content), 0644))

		info, err := parsePipConfForHAR(confPath, "")
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "my-pip-reg", info.RegistryIdentifier)
		assert.Equal(t, "acct123", info.AccountID)
	})

	t.Run("valid pip.conf with extra-index-url", func(t *testing.T) {
		tmpDir := t.TempDir()
		confPath := filepath.Join(tmpDir, "pip.conf")
		content := `[global]
extra-index-url = https://app.harness.io/pkg/acctXYZ/extra-reg/pypi/simple/
`
		require.NoError(t, os.WriteFile(confPath, []byte(content), 0644))

		info, err := parsePipConfForHAR(confPath, "")
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "extra-reg", info.RegistryIdentifier)
		assert.Equal(t, "acctXYZ", info.AccountID)
	})

	t.Run("explicit registry match", func(t *testing.T) {
		tmpDir := t.TempDir()
		confPath := filepath.Join(tmpDir, "pip.conf")
		content := `[global]
index-url = https://pkg.harness.io/acct/target-reg/pypi/simple/
`
		require.NoError(t, os.WriteFile(confPath, []byte(content), 0644))

		info, err := parsePipConfForHAR(confPath, "target-reg")
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "target-reg", info.RegistryIdentifier)
	})

	t.Run("explicit registry mismatch", func(t *testing.T) {
		tmpDir := t.TempDir()
		confPath := filepath.Join(tmpDir, "pip.conf")
		content := `[global]
index-url = https://pkg.harness.io/acct/other-reg/pypi/simple/
`
		require.NoError(t, os.WriteFile(confPath, []byte(content), 0644))

		info, err := parsePipConfForHAR(confPath, "wanted-reg")
		assert.Error(t, err)
		assert.Nil(t, info)
	})

	t.Run("no HAR URL in conf", func(t *testing.T) {
		tmpDir := t.TempDir()
		confPath := filepath.Join(tmpDir, "pip.conf")
		content := `[global]
index-url = https://pypi.org/simple/
`
		require.NoError(t, os.WriteFile(confPath, []byte(content), 0644))

		info, err := parsePipConfForHAR(confPath, "")
		assert.Error(t, err)
		assert.Nil(t, info)
	})

	t.Run("missing file", func(t *testing.T) {
		info, err := parsePipConfForHAR("/nonexistent/pip.conf", "")
		assert.Error(t, err)
		assert.Nil(t, info)
	})
}

func TestParsePipReportEdgeCases(t *testing.T) {
	t.Run("empty install array", func(t *testing.T) {
		tmpDir := t.TempDir()
		reportPath := filepath.Join(tmpDir, "report.json")
		require.NoError(t, os.WriteFile(reportPath, []byte(`{"install": []}`), 0644))

		deps, err := parsePipReport(reportPath)
		require.NoError(t, err)
		assert.Empty(t, deps)
	})

	t.Run("skips packages with empty name", func(t *testing.T) {
		tmpDir := t.TempDir()
		reportPath := filepath.Join(tmpDir, "report.json")
		content := `{"install": [
			{"metadata": {"name": "", "version": "1.0"}, "requested": true},
			{"metadata": {"name": "valid-pkg", "version": "2.0"}, "requested": false}
		]}`
		require.NoError(t, os.WriteFile(reportPath, []byte(content), 0644))

		deps, err := parsePipReport(reportPath)
		require.NoError(t, err)
		assert.Len(t, deps, 1)
		assert.Equal(t, "valid-pkg", deps[0].Name)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		tmpDir := t.TempDir()
		reportPath := filepath.Join(tmpDir, "report.json")
		require.NoError(t, os.WriteFile(reportPath, []byte(`{not json}`), 0644))

		_, err := parsePipReport(reportPath)
		assert.Error(t, err)
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := parsePipReport("/nonexistent/report.json")
		assert.Error(t, err)
	})
}

func TestGetPipConfPaths(t *testing.T) {
	paths := getPipConfPaths()
	assert.NotEmpty(t, paths)
	assert.Contains(t, paths[len(paths)-1], "pip.conf")
}
