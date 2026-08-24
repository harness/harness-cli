package mock_jfrog

import (
	"slices"
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/adapter/jfrog"
	"github.com/harness/harness-cli/module/ar/migrate/tree"
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

// TestGetVersionsSwiftMissesSiblingVersions documents ticket #18: GetVersions for
// SWIFT does not scan the registry for all versions of a logical package. The
// mock swift-local registry holds myscope.harness at 1.0.0 and 1.0.1, but
// GetVersions echoes only the version already on the Package struct.
func TestGetVersionsSwiftMissesSiblingVersions(t *testing.T) {
	const registry = "swift-local"
	const logicalPkg = "myscope.harness"

	adapter := jfrog.NewAdapterWithClient(types.RegistryConfig{Type: types.MOCK_JFROG}, NewMockClient())
	files, err := adapter.GetFiles(registry)
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	root := tree.TransformToTree(files)

	pkgs, err := adapter.GetPackages(registry, types.SWIFT, root)
	if err != nil {
		t.Fatalf("GetPackages: %v", err)
	}

	var first types.Package
	var versionsInRegistry []string
	for _, pkg := range pkgs {
		if pkg.Name != logicalPkg {
			continue
		}
		versionsInRegistry = append(versionsInRegistry, pkg.Version)
		if first.Name == "" {
			first = pkg
		}
	}
	slices.Sort(versionsInRegistry)

	wantInRegistry := []string{"1.0.0", "1.0.1"}
	if len(versionsInRegistry) != len(wantInRegistry) {
		t.Fatalf("registry fixture: got versions %v, want %v (check swift-local.json)", versionsInRegistry, wantInRegistry)
	}
	for i, want := range wantInRegistry {
		if versionsInRegistry[i] != want {
			t.Fatalf("registry fixture: got versions %v, want %v", versionsInRegistry, wantInRegistry)
		}
	}
	t.Logf("swift-local has %d package rows for %q: versions %v", len(versionsInRegistry), logicalPkg, versionsInRegistry)

	got, err := adapter.GetVersions(first, root, registry, logicalPkg, types.SWIFT)
	if err != nil {
		t.Fatalf("GetVersions(%q): %v", logicalPkg, err)
	}
	gotNames := versionNames(got)
	t.Logf("GetVersions returned %d version(s): %v (want scan of all %v)", len(gotNames), gotNames, wantInRegistry)

	if len(got) != len(wantInRegistry) {
		t.Fatalf("GetVersions returned %d version(s) %v, want %d (%v) from registry scan",
			len(got), gotNames, len(wantInRegistry), wantInRegistry)
	}
	for _, want := range wantInRegistry {
		if !slices.Contains(gotNames, want) {
			t.Errorf("GetVersions missing version %q; got %v", want, gotNames)
		}
	}
}

// TestGetVersionsComposerScansAllVersions verifies COMPOSER GetVersions scans
// the tree for all versions of a logical package. composer-local holds
// harness-migtest-1.0.0.zip and harness-migtest-2.0.0.zip (logical: harness/migtest).
func TestGetVersionsComposerScansAllVersions(t *testing.T) {
	const registry = "composer-local"
	const logicalPkg = "harness/migtest"

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
	t.Logf("composer-local package rows (one per zip): %+v", pkgs)

	// Find any package row with the target logical name.
	var first types.Package
	for _, pkg := range pkgs {
		if pkg.Name == logicalPkg {
			first = pkg
			break
		}
	}
	if first.Name == "" {
		t.Fatalf("no package found with logical name %q in %d packages", logicalPkg, len(pkgs))
	}

	wantVersions := []string{"1.0.0", "2.0.0"}
	got, err := adapter.GetVersions(first, root, registry, logicalPkg, types.COMPOSER)
	if err != nil {
		t.Fatalf("GetVersions(%q): %v", logicalPkg, err)
	}
	gotNames := versionNames(got)
	t.Logf("GetVersions returned %d version(s): %v (want scan of all %v)", len(gotNames), gotNames, wantVersions)

	if len(got) != len(wantVersions) {
		t.Fatalf("GetVersions returned %d version(s) %v, want %d (%v) from registry scan",
			len(got), gotNames, len(wantVersions), wantVersions)
	}
	for _, want := range wantVersions {
		if !slices.Contains(gotNames, want) {
			t.Errorf("GetVersions missing version %q; got %v", want, gotNames)
		}
	}
}

func versionNames(versions []types.Version) []string {
	names := make([]string, 0, len(versions))
	for _, v := range versions {
		names = append(names, v.Name)
	}
	slices.Sort(names)
	return names
}
