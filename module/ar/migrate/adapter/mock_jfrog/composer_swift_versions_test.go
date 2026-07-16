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

// TestGetVersionsComposerMissesSiblingVersions documents ticket #18 for COMPOSER:
// GetVersions returns a single entry built from Package fields instead of
// discovering every .zip for the logical package. composer-local holds
// vendor-package-1.0.0.zip and vendor-package-2.0.0.zip.
func TestGetVersionsComposerMissesSiblingVersions(t *testing.T) {
	const registry = "composer-local"
	const logicalPkg = "vendor/package"

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
	if len(pkgs) != 2 {
		t.Fatalf("registry fixture: expected 2 composer zips, got %d: %+v", len(pkgs), pkgs)
	}
	t.Logf("composer-local package rows (one per zip): %+v", pkgs)

	// One logical package row — what a version-scanning GetVersions should fan out from.
	p := types.Package{
		Registry: registry,
		Name:     logicalPkg,
		Path:     "/",
		URL:      pkgs[0].URL,
		Size:     pkgs[0].Size,
	}

	wantVersions := []string{"1.0.0", "2.0.0"}
	got, err := adapter.GetVersions(p, root, registry, logicalPkg, types.COMPOSER)
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

	// COMPOSER also sets Version.Name to the package name, not a semver string.
	if len(got) == 1 && got[0].Name == logicalPkg {
		t.Logf("COMPOSER GetVersions uses package name %q as version Name (not semver)", got[0].Name)
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
