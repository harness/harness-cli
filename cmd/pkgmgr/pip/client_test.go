package pip

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectFirewallError(t *testing.T) {
	client := NewClient()

	tests := []struct {
		name   string
		stderr string
		want   bool
	}{
		{"403 Forbidden", "ERROR: 403 Forbidden", true},
		{"HTTP error 403", "HTTP error 403 for url", true},
		{"Client Error 403", "Client Error: 403 Forbidden", true},
		{"no error", "Successfully installed requests-2.28.0", false},
		{"404 not 403", "ERROR: HTTP error 404 for url", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := client.DetectFirewallError(tt.stderr)
			if got != tt.want {
				t.Errorf("DetectFirewallError(%q) = %v, want %v", tt.stderr, got, tt.want)
			}
		})
	}
}

func TestParsePipReport(t *testing.T) {
	reportContent := `{
  "install": [
    {"metadata": {"name": "requests", "version": "2.28.0"}, "requested": true},
    {"metadata": {"name": "urllib3", "version": "1.26.12"}, "requested": false},
    {"metadata": {"name": "certifi", "version": "2022.9.24"}, "requested": false}
  ]
}`

	tmpDir := t.TempDir()
	reportPath := filepath.Join(tmpDir, "report.json")
	if err := os.WriteFile(reportPath, []byte(reportContent), 0644); err != nil {
		t.Fatal(err)
	}

	deps, err := parsePipReport(reportPath)
	if err != nil {
		t.Fatalf("parsePipReport failed: %v", err)
	}

	if len(deps) != 3 {
		t.Fatalf("expected 3 deps, got %d", len(deps))
	}

	if deps[0].Name != "requests" || deps[0].Version != "2.28.0" {
		t.Errorf("expected requests@2.28.0, got %s@%s", deps[0].Name, deps[0].Version)
	}
	if deps[1].Name != "urllib3" {
		t.Errorf("expected urllib3, got %s", deps[1].Name)
	}
}
