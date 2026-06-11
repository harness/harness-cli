package artifact

import (
	"strings"
	"testing"
)

func TestParseArtifactKeyString(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    ArtifactKey
		expectError bool
	}{
		{
			name:  "Debian artifact key",
			input: "architecture=amd64,distribution=focal,component=main,artifactType=binary",
			expected: ArtifactKey{
				"architecture": "amd64",
				"distribution": "focal",
				"component":    "main",
				"artifactType": "binary",
			},
			expectError: false,
		},
		{
			name:  "Partial key",
			input: "architecture=arm64,distribution=bookworm",
			expected: ArtifactKey{
				"architecture": "arm64",
				"distribution": "bookworm",
			},
			expectError: false,
		},
		{
			name:  "Generic custom keys",
			input: "platform=linux,region=us-east-1,tier=premium",
			expected: ArtifactKey{
				"platform": "linux",
				"region":   "us-east-1",
				"tier":     "premium",
			},
			expectError: false,
		},
		{
			name:  "Single key",
			input: "environment=production",
			expected: ArtifactKey{
				"environment": "production",
			},
			expectError: false,
		},
		{
			name:  "With spaces",
			input: "architecture = amd64 , distribution = focal",
			expected: ArtifactKey{
				"architecture": "amd64",
				"distribution": "focal",
			},
			expectError: false,
		},
		{
			name:  "Keys with underscores and numbers",
			input: "key_1=value1,key_2=value2",
			expected: ArtifactKey{
				"key_1": "value1",
				"key_2": "value2",
			},
			expectError: false,
		},
		{
			name:        "Empty string",
			input:       "",
			expected:    nil,
			expectError: false,
		},
		{
			name:        "Invalid format - missing value",
			input:       "architecture=amd64,distribution",
			expected:    nil,
			expectError: true,
		},
		{
			name:        "Invalid format - empty value",
			input:       "architecture=,distribution=focal",
			expected:    nil,
			expectError: true,
		},
		{
			name:        "Invalid format - empty key",
			input:       "=value,distribution=focal",
			expected:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseArtifactKeyString(tt.input)

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

			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil result but got %+v", result)
				}
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d keys, got %d", len(tt.expected), len(result))
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

func TestArtifactKey_String(t *testing.T) {
	tests := []struct {
		name     string
		key      ArtifactKey
		contains []string
	}{
		{
			name: "Multiple keys",
			key: ArtifactKey{
				"architecture": "amd64",
				"distribution": "focal",
			},
			contains: []string{"architecture=amd64", "distribution=focal"},
		},
		{
			name: "Single key",
			key: ArtifactKey{
				"platform": "linux",
			},
			contains: []string{"platform=linux"},
		},
		{
			name:     "Empty key",
			key:      ArtifactKey{},
			contains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.key.String()

			if len(tt.contains) == 0 && result != "" {
				t.Errorf("expected empty string, got %q", result)
				return
			}

			for _, substr := range tt.contains {
				if !strings.Contains(result, substr) {
					t.Errorf("expected string to contain %q, got %q", substr, result)
				}
			}
		})
	}
}

func TestArtifactKey_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		key      ArtifactKey
		expected bool
	}{
		{
			name:     "Empty key",
			key:      ArtifactKey{},
			expected: true,
		},
		{
			name:     "Nil key",
			key:      nil,
			expected: true,
		},
		{
			name: "Non-empty key",
			key: ArtifactKey{
				"architecture": "amd64",
			},
			expected: false,
		},
		{
			name: "Multiple keys",
			key: ArtifactKey{
				"key1": "value1",
				"key2": "value2",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.key.IsEmpty()
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
