package mock_jfrog

import (
	"sync"
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/adapter/jfrog"
	"github.com/harness/harness-cli/module/ar/migrate/tree"
	"github.com/harness/harness-cli/module/ar/migrate/types"
	"github.com/harness/harness-cli/module/ar/migrate/util"
)

// TestGetPackagesPython verifies basic PYTHON GetPackages behavior against
// the python-local fixture (which has a .pypi/simple.html index).
func TestGetPackagesPython(t *testing.T) {
	const registry = "python-local"
	adapter := jfrog.NewAdapterWithClient(types.RegistryConfig{Type: types.MOCK_JFROG}, NewMockClient())

	files, err := adapter.GetFiles(registry)
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	root := tree.TransformToTree(files)

	pkgs, err := adapter.GetPackages(registry, types.PYTHON, root)
	if err != nil {
		t.Fatalf("GetPackages: %v", err)
	}

	// extractPythonPackageNames returns hrefs verbatim from simple.html, which
	// for python-local's fixture is "requests/" (trailing slash preserved).
	if len(pkgs) != 1 || pkgs[0].Name != "requests/" {
		t.Fatalf("expected exactly package %q, got %+v", "requests/", pkgs)
	}
}

// TestGetPackagesPythonDateFilterPreservesIndex is the AH-4518 regression test.
//
// A downloadedAfter/createdAfter date filter would otherwise drop the old
// .pypi/*.html index files before tree.TransformToTree builds `root`, and
// GetPackages's PYTHON branch looks the index up via
// tree.GetNodeForPath(root, "/.pypi/simple.html") — so enumeration used to
// fail with "path not found: /.pypi/simple.html".
//
// The fix keeps index files exempt from the date filter
// (util.IsPackageIndexFile + the preservation step in migratable/registry.go),
// so the index survives into `root` and enumeration still succeeds. This test
// reproduces registry.go's filtering flow: a filter set that matches NO files
// (so every .pypi/* metadata file would be dropped), then the index-file
// preservation, then FilterFilesByDate — and asserts GetPackages still works.
func TestGetPackagesPythonDateFilterPreservesIndex(t *testing.T) {
	const registry = "python-local"
	adapter := jfrog.NewAdapterWithClient(types.RegistryConfig{Type: types.MOCK_JFROG}, NewMockClient())

	files, err := adapter.GetFiles(registry)
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}

	// Simulate an aggressive date filter that matches none of the files (the
	// worst case, where even the artifacts are out of range). Mirrors
	// registry.go: the index files are added back before FilterFilesByDate.
	filteredURIs := map[string]struct{}{}
	for _, f := range files {
		if util.IsPackageIndexFile(types.PYTHON, f.Uri) {
			filteredURIs[f.Uri] = struct{}{}
		}
	}
	kept := util.FilterFilesByDate(files, filteredURIs)
	root := tree.TransformToTree(kept)

	// The preserved index must still be present in the tree for the lookup.
	if _, err := tree.GetNodeForPath(root, "/.pypi/simple.html"); err != nil {
		t.Fatalf("index preservation failed: /.pypi/simple.html should survive the date filter: %v", err)
	}

	pkgs, err := adapter.GetPackages(registry, types.PYTHON, root)
	if err != nil {
		t.Fatalf("GetPackages: %v (index files must be exempt from the date filter so enumeration still works)", err)
	}

	if len(pkgs) != 1 || pkgs[0].Name != "requests/" {
		t.Fatalf("expected exactly package %q, got %+v", "requests/", pkgs)
	}
}

// TestPythonEnumerationConcurrentAccess drives GetPackages and GetVersions
// concurrently (concurrency=2 style, per the AH-4518 report of a
// "concurrent map writes" crash in jfrog.(*adapter).GetVersions during PyPI
// enumeration) across multiple goroutines and packages. Must pass under
// -race. This is a regression guard: if any shared/package-level cache is
// ever introduced to speed up PyPI enumeration, this test should start
// failing under -race unless that cache is properly synchronized.
func TestPythonEnumerationConcurrentAccess(t *testing.T) {
	const registry = "python-local"
	adapter := jfrog.NewAdapterWithClient(types.RegistryConfig{Type: types.MOCK_JFROG}, NewMockClient())

	files, err := adapter.GetFiles(registry)
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	root := tree.TransformToTree(files)

	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)
	errCh := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()

			pkgs, err := adapter.GetPackages(registry, types.PYTHON, root)
			if err != nil {
				errCh <- err
				return
			}
			if len(pkgs) != 1 || pkgs[0].Name != "requests/" {
				errCh <- err
				return
			}

			pkgNode, err := tree.GetNodeForPath(root, "requests")
			if err != nil {
				errCh <- err
				return
			}

			versions, err := adapter.GetVersions(pkgs[0], pkgNode, registry, "requests", types.PYTHON)
			if err != nil {
				errCh <- err
				return
			}

			versionSet := make(map[string]bool)
			for _, v := range versions {
				versionSet[v.Name] = true
			}
			if len(versionSet) != 2 || !versionSet["2.28.0"] || !versionSet["2.29.0"] {
				errCh <- err
				return
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent access error: %v", err)
		}
	}
}
