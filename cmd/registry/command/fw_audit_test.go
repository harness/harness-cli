package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateFileForPackageType(t *testing.T) {
	tests := []struct {
		name        string
		fileName    string
		packageType string
		wantErr     bool
	}{
		{
			name:        "valid npm package.json",
			fileName:    "package.json",
			packageType: "NPM",
			wantErr:     false,
		},
		{
			name:        "valid npm package-lock.json",
			fileName:    "package-lock.json",
			packageType: "NPM",
			wantErr:     false,
		},
		{
			name:        "valid npm yarn.lock",
			fileName:    "yarn.lock",
			packageType: "NPM",
			wantErr:     false,
		},
		{
			name:        "valid npm pnpm-lock.yaml",
			fileName:    "pnpm-lock.yaml",
			packageType: "NPM",
			wantErr:     false,
		},
		{
			name:        "valid python requirements.txt",
			fileName:    "requirements.txt",
			packageType: "PYTHON",
			wantErr:     false,
		},
		{
			name:        "valid python pyproject.toml",
			fileName:    "pyproject.toml",
			packageType: "PYTHON",
			wantErr:     false,
		},
		{
			name:        "valid maven pom.xml",
			fileName:    "pom.xml",
			packageType: "MAVEN",
			wantErr:     false,
		},
		{
			name:        "invalid npm file for python",
			fileName:    "package.json",
			packageType: "PYTHON",
			wantErr:     true,
		},
		{
			name:        "invalid python file for npm",
			fileName:    "requirements.txt",
			packageType: "NPM",
			wantErr:     true,
		},
		{
			name:        "unsupported package type",
			fileName:    "package.json",
			packageType: "UNSUPPORTED",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFileForPackageType(tt.fileName, tt.packageType)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestParsePackageJson(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    int
		wantErr bool
	}{
		{
			name: "valid package.json with dependencies",
			data: `{
				"dependencies": {
					"express": "^4.18.2",
					"lodash": "^4.17.21"
				},
				"devDependencies": {
					"jest": "^29.5.0"
				}
			}`,
			want:    3,
			wantErr: false,
		},
		{
			name: "package.json with version prefixes",
			data: `{
				"dependencies": {
					"express": "^4.18.2",
					"lodash": "~4.17.21",
					"axios": ">=1.0.0"
				}
			}`,
			want:    3,
			wantErr: false,
		},
		{
			name:    "invalid json",
			data:    `{invalid json}`,
			want:    0,
			wantErr: true,
		},
		{
			name: "empty dependencies",
			data: `{
				"dependencies": {}
			}`,
			want:    0,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, err := parsePackageJson([]byte(tt.data))
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, len(deps))
			}
		})
	}
}

func TestParseRequirementsTxt(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    int
		wantErr bool
	}{
		{
			name: "valid requirements.txt",
			data: `requests==2.28.0
flask>=2.0.0
django~=4.0.0
# comment line
pytest`,
			want:    4,
			wantErr: false,
		},
		{
			name: "requirements with extras",
			data: `requests[security]==2.28.0
flask[async]>=2.0.0`,
			want:    2,
			wantErr: false,
		},
		{
			name:    "empty file",
			data:    ``,
			want:    0,
			wantErr: false,
		},
		{
			name: "only comments",
			data: `# comment 1
# comment 2`,
			want:    0,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, err := parseRequirementsTxt([]byte(tt.data))
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, len(deps))
			}
		})
	}
}

// Tests for extractScanResults, DisplayResults, BatchInfo, and AuditContext
// have been moved to fw_util_test.go since these functions are now in fw_util.go

func TestParsePomXml(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    int
		wantErr bool
	}{
		{
			name: "valid pom.xml",
			data: `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
	<dependencies>
		<dependency>
			<groupId>org.springframework</groupId>
			<artifactId>spring-core</artifactId>
			<version>5.3.0</version>
		</dependency>
		<dependency>
			<groupId>junit</groupId>
			<artifactId>junit</artifactId>
			<version>4.13.2</version>
		</dependency>
	</dependencies>
</project>`,
			want:    2,
			wantErr: false,
		},
		{
			name:    "invalid xml",
			data:    `<invalid xml`,
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, err := parsePomXml([]byte(tt.data))
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, len(deps))
			}
		})
	}
}

func TestParseYarnLock(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    int
		wantErr bool
	}{
		{
			name: "valid yarn.lock",
			data: `# THIS IS AN AUTOGENERATED FILE. DO NOT EDIT THIS FILE DIRECTLY.

"express@^4.18.2":
  version "4.18.2"
  resolved "https://registry.yarnpkg.com/express/-/express-4.18.2.tgz"

"lodash@^4.17.21":
  version "4.17.21"
  resolved "https://registry.yarnpkg.com/lodash/-/lodash-4.17.21.tgz"`,
			want:    2,
			wantErr: false,
		},
		{
			name:    "empty file",
			data:    ``,
			want:    0,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, err := parseYarnLock([]byte(tt.data))
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, len(deps))
			}
		})
	}
}

// Helper functions have been moved to fw_util_test.go
