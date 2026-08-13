package jfrog

import (
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/tree"
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

func composerTestAdapter() *adapter {
	return &adapter{reg: types.RegistryConfig{Type: types.JFROG}}
}

func composerTestTree() *types.TreeNode {
	files := []types.File{
		{Registry: "composer-local", Name: "packages.json", Uri: "/.composer/packages.json", Size: 304},
		{Registry: "composer-local", Name: "readme.txt", Uri: "/harness-migtest/readme.txt", Size: 128},
		{Registry: "composer-local", Name: "archive.tar.gz", Uri: "/harness-migtest/archive.tar.gz", Size: 512},
		{Registry: "composer-local", Name: "harness-migtest-1.0.0.zip", Uri: "/harness-migtest/harness-migtest-1.0.0.zip", Size: 509},
		{Registry: "composer-local", Name: "harness-migtest-2.0.0.zip", Uri: "/harness-migtest/harness-migtest-2.0.0.zip", Size: 510},
		{Registry: "composer-local", Name: "harness-migtest-3.0.0.zip", Uri: "/harness-migtest/harness-migtest-3.0.0.zip", Size: 294},
		{Registry: "composer-local", Name: "acme-demo-1.0.0.zip", Uri: "/acme-demo/acme-demo-1.0.0.zip", Size: 290},
		{Registry: "composer-local", Name: "payments-api-1.0.0.zip", Uri: "/acme/payments-api/payments-api-1.0.0.zip", Size: 279},
		{Registry: "composer-local", Name: "payments-api-2.0.0.zip", Uri: "/acme/payments-api/payments-api-2.0.0.zip", Size: 279},
		{Registry: "composer-local", Name: "acme-demo-2.0.0.zip", Uri: "/acme-demo-2.0.0.zip", Size: 291},
		{Registry: "composer-local", Name: "ignored.json", Uri: "/harness/migtest/ignored.json", Folder: false, Size: 10},
		{Registry: "composer-local", Name: "placeholder", Uri: "/harness/migtest/placeholder", Folder: true, Size: 0},
	}
	return tree.TransformToTree(files)
}

func TestGetPackagesComposerGroupsLogicalNames(t *testing.T) {
	const registry = "composer-local"
	a := composerTestAdapter()
	root := composerTestTree()

	pkgs, err := a.GetPackages(registry, types.COMPOSER, root)
	if err != nil {
		t.Fatalf("GetPackages: %v", err)
	}

	want := map[string]bool{
		"harness/migtest":   true,
		"acme/demo":         true,
		"acme/payments-api": true,
	}
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

func TestGetPackagesComposerDedupesMultipleZipsPerLogicalName(t *testing.T) {
	a := composerTestAdapter()
	root := composerTestTree()
	pkgs, err := a.GetPackages("composer-local", types.COMPOSER, root)
	if err != nil {
		t.Fatalf("GetPackages: %v", err)
	}
	var migtest int
	for _, p := range pkgs {
		if p.Name == "harness/migtest" {
			migtest++
		}
	}
	if migtest != 1 {
		t.Fatalf("expected one harness/migtest package row, got %d", migtest)
	}
}

func TestGetVersionsComposerReturnsAllMatchingZips(t *testing.T) {
	const registry = "composer-local"
	a := composerTestAdapter()
	root := composerTestTree()

	tests := []struct {
		pkg   string
		count int
		want  map[string]string
	}{
		{
			pkg:   "harness/migtest",
			count: 3,
			want: map[string]string{
				"1.0.0": "/harness-migtest/harness-migtest-1.0.0.zip",
				"2.0.0": "/harness-migtest/harness-migtest-2.0.0.zip",
				"3.0.0": "/harness-migtest/harness-migtest-3.0.0.zip",
			},
		},
		{
			pkg:   "acme/payments-api",
			count: 2,
			want: map[string]string{
				"1.0.0": "/acme/payments-api/payments-api-1.0.0.zip",
				"2.0.0": "/acme/payments-api/payments-api-2.0.0.zip",
			},
		},
		{
			pkg:   "acme/demo",
			count: 2,
			want: map[string]string{
				"1.0.0": "/acme-demo/acme-demo-1.0.0.zip",
				"2.0.0": "/acme-demo-2.0.0.zip",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.pkg, func(t *testing.T) {
			pkg := types.Package{Registry: registry, Name: tt.pkg, Path: "/"}
			versions, err := a.GetVersions(pkg, root, registry, tt.pkg, types.COMPOSER)
			if err != nil {
				t.Fatalf("GetVersions: %v", err)
			}
			if len(versions) != tt.count {
				t.Fatalf("expected %d versions, got %d: %+v", tt.count, len(versions), versions)
			}
			got := map[string]string{}
			for _, v := range versions {
				if v.Pkg != tt.pkg {
					t.Errorf("version pkg = %q, want %q", v.Pkg, tt.pkg)
				}
				got[v.Name] = v.Path
			}
			for ver, path := range tt.want {
				if got[ver] != path {
					t.Errorf("version %q path = %q, want %q", ver, got[ver], path)
				}
			}
		})
	}
}

func TestGetVersionsComposerNilNode(t *testing.T) {
	a := composerTestAdapter()
	pkg := types.Package{Name: "harness/migtest"}
	_, err := a.GetVersions(pkg, nil, "composer-local", pkg.Name, types.COMPOSER)
	if err == nil {
		t.Fatal("expected error for nil node")
	}
}

func TestGetVersionsComposerUnknownPackage(t *testing.T) {
	a := composerTestAdapter()
	root := composerTestTree()
	pkg := types.Package{Name: "missing/pkg"}
	versions, err := a.GetVersions(pkg, root, "composer-local", pkg.Name, types.COMPOSER)
	if err != nil {
		t.Fatalf("GetVersions: %v", err)
	}
	if len(versions) != 0 {
		t.Fatalf("expected 0 versions, got %+v", versions)
	}
}
