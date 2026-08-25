package mock_jfrog

import (
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/adapter/jfrog"
	"github.com/harness/harness-cli/module/ar/migrate/tree"
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

// TestGetPackagesCRAN exercises CRAN enumeration through the mock client: live
// contrib archives, Archive/ superseded versions, and a windows binary. PACKAGES
// index files must not appear as packages.
func TestGetPackagesCRAN(t *testing.T) {
	const registry = "cran-local"
	adapter := jfrog.NewAdapterWithClient(types.RegistryConfig{Type: types.MOCK_JFROG}, NewMockClient())

	files, err := adapter.GetFiles(registry)
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	root := tree.TransformToTree(files)

	pkgs, err := adapter.GetPackages(registry, types.CRAN, root)
	if err != nil {
		t.Fatalf("GetPackages: %v", err)
	}

	byName := make(map[string]types.Package)
	for _, p := range pkgs {
		byName[p.Name] = p
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages (jsonlite, data.table), got %d: %+v", len(pkgs), pkgs)
	}
	for _, name := range []string{"jsonlite", "data.table"} {
		if _, ok := byName[name]; !ok {
			t.Errorf("missing package %q in %+v", name, byName)
		}
	}
	for _, p := range pkgs {
		if p.Name == "PACKAGES" || p.Name == "PACKAGES.gz" {
			t.Errorf("index file enumerated as package: %+v", p)
		}
	}
}
