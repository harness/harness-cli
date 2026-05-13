package nuget

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
		{"403 Forbidden", "Response status code does not indicate success: 403 (Forbidden)", true},
		{"HTTP 403", "HTTP 403 error", true},
		{"no error", "Restore completed successfully", false},
		{"404 not 403", "Response status code does not indicate success: 404 (Not Found)", false},
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

func TestParsePackagesLockJson(t *testing.T) {
	content := `{
  "version": 1,
  "dependencies": {
    "net6.0": {
      "Newtonsoft.Json": {
        "type": "Direct",
        "resolved": "13.0.1"
      },
      "System.Text.Json": {
        "type": "Transitive",
        "resolved": "6.0.0"
      }
    }
  }
}`

	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "packages.lock.json")
	if err := os.WriteFile(lockPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	deps, err := parsePackagesLockJson(lockPath)
	if err != nil {
		t.Fatalf("parsePackagesLockJson failed: %v", err)
	}

	if len(deps) != 2 {
		t.Fatalf("expected 2 deps, got %d", len(deps))
	}

	found := map[string]bool{}
	for _, dep := range deps {
		found[dep.Name] = true
		if dep.Source != "packages.lock.json" {
			t.Errorf("expected source packages.lock.json, got %s", dep.Source)
		}
	}

	if !found["Newtonsoft.Json"] {
		t.Error("expected Newtonsoft.Json in deps")
	}
	if !found["System.Text.Json"] {
		t.Error("expected System.Text.Json in deps")
	}
}

func TestParseCsprojFiles(t *testing.T) {
	tmpDir := t.TempDir()
	csproj := `<Project Sdk="Microsoft.NET.Sdk">
  <ItemGroup>
    <PackageReference Include="Newtonsoft.Json" Version="13.0.1" />
    <PackageReference Include="Serilog" Version="2.12.0" />
  </ItemGroup>
</Project>`

	if err := os.WriteFile(filepath.Join(tmpDir, "TestApp.csproj"), []byte(csproj), 0644); err != nil {
		t.Fatal(err)
	}

	// Change to temp dir for the test
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	deps, err := parseCsprojFiles()
	if err != nil {
		t.Fatalf("parseCsprojFiles failed: %v", err)
	}

	if len(deps) != 2 {
		t.Fatalf("expected 2 deps, got %d", len(deps))
	}

	if deps[0].Name != "Newtonsoft.Json" || deps[0].Version != "13.0.1" {
		t.Errorf("expected Newtonsoft.Json@13.0.1, got %s@%s", deps[0].Name, deps[0].Version)
	}
}
