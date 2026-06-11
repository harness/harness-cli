package command

import (
	"io"
	"testing"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar_v2"
	"github.com/harness/harness-cli/util/artifact"
)

func TestNewMetadataDeleteCmd_RequiredFlags(t *testing.T) {
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
			args:       []string{"--package", "nginx", "--metadata", "key:value"},
			wantErr:    true,
			errContain: "required flag",
		},
		{
			name:       "missing package flag",
			args:       []string{"--registry", "test-reg", "--metadata", "key:value"},
			wantErr:    true,
			errContain: "required flag",
		},
		{
			name:       "missing metadata flag",
			args:       []string{"--registry", "test-reg", "--package", "nginx"},
			wantErr:    true,
			errContain: "required flag",
		},
		{
			name:    "all required flags present",
			args:    []string{"--registry", "test-reg", "--package", "nginx", "--metadata", "key:value"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockClient(200, `{"status":"SUCCESS"}`)
			f := &cmdutils.Factory{
				RegistryV2HttpClient: func() *ar_v2.ClientWithResponses { return mockClient },
			}

			cmd := NewMetadataDeleteCmd(f)
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

func TestNewMetadataDeleteCmd_InvalidMetadataFormat(t *testing.T) {
	config.Global = config.GlobalFlags{
		AccountID: "test-account",
	}

	tests := []struct {
		name       string
		metadata   string
		wantErr    bool
		errContain string
	}{
		{
			name:       "invalid metadata format - no colon",
			metadata:   "invalid",
			wantErr:    true,
			errContain: "key:value format",
		},
		{
			name:       "invalid metadata format - empty key",
			metadata:   ":value",
			wantErr:    true,
			errContain: "key cannot be empty",
		},
		{
			name:       "invalid metadata format - empty value",
			metadata:   "key:",
			wantErr:    true,
			errContain: "value cannot be empty",
		},
		{
			name:       "valid single metadata",
			metadata:   "owner:team-a",
			wantErr:    false,
			errContain: "",
		},
		{
			name:       "valid multiple metadata",
			metadata:   "owner:team-a,approved:true",
			wantErr:    false,
			errContain: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockClient(200, `{"status":"SUCCESS"}`)
			f := &cmdutils.Factory{
				RegistryV2HttpClient: func() *ar_v2.ClientWithResponses { return mockClient },
			}

			cmd := NewMetadataDeleteCmd(f)
			args := []string{
				"--registry", "test-reg",
				"--package", "nginx",
				"--metadata", tt.metadata,
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

func TestNewMetadataDeleteCmd_WithVersion(t *testing.T) {
	config.Global = config.GlobalFlags{
		AccountID: "test-account",
	}

	mockClient := newMockClient(200, `{"status":"SUCCESS"}`)
	f := &cmdutils.Factory{
		RegistryV2HttpClient: func() *ar_v2.ClientWithResponses { return mockClient },
	}

	cmd := NewMetadataDeleteCmd(f)
	args := []string{
		"--registry", "test-reg",
		"--package", "nginx",
		"--version", "1.2.3",
		"--metadata", "approved:true",
	}
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	if err != nil {
		t.Errorf("Execute() unexpected error = %v", err)
	}
}

func TestNewMetadataDeleteCmd_WithArtifactKey(t *testing.T) {
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
			mockClient := newMockClient(200, `{"status":"SUCCESS"}`)
			f := &cmdutils.Factory{
				RegistryV2HttpClient: func() *ar_v2.ClientWithResponses { return mockClient },
			}

			cmd := NewMetadataDeleteCmd(f)
			args := []string{
				"--registry", "test-reg",
				"--package", "nginx",
				"--metadata", "owner:team-a",
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

func TestNewMetadataDeleteCmd_APIErrors(t *testing.T) {
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
			name:       "200 success",
			statusCode: 200,
			response:   `{"status":"SUCCESS"}`,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockClient(tt.statusCode, tt.response)
			f := &cmdutils.Factory{
				RegistryV2HttpClient: func() *ar_v2.ClientWithResponses { return mockClient },
			}

			cmd := NewMetadataDeleteCmd(f)
			args := []string{
				"--registry", "test-reg",
				"--package", "nginx",
				"--metadata", "owner:team-a",
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

func TestNewMetadataDeleteCmd_CombinedScenarios(t *testing.T) {
	config.Global = config.GlobalFlags{
		AccountID: "test-account",
	}

	tests := []struct {
		name        string
		registry    string
		pkg         string
		version     string
		metadata    string
		artifactKey string
		wantErr     bool
	}{
		{
			name:     "package-level metadata only",
			registry: "r1",
			pkg:      "nginx",
			metadata: "owner:team-a",
			wantErr:  false,
		},
		{
			name:     "version-level metadata",
			registry: "r1",
			pkg:      "nginx",
			version:  "1.2.3",
			metadata: "approved:true",
			wantErr:  false,
		},
		{
			name:        "with artifact key and version",
			registry:    "deb11",
			pkg:         "1oom",
			version:     "1.11.7-1",
			metadata:    "approved:true",
			artifactKey: "architecture=riscv64,distribution=bookworm3,component=test",
			wantErr:     false,
		},
		{
			name:     "multiple metadata values",
			registry: "r1",
			pkg:      "nginx",
			metadata: "owner:team-a,approved:true,env:prod",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockClient(200, `{"status":"SUCCESS"}`)
			f := &cmdutils.Factory{
				RegistryV2HttpClient: func() *ar_v2.ClientWithResponses { return mockClient },
			}

			cmd := NewMetadataDeleteCmd(f)
			args := []string{
				"--registry", tt.registry,
				"--package", tt.pkg,
				"--metadata", tt.metadata,
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

func TestBuildFiltersMapFromArtifactKey_DeleteMetadata(t *testing.T) {
	tests := []struct {
		name     string
		key      artifact.ArtifactKey
		expected map[string]string
	}{
		{
			name: "debian package artifact key",
			key: artifact.ArtifactKey{
				"architecture": "riscv64",
				"distribution": "bookworm3",
				"component":    "test",
			},
			expected: map[string]string{
				"architecture": "riscv64",
				"distribution": "bookworm3",
				"component":    "test",
			},
		},
		{
			name: "single field artifact key",
			key: artifact.ArtifactKey{
				"architecture": "amd64",
			},
			expected: map[string]string{
				"architecture": "amd64",
			},
		},
		{
			name:     "empty artifact key",
			key:      artifact.ArtifactKey{},
			expected: map[string]string{},
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
		})
	}
}
