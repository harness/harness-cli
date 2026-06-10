package command

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/harness/harness-cli/cmd/cmdutils"
	regcmd "github.com/harness/harness-cli/cmd/registry/command"
	ar "github.com/harness/harness-cli/internal/api/ar"
	ar_v3 "github.com/harness/harness-cli/internal/api/ar_v3"
	"github.com/harness/harness-cli/util/common/progress"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePackageJsonPath(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		setup   func() string
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid package.json",
			setup: func() string {
				filePath := filepath.Join(tmpDir, "valid", "package.json")
				os.MkdirAll(filepath.Dir(filePath), 0755)
				data := []byte(`{"name": "test-package", "version": "1.0.0"}`)
				os.WriteFile(filePath, data, 0644)
				return filePath
			},
			wantErr: false,
		},
		{
			name: "file does not exist",
			setup: func() string {
				return filepath.Join(tmpDir, "nonexistent", "package.json")
			},
			wantErr: true,
			errMsg:  "file not found",
		},
		{
			name: "path is a directory",
			setup: func() string {
				dirPath := filepath.Join(tmpDir, "directory")
				os.MkdirAll(dirPath, 0755)
				return dirPath
			},
			wantErr: true,
			errMsg:  "path is a directory",
		},
		{
			name: "wrong filename",
			setup: func() string {
				filePath := filepath.Join(tmpDir, "wrong", "test.json")
				os.MkdirAll(filepath.Dir(filePath), 0755)
				data := []byte(`{"name": "test"}`)
				os.WriteFile(filePath, data, 0644)
				return filePath
			},
			wantErr: true,
			errMsg:  "file must be named 'package.json'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := tt.setup()
			err := validatePackageJsonPath(filePath)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFilterVulnerablePackages(t *testing.T) {
	tests := []struct {
		name    string
		results []regcmd.ScanResult
		want    int
	}{
		{
			name: "mixed statuses",
			results: []regcmd.ScanResult{
				{PackageName: "pkg1", ScanStatus: "BLOCKED"},
				{PackageName: "pkg2", ScanStatus: "ALLOWED"},
				{PackageName: "pkg3", ScanStatus: "WARN"},
				{PackageName: "pkg4", ScanStatus: "UNKNOWN"},
			},
			want: 2, // BLOCKED and WARN
		},
		{
			name: "all vulnerable",
			results: []regcmd.ScanResult{
				{PackageName: "pkg1", ScanStatus: "BLOCKED"},
				{PackageName: "pkg2", ScanStatus: "WARN"},
			},
			want: 2,
		},
		{
			name: "none vulnerable",
			results: []regcmd.ScanResult{
				{PackageName: "pkg1", ScanStatus: "ALLOWED"},
				{PackageName: "pkg2", ScanStatus: "UNKNOWN"},
			},
			want: 0,
		},
		{
			name:    "empty results",
			results: []regcmd.ScanResult{},
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vulnerable := filterVulnerablePackages(tt.results)
			assert.Equal(t, tt.want, len(vulnerable))

			// Verify all returned packages are actually vulnerable
			for _, pkg := range vulnerable {
				assert.True(t, pkg.ScanStatus == "BLOCKED" || pkg.ScanStatus == "WARN")
			}
		})
	}
}

func TestMajorChange(t *testing.T) {
	tests := []struct {
		name    string
		fixInfo SecurityFixInfo
		want    bool
	}{
		{
			name: "major version change - 7 to 6",
			fixInfo: SecurityFixInfo{
				CurrentVersion: "7.4.5",
				FixVersion:     "6.4.6",
			},
			want: true,
		},
		{
			name: "major version change - 1 to 2",
			fixInfo: SecurityFixInfo{
				CurrentVersion: "1.0.0",
				FixVersion:     "2.0.0",
			},
			want: true,
		},
		{
			name: "minor version change",
			fixInfo: SecurityFixInfo{
				CurrentVersion: "4.17.15",
				FixVersion:     "4.17.21",
			},
			want: false,
		},
		{
			name: "patch version change",
			fixInfo: SecurityFixInfo{
				CurrentVersion: "1.2.3",
				FixVersion:     "1.2.4",
			},
			want: false,
		},
		{
			name: "same major version",
			fixInfo: SecurityFixInfo{
				CurrentVersion: "3.1.0",
				FixVersion:     "3.5.0",
			},
			want: false,
		},
		{
			name: "version with caret prefix",
			fixInfo: SecurityFixInfo{
				CurrentVersion: "^4.0.0",
				FixVersion:     "5.0.0",
			},
			want: true,
		},
		{
			name: "version with tilde prefix",
			fixInfo: SecurityFixInfo{
				CurrentVersion: "~2.1.0",
				FixVersion:     "2.2.0",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := majorChange(tt.fixInfo)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestExtractMajorVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    int
	}{
		{
			name:    "simple version",
			version: "4.17.21",
			want:    4,
		},
		{
			name:    "version with v prefix",
			version: "v10.3.1",
			want:    10,
		},
		{
			name:    "version with caret",
			version: "^2.1.0",
			want:    2,
		},
		{
			name:    "version with tilde",
			version: "~3.5.0",
			want:    3,
		},
		{
			name:    "single digit",
			version: "7",
			want:    7,
		},
		{
			name:    "double digit major",
			version: "15.2.1",
			want:    15,
		},
		{
			name:    "invalid version",
			version: "invalid",
			want:    -1,
		},
		{
			name:    "empty version",
			version: "",
			want:    -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractMajorVersion(tt.version)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestBackupPackageJson(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		setup   func() string
		wantErr bool
	}{
		{
			name: "successful backup",
			setup: func() string {
				filePath := filepath.Join(tmpDir, "backup-test", "package.json")
				os.MkdirAll(filepath.Dir(filePath), 0755)
				data := []byte(`{"name": "test", "version": "1.0.0"}`)
				os.WriteFile(filePath, data, 0644)
				return filePath
			},
			wantErr: false,
		},
		{
			name: "file does not exist",
			setup: func() string {
				return filepath.Join(tmpDir, "nonexistent", "package.json")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := tt.setup()
			err := backupPackageJson(filePath)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify backup file exists
				backupPath := filePath + ".backup"
				_, err := os.Stat(backupPath)
				assert.NoError(t, err, "backup file should exist")

				// Verify backup content matches original
				originalData, _ := os.ReadFile(filePath)
				backupData, _ := os.ReadFile(backupPath)
				assert.Equal(t, originalData, backupData)
			}
		})
	}
}

func TestUpdateDependencySectionWithFix(t *testing.T) {
	tests := []struct {
		name    string
		pkgJson map[string]interface{}
		section string
		fix     SecurityFixInfo
		want    int
	}{
		{
			name: "update existing dependency",
			pkgJson: map[string]interface{}{
				"dependencies": map[string]interface{}{
					"lodash": "4.17.15",
					"axios":  "0.21.0",
				},
			},
			section: "dependencies",
			fix: SecurityFixInfo{
				PackageName: "lodash",
				FixVersion:  "4.17.21",
			},
			want: 1,
		},
		{
			name: "package not in section",
			pkgJson: map[string]interface{}{
				"dependencies": map[string]interface{}{
					"express": "4.18.2",
				},
			},
			section: "dependencies",
			fix: SecurityFixInfo{
				PackageName: "lodash",
				FixVersion:  "4.17.21",
			},
			want: 0,
		},
		{
			name:    "section does not exist",
			pkgJson: map[string]interface{}{},
			section: "dependencies",
			fix: SecurityFixInfo{
				PackageName: "lodash",
				FixVersion:  "4.17.21",
			},
			want: 0,
		},
		{
			name: "update devDependencies",
			pkgJson: map[string]interface{}{
				"devDependencies": map[string]interface{}{
					"jest": "29.0.0",
				},
			},
			section: "devDependencies",
			fix: SecurityFixInfo{
				PackageName: "jest",
				FixVersion:  "29.5.0",
			},
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := updateDependencySectionWithFix(tt.pkgJson, tt.section, tt.fix)
			assert.Equal(t, tt.want, result)

			// If update was successful, verify the version was actually changed
			if result > 0 {
				deps := tt.pkgJson[tt.section].(map[string]interface{})
				assert.Equal(t, tt.fix.FixVersion, deps[tt.fix.PackageName])
			}
		})
	}
}

func TestUpdatePackageJsonWithFixes(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("update multiple dependencies", func(t *testing.T) {
		// Create test package.json
		filePath := filepath.Join(tmpDir, "update-test", "package.json")
		os.MkdirAll(filepath.Dir(filePath), 0755)

		pkgData := map[string]interface{}{
			"name":    "test-package",
			"version": "1.0.0",
			"dependencies": map[string]interface{}{
				"lodash": "4.17.15",
				"axios":  "0.21.0",
			},
			"devDependencies": map[string]interface{}{
				"jest": "29.0.0",
			},
		}

		data, _ := json.MarshalIndent(pkgData, "", "  ")
		os.WriteFile(filePath, data, 0644)

		// Create fixes
		fixes := []SecurityFixInfo{
			{PackageName: "lodash", FixVersion: "4.17.21"},
			{PackageName: "jest", FixVersion: "29.5.0"},
		}

		// Use actual progress reporter
		prog := progress.NewConsoleReporter()

		// Update package.json
		err := updatePackageJsonWithFixes(filePath, fixes, prog)
		require.NoError(t, err)

		// Read updated file
		updatedData, err := os.ReadFile(filePath)
		require.NoError(t, err)

		var updatedPkg map[string]interface{}
		err = json.Unmarshal(updatedData, &updatedPkg)
		require.NoError(t, err)

		// Verify updates
		deps := updatedPkg["dependencies"].(map[string]interface{})
		assert.Equal(t, "4.17.21", deps["lodash"])

		devDeps := updatedPkg["devDependencies"].(map[string]interface{})
		assert.Equal(t, "29.5.0", devDeps["jest"])
	})
}

func TestSecurityFixInfoStruct(t *testing.T) {
	fix := SecurityFixInfo{
		PackageName:      "lodash",
		CurrentVersion:   "4.17.15",
		FixVersion:       "4.17.21",
		FixAvailable:     true,
		HasSecurityIssue: true,
	}

	assert.Equal(t, "lodash", fix.PackageName)
	assert.Equal(t, "4.17.15", fix.CurrentVersion)
	assert.Equal(t, "4.17.21", fix.FixVersion)
	assert.True(t, fix.FixAvailable)
	assert.True(t, fix.HasSecurityIssue)
}

func TestRegistryInfoStruct(t *testing.T) {
	info := registryInfo{
		RegistryURL:        "https://registry.example.com",
		RegistryIdentifier: "test-registry",
		OrgID:              "test-org",
		ProjectID:          "test-project",
	}

	assert.Equal(t, "https://registry.example.com", info.RegistryURL)
	assert.Equal(t, "test-registry", info.RegistryIdentifier)
	assert.Equal(t, "test-org", info.OrgID)
	assert.Equal(t, "test-project", info.ProjectID)
}

func TestNewNpmAuditCmd(t *testing.T) {
	// Create a mock factory (nil is acceptable for command creation)
	cmd := NewNpmAuditCmd(nil)

	assert.NotNil(t, cmd)
	assert.Equal(t, "audit", cmd.Use)
	assert.Equal(t, "Audit npm dependencies for vulnerabilities", cmd.Short)
	assert.NotNil(t, cmd.RunE)

	// Check flags
	fixFlag := cmd.Flags().Lookup("fix")
	assert.NotNil(t, fixFlag)
	assert.Equal(t, "false", fixFlag.DefValue)

	fileFlag := cmd.Flags().Lookup("file")
	assert.NotNil(t, fileFlag)
	assert.Equal(t, "package.json", fileFlag.DefValue)
}

func TestDetectNpmRegistry(t *testing.T) {
	// This test verifies the detectNpmRegistry function behavior
	// Note: This function depends on regcmd.LoadNpmRegistryConfig() which reads from
	// the user's home directory, making it difficult to test in isolation without mocking.
	// We test that it returns the expected error when no config exists.

	t.Run("returns error when no config", func(t *testing.T) {
		// Save current HOME
		originalHome := os.Getenv("HOME")
		defer os.Setenv("HOME", originalHome)

		// Set HOME to a non-existent directory
		tmpDir := t.TempDir()
		nonExistentDir := filepath.Join(tmpDir, "nonexistent")
		os.Setenv("HOME", nonExistentDir)

		_, err := detectNpmRegistry()
		// Should return an error when config doesn't exist
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no HAR registry found")
	})

	t.Run("registryInfo struct", func(t *testing.T) {
		// Test the registryInfo struct creation
		info := &registryInfo{
			RegistryURL:        "https://test-registry.com",
			RegistryIdentifier: "test-registry",
			OrgID:              "test-org",
			ProjectID:          "test-project",
		}

		assert.Equal(t, "https://test-registry.com", info.RegistryURL)
		assert.Equal(t, "test-registry", info.RegistryIdentifier)
		assert.Equal(t, "test-org", info.OrgID)
		assert.Equal(t, "test-project", info.ProjectID)
	})
}

func TestDisplaySecurityFixes(t *testing.T) {
	tests := []struct {
		name  string
		fixes []SecurityFixInfo
	}{
		{
			name: "display multiple fixes",
			fixes: []SecurityFixInfo{
				{
					PackageName:    "lodash",
					CurrentVersion: "4.17.15",
					FixVersion:     "4.17.21",
					FixAvailable:   true,
				},
				{
					PackageName:    "axios",
					CurrentVersion: "0.21.0",
					FixVersion:     "0.21.4",
					FixAvailable:   true,
				},
			},
		},
		{
			name: "display fix without version",
			fixes: []SecurityFixInfo{
				{
					PackageName:    "express",
					CurrentVersion: "4.17.0",
					FixVersion:     "",
					FixAvailable:   false,
				},
			},
		},
		{
			name:  "empty fixes",
			fixes: []SecurityFixInfo{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog := progress.NewConsoleReporter()
			// This function prints to stdout, so we just verify it doesn't panic
			assert.NotPanics(t, func() {
				displaySecurityFixes(tt.fixes, prog)
			})
		})
	}
}

func TestDisplayFixComparisonFromSecurityFixes(t *testing.T) {
	tests := []struct {
		name  string
		fixes []SecurityFixInfo
	}{
		{
			name: "display comparison for multiple packages",
			fixes: []SecurityFixInfo{
				{
					PackageName:    "lodash",
					CurrentVersion: "4.17.15",
					FixVersion:     "4.17.21",
				},
				{
					PackageName:    "axios",
					CurrentVersion: "0.21.0",
					FixVersion:     "1.2.0",
				},
			},
		},
		{
			name: "single package comparison",
			fixes: []SecurityFixInfo{
				{
					PackageName:    "react",
					CurrentVersion: "17.0.0",
					FixVersion:     "18.0.0",
				},
			},
		},
		{
			name:  "empty fixes",
			fixes: []SecurityFixInfo{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This function prints to stdout, so we just verify it doesn't panic
			assert.NotPanics(t, func() {
				displayFixComparisonFromSecurityFixes(tt.fixes)
			})
		})
	}
}

func TestEvaluateFixVersions_ErrorHandling(t *testing.T) {
	// Test with empty vulnerable packages
	fixes, manualFix, err := evaluateFixVersions(nil, []regcmd.ScanResult{})
	assert.NoError(t, err)
	assert.Empty(t, fixes)
	assert.Empty(t, manualFix)

	// Test with packages without scan IDs
	packages := []regcmd.ScanResult{
		{
			PackageName: "test-pkg",
			Version:     "1.0.0",
			ScanID:      "",
			ScanStatus:  "BLOCKED",
		},
	}
	fixes, manualFix, err = evaluateFixVersions(nil, packages)
	assert.NoError(t, err)
	assert.Empty(t, fixes)
	assert.Empty(t, manualFix)
}

func TestRunNpmAudit_ValidationError(t *testing.T) {
	// Note: runNpmAudit requires a valid factory and registry configuration.
	// Testing the full flow requires mocking the HTTP client and registry config.
	// Instead, we test the validation that happens before runNpmAudit is called.

	tmpDir := t.TempDir()

	t.Run("validation catches invalid path", func(t *testing.T) {
		// This tests that validatePackageJsonPath (called before runNpmAudit) works
		invalidPath := filepath.Join(tmpDir, "nonexistent", "package.json")
		err := validatePackageJsonPath(invalidPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "file not found")
	})

	t.Run("validation catches directory instead of file", func(t *testing.T) {
		dirPath := tmpDir
		err := validatePackageJsonPath(dirPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "path is a directory")
	})
}

func TestMajorChangeEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		fixInfo SecurityFixInfo
		want    bool
	}{
		{
			name: "both versions invalid",
			fixInfo: SecurityFixInfo{
				CurrentVersion: "invalid",
				FixVersion:     "also-invalid",
			},
			want: false,
		},
		{
			name: "current version invalid",
			fixInfo: SecurityFixInfo{
				CurrentVersion: "invalid",
				FixVersion:     "2.0.0",
			},
			want: false,
		},
		{
			name: "fix version invalid",
			fixInfo: SecurityFixInfo{
				CurrentVersion: "1.0.0",
				FixVersion:     "invalid",
			},
			want: false,
		},
		{
			name: "empty versions",
			fixInfo: SecurityFixInfo{
				CurrentVersion: "",
				FixVersion:     "",
			},
			want: false,
		},
		{
			name: "version with multiple prefixes",
			fixInfo: SecurityFixInfo{
				CurrentVersion: "^~v1.0.0",
				FixVersion:     "2.0.0",
			},
			want: false, // Will fail to parse after stripping known prefixes
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := majorChange(tt.fixInfo)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestExtractMajorVersionEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    int
	}{
		{
			name:    "version with spaces",
			version: " 4.17.21 ",
			want:    4, // Spaces are trimmed by strings.Split
		},
		{
			name:    "version with letters",
			version: "v1.2.3-alpha",
			want:    1,
		},
		{
			name:    "only dots",
			version: "...",
			want:    -1,
		},
		{
			name:    "negative version",
			version: "-1.0.0",
			want:    -1,
		},
		{
			name:    "very large major version",
			version: "999.0.0",
			want:    999,
		},
		{
			name:    "version with text prefix",
			version: "version4.0.0",
			want:    -1, // Will fail to parse "version4" as int
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractMajorVersion(tt.version)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestBackupPackageJsonEdgeCases(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("backup already exists", func(t *testing.T) {
		// Create original file
		filePath := filepath.Join(tmpDir, "existing-backup", "package.json")
		os.MkdirAll(filepath.Dir(filePath), 0755)
		originalData := []byte(`{"name": "test", "version": "1.0.0"}`)
		os.WriteFile(filePath, originalData, 0644)

		// Create existing backup
		backupPath := filePath + ".backup"
		oldBackupData := []byte(`{"name": "old", "version": "0.9.0"}`)
		os.WriteFile(backupPath, oldBackupData, 0644)

		// Create new backup (should overwrite)
		err := backupPackageJson(filePath)
		assert.NoError(t, err)

		// Verify backup was overwritten with current content
		newBackupData, _ := os.ReadFile(backupPath)
		assert.Equal(t, originalData, newBackupData)
	})

	t.Run("read-only directory", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("Skipping test when running as root")
		}

		readOnlyDir := filepath.Join(tmpDir, "readonly")
		os.MkdirAll(readOnlyDir, 0755)
		filePath := filepath.Join(readOnlyDir, "package.json")
		os.WriteFile(filePath, []byte(`{"name": "test"}`), 0644)

		// Make directory read-only
		os.Chmod(readOnlyDir, 0555)
		defer os.Chmod(readOnlyDir, 0755) // Restore for cleanup

		err := backupPackageJson(filePath)
		assert.Error(t, err)
	})
}

// ----- Struct tests -----

func TestPackageFixStruct(t *testing.T) {
	fix := PackageFix{
		Name:           "lodash",
		CurrentVersion: "4.17.15",
		FixVersion:     "4.17.21",
		ScanStatus:     "BLOCKED",
		Vulnerabilities: []VulnerabilityInfo{
			{CveId: "CVE-2021-1234", CvssScore: 9.8, Severity: "CRITICAL"},
		},
	}
	assert.Equal(t, "lodash", fix.Name)
	assert.Equal(t, "4.17.15", fix.CurrentVersion)
	assert.Equal(t, "4.17.21", fix.FixVersion)
	assert.Equal(t, "BLOCKED", fix.ScanStatus)
	assert.Len(t, fix.Vulnerabilities, 1)
	assert.Equal(t, "CVE-2021-1234", fix.Vulnerabilities[0].CveId)
	assert.Equal(t, 9.8, fix.Vulnerabilities[0].CvssScore)
	assert.Equal(t, "CRITICAL", fix.Vulnerabilities[0].Severity)
}

func TestVulnerabilityInfoStruct(t *testing.T) {
	v := VulnerabilityInfo{
		CveId:     "CVE-2022-5678",
		CvssScore: 7.5,
		Severity:  "HIGH",
	}
	assert.Equal(t, "CVE-2022-5678", v.CveId)
	assert.Equal(t, 7.5, v.CvssScore)
	assert.Equal(t, "HIGH", v.Severity)
}

// ----- validatePackageJsonPath edge case -----

func TestValidatePackageJsonPath_DirectFilename(t *testing.T) {
	// "package.json" without any "/" - fileName branch uses filePath directly
	tmpDir := t.TempDir()
	// Change to tmp dir and back so we can use a relative path
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(tmpDir)

	// Create package.json in tmpDir
	os.WriteFile("package.json", []byte(`{"name":"test"}`), 0644)
	err := validatePackageJsonPath("package.json")
	assert.NoError(t, err)
}

// ----- detectNpmRegistry success path -----

func TestDetectNpmRegistry_Success(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Write the config file directly (saveNpmRegistryConfig is unexported)
	configDir := filepath.Join(tmpDir, ".harness")
	os.MkdirAll(configDir, 0755)
	cfgJSON := `{"registryIdentifier":"my-registry","registryUrl":"https://registry.example.com","orgId":"my-org","projectId":"my-project","npmrcPath":""}`
	os.WriteFile(filepath.Join(configDir, "npm-config.json"), []byte(cfgJSON), 0600)

	info, err := detectNpmRegistry()
	require.NoError(t, err)
	assert.Equal(t, "https://registry.example.com", info.RegistryURL)
	assert.Equal(t, "my-registry", info.RegistryIdentifier)
	assert.Equal(t, "my-org", info.OrgID)
	assert.Equal(t, "my-project", info.ProjectID)
}

// ----- resolveRegistryUUID tests via httptest.Server -----

func newMockRegistryServer(statusCode int, uuidStr string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if statusCode == 200 && uuidStr != "" {
			fmt.Fprintf(w, `{"data":{"identifier":"test-registry","packageType":"NPM","url":"https://example.com","uuid":%q}}`, uuidStr)
		} else if statusCode == 200 {
			fmt.Fprintf(w, `{"data":{"identifier":"test-registry","packageType":"NPM","url":"https://example.com"}}`)
		} else {
			fmt.Fprintf(w, `{"message":"not found"}`)
		}
	}))
}

func TestResolveRegistryUUID_Success(t *testing.T) {
	expectedUUID := "550e8400-e29b-41d4-a716-446655440000"
	srv := newMockRegistryServer(200, expectedUUID)
	defer srv.Close()

	client, err := ar.NewClientWithResponses(srv.URL)
	require.NoError(t, err)

	f := &cmdutils.Factory{
		RegistryHttpClient: func() *ar.ClientWithResponses { return client },
	}

	prog := progress.NewConsoleReporter()
	uid, err := resolveRegistryUUID(f, "my-registry", "org", "project", prog)
	require.NoError(t, err)
	assert.Equal(t, expectedUUID, uid.String())
}

func TestResolveRegistryUUID_Non200Response(t *testing.T) {
	srv := newMockRegistryServer(404, "")
	defer srv.Close()

	client, err := ar.NewClientWithResponses(srv.URL)
	require.NoError(t, err)

	f := &cmdutils.Factory{
		RegistryHttpClient: func() *ar.ClientWithResponses { return client },
	}

	prog := progress.NewConsoleReporter()
	_, err = resolveRegistryUUID(f, "missing-registry", "org", "project", prog)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing-registry")
}

func TestResolveRegistryUUID_NilUUID(t *testing.T) {
	srv := newMockRegistryServer(200, "")
	defer srv.Close()

	client, err := ar.NewClientWithResponses(srv.URL)
	require.NoError(t, err)

	f := &cmdutils.Factory{
		RegistryHttpClient: func() *ar.ClientWithResponses { return client },
	}

	prog := progress.NewConsoleReporter()
	_, err = resolveRegistryUUID(f, "no-uuid-registry", "org", "project", prog)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestResolveRegistryUUID_InvalidUUID(t *testing.T) {
	srv := newMockRegistryServer(200, "not-a-valid-uuid")
	defer srv.Close()

	client, err := ar.NewClientWithResponses(srv.URL)
	require.NoError(t, err)

	f := &cmdutils.Factory{
		RegistryHttpClient: func() *ar.ClientWithResponses { return client },
	}

	prog := progress.NewConsoleReporter()
	_, err = resolveRegistryUUID(f, "bad-uuid-registry", "org", "project", prog)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid registry UUID")
}

// ----- evaluateFixVersions via httptest.Server -----

func newMockScanDetailsServer(statusCode int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		fmt.Fprint(w, body)
	}))
}

func TestEvaluateFixVersions_NoSecurityViolation(t *testing.T) {
	body := `{"data":{"id":"00000000-0000-0000-0000-000000000001","packageName":"lodash","packageType":"NPM","registryName":"test","scanStatus":"BLOCKED","version":"4.17.15","policySetFailureDetails":[{"policyFailureDetails":[{"category":"License","policyName":"license-policy","policyRef":"lp"}],"policySetName":"ps","policySetRef":"psr"}]}}`
	srv := newMockScanDetailsServer(200, body)
	defer srv.Close()

	client, err := ar_v3.NewClientWithResponses(srv.URL)
	require.NoError(t, err)

	f := &cmdutils.Factory{
		RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return client },
	}

	packages := []regcmd.ScanResult{{PackageName: "lodash", Version: "4.17.15", ScanID: "scan-123", ScanStatus: "BLOCKED"}}
	fixes, manualFix, err := evaluateFixVersions(f, packages)
	assert.NoError(t, err)
	assert.Empty(t, fixes)
	assert.Empty(t, manualFix)
}

func TestEvaluateFixVersions_SecurityViolation_MinorFix(t *testing.T) {
	fixVer := "4.17.21"
	body := fmt.Sprintf(`{"data":{"id":"00000000-0000-0000-0000-000000000001","packageName":"lodash","packageType":"NPM","registryName":"test","scanStatus":"BLOCKED","version":"4.17.15","policySetFailureDetails":[{"policyFailureDetails":[{"category":"Security","policyName":"sec-policy","policyRef":"sp"}],"policySetName":"ps","policySetRef":"psr"}],"fixVersionDetails":{"currentVersion":"4.17.15","fixVersion":%q,"fixVersionAvailable":true}}}`, fixVer)
	srv := newMockScanDetailsServer(200, body)
	defer srv.Close()

	client, err := ar_v3.NewClientWithResponses(srv.URL)
	require.NoError(t, err)

	f := &cmdutils.Factory{
		RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return client },
	}

	packages := []regcmd.ScanResult{{PackageName: "lodash", Version: "4.17.15", ScanID: "scan-123", ScanStatus: "BLOCKED"}}
	fixes, manualFix, err := evaluateFixVersions(f, packages)
	assert.NoError(t, err)
	require.Len(t, fixes, 1)
	assert.Empty(t, manualFix)
	assert.Equal(t, "lodash", fixes[0].PackageName)
	assert.Equal(t, "4.17.21", fixes[0].FixVersion)
	assert.True(t, fixes[0].FixAvailable)
}

func TestEvaluateFixVersions_SecurityViolation_MajorFix(t *testing.T) {
	fixVer := "5.0.0"
	body := fmt.Sprintf(`{"data":{"id":"00000000-0000-0000-0000-000000000001","packageName":"express","packageType":"NPM","registryName":"test","scanStatus":"BLOCKED","version":"4.18.2","policySetFailureDetails":[{"policyFailureDetails":[{"category":"Security","policyName":"sec-policy","policyRef":"sp"}],"policySetName":"ps","policySetRef":"psr"}],"fixVersionDetails":{"currentVersion":"4.18.2","fixVersion":%q,"fixVersionAvailable":true}}}`, fixVer)
	srv := newMockScanDetailsServer(200, body)
	defer srv.Close()

	client, err := ar_v3.NewClientWithResponses(srv.URL)
	require.NoError(t, err)

	f := &cmdutils.Factory{
		RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return client },
	}

	packages := []regcmd.ScanResult{{PackageName: "express", Version: "4.18.2", ScanID: "scan-456", ScanStatus: "BLOCKED"}}
	fixes, manualFix, err := evaluateFixVersions(f, packages)
	assert.NoError(t, err)
	assert.Empty(t, fixes)
	require.Len(t, manualFix, 1)
	assert.Equal(t, "express", manualFix[0].PackageName)
	assert.Equal(t, "5.0.0", manualFix[0].FixVersion)
}

func TestEvaluateFixVersions_SecurityViolation_NilFixVersion(t *testing.T) {
	body := `{"data":{"id":"00000000-0000-0000-0000-000000000001","packageName":"lodash","packageType":"NPM","registryName":"test","scanStatus":"BLOCKED","version":"4.17.15","policySetFailureDetails":[{"policyFailureDetails":[{"category":"Security","policyName":"sec","policyRef":"sp"}],"policySetName":"ps","policySetRef":"psr"}],"fixVersionDetails":{"currentVersion":"4.17.15","fixVersionAvailable":false}}}`
	srv := newMockScanDetailsServer(200, body)
	defer srv.Close()

	client, err := ar_v3.NewClientWithResponses(srv.URL)
	require.NoError(t, err)

	f := &cmdutils.Factory{
		RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return client },
	}

	packages := []regcmd.ScanResult{{PackageName: "lodash", Version: "4.17.15", ScanID: "scan-789", ScanStatus: "BLOCKED"}}
	fixes, manualFix, err := evaluateFixVersions(f, packages)
	assert.NoError(t, err)
	// No fixVersion → FixVersion is "", major check: extractMajorVersion("") = -1, so not a major change → goes to fixes
	require.Len(t, fixes, 1)
	assert.Empty(t, manualFix)
	assert.Equal(t, "", fixes[0].FixVersion)
}

func TestEvaluateFixVersions_Non200Response(t *testing.T) {
	srv := newMockScanDetailsServer(404, `{"message":"not found"}`)
	defer srv.Close()

	client, err := ar_v3.NewClientWithResponses(srv.URL)
	require.NoError(t, err)

	f := &cmdutils.Factory{
		RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return client },
	}

	packages := []regcmd.ScanResult{{PackageName: "lodash", Version: "4.17.15", ScanID: "scan-123", ScanStatus: "BLOCKED"}}
	fixes, manualFix, err := evaluateFixVersions(f, packages)
	// Non-200 skips the package, returns empty with no error
	assert.NoError(t, err)
	assert.Empty(t, fixes)
	assert.Empty(t, manualFix)
}

func TestEvaluateFixVersions_MultiplePackages(t *testing.T) {
	// Two packages: one with security+minor fix, one without violation
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		callCount++
		if callCount == 1 {
			// First package: security violation with minor fix
			fmt.Fprint(w, `{"data":{"id":"00000000-0000-0000-0000-000000000001","packageName":"lodash","packageType":"NPM","registryName":"test","scanStatus":"BLOCKED","version":"4.17.15","policySetFailureDetails":[{"policyFailureDetails":[{"category":"Security","policyName":"s","policyRef":"s"}],"policySetName":"ps","policySetRef":"psr"}],"fixVersionDetails":{"currentVersion":"4.17.15","fixVersion":"4.17.21","fixVersionAvailable":true}}}`)
		} else {
			// Second package: no violation
			fmt.Fprint(w, `{"data":{"id":"00000000-0000-0000-0000-000000000002","packageName":"axios","packageType":"NPM","registryName":"test","scanStatus":"WARN","version":"0.21.0"}}`)
		}
	}))
	defer srv.Close()

	client, err := ar_v3.NewClientWithResponses(srv.URL)
	require.NoError(t, err)

	f := &cmdutils.Factory{
		RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return client },
	}

	packages := []regcmd.ScanResult{
		{PackageName: "lodash", Version: "4.17.15", ScanID: "scan-1", ScanStatus: "BLOCKED"},
		{PackageName: "axios", Version: "0.21.0", ScanID: "scan-2", ScanStatus: "WARN"},
	}
	fixes, manualFix, err := evaluateFixVersions(f, packages)
	assert.NoError(t, err)
	assert.Len(t, fixes, 1)
	assert.Empty(t, manualFix)
	assert.Equal(t, "lodash", fixes[0].PackageName)
}

// ----- updatePackageJsonWithFixes edge case -----

func TestUpdatePackageJsonWithFixes_NonexistentFile(t *testing.T) {
	prog := progress.NewConsoleReporter()
	err := updatePackageJsonWithFixes("/nonexistent/path/package.json", []SecurityFixInfo{
		{PackageName: "lodash", FixVersion: "4.17.21"},
	}, prog)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read package.json")
}

func TestUpdatePackageJsonWithFixesEdgeCases(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("malformed json", func(t *testing.T) {
		filePath := filepath.Join(tmpDir, "malformed", "package.json")
		os.MkdirAll(filepath.Dir(filePath), 0755)
		os.WriteFile(filePath, []byte(`{invalid json}`), 0644)

		fixes := []SecurityFixInfo{
			{PackageName: "lodash", FixVersion: "4.17.21"},
		}

		prog := progress.NewConsoleReporter()
		err := updatePackageJsonWithFixes(filePath, fixes, prog)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse package.json")
	})

	t.Run("no matching dependencies", func(t *testing.T) {
		filePath := filepath.Join(tmpDir, "no-match", "package.json")
		os.MkdirAll(filepath.Dir(filePath), 0755)

		pkgData := map[string]interface{}{
			"name":    "test",
			"version": "1.0.0",
			"dependencies": map[string]interface{}{
				"express": "4.18.2",
			},
		}
		data, _ := json.MarshalIndent(pkgData, "", "  ")
		os.WriteFile(filePath, data, 0644)

		fixes := []SecurityFixInfo{
			{PackageName: "lodash", FixVersion: "4.17.21"}, // Not in dependencies
		}

		prog := progress.NewConsoleReporter()
		err := updatePackageJsonWithFixes(filePath, fixes, prog)
		assert.NoError(t, err) // Should succeed but update 0 packages
	})

	t.Run("update all dependency types", func(t *testing.T) {
		filePath := filepath.Join(tmpDir, "all-types", "package.json")
		os.MkdirAll(filepath.Dir(filePath), 0755)

		pkgData := map[string]interface{}{
			"name":    "test",
			"version": "1.0.0",
			"dependencies": map[string]interface{}{
				"lodash": "4.17.15",
			},
			"devDependencies": map[string]interface{}{
				"jest": "29.0.0",
			},
			"peerDependencies": map[string]interface{}{
				"react": "17.0.0",
			},
			"optionalDependencies": map[string]interface{}{
				"fsevents": "2.3.0",
			},
		}
		data, _ := json.MarshalIndent(pkgData, "", "  ")
		os.WriteFile(filePath, data, 0644)

		fixes := []SecurityFixInfo{
			{PackageName: "lodash", FixVersion: "4.17.21"},
			{PackageName: "jest", FixVersion: "29.5.0"},
			{PackageName: "react", FixVersion: "18.0.0"},
			{PackageName: "fsevents", FixVersion: "2.3.2"},
		}

		prog := progress.NewConsoleReporter()
		err := updatePackageJsonWithFixes(filePath, fixes, prog)
		assert.NoError(t, err)

		// Verify all sections were updated
		updatedData, _ := os.ReadFile(filePath)
		var updatedPkg map[string]interface{}
		json.Unmarshal(updatedData, &updatedPkg)

		deps := updatedPkg["dependencies"].(map[string]interface{})
		assert.Equal(t, "4.17.21", deps["lodash"])

		devDeps := updatedPkg["devDependencies"].(map[string]interface{})
		assert.Equal(t, "29.5.0", devDeps["jest"])

		peerDeps := updatedPkg["peerDependencies"].(map[string]interface{})
		assert.Equal(t, "18.0.0", peerDeps["react"])

		optDeps := updatedPkg["optionalDependencies"].(map[string]interface{})
		assert.Equal(t, "2.3.2", optDeps["fsevents"])
	})
}
