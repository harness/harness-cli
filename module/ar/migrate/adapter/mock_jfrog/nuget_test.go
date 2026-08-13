package mock_jfrog

import (
	"sync"
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/adapter/jfrog"
	"github.com/harness/harness-cli/module/ar/migrate/tree"
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

// TestGetPackagesNuget verifies the NUGET GetPackages behavior against the
// nuget-local fixture. This is a characterization test locking current behavior
// before Task 3's index cache refactor.
func TestGetPackagesNuget(t *testing.T) {
	const registry = "nuget-local"
	adapter := jfrog.NewAdapterWithClient(types.RegistryConfig{Type: types.MOCK_JFROG}, NewMockClient())

	files, err := adapter.GetFiles(registry)
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	root := tree.TransformToTree(files)

	pkgs, err := adapter.GetPackages(registry, types.NUGET, root)
	if err != nil {
		t.Fatalf("GetPackages: %v", err)
	}

	// Build a set of package names for order-independent comparison
	pkgSet := make(map[string]bool)
	for _, p := range pkgs {
		pkgSet[p.Name] = true
	}

	// Expected: exactly 1 package "company.grpc.pkg" (lowercased by ParseNugetFileNameWithPath)
	// .sha512 files fail the extension check; .nuspec files have <3 dots and fail to parse.
	expected := map[string]bool{"company.grpc.pkg": true}

	if len(pkgSet) != len(expected) {
		t.Fatalf("expected %d packages, got %d: %+v", len(expected), len(pkgSet), pkgs)
	}

	for pkg := range expected {
		if !pkgSet[pkg] {
			t.Errorf("missing expected package %q", pkg)
		}
	}

	for pkg := range pkgSet {
		if !expected[pkg] {
			t.Errorf("unexpected package %q", pkg)
		}
	}
}

// TestGetVersionsNuget verifies the NUGET GetVersions behavior against the
// nuget-local fixture. This is a characterization test locking current behavior
// before Task 3's index cache refactor.
func TestGetVersionsNuget(t *testing.T) {
	const registry = "nuget-local"
	adapter := jfrog.NewAdapterWithClient(types.RegistryConfig{Type: types.MOCK_JFROG}, NewMockClient())

	files, err := adapter.GetFiles(registry)
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	root := tree.TransformToTree(files)

	pkg := types.Package{Name: "company.grpc.pkg"}
	versions, err := adapter.GetVersions(pkg, root, registry, "company.grpc.pkg", types.NUGET)
	if err != nil {
		t.Fatalf("GetVersions: %v", err)
	}

	// Build a set of version names for order-independent comparison
	versionSet := make(map[string]bool)
	for _, v := range versions {
		versionSet[v.Name] = true
	}

	// Expected: exactly 2 versions {"1.0.0", "2.0.0"}
	expected := map[string]bool{"1.0.0": true, "2.0.0": true}

	if len(versionSet) != len(expected) {
		t.Fatalf("expected %d versions, got %d: %+v", len(expected), len(versionSet), versions)
	}

	for ver := range expected {
		if !versionSet[ver] {
			t.Errorf("missing expected version %q", ver)
		}
	}

	for ver := range versionSet {
		if !expected[ver] {
			t.Errorf("unexpected version %q", ver)
		}
	}
}

// TestNugetIndexConcurrentAccess verifies that concurrent calls to GetPackages
// and GetVersions (after Task 3's index cache is added) do not race and return
// consistent results. This test MUST pass under -race.
func TestNugetIndexConcurrentAccess(t *testing.T) {
	const registry = "nuget-local"
	adapter := jfrog.NewAdapterWithClient(types.RegistryConfig{Type: types.MOCK_JFROG}, NewMockClient())

	files, err := adapter.GetFiles(registry)
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	root := tree.TransformToTree(files)

	// Launch many goroutines that concurrently call GetPackages and GetVersions
	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errCh := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()

			// Call GetPackages
			pkgs, err := adapter.GetPackages(registry, types.NUGET, root)
			if err != nil {
				errCh <- err
				return
			}

			// Verify expected package set
			if len(pkgs) != 1 || pkgs[0].Name != "company.grpc.pkg" {
				errCh <- err
				return
			}

			// Call GetVersions
			pkg := types.Package{Name: "company.grpc.pkg"}
			versions, err := adapter.GetVersions(pkg, root, registry, "company.grpc.pkg", types.NUGET)
			if err != nil {
				errCh <- err
				return
			}

			// Verify expected version set (order-independent)
			versionSet := make(map[string]bool)
			for _, v := range versions {
				versionSet[v.Name] = true
			}
			if len(versionSet) != 2 || !versionSet["1.0.0"] || !versionSet["2.0.0"] {
				errCh <- err
				return
			}
		}()
	}

	wg.Wait()
	close(errCh)

	// Check for errors from goroutines
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent access error: %v", err)
		}
	}
}
