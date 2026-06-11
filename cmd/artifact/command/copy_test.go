package command

import (
	"testing"

	v2client "github.com/harness/harness-cli/internal/api/ar_v2"
)

func TestParsePackagePath(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedParts []string
		expectError   bool
	}{
		{
			name:          "Valid package path",
			input:         "my-registry/my-package/1.0.0",
			expectedParts: []string{"my-registry", "my-package", "1.0.0"},
			expectError:   false,
		},
		{
			name:          "Package path with nested artifact",
			input:         "registry/org/package/2.3.4",
			expectedParts: []string{"registry", "org/package", "2.3.4"},
			expectError:   false,
		},
		{
			name:          "Package path with version containing special chars",
			input:         "reg/pkg/1.0.0-beta.1",
			expectedParts: []string{"reg", "pkg", "1.0.0-beta.1"},
			expectError:   false,
		},
		{
			name:          "Empty input",
			input:         "",
			expectedParts: nil,
			expectError:   true,
		},
		{
			name:          "Whitespace only",
			input:         "   ",
			expectedParts: nil,
			expectError:   true,
		},
		{
			name:          "Missing slashes",
			input:         "registry-package-version",
			expectedParts: nil,
			expectError:   true,
		},
		{
			name:          "Only one slash",
			input:         "registry/package",
			expectedParts: nil,
			expectError:   true,
		},
		{
			name:          "Empty registry",
			input:         "/package/1.0.0",
			expectedParts: nil,
			expectError:   true,
		},
		{
			name:          "Empty artifact",
			input:         "registry//1.0.0",
			expectedParts: nil,
			expectError:   true,
		},
		{
			name:          "Empty version",
			input:         "registry/package/",
			expectedParts: nil,
			expectError:   true,
		},
		{
			name:          "Complex nested path",
			input:         "my-reg/@scope/package/v1.2.3",
			expectedParts: []string{"my-reg", "@scope/package", "v1.2.3"},
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts, err := parsePackagePath(tt.input)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(parts) != len(tt.expectedParts) {
				t.Errorf("expected %d parts, got %d", len(tt.expectedParts), len(parts))
				return
			}

			for i, expected := range tt.expectedParts {
				if parts[i] != expected {
					t.Errorf("part[%d]: expected %q, got %q", i, expected, parts[i])
				}
			}
		})
	}
}

func TestValidateCopyRegistryPackageParams(t *testing.T) {
	tests := []struct {
		name        string
		params      *v2client.CopyRegistryPackageParams
		expectError bool
	}{
		{
			name: "Valid params",
			params: &v2client.CopyRegistryPackageParams{
				AccountIdentifier:        "account123",
				SrcRegistryIdentifier:    "src-reg",
				TargetRegistryIdentifier: "target-reg",
				SrcArtifact:              "my-package",
				SrcVersion:               "1.0.0",
			},
			expectError: false,
		},
		{
			name:        "Nil params",
			params:      nil,
			expectError: true,
		},
		{
			name: "Missing account identifier",
			params: &v2client.CopyRegistryPackageParams{
				AccountIdentifier:        "",
				SrcRegistryIdentifier:    "src-reg",
				TargetRegistryIdentifier: "target-reg",
				SrcArtifact:              "my-package",
				SrcVersion:               "1.0.0",
			},
			expectError: true,
		},
		{
			name: "Missing source registry",
			params: &v2client.CopyRegistryPackageParams{
				AccountIdentifier:        "account123",
				SrcRegistryIdentifier:    "",
				TargetRegistryIdentifier: "target-reg",
				SrcArtifact:              "my-package",
				SrcVersion:               "1.0.0",
			},
			expectError: true,
		},
		{
			name: "Missing target registry",
			params: &v2client.CopyRegistryPackageParams{
				AccountIdentifier:        "account123",
				SrcRegistryIdentifier:    "src-reg",
				TargetRegistryIdentifier: "",
				SrcArtifact:              "my-package",
				SrcVersion:               "1.0.0",
			},
			expectError: true,
		},
		{
			name: "Missing artifact",
			params: &v2client.CopyRegistryPackageParams{
				AccountIdentifier:        "account123",
				SrcRegistryIdentifier:    "src-reg",
				TargetRegistryIdentifier: "target-reg",
				SrcArtifact:              "",
				SrcVersion:               "1.0.0",
			},
			expectError: true,
		},
		{
			name: "Missing version",
			params: &v2client.CopyRegistryPackageParams{
				AccountIdentifier:        "account123",
				SrcRegistryIdentifier:    "src-reg",
				TargetRegistryIdentifier: "target-reg",
				SrcArtifact:              "my-package",
				SrcVersion:               "",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCopyRegistryPackageParams(tt.params)

			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
