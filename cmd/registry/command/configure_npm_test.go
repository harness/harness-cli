package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar"
	"github.com/harness/harness-cli/internal/api/ar_v3"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackupNpmrc(t *testing.T) {
	t.Run("no existing npmrc returns empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		npmrcPath := filepath.Join(tmpDir, ".npmrc")

		backupPath, err := backupNpmrc(npmrcPath)
		assert.NoError(t, err)
		assert.Empty(t, backupPath)
	})

	t.Run("empty npmrc returns empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		npmrcPath := filepath.Join(tmpDir, ".npmrc")
		require.NoError(t, os.WriteFile(npmrcPath, []byte("   \n  "), 0600))

		backupPath, err := backupNpmrc(npmrcPath)
		assert.NoError(t, err)
		assert.Empty(t, backupPath)
	})

	t.Run("existing npmrc gets backed up", func(t *testing.T) {
		tmpDir := t.TempDir()
		npmrcPath := filepath.Join(tmpDir, ".npmrc")
		content := "registry=https://registry.npmjs.org/\n"
		require.NoError(t, os.WriteFile(npmrcPath, []byte(content), 0600))

		backupPath, err := backupNpmrc(npmrcPath)
		assert.NoError(t, err)
		assert.NotEmpty(t, backupPath)

		backupData, err := os.ReadFile(backupPath)
		require.NoError(t, err)
		assert.Equal(t, content, string(backupData))
	})

	t.Run("unreadable file returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		npmrcPath := filepath.Join(tmpDir, "subdir", "nested", ".npmrc")

		_, err := backupNpmrc(npmrcPath)
		assert.NoError(t, err) // file doesn't exist, so no error
	})
}

func TestSaveAndLoadNpmRegistryConfig(t *testing.T) {
	t.Run("round trip save and load", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		cfg := NpmRegistryConfig{
			RegistryIdentifier: "my-npm-registry",
			RegistryURL:        "https://pkg.harness.io/pkg/acct123/my-npm-registry/npm",
			Scope:              "@myorg",
			OrgID:              "default",
			ProjectID:          "project1",
			NpmrcBackupPath:    "/tmp/backup",
			NpmrcPath:          "/tmp/.npmrc",
		}

		err := saveNpmRegistryConfig(cfg)
		require.NoError(t, err)

		loaded, err := LoadNpmRegistryConfig()
		require.NoError(t, err)
		assert.Equal(t, cfg.RegistryIdentifier, loaded.RegistryIdentifier)
		assert.Equal(t, cfg.RegistryURL, loaded.RegistryURL)
		assert.Equal(t, cfg.Scope, loaded.Scope)
		assert.Equal(t, cfg.OrgID, loaded.OrgID)
		assert.Equal(t, cfg.ProjectID, loaded.ProjectID)
		assert.Equal(t, cfg.NpmrcBackupPath, loaded.NpmrcBackupPath)
		assert.Equal(t, cfg.NpmrcPath, loaded.NpmrcPath)
	})

	t.Run("load from nonexistent returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		_, err := LoadNpmRegistryConfig()
		assert.Error(t, err)
	})
}

func TestWriteNpmrcConfig(t *testing.T) {
	t.Run("new file without scope", func(t *testing.T) {
		tmpDir := t.TempDir()
		npmrcPath := filepath.Join(tmpDir, ".npmrc")

		err := writeNpmrcConfig(npmrcPath, "https://pkg.harness.io/pkg/acct/reg/npm", "", "mytoken", "pkg.harness.io/pkg/acct/reg/npm")
		require.NoError(t, err)

		data, err := os.ReadFile(npmrcPath)
		require.NoError(t, err)
		content := string(data)

		assert.Contains(t, content, "registry=https://pkg.harness.io/pkg/acct/reg/npm/")
		assert.Contains(t, content, "//pkg.harness.io/pkg/acct/reg/npm/:_authToken=mytoken")
		assert.Contains(t, content, "always-auth=true")
	})

	t.Run("new file with scope", func(t *testing.T) {
		tmpDir := t.TempDir()
		npmrcPath := filepath.Join(tmpDir, ".npmrc")

		err := writeNpmrcConfig(npmrcPath, "https://pkg.harness.io/pkg/acct/reg/npm", "@myorg", "mytoken", "pkg.harness.io/pkg/acct/reg/npm")
		require.NoError(t, err)

		data, err := os.ReadFile(npmrcPath)
		require.NoError(t, err)
		content := string(data)

		assert.Contains(t, content, "@myorg:registry=https://pkg.harness.io/pkg/acct/reg/npm/")
		assert.Contains(t, content, "//pkg.harness.io/pkg/acct/reg/npm/:_authToken=mytoken")
		assert.Contains(t, content, "always-auth=true")
	})

	t.Run("appends new registry config when existing has different host", func(t *testing.T) {
		tmpDir := t.TempDir()
		npmrcPath := filepath.Join(tmpDir, ".npmrc")

		existing := "registry=https://registry.npmjs.org/\nalways-auth=true\n"
		require.NoError(t, os.WriteFile(npmrcPath, []byte(existing), 0600))

		err := writeNpmrcConfig(npmrcPath, "https://pkg.harness.io/pkg/acct/reg/npm", "", "newtoken", "pkg.harness.io/pkg/acct/reg/npm")
		require.NoError(t, err)

		data, err := os.ReadFile(npmrcPath)
		require.NoError(t, err)
		content := string(data)

		assert.Contains(t, content, "registry=https://pkg.harness.io/pkg/acct/reg/npm/")
		assert.Contains(t, content, "//pkg.harness.io/pkg/acct/reg/npm/:_authToken=newtoken")
	})

	t.Run("replaces auth token for same host", func(t *testing.T) {
		tmpDir := t.TempDir()
		npmrcPath := filepath.Join(tmpDir, ".npmrc")

		existing := "registry=https://pkg.harness.io/pkg/acct/reg/npm/\n//pkg.harness.io/pkg/acct/reg/npm/:_authToken=oldtoken\nalways-auth=true\n"
		require.NoError(t, os.WriteFile(npmrcPath, []byte(existing), 0600))

		err := writeNpmrcConfig(npmrcPath, "https://pkg.harness.io/pkg/acct/reg/npm", "", "newtoken", "pkg.harness.io/pkg/acct/reg/npm")
		require.NoError(t, err)

		data, err := os.ReadFile(npmrcPath)
		require.NoError(t, err)
		content := string(data)

		assert.Contains(t, content, "//pkg.harness.io/pkg/acct/reg/npm/:_authToken=newtoken")
		assert.NotContains(t, content, "oldtoken")
	})

	t.Run("updates existing scoped registry line", func(t *testing.T) {
		tmpDir := t.TempDir()
		npmrcPath := filepath.Join(tmpDir, ".npmrc")

		existing := "@myorg:registry=https://pkg.harness.io/pkg/acct/old-reg/npm/\n//pkg.harness.io/pkg/acct/old-reg/npm/:_authToken=oldtoken\n"
		require.NoError(t, os.WriteFile(npmrcPath, []byte(existing), 0600))

		err := writeNpmrcConfig(npmrcPath, "https://pkg.harness.io/pkg/acct/reg/npm", "@myorg", "newtoken", "pkg.harness.io/pkg/acct/reg/npm")
		require.NoError(t, err)

		data, err := os.ReadFile(npmrcPath)
		require.NoError(t, err)
		content := string(data)

		assert.Contains(t, content, "@myorg:registry=https://pkg.harness.io/pkg/acct/reg/npm/")
		assert.Contains(t, content, "//pkg.harness.io/pkg/acct/reg/npm/:_authToken=newtoken")
		// Scoped registry line is replaced, but old auth line for different host remains
		assert.NotContains(t, content, "@myorg:registry=https://pkg.harness.io/pkg/acct/old-reg")
	})

	t.Run("preserves unrelated lines", func(t *testing.T) {
		tmpDir := t.TempDir()
		npmrcPath := filepath.Join(tmpDir, ".npmrc")

		existing := "# my custom config\n@other:registry=https://other.example.com/\n//other.example.com/:_authToken=othertoken\n"
		require.NoError(t, os.WriteFile(npmrcPath, []byte(existing), 0600))

		err := writeNpmrcConfig(npmrcPath, "https://pkg.harness.io/pkg/acct/reg/npm", "@myorg", "newtoken", "pkg.harness.io/pkg/acct/reg/npm")
		require.NoError(t, err)

		data, err := os.ReadFile(npmrcPath)
		require.NoError(t, err)
		content := string(data)

		assert.Contains(t, content, "# my custom config")
		assert.Contains(t, content, "@other:registry=https://other.example.com/")
		assert.Contains(t, content, "//other.example.com/:_authToken=othertoken")
		assert.Contains(t, content, "@myorg:registry=https://pkg.harness.io/pkg/acct/reg/npm/")
		assert.Contains(t, content, "//pkg.harness.io/pkg/acct/reg/npm/:_authToken=newtoken")
	})

	t.Run("file has correct permissions", func(t *testing.T) {
		tmpDir := t.TempDir()
		npmrcPath := filepath.Join(tmpDir, ".npmrc")

		err := writeNpmrcConfig(npmrcPath, "https://pkg.harness.io/npm", "", "token", "pkg.harness.io/npm")
		require.NoError(t, err)

		info, err := os.Stat(npmrcPath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	})

	t.Run("replaces both scope and auth when both exist for same host", func(t *testing.T) {
		tmpDir := t.TempDir()
		npmrcPath := filepath.Join(tmpDir, ".npmrc")

		host := "pkg.harness.io/pkg/acct/reg/npm"
		existing := fmt.Sprintf("@myorg:registry=https://%s/\n//%s/:_authToken=oldtoken\nalways-auth=true\n", host, host)
		require.NoError(t, os.WriteFile(npmrcPath, []byte(existing), 0600))

		err := writeNpmrcConfig(npmrcPath, "https://"+host, "@myorg", "newtoken", host)
		require.NoError(t, err)

		data, err := os.ReadFile(npmrcPath)
		require.NoError(t, err)
		content := string(data)

		assert.Contains(t, content, "@myorg:registry=https://"+host+"/")
		assert.Contains(t, content, "//"+host+"/:_authToken=newtoken")
		assert.NotContains(t, content, "oldtoken")
		// When both are found, always-auth is preserved from original
		assert.Contains(t, content, "always-auth=true")
	})

	t.Run("content ends with newline", func(t *testing.T) {
		tmpDir := t.TempDir()
		npmrcPath := filepath.Join(tmpDir, ".npmrc")

		err := writeNpmrcConfig(npmrcPath, "https://pkg.harness.io/npm", "", "token", "pkg.harness.io/npm")
		require.NoError(t, err)

		data, err := os.ReadFile(npmrcPath)
		require.NoError(t, err)
		assert.True(t, strings.HasSuffix(string(data), "\n"))
	})
}

func TestConfigureNpm(t *testing.T) {
	t.Run("invalid URL returns error", func(t *testing.T) {
		err := configureNpm("://invalid", "", "token", true, false)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid registry URL")
	})

	t.Run("neither global nor project returns nil", func(t *testing.T) {
		err := configureNpm("https://pkg.harness.io/npm", "", "token", false, false)
		assert.NoError(t, err)
	})

	t.Run("global writes to home dir npmrc", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		err := configureNpm("https://pkg.harness.io/pkg/acct/reg/npm", "@myorg", "tok123", true, false)
		require.NoError(t, err)

		data, err := os.ReadFile(filepath.Join(tmpDir, ".npmrc"))
		require.NoError(t, err)
		content := string(data)

		assert.Contains(t, content, "@myorg:registry=https://pkg.harness.io/pkg/acct/reg/npm/")
		assert.Contains(t, content, "//pkg.harness.io/pkg/acct/reg/npm/:_authToken=tok123")
	})

	t.Run("project level writes to cwd npmrc", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(tmpDir))
		defer os.Chdir(origDir)

		err := configureNpm("https://pkg.harness.io/pkg/acct/reg/npm", "", "tok456", false, true)
		require.NoError(t, err)

		data, err := os.ReadFile(filepath.Join(tmpDir, ".npmrc"))
		require.NoError(t, err)
		content := string(data)

		assert.Contains(t, content, "registry=https://pkg.harness.io/pkg/acct/reg/npm/")
		assert.Contains(t, content, "//pkg.harness.io/pkg/acct/reg/npm/:_authToken=tok456")
	})
}

func TestRestoreNpmrc(t *testing.T) {
	t.Run("no config file returns nil", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		err := RestoreNpmrc()
		assert.NoError(t, err)
	})

	t.Run("config with empty backup path returns nil", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		configDir := filepath.Join(tmpDir, ".harness")
		require.NoError(t, os.MkdirAll(configDir, 0755))

		cfg := NpmRegistryConfig{
			RegistryIdentifier: "test",
			RegistryURL:        "https://example.com",
			NpmrcBackupPath:    "",
			NpmrcPath:          "/tmp/.npmrc",
		}
		data, _ := json.MarshalIndent(cfg, "", "  ")
		require.NoError(t, os.WriteFile(filepath.Join(configDir, "npm-config.json"), data, 0600))

		err := RestoreNpmrc()
		assert.NoError(t, err)
	})

	t.Run("restores backup successfully", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		configDir := filepath.Join(tmpDir, ".harness")
		require.NoError(t, os.MkdirAll(configDir, 0755))

		backupContent := "registry=https://registry.npmjs.org/\n"
		backupPath := filepath.Join(tmpDir, "npmrc-backup")
		require.NoError(t, os.WriteFile(backupPath, []byte(backupContent), 0600))

		npmrcPath := filepath.Join(tmpDir, ".npmrc")
		require.NoError(t, os.WriteFile(npmrcPath, []byte("harness config"), 0600))

		cfg := NpmRegistryConfig{
			RegistryIdentifier: "test",
			RegistryURL:        "https://example.com",
			NpmrcBackupPath:    backupPath,
			NpmrcPath:          npmrcPath,
		}
		data, _ := json.MarshalIndent(cfg, "", "  ")
		require.NoError(t, os.WriteFile(filepath.Join(configDir, "npm-config.json"), data, 0600))

		err := RestoreNpmrc()
		assert.NoError(t, err)

		restored, err := os.ReadFile(npmrcPath)
		require.NoError(t, err)
		assert.Equal(t, backupContent, string(restored))
	})

	t.Run("missing backup file returns nil", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		configDir := filepath.Join(tmpDir, ".harness")
		require.NoError(t, os.MkdirAll(configDir, 0755))

		cfg := NpmRegistryConfig{
			RegistryIdentifier: "test",
			RegistryURL:        "https://example.com",
			NpmrcBackupPath:    "/nonexistent/backup",
			NpmrcPath:          filepath.Join(tmpDir, ".npmrc"),
		}
		data, _ := json.MarshalIndent(cfg, "", "  ")
		require.NoError(t, os.WriteFile(filepath.Join(configDir, "npm-config.json"), data, 0600))

		err := RestoreNpmrc()
		assert.NoError(t, err)
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newMockV1Client(statusCode int, body string) *ar.ClientWithResponses {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			header := make(http.Header)
			header.Set("Content-Type", "application/json")
			return &http.Response{
				StatusCode: statusCode,
				Body:       io.NopCloser(bytes.NewBufferString(body)),
				Header:     header,
			}, nil
		}),
	}
	client, _ := ar.NewClientWithResponses("http://test", ar.WithHTTPClient(httpClient))
	return client
}

func newMockV3Client(statusCode int, body string) *ar_v3.ClientWithResponses {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			header := make(http.Header)
			header.Set("Content-Type", "application/json")
			return &http.Response{
				StatusCode: statusCode,
				Body:       io.NopCloser(bytes.NewBufferString(body)),
				Header:     header,
			}, nil
		}),
	}
	client, _ := ar_v3.NewClientWithResponses("http://test", ar_v3.WithHTTPClient(httpClient))
	return client
}

func TestNewConfigureNpmCmd(t *testing.T) {
	t.Run("missing registry flag", func(t *testing.T) {
		config.Global = config.GlobalFlags{
			AccountID: "test-account",
			AuthToken: "test-token",
		}

		f := &cmdutils.Factory{
			RegistryHttpClient:   func() *ar.ClientWithResponses { return newMockV1Client(200, "{}") },
			RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return newMockV3Client(200, "{}") },
		}

		cmd := NewConfigureNpmCmd(f)
		cmd.SetArgs([]string{})
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)

		err := cmd.Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "--registry flag is required")
	})

	t.Run("missing account ID", func(t *testing.T) {
		config.Global = config.GlobalFlags{
			AccountID: "",
			AuthToken: "test-token",
		}

		f := &cmdutils.Factory{
			RegistryHttpClient:   func() *ar.ClientWithResponses { return newMockV1Client(200, "{}") },
			RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return newMockV3Client(200, "{}") },
		}

		cmd := NewConfigureNpmCmd(f)
		cmd.SetArgs([]string{"--registry", "my-reg"})
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)

		err := cmd.Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "account ID not configured")
	})

	t.Run("missing auth token", func(t *testing.T) {
		config.Global = config.GlobalFlags{
			AccountID: "test-account",
			AuthToken: "",
		}

		f := &cmdutils.Factory{
			RegistryHttpClient:   func() *ar.ClientWithResponses { return newMockV1Client(200, "{}") },
			RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return newMockV3Client(200, "{}") },
		}

		cmd := NewConfigureNpmCmd(f)
		cmd.SetArgs([]string{"--registry", "my-reg"})
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)

		err := cmd.Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "auth token not configured")
	})

	t.Run("conflicting global and project-level flags", func(t *testing.T) {
		config.Global = config.GlobalFlags{
			AccountID: "test-account",
			AuthToken: "test-token",
		}

		f := &cmdutils.Factory{
			RegistryHttpClient:   func() *ar.ClientWithResponses { return newMockV1Client(200, "{}") },
			RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return newMockV3Client(200, "{}") },
		}

		cmd := NewConfigureNpmCmd(f)
		cmd.SetArgs([]string{"--registry", "my-reg", "--global", "--project-level"})
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)

		err := cmd.Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot use both --global and --project-level flags")
	})

	t.Run("registry not found returns error", func(t *testing.T) {
		config.Global = config.GlobalFlags{
			AccountID: "test-account",
			AuthToken: "test-token",
		}

		f := &cmdutils.Factory{
			RegistryHttpClient:   func() *ar.ClientWithResponses { return newMockV1Client(404, `{"code":"NOT_FOUND"}`) },
			RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return newMockV3Client(200, "{}") },
		}

		cmd := NewConfigureNpmCmd(f)
		cmd.SetArgs([]string{"--registry", "nonexistent-reg", "--global"})
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)

		err := cmd.Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("system info failure returns error", func(t *testing.T) {
		registryBody := `{"status":"SUCCESS","data":{"identifier":"my-reg","packageType":"NPM"}}`
		config.Global = config.GlobalFlags{
			AccountID: "test-account",
			AuthToken: "test-token",
		}

		f := &cmdutils.Factory{
			RegistryHttpClient:   func() *ar.ClientWithResponses { return newMockV1Client(200, registryBody) },
			RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return newMockV3Client(500, `{"error":"internal"}`) },
		}

		cmd := NewConfigureNpmCmd(f)
		cmd.SetArgs([]string{"--registry", "my-reg", "--global"})
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)

		err := cmd.Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get registry base URL")
	})

	t.Run("successful global configure", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		registryBody := `{"status":"SUCCESS","data":{"identifier":"my-reg","packageType":"NPM"}}`
		systemInfoBody := `{"data":{"registryUrl":"https://pkg.harness.io","registryHost":"pkg.harness.io"}}`

		config.Global = config.GlobalFlags{
			AccountID: "test-account",
			AuthToken: "test-token",
		}

		f := &cmdutils.Factory{
			RegistryHttpClient:   func() *ar.ClientWithResponses { return newMockV1Client(200, registryBody) },
			RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return newMockV3Client(200, systemInfoBody) },
		}

		cmd := NewConfigureNpmCmd(f)
		cmd.SetArgs([]string{"--registry", "my-reg", "--global"})
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)

		err := cmd.Execute()
		require.NoError(t, err)

		// Verify .npmrc was written
		data, err := os.ReadFile(filepath.Join(tmpDir, ".npmrc"))
		require.NoError(t, err)
		content := string(data)
		assert.Contains(t, content, "registry=https://pkg.harness.io/pkg/test-account/my-reg/npm/")
		assert.Contains(t, content, "_authToken=test-token")
	})

	t.Run("successful configure with scope adds @ prefix", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		registryBody := `{"status":"SUCCESS","data":{"identifier":"my-reg","packageType":"NPM"}}`
		systemInfoBody := `{"data":{"registryUrl":"https://pkg.harness.io","registryHost":"pkg.harness.io"}}`

		config.Global = config.GlobalFlags{
			AccountID: "test-account",
			AuthToken: "test-token",
		}

		f := &cmdutils.Factory{
			RegistryHttpClient:   func() *ar.ClientWithResponses { return newMockV1Client(200, registryBody) },
			RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return newMockV3Client(200, systemInfoBody) },
		}

		cmd := NewConfigureNpmCmd(f)
		cmd.SetArgs([]string{"--registry", "my-reg", "--global", "--scope", "myorg"})
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)

		err := cmd.Execute()
		require.NoError(t, err)

		data, err := os.ReadFile(filepath.Join(tmpDir, ".npmrc"))
		require.NoError(t, err)
		content := string(data)
		assert.Contains(t, content, "@myorg:registry=https://pkg.harness.io/pkg/test-account/my-reg/npm/")
	})

	t.Run("project level without package.json fails", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(tmpDir))
		defer os.Chdir(origDir)

		registryBody := `{"status":"SUCCESS","data":{"identifier":"my-reg","packageType":"NPM"}}`
		systemInfoBody := `{"data":{"registryUrl":"https://pkg.harness.io","registryHost":"pkg.harness.io"}}`

		config.Global = config.GlobalFlags{
			AccountID: "test-account",
			AuthToken: "test-token",
		}

		f := &cmdutils.Factory{
			RegistryHttpClient:   func() *ar.ClientWithResponses { return newMockV1Client(200, registryBody) },
			RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return newMockV3Client(200, systemInfoBody) },
		}

		cmd := NewConfigureNpmCmd(f)
		cmd.SetArgs([]string{"--registry", "my-reg"})
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)

		err := cmd.Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "package.json not found")
	})

	t.Run("project level with package.json succeeds", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(tmpDir))
		defer os.Chdir(origDir)

		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(`{"name":"test"}`), 0644))

		registryBody := `{"status":"SUCCESS","data":{"identifier":"my-reg","packageType":"NPM"}}`
		systemInfoBody := `{"data":{"registryUrl":"https://pkg.harness.io","registryHost":"pkg.harness.io"}}`

		config.Global = config.GlobalFlags{
			AccountID: "test-account",
			AuthToken: "test-token",
		}

		f := &cmdutils.Factory{
			RegistryHttpClient:   func() *ar.ClientWithResponses { return newMockV1Client(200, registryBody) },
			RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return newMockV3Client(200, systemInfoBody) },
		}

		cmd := NewConfigureNpmCmd(f)
		cmd.SetArgs([]string{"--registry", "my-reg", "--project-level"})
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)

		err := cmd.Execute()
		require.NoError(t, err)

		data, err := os.ReadFile(filepath.Join(tmpDir, ".npmrc"))
		require.NoError(t, err)
		content := string(data)
		assert.Contains(t, content, "registry=https://pkg.harness.io/pkg/test-account/my-reg/npm/")
	})
}

func TestGetRegistryBaseURL(t *testing.T) {
	t.Run("successful response", func(t *testing.T) {
		systemInfoBody := `{"data":{"registryUrl":"https://pkg.harness.io/","registryHost":"pkg.harness.io"}}`
		f := &cmdutils.Factory{
			RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return newMockV3Client(200, systemInfoBody) },
		}

		url, err := getRegistryBaseURL(f, "test-account")
		require.NoError(t, err)
		assert.Equal(t, "https://pkg.harness.io", url)
	})

	t.Run("empty registryUrl returns error", func(t *testing.T) {
		systemInfoBody := `{"data":{"registryUrl":"","registryHost":"pkg.harness.io"}}`
		f := &cmdutils.Factory{
			RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return newMockV3Client(200, systemInfoBody) },
		}

		_, err := getRegistryBaseURL(f, "test-account")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to extract registryUrl")
	})

	t.Run("non-200 response returns error", func(t *testing.T) {
		f := &cmdutils.Factory{
			RegistryV3HttpClient: func() *ar_v3.ClientWithResponses {
				return newMockV3Client(500, `{"error":{"errors":[{"message":"internal"}]}}`)
			},
		}

		_, err := getRegistryBaseURL(f, "test-account")
		assert.Error(t, err)
	})

	t.Run("trims trailing slash", func(t *testing.T) {
		systemInfoBody := `{"data":{"registryUrl":"https://pkg.harness.io/"}}`
		f := &cmdutils.Factory{
			RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return newMockV3Client(200, systemInfoBody) },
		}

		url, err := getRegistryBaseURL(f, "test-account")
		require.NoError(t, err)
		assert.Equal(t, "https://pkg.harness.io", url)
	})
}

func TestNpmRegistryConfigJSON(t *testing.T) {
	t.Run("serialization includes all fields", func(t *testing.T) {
		cfg := NpmRegistryConfig{
			RegistryIdentifier: "my-reg",
			RegistryURL:        "https://pkg.harness.io/pkg/acct/my-reg/npm",
			Scope:              "@myorg",
			OrgID:              "org1",
			ProjectID:          "proj1",
			NpmrcBackupPath:    "/backup/path",
			NpmrcPath:          "/home/user/.npmrc",
		}

		data, err := json.Marshal(cfg)
		require.NoError(t, err)

		var decoded map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &decoded))

		assert.Equal(t, "my-reg", decoded["registryIdentifier"])
		assert.Equal(t, "https://pkg.harness.io/pkg/acct/my-reg/npm", decoded["registryUrl"])
		assert.Equal(t, "@myorg", decoded["scope"])
		assert.Equal(t, "org1", decoded["orgId"])
		assert.Equal(t, "proj1", decoded["projectId"])
		assert.Equal(t, "/backup/path", decoded["npmrcBackupPath"])
		assert.Equal(t, "/home/user/.npmrc", decoded["npmrcPath"])
	})

	t.Run("omits empty optional fields", func(t *testing.T) {
		cfg := NpmRegistryConfig{
			RegistryIdentifier: "my-reg",
			RegistryURL:        "https://example.com",
			NpmrcPath:          "/path",
		}

		data, err := json.Marshal(cfg)
		require.NoError(t, err)

		var decoded map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &decoded))

		_, hasScope := decoded["scope"]
		_, hasOrgID := decoded["orgId"]
		_, hasProjectID := decoded["projectId"]
		_, hasBackup := decoded["npmrcBackupPath"]
		assert.False(t, hasScope)
		assert.False(t, hasOrgID)
		assert.False(t, hasProjectID)
		assert.False(t, hasBackup)
	})
}
