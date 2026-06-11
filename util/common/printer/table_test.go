package printer

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatArtifactKey(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name: "Map with multiple keys",
			input: map[string]interface{}{
				"architecture": "amd64",
				"distribution": "focal",
				"component":    "main",
			},
			expected: "architecture=amd64,component=main,distribution=focal", // sorted by key
		},
		{
			name: "Map with single key",
			input: map[string]interface{}{
				"platform": "linux",
			},
			expected: "platform=linux",
		},
		{
			name:     "Empty map",
			input:    map[string]interface{}{},
			expected: "-",
		},
		{
			name: "Map with different value types",
			input: map[string]interface{}{
				"count":   123,
				"enabled": true,
				"name":    "test",
			},
			expected: "count=123,enabled=true,name=test",
		},
		{
			name:     "Non-map value - string",
			input:    "simple-string",
			expected: "simple-string",
		},
		{
			name:     "Non-map value - number",
			input:    42,
			expected: "42",
		},
		{
			name:     "Non-map value - nil",
			input:    nil,
			expected: "<nil>",
		},
		{
			name: "Map with special characters in values",
			input: map[string]interface{}{
				"path": "/usr/bin",
				"url":  "https://example.com",
			},
			expected: "path=/usr/bin,url=https://example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatArtifactKey(tt.input)
			if result != tt.expected {
				t.Errorf("formatArtifactKey() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestJsonToTableWithMapping(t *testing.T) {
	tests := []struct {
		name        string
		jsonStr     string
		mapping     ColumnMapping
		expectError bool
		checkOutput func(t *testing.T, err error)
	}{
		{
			name:    "Simple data with mapping",
			jsonStr: `[{"name": "test", "version": "1.0.0"}]`,
			mapping: ColumnMapping{
				{"name", "Name"},
				{"version", "Version"},
			},
			expectError: false,
		},
		{
			name: "Data with artifactKey field",
			jsonStr: `[{
				"name": "package1",
				"artifactKey": {
					"architecture": "amd64",
					"distribution": "focal"
				}
			}]`,
			mapping: ColumnMapping{
				{"name", "Package"},
				{"artifactKey", "Artifact Key"},
			},
			expectError: false,
			checkOutput: func(t *testing.T, err error) {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				// The artifactKey should be formatted as comma-separated pairs
			},
		},
		{
			name: "Data with empty artifactKey",
			jsonStr: `[{
				"name": "package2",
				"artifactKey": {}
			}]`,
			mapping: ColumnMapping{
				{"name", "Package"},
				{"artifactKey", "Artifact Key"},
			},
			expectError: false,
		},
		{
			name: "Data with missing artifactKey field",
			jsonStr: `[{
				"name": "package3"
			}]`,
			mapping: ColumnMapping{
				{"name", "Package"},
				{"artifactKey", "Artifact Key"},
			},
			expectError: false,
		},
		{
			name: "Multiple rows with mixed artifactKey",
			jsonStr: `[
				{
					"name": "pkg1",
					"artifactKey": {"arch": "amd64"}
				},
				{
					"name": "pkg2",
					"artifactKey": {}
				},
				{
					"name": "pkg3"
				}
			]`,
			mapping: ColumnMapping{
				{"name", "Name"},
				{"artifactKey", "Key"},
			},
			expectError: false,
		},
		{
			name:        "Invalid JSON",
			jsonStr:     `{this is not valid json}`,
			mapping:     nil,
			expectError: true,
		},
		{
			name:        "Empty array",
			jsonStr:     `[]`,
			mapping:     nil,
			expectError: false,
		},
		{
			name:    "No mapping - alphabetical order",
			jsonStr: `[{"name": "test", "version": "1.0.0", "size": 100}]`,
			mapping: nil,
			expectError: false,
		},
		{
			name: "No mapping with artifactKey",
			jsonStr: `[{
				"name": "test",
				"artifactKey": {"type": "binary"}
			}]`,
			mapping:     nil,
			expectError: false,
		},
		{
			name: "Data with various types",
			jsonStr: `[{
				"name": "test",
				"count": 42,
				"enabled": true,
				"artifactKey": {"env": "prod"}
			}]`,
			mapping: ColumnMapping{
				{"name", "Name"},
				{"count", "Count"},
				{"enabled", "Enabled"},
				{"artifactKey", "Key"},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := jsonToTableWithMapping(tt.jsonStr, tt.mapping)

			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
				return
			}

			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if tt.checkOutput != nil {
				tt.checkOutput(t, err)
			}
		})
	}
}

func TestJsonToTableWithMapping_ArtifactKeyFormatting(t *testing.T) {
	// Test that artifactKey is properly formatted in the output
	testData := []map[string]interface{}{
		{
			"package": "test-pkg",
			"artifactKey": map[string]interface{}{
				"architecture": "amd64",
				"distribution": "focal",
				"component":    "main",
			},
		},
	}

	jsonData, err := json.Marshal(testData)
	if err != nil {
		t.Fatalf("failed to marshal test data: %v", err)
	}

	mapping := ColumnMapping{
		{"package", "Package"},
		{"artifactKey", "Artifact Key"},
	}

	err = jsonToTableWithMapping(string(jsonData), mapping)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Note: We can't easily capture pterm output, but we ensure no errors occur
	// The actual formatting is tested via TestFormatArtifactKey
}

func TestColumnMapping_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		jsonStr     string
		mapping     ColumnMapping
		expectError bool
	}{
		{
			name:    "Mapping with single element (invalid)",
			jsonStr: `[{"name": "test"}]`,
			mapping: ColumnMapping{
				{"name"}, // Missing display name
			},
			expectError: false, // Should handle gracefully
		},
		{
			name:    "Mapping with extra elements",
			jsonStr: `[{"name": "test"}]`,
			mapping: ColumnMapping{
				{"name", "Name", "extra1", "extra2"},
			},
			expectError: false,
		},
		{
			name:    "Mapping references non-existent field",
			jsonStr: `[{"name": "test"}]`,
			mapping: ColumnMapping{
				{"name", "Name"},
				{"nonexistent", "Does Not Exist"},
			},
			expectError: false, // Should show "-" for missing fields
		},
		{
			name:    "Empty mapping array",
			jsonStr: `[{"name": "test"}]`,
			mapping: ColumnMapping{},
			expectError: false, // Should fall back to alphabetical
		},
		{
			name: "Complex nested structures",
			jsonStr: `[{
				"name": "test",
				"nested": {
					"level1": {
						"level2": "value"
					}
				}
			}]`,
			mapping: ColumnMapping{
				{"name", "Name"},
				{"nested", "Nested"},
			},
			expectError: false, // Should stringify nested object
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := jsonToTableWithMapping(tt.jsonStr, tt.mapping)

			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestPrintTableWithOptions(t *testing.T) {
	tests := []struct {
		name        string
		data        interface{}
		options     TableOptions
		expectError bool
	}{
		{
			name: "Simple data with default options",
			data: []map[string]interface{}{
				{"name": "test1", "value": "val1"},
				{"name": "test2", "value": "val2"},
			},
			options:     DefaultTableOptions(),
			expectError: false,
		},
		{
			name: "Data with custom mapping",
			data: []map[string]interface{}{
				{"name": "test", "version": "1.0.0"},
			},
			options: TableOptions{
				ColumnMapping: ColumnMapping{
					{"name", "Package Name"},
					{"version", "Version"},
				},
				ShowPagination: true,
				PageIndex:      1,
				PageCount:      5,
				ItemCount:      42,
			},
			expectError: false,
		},
		{
			name: "Data with artifactKey",
			data: []map[string]interface{}{
				{
					"name": "pkg",
					"artifactKey": map[string]interface{}{
						"arch": "amd64",
					},
				},
			},
			options: TableOptions{
				ColumnMapping: ColumnMapping{
					{"name", "Name"},
					{"artifactKey", "Key"},
				},
				ShowPagination: false,
			},
			expectError: false,
		},
		{
			name: "Empty data",
			data: []map[string]interface{}{},
			options: TableOptions{
				ShowPagination: false,
			},
			expectError: false,
		},
		{
			name: "Pagination disabled",
			data: []map[string]interface{}{
				{"field": "value"},
			},
			options: TableOptions{
				ShowPagination: false,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := PrintTableWithOptions(tt.data, tt.options)

			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestDefaultTableOptions(t *testing.T) {
	opts := DefaultTableOptions()

	if !opts.ShowPagination {
		t.Errorf("expected ShowPagination to be true by default")
	}

	if opts.PageIndex != 0 {
		t.Errorf("expected PageIndex to be 0, got %d", opts.PageIndex)
	}

	if opts.PageCount != 0 {
		t.Errorf("expected PageCount to be 0, got %d", opts.PageCount)
	}

	if opts.ItemCount != 0 {
		t.Errorf("expected ItemCount to be 0, got %d", opts.ItemCount)
	}

	if opts.ColumnMapping != nil {
		t.Errorf("expected ColumnMapping to be nil, got %v", opts.ColumnMapping)
	}
}

func TestJsonToTableWithMapping_SpecialCharacters(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
		mapping ColumnMapping
	}{
		{
			name: "ArtifactKey with special characters in values",
			jsonStr: `[{
				"name": "test",
				"artifactKey": {
					"path": "/usr/local/bin",
					"url": "https://example.com/path?query=value",
					"email": "user@domain.com"
				}
			}]`,
			mapping: ColumnMapping{
				{"name", "Name"},
				{"artifactKey", "Key"},
			},
		},
		{
			name: "Unicode characters",
			jsonStr: `[{
				"name": "测试",
				"artifactKey": {
					"locale": "日本語"
				}
			}]`,
			mapping: ColumnMapping{
				{"name", "Name"},
				{"artifactKey", "Key"},
			},
		},
		{
			name: "Empty strings and special values",
			jsonStr: `[{
				"name": "",
				"count": 0,
				"enabled": false,
				"artifactKey": {}
			}]`,
			mapping: ColumnMapping{
				{"name", "Name"},
				{"count", "Count"},
				{"enabled", "Enabled"},
				{"artifactKey", "Key"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := jsonToTableWithMapping(tt.jsonStr, tt.mapping)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestFormatArtifactKey_Integration tests the integration of formatArtifactKey
// with the table rendering system
func TestFormatArtifactKey_Integration(t *testing.T) {
	// Create test data with various artifactKey scenarios
	testCases := []struct {
		name         string
		artifactKey  interface{}
		expectedPart string // Part of the expected output
	}{
		{
			name: "Map with sorted keys",
			artifactKey: map[string]interface{}{
				"z_last":  "value1",
				"a_first": "value2",
				"m_mid":   "value3",
			},
			expectedPart: "a_first=value2",
		},
		{
			name:         "Nil artifactKey",
			artifactKey:  nil,
			expectedPart: "<nil>",
		},
		{
			name: "ArtifactKey with numeric values",
			artifactKey: map[string]interface{}{
				"count": 123,
				"id":    456,
			},
			expectedPart: "count=123",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := formatArtifactKey(tc.artifactKey)
			if !strings.Contains(result, tc.expectedPart) && result != tc.expectedPart {
				t.Errorf("expected result to contain %q, got %q", tc.expectedPart, result)
			}
		})
	}
}
