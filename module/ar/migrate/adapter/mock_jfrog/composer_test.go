package mock_jfrog

import (
	"sync"
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/adapter/jfrog"
	"github.com/harness/harness-cli/module/ar/migrate/tree"
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

// TestGetPackagesComposer verifies COMPOSER GetPackages groups version zips by logical
// vendor/package name and ignores index/metadata/non-zip artifacts.
func TestGetPackagesComposer(t *testing.T) {
	const registry = "composer-local"
	adapter := jfrog.NewAdapterWithClient(types.RegistryConfig{Type: types.MOCK_JFROG}, NewMockClient())

	files, err := adapter.GetFiles(registry)
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	root := tree.TransformToTree(files)

	pkgs, err := adapter.GetPackages(registry, types.COMPOSER, root)
	if err != nil {
		t.Fatalf("GetPackages: %v", err)
	}

	want := map[string]bool{
		"harness/migtest": true,
		"acme/demo":       true,
	}
	if len(pkgs) != len(want) {
		t.Fatalf("expected %d logical packages, got %d: %+v", len(want), len(pkgs), pkgs)
	}
	for _, p := range pkgs {
		if !want[p.Name] {
			t.Errorf("unexpected package %q", p.Name)
		}
	}
}

// TestGetVersionsComposer scans the tree and returns every version zip for the logical package.
func TestGetVersionsComposer(t *testing.T) {
	const registry = "composer-local"
	adapter := jfrog.NewAdapterWithClient(types.RegistryConfig{Type: types.MOCK_JFROG}, NewMockClient())

	files, err := adapter.GetFiles(registry)
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	root := tree.TransformToTree(files)

	pkg := types.Package{
		Registry: registry,
		Name:     "harness/migtest",
		Path:     "/",
	}
	versions, err := adapter.GetVersions(pkg, root, registry, pkg.Name, types.COMPOSER)
	if err != nil {
		t.Fatalf("GetVersions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions for harness/migtest, got %d: %+v", len(versions), versions)
	}
	got := map[string]string{}
	for _, v := range versions {
		got[v.Name] = v.Path
	}
	want := map[string]string{
		"1.0.0": "/harness-migtest/harness-migtest-1.0.0.zip",
		"2.0.0": "/harness-migtest/harness-migtest-2.0.0.zip",
	}
	for name, path := range want {
		if got[name] != path {
			t.Errorf("version %q path = %q, want %q", name, got[name], path)
		}
	}
}

// TestGetPackagesComposerConcurrentAccess guards against data races during concurrent COMPOSER enumeration.
func TestGetPackagesComposerConcurrentAccess(t *testing.T) {
	const registry = "composer-local"
	adapter := jfrog.NewAdapterWithClient(types.RegistryConfig{Type: types.MOCK_JFROG}, NewMockClient())

	files, err := adapter.GetFiles(registry)
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	root := tree.TransformToTree(files)

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := adapter.GetPackages(registry, types.COMPOSER, root); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent GetPackages: %v", err)
	}
}
