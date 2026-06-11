package artifact

import "testing"

func TestExtractUpstreamVersion(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		expected string
	}{
		{
			name:     "Simple version with debian revision",
			version:  "1.2.3-4",
			expected: "1.2.3",
		},
		{
			name:     "Version with epoch and debian revision",
			version:  "2:1.5.0-1",
			expected: "1.5.0",
		},
		{
			name:     "Version with epoch, upstream, and debian revision",
			version:  "1:2.4.52-1ubuntu1",
			expected: "2.4.52",
		},
		{
			name:     "Version with only epoch",
			version:  "3:1.0.0",
			expected: "1.0.0",
		},
		{
			name:     "Version with multiple hyphens",
			version:  "1.2.3-rc1-4",
			expected: "1.2.3-rc1",
		},
		{
			name:     "Simple version without revision",
			version:  "1.0.0",
			expected: "1.0.0",
		},
		{
			name:     "Complex debian version",
			version:  "2:2.4.52-1ubuntu2.1",
			expected: "2.4.52",
		},
		{
			name:     "Version with build metadata",
			version:  "1.0.0+git20240101-1",
			expected: "1.0.0+git20240101",
		},
		{
			name:     "Empty string",
			version:  "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractUpstreamVersion(tt.version)
			if result != tt.expected {
				t.Errorf("ExtractUpstreamVersion(%q) = %q, want %q", tt.version, result, tt.expected)
			}
		})
	}
}
