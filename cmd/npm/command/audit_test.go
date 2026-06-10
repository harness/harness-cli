package command

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	regcmd "github.com/harness/harness-cli/cmd/registry/command"
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

