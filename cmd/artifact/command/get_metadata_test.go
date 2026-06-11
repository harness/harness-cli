package command

import (
	"io"
	"testing"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar_v2"
	"github.com/harness/harness-cli/util/artifact"
)

func TestNewMetadataGetCmd_RequiredFlags(t *testing.T) {
	config.Global = config.GlobalFlags{
		AccountID: "test-account",
	}

	tests := []struct {
		name       string
		args       []string
		wantErr    bool
		errContain string
	}{
		{
			name:       "missing registry flag",
			args:       []string{"--package", "nginx"},
			wantErr:    true,
			errContain: "required flag",
		},
		{
			name:       "missing package flag",
			args:       []string{"--registry", "test-reg"},
			wantErr:    true,
			errContain: "required flag",
		},
		{
			name:    "all required flags present",
			args:    []string{"--registry", "test-reg", "--package", "nginx"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockClient(200, `{"status":"SUCCESS","data":{"metadata":[]}}`)
			f := &cmdutils.Factory{
				RegistryV2HttpClient: func() *ar_v2.ClientWithResponses { return mockClient },
			}

			cmd := NewMetadataGetCmd(f)
			cmd.SetArgs(tt.args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			err := cmd.Execute()
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && tt.errContain != "" {
				if !contains(err.Error(), tt.errContain) {
					t.Errorf("Execute() error = %v, want error containing %v", err, tt.errContain)
				}
			}
		})
	}
}

func TestNewMetadataGetCmd_WithVersion(t *testing.T) {
	config.Global = config.GlobalFlags{
		AccountID: "test-account",
	}

	mockClient := newMockClient(200, `{"status":"SUCCESS","data":{"metadata":[]}}`)
	f := &cmdutils.Factory{
		RegistryV2HttpClient: func() *ar_v2.ClientWithResponses { return mockClient },
	}

	cmd := NewMetadataGetCmd(f)
	args := []string{
		"--registry", "test-reg",
		"--package", "nginx",
		"--version", "1.2.3",
	}
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	if err != nil {
		t.Errorf("Execute() unexpected error = %v", err)
	}
}

func TestNewMetadataGetCmd_WithArtifactKey(t *testing.T) {
	config.Global = config.GlobalFlags{
		AccountID: "test-account",
	}

	tests := []struct {
		name        string
		artifactKey string
		wantErr     bool
		errContain  string
	}{
		{
			name:        "valid artifact key",
			artifactKey: "architecture=amd64,distribution=focal,component=main",
			wantErr:     false,
		},
		{
			name:        "valid artifact key - single field",
			artifactKey: "architecture=arm64",
			wantErr:     false,
		},
		{
			name:        "invalid artifact key - no equals",
			artifactKey: "architecture:amd64",
			wantErr:     true,
			errContain:  "invalid artifact key format",
		},
		{
			name:        "invalid artifact key - trailing comma",
			artifactKey: "architecture=amd64,",
			wantErr:     true,
			errContain:  "invalid artifact key format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockClient(200, `{"status":"SUCCESS","data":{"metadata":[]}}`)
			f := &cmdutils.Factory{
				RegistryV2HttpClient: func() *ar_v2.ClientWithResponses { return mockClient },
			}

			cmd := NewMetadataGetCmd(f)
			args := []string{
				"--registry", "test-reg",
				"--package", "nginx",
				"--artifact-key", tt.artifactKey,
			}
			cmd.SetArgs(args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			err := cmd.Execute()
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && tt.errContain != "" {
				if !contains(err.Error(), tt.errContain) {
					t.Errorf("Execute() error = %v, want error containing %v", err, tt.errContain)
				}
			}
		})
	}
}

func TestNewMetadataGetCmd_APIErrors(t *testing.T) {
	config.Global = config.GlobalFlags{
		AccountID: "test-account",
	}

	tests := []struct {
		name       string
		statusCode int
		response   string
		wantErr    bool
		errContain string
	}{
		{
			name:       "400 bad request",
			statusCode: 400,
			response:   `{"message":"invalid request"}`,
			wantErr:    true,
			errContain: "request failed with status 400",
		},
		{
			name:       "404 not found",
			statusCode: 404,
			response:   `{"message":"package not found"}`,
			wantErr:    true,
			errContain: "not found",
		},
		{
			name:       "500 internal server error",
			statusCode: 500,
			response:   `internal server error`,
			wantErr:    true,
			errContain: "request failed",
		},
		{
			name:       "200 success with metadata",
			statusCode: 200,
			response:   `{"status":"SUCCESS","data":{"metadata":[{"key":"env","value":"prod"}]}}`,
			wantErr:    false,
		},
		{
			name:       "200 success without metadata",
			statusCode: 200,
			response:   `{"status":"SUCCESS","data":{"metadata":[]}}`,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockClient(tt.statusCode, tt.response)
			f := &cmdutils.Factory{
				RegistryV2HttpClient: func() *ar_v2.ClientWithResponses { return mockClient },
			}

			cmd := NewMetadataGetCmd(f)
			args := []string{
				"--registry", "test-reg",
				"--package", "nginx",
			}
			cmd.SetArgs(args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			err := cmd.Execute()
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && tt.errContain != "" {
				if !contains(err.Error(), tt.errContain) {
					t.Errorf("Execute() error = %v, want error containing %v", err, tt.errContain)
				}
			}
		})
	}
}

func TestNewMetadataGetCmd_CombinedScenarios(t *testing.T) {
	config.Global = config.GlobalFlags{
		AccountID: "test-account",
	}

	tests := []struct {
		name        string
		registry    string
		pkg         string
		version     string
		artifactKey string
		wantErr     bool
	}{
		{
			name:     "package-level metadata only",
			registry: "r1",
			pkg:      "nginx",
			wantErr:  false,
		},
		{
			name:     "version-level metadata",
			registry: "r1",
			pkg:      "nginx",
			version:  "1.2.3",
			wantErr:  false,
		},
		{
			name:        "with artifact key and version",
			registry:    "deb11",
			pkg:         "1oom",
			version:     "1.11.7-1",
			artifactKey: "architecture=riscv64,distribution=bookworm3,component=test",
			wantErr:     false,
		},
		{
			name:        "with artifact key only",
			registry:    "deb11",
			pkg:         "1oom",
			artifactKey: "architecture=amd64",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockClient(200, `{"status":"SUCCESS","data":{"metadata":[]}}`)
			f := &cmdutils.Factory{
				RegistryV2HttpClient: func() *ar_v2.ClientWithResponses { return mockClient },
			}

			cmd := NewMetadataGetCmd(f)
			args := []string{
				"--registry", tt.registry,
				"--package", tt.pkg,
			}
			if tt.version != "" {
				args = append(args, "--version", tt.version)
			}
			if tt.artifactKey != "" {
				args = append(args, "--artifact-key", tt.artifactKey)
			}
			cmd.SetArgs(args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			err := cmd.Execute()
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBuildFiltersFromArtifactKey(t *testing.T) {
	tests := []struct {
		name         string
		key          artifact.ArtifactKey
		expectedKeys map[string]bool // Use map to handle order-independent comparison
	}{
		{
			name: "Full artifact key",
			key: artifact.ArtifactKey{
				"architecture": "amd64",
				"distribution": "focal",
				"component":    "main",
				"artifactType": "binary",
			},
			expectedKeys: map[string]bool{
				"architecture:amd64":  false,
				"distribution:focal":  false,
				"component:main":      false,
				"artifactType:binary": false,
			},
		},
		{
			name: "Partial artifact key - single field",
			key: artifact.ArtifactKey{
				"architecture": "arm64",
			},
			expectedKeys: map[string]bool{
				"architecture:arm64": false,
			},
		},
		{
			name: "Partial artifact key - multiple fields",
			key: artifact.ArtifactKey{
				"distribution": "bookworm",
				"component":    "contrib",
			},
			expectedKeys: map[string]bool{
				"distribution:bookworm": false,
				"component:contrib":     false,
			},
		},
		{
			name:         "Empty artifact key",
			key:          artifact.ArtifactKey{},
			expectedKeys: map[string]bool{},
		},
		{
			name: "Generic custom keys",
			key: artifact.ArtifactKey{
				"platform": "linux",
				"region":   "us-east-1",
			},
			expectedKeys: map[string]bool{
				"platform:linux":   false,
				"region:us-east-1": false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildFiltersFromArtifactKey(tt.key)

			if len(result) != len(tt.expectedKeys) {
				t.Errorf("expected %d filters, got %d", len(tt.expectedKeys), len(result))
			}

			// Mark found keys
			for _, filter := range result {
				if _, exists := tt.expectedKeys[filter]; exists {
					tt.expectedKeys[filter] = true
				} else {
					t.Errorf("unexpected filter: %s", filter)
				}
			}

			// Check all expected keys were found
			for key, found := range tt.expectedKeys {
				if !found {
					t.Errorf("expected filter %s not found", key)
				}
			}
		})
	}
}

func TestBuildFiltersMapFromArtifactKey(t *testing.T) {
	tests := []struct {
		name     string
		key      artifact.ArtifactKey
		expected map[string]string
	}{
		{
			name: "Full artifact key",
			key: artifact.ArtifactKey{
				"architecture": "amd64",
				"distribution": "focal",
				"component":    "main",
				"artifactType": "binary",
			},
			expected: map[string]string{
				"architecture": "amd64",
				"distribution": "focal",
				"component":    "main",
				"artifactType": "binary",
			},
		},
		{
			name: "Partial artifact key - single field",
			key: artifact.ArtifactKey{
				"architecture": "arm64",
			},
			expected: map[string]string{
				"architecture": "arm64",
			},
		},
		{
			name: "Partial artifact key - multiple fields",
			key: artifact.ArtifactKey{
				"distribution": "bookworm",
				"component":    "contrib",
			},
			expected: map[string]string{
				"distribution": "bookworm",
				"component":    "contrib",
			},
		},
		{
			name:     "Empty artifact key",
			key:      artifact.ArtifactKey{},
			expected: map[string]string{},
		},
		{
			name: "Generic custom keys",
			key: artifact.ArtifactKey{
				"platform": "linux",
				"region":   "us-east-1",
			},
			expected: map[string]string{
				"platform": "linux",
				"region":   "us-east-1",
			},
		},
		{
			name: "Keys with special characters",
			key: artifact.ArtifactKey{
				"artifact-type": "binary",
				"build_number":  "123",
			},
			expected: map[string]string{
				"artifact-type": "binary",
				"build_number":  "123",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildFiltersMapFromArtifactKey(tt.key)

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d entries, got %d", len(tt.expected), len(result))
			}

			for key, expectedValue := range tt.expected {
				if actualValue, ok := result[key]; !ok {
					t.Errorf("expected key %q not found in result", key)
				} else if actualValue != expectedValue {
					t.Errorf("key %q: expected %q, got %q", key, expectedValue, actualValue)
				}
			}

			// Check for unexpected keys
			for key := range result {
				if _, ok := tt.expected[key]; !ok {
					t.Errorf("unexpected key %q in result", key)
				}
			}
		})
	}
}
