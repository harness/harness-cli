package command

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar_v2"
)

type mockTransport struct {
	response *http.Response
	err      error
}

func (m *mockTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return m.response, m.err
}

func newMockClient(statusCode int, body string) *ar_v2.ClientWithResponses {
	transport := &mockTransport{
		response: &http.Response{
			StatusCode: statusCode,
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Header:     make(http.Header),
		},
	}
	httpClient := &http.Client{Transport: transport}
	client, _ := ar_v2.NewClientWithResponses("http://test", ar_v2.WithHTTPClient(httpClient))
	return client
}

func TestNewSetMetadataCmd(t *testing.T) {
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
			args:       []string{"--metadata", "key:value"},
			wantErr:    true,
			errContain: "required flag",
		},
		{
			name:       "missing metadata flag",
			args:       []string{"--registry", "test-reg"},
			wantErr:    true,
			errContain: "required flag",
		},
		{
			name:       "invalid metadata format",
			args:       []string{"--registry", "test-reg", "--metadata", "invalid"},
			wantErr:    true,
			errContain: "key:value format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockClient(200, `{"status":"SUCCESS","data":{"metadata":[]}}`)
			f := &cmdutils.Factory{
				RegistryV2HttpClient: func() *ar_v2.ClientWithResponses { return mockClient },
			}

			cmd := NewSetMetadataCmd(f)
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

func TestNewGetMetadataCmd(t *testing.T) {
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
			args:       []string{},
			wantErr:    true,
			errContain: "required flag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockClient(200, `{"status":"SUCCESS","data":{"metadata":[]}}`)
			f := &cmdutils.Factory{
				RegistryV2HttpClient: func() *ar_v2.ClientWithResponses { return mockClient },
			}

			cmd := NewGetMetadataCmd(f)
			cmd.SetArgs(tt.args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			err := cmd.Execute()
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewRemoveMetadataCmd(t *testing.T) {
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
			args:       []string{"--metadata", "key:value"},
			wantErr:    true,
			errContain: "required flag",
		},
		{
			name:       "missing metadata flag",
			args:       []string{"--registry", "test-reg"},
			wantErr:    true,
			errContain: "required flag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockClient(200, `{"status":"SUCCESS"}`)
			f := &cmdutils.Factory{
				RegistryV2HttpClient: func() *ar_v2.ClientWithResponses { return mockClient },
			}

			cmd := NewRemoveMetadataCmd(f)
			cmd.SetArgs(tt.args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			err := cmd.Execute()
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
