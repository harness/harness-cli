package jfrog

import (
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/tree"
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

func rubyTestAdapter() *adapter {
	return &adapter{reg: types.RegistryConfig{Type: types.JFROG}}
}

func rubyTestTree() *types.TreeNode {
	files := []types.File{
		{Registry: "gems-local", Name: "rails-8.0.2.gem", Uri: "/gems/rails-8.0.2.gem", Size: 1024},
		{Registry: "gems-local", Name: "rails-8.0.2-x86_64-linux.gem", Uri: "/gems/rails-8.0.2-x86_64-linux.gem", Size: 2048},
		{Registry: "gems-local", Name: "nokogiri-1.15.0.gem", Uri: "/gems/nokogiri-1.15.0.gem", Size: 4096},
		{Registry: "gems-local", Name: "rails-8.0.2.gem.md5", Uri: "/gems/rails-8.0.2.gem.md5", Size: 32},
	}
	return tree.TransformToTree(files)
}

func TestGetPackagesRubyDedupesGemNames(t *testing.T) {
	const registry = "gems-local"
	a := rubyTestAdapter()
	root := rubyTestTree()

	pkgs, err := a.GetPackages(registry, types.RUBY, root)
	if err != nil {
		t.Fatalf("GetPackages: %v", err)
	}

	want := map[string]bool{"rails": true, "nokogiri": true}
	if len(pkgs) != len(want) {
		t.Fatalf("expected %d packages, got %d: %+v", len(want), len(pkgs), pkgs)
	}
	for _, p := range pkgs {
		if !want[p.Name] {
			t.Errorf("unexpected package %q", p.Name)
		}
		if p.Path != "/" || p.Registry != registry {
			t.Errorf("package %q metadata = %+v", p.Name, p)
		}
	}
}

func TestGetVersionsRubyGroupsPlatformVariants(t *testing.T) {
	const registry = "gems-local"
	a := rubyTestAdapter()
	root := rubyTestTree()

	pkg := types.Package{Registry: registry, Name: "rails", Path: "/"}
	versions, err := a.GetVersions(pkg, root, registry, pkg.Name, types.RUBY)
	if err != nil {
		t.Fatalf("GetVersions: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("expected 1 version for rails (both platform variants), got %d: %+v", len(versions), versions)
	}
	if versions[0].Name != "8.0.2" {
		t.Fatalf("version name = %q, want 8.0.2", versions[0].Name)
	}
}

func TestGetVersionsRubyUnknownPackage(t *testing.T) {
	a := rubyTestAdapter()
	root := rubyTestTree()
	pkg := types.Package{Name: "missing-gem"}
	versions, err := a.GetVersions(pkg, root, "gems-local", pkg.Name, types.RUBY)
	if err != nil {
		t.Fatalf("GetVersions: %v", err)
	}
	if len(versions) != 0 {
		t.Fatalf("expected 0 versions, got %+v", versions)
	}
}
