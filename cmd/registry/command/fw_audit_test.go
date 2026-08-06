package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	ar_v3 "github.com/harness/harness-cli/internal/api/ar_v3"
	"github.com/harness/harness-cli/util/common/progress"
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

func TestExtractScanResults(t *testing.T) {
	tests := []struct {
		name     string
		response *ar_v3.GetBulkScanEvaluationStatusResp
		batchIdx int
		want     int
	}{
		{
			name: "valid scan results",
			response: &ar_v3.GetBulkScanEvaluationStatusResp{
				JSON200: &ar_v3.BulkScanEvaluationStatusResponse{
					Data: &ar_v3.BulkScanEvaluationStatusData{
						Scans: &[]ar_v3.BulkScanResultItem{
							{
								PackageName: stringPtr("express"),
								Version:     stringPtr("4.18.2"),
								ScanId:      uuidPtr(uuid.New()),
								ScanStatus:  scanStatusPtr(ar_v3.BulkScanResultItemScanStatusBLOCKED),
							},
							{
								PackageName: stringPtr("lodash"),
								Version:     stringPtr("4.17.21"),
								ScanId:      uuidPtr(uuid.New()),
								ScanStatus:  scanStatusPtr(ar_v3.BulkScanResultItemScanStatusALLOWED),
							},
						},
					},
				},
			},
			batchIdx: 0,
			want:     2,
		},
		{
			name: "nil scans",
			response: &ar_v3.GetBulkScanEvaluationStatusResp{
				JSON200: &ar_v3.BulkScanEvaluationStatusResponse{
					Data: &ar_v3.BulkScanEvaluationStatusData{
						Scans: nil,
					},
				},
			},
			batchIdx: 0,
			want:     0,
		},
		{
			name: "empty scans",
			response: &ar_v3.GetBulkScanEvaluationStatusResp{
				JSON200: &ar_v3.BulkScanEvaluationStatusResponse{
					Data: &ar_v3.BulkScanEvaluationStatusData{
						Scans: &[]ar_v3.BulkScanResultItem{},
					},
				},
			},
			batchIdx: 0,
			want:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := extractScanResults(tt.response, tt.batchIdx)
			assert.Equal(t, tt.want, len(results))
		})
	}
}

func TestDisplayResults(t *testing.T) {
	tests := []struct {
		name    string
		results []ScanResult
		wantErr bool
	}{
		{
			name: "valid results",
			results: []ScanResult{
				{
					PackageName: "express",
					Version:     "4.18.2",
					ScanID:      uuid.New().String(),
					ScanStatus:  "BLOCKED",
				},
				{
					PackageName: "lodash",
					Version:     "4.17.21",
					ScanID:      uuid.New().String(),
					ScanStatus:  "ALLOWED",
				},
			},
			wantErr: false,
		},
		{
			name:    "empty results",
			results: []ScanResult{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := progress.NewConsoleReporter()
			err := displayResults(tt.results, p)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBatchInfo(t *testing.T) {
	info := batchInfo{
		batchIdx:     0,
		totalBatches: 3,
		registryName: "test-registry",
	}

	assert.Equal(t, 0, info.batchIdx)
	assert.Equal(t, 3, info.totalBatches)
	assert.Equal(t, "test-registry", info.registryName)
}

func TestAuditContext(t *testing.T) {
	testUUID := uuid.New()
	p := progress.NewConsoleReporter()

	ctx := &auditContext{
		f:            nil,
		registryUUID: testUUID,
		org:          "test-org",
		project:      "test-project",
		p:            p,
	}

	assert.Equal(t, testUUID, ctx.registryUUID)
	assert.Equal(t, "test-org", ctx.org)
	assert.Equal(t, "test-project", ctx.project)
	assert.NotNil(t, ctx.p)
}

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
		{
			name: "yarn berry (v6 __metadata) lock file",
			data: `# This file is generated by running "yarn install" inside your project.
# Manual changes might be lost - proceed with caution!

__metadata:
  version: 6
  cacheKey: 8

"@babel/core@npm:^7.0.0, @babel/core@npm:^7.12.0":
  version: 7.20.0
  resolution: "@babel/core@npm:7.20.0"
  dependencies:
    "@babel/code-frame": "npm:^7.0.0"
  languageName: node
  linkType: hard

"express@npm:^4.18.2":
  version: 4.18.2
  resolution: "express@npm:4.18.2"
  languageName: node
  linkType: hard`,
			want:    2,
			wantErr: false,
		},
		{
			name: "yarn berry nested @-key is not counted as a package",
			data: `__metadata:
  version: 6

"react@npm:^18.0.0":
  version: 18.2.0
  resolution: "react@npm:18.2.0"
  peerDependenciesMeta:
    "@babel/core":
      optional: true
  languageName: node
  linkType: hard`,
			want:    1,
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

// TestParseYarnLockContents verifies the parsed name/version pairs, covering
// both Yarn Classic (version "x") and Yarn Berry (version: x, resolution field).
func TestParseYarnLockContents(t *testing.T) {
	tests := []struct {
		name string
		data string
		want map[string]string // name -> version
	}{
		{
			name: "classic scoped and unscoped",
			data: `"@babel/core@^7.0.0", "@babel/core@^7.12.0":
  version "7.20.0"
  resolved "https://registry.yarnpkg.com/@babel/core/-/core-7.20.0.tgz"

"express@^4.18.2":
  version "4.18.2"
  resolved "https://registry.yarnpkg.com/express/-/express-4.18.2.tgz"`,
			want: map[string]string{"@babel/core": "7.20.0", "express": "4.18.2"},
		},
		{
			name: "berry prefers resolution for name",
			data: `__metadata:
  version: 6

"@babel/core@npm:^7.0.0, @babel/core@npm:^7.12.0":
  version: 7.20.0
  resolution: "@babel/core@npm:7.20.0"
  languageName: node
  linkType: hard

"express@npm:^4.18.2":
  version: 4.18.2
  resolution: "express@npm:4.18.2"`,
			want: map[string]string{"@babel/core": "7.20.0", "express": "4.18.2"},
		},
		{
			// Non-npm protocols must be skipped: the workspace root resolves to
			// 0.0.0-use.local (the project itself, not a dependency) and a patch
			// entry only wraps a base npm package that is already listed. Both
			// would otherwise pad the audit with unactionable "Unknown" rows.
			name: "berry skips workspace and patch, keeps base npm package",
			data: `__metadata:
  version: 6

"my-project@workspace:.":
  version: 0.0.0-use.local
  resolution: "my-project@workspace:."
  languageName: unknown
  linkType: soft

"typescript@npm:5.0.4":
  version: 5.0.4
  resolution: "typescript@npm:5.0.4"
  languageName: node
  linkType: hard

"typescript@patch:typescript@npm%3A5.0.4#optional!builtin<compat/typescript>":
  version: 5.0.4
  resolution: "typescript@patch:typescript@npm%3A5.0.4#optional!builtin<compat/typescript>::version=5.0.4&hash=1234"
  languageName: node
  linkType: hard`,
			want: map[string]string{"typescript": "5.0.4"},
		},
		{
			// npm alias: the name is everything up to the FIRST "@" after index 0,
			// so "foo@npm:bar@^1.0.0" resolves to "foo", not "foo@npm:bar".
			name: "berry npm alias resolves to alias name",
			data: `__metadata:
  version: 6

"my-lodash@npm:lodash@^4.17.21":
  version: 4.17.21
  resolution: "lodash@npm:4.17.21"`,
			want: map[string]string{"lodash": "4.17.21"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, err := parseYarnLock([]byte(tt.data))
			assert.NoError(t, err)
			got := make(map[string]string, len(deps))
			for _, d := range deps {
				got[d.Name] = d.Version
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestYarnSelectorNameAndProtocol pins the selector-parsing helpers that decide
// a package's name and whether its entry is an auditable npm package. The name
// is taken up to the first "@" after index 0 (so npm aliases and Berry
// protocols don't leak into the name), and the protocol is the token between
// that "@" and the next ":" (empty for Yarn Classic version ranges).
func TestYarnSelectorNameAndProtocol(t *testing.T) {
	tests := []struct {
		in        string
		wantName  string
		wantProto string
	}{
		{`@babel/core@npm:7.20.0`, "@babel/core", "npm"},
		{`express@npm:4.18.2`, "express", "npm"},
		{`foo@npm:bar@^1`, "foo", "npm"},
		{`statement-reconciliation@workspace:.`, "statement-reconciliation", "workspace"},
		{`fsevents@patch:fsevents@npm%3A2.3.3#~builtin<compat/fsevents>`, "fsevents", "patch"},
		{`@scope/pkg@link:../local`, "@scope/pkg", "link"},
		{`pkg@portal:../local`, "pkg", "portal"},
		{`pkg@file:./vendor/pkg.tgz`, "pkg", "file"},
		// Yarn Classic ranges carry no protocol.
		{`"@babel/core@^7.0.0", "@babel/core@^7.12.0"`, "@babel/core", ""},
		{`express@^4.18.2`, "express", ""},
		{`lodash@~4.17.21`, "lodash", ""},
		{``, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.wantName, yarnSelectorName(tt.in), "name")
			assert.Equal(t, tt.wantProto, yarnSelectorProtocol(tt.in), "protocol")
		})
	}
}

// TestParseLockFileYarnBerryDispatch guards the end-to-end path the audit
// command uses: a file named "yarn.lock" carrying Yarn Berry (v2+) content must
// route through ParseLockFile and yield a non-empty dependency set. A zero
// result here is exactly the silent false-negative the fix addresses, since the
// command treats zero parsed dependencies as an error rather than a pass.
func TestParseLockFileYarnBerryDispatch(t *testing.T) {
	berry := `# This file is generated by running "yarn install" inside your project.
__metadata:
  version: 6
  cacheKey: 8

"@babel/core@npm:^7.0.0":
  version: 7.20.0
  resolution: "@babel/core@npm:7.20.0"
  languageName: node
  linkType: hard

"express@npm:^4.18.2":
  version: 4.18.2
  resolution: "express@npm:4.18.2"
  languageName: node
  linkType: hard`

	dir := t.TempDir()
	path := filepath.Join(dir, "yarn.lock")
	if err := os.WriteFile(path, []byte(berry), 0o644); err != nil {
		t.Fatalf("write temp yarn.lock: %v", err)
	}

	deps, err := ParseLockFile(path)
	assert.NoError(t, err)
	assert.NotEmpty(t, deps, "Yarn Berry lock file must not parse to zero dependencies")

	got := make(map[string]string, len(deps))
	for _, d := range deps {
		got[d.Name] = d.Version
	}
	assert.Equal(t, map[string]string{"@babel/core": "7.20.0", "express": "4.18.2"}, got)
}

func stringPtr(s string) *string {
	return &s
}

func uuidPtr(u uuid.UUID) *uuid.UUID {
	return &u
}

func scanStatusPtr(s ar_v3.BulkScanResultItemScanStatus) *ar_v3.BulkScanResultItemScanStatus {
	return &s
}
