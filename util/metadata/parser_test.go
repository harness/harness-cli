package metadata

import (
	"testing"

	"github.com/harness/harness-cli/internal/api/ar_v2"
)

func TestParseMetadataString(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []ar_v2.MetadataItemInput
		wantErr bool
		errMsg  string
	}{
		{
			name:  "single key-value pair",
			input: "env:prod",
			want: []ar_v2.MetadataItemInput{
				{Key: "env", Value: "prod"},
			},
			wantErr: false,
		},
		{
			name:  "multiple key-value pairs",
			input: "env:prod,region:us,team:backend",
			want: []ar_v2.MetadataItemInput{
				{Key: "env", Value: "prod"},
				{Key: "region", Value: "us"},
				{Key: "team", Value: "backend"},
			},
			wantErr: false,
		},
		{
			name:  "value with colon (URL)",
			input: "url:http://example.com",
			want: []ar_v2.MetadataItemInput{
				{Key: "url", Value: "http://example.com"},
			},
			wantErr: false,
		},
		{
			name:  "whitespace trimming",
			input: " env : prod , region : us ",
			want: []ar_v2.MetadataItemInput{
				{Key: "env", Value: "prod"},
				{Key: "region", Value: "us"},
			},
			wantErr: false,
		},
		{
			name:    "empty string",
			input:   "",
			want:    nil,
			wantErr: true,
			errMsg:  "metadata string cannot be empty",
		},
		{
			name:    "missing value",
			input:   "env",
			want:    nil,
			wantErr: true,
			errMsg:  "metadata must be in key:value format",
		},
		{
			name:    "empty key",
			input:   ":prod",
			want:    nil,
			wantErr: true,
			errMsg:  "metadata key cannot be empty",
		},
		{
			name:    "empty value",
			input:   "env:",
			want:    nil,
			wantErr: true,
			errMsg:  "metadata value cannot be empty",
		},
		{
			name:  "multiple colons in value",
			input: "env:test:prod:staging",
			want: []ar_v2.MetadataItemInput{
				{Key: "env", Value: "test:prod:staging"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMetadataString(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMetadataString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil {
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					// Check if error message contains expected substring
					if !contains(err.Error(), tt.errMsg) {
						t.Errorf("ParseMetadataString() error = %v, want error containing %v", err, tt.errMsg)
					}
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("ParseMetadataString() got %d items, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i].Key != tt.want[i].Key || got[i].Value != tt.want[i].Value {
					t.Errorf("ParseMetadataString() item %d = {%s:%s}, want {%s:%s}",
						i, got[i].Key, got[i].Value, tt.want[i].Key, tt.want[i].Value)
				}
			}
		})
	}
}

func TestFormatMetadataOutput(t *testing.T) {
	tests := []struct {
		name  string
		input []ar_v2.MetadataItemOutput
		want  string
	}{
		{
			name:  "empty list",
			input: []ar_v2.MetadataItemOutput{},
			want:  "No metadata found",
		},
		{
			name: "single item",
			input: []ar_v2.MetadataItemOutput{
				{Id: "1", Key: "env", Value: "prod", Type: ar_v2.MANUAL},
			},
			want: "env: prod",
		},
		{
			name: "multiple items",
			input: []ar_v2.MetadataItemOutput{
				{Id: "1", Key: "env", Value: "prod", Type: ar_v2.MANUAL},
				{Id: "2", Key: "region", Value: "us", Type: ar_v2.AUTO},
			},
			want: "env: prod\nregion: us",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatMetadataOutput(tt.input)
			if got != tt.want {
				t.Errorf("FormatMetadataOutput() = %v, want %v", got, tt.want)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
