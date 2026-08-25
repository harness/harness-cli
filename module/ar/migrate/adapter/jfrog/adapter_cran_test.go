package jfrog

import (
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/tree"
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

func TestGetPackagesCRANDirect(t *testing.T) {
	files := makeFiles(
		"/src/contrib/jsonlite_1.8.0.tar.gz",
		"/src/contrib/Archive/jsonlite/1.7.0/jsonlite_1.7.0.tar.gz",
		"/src/contrib/PACKAGES",
		"/src/contrib/PACKAGES.gz",
		"/bin/windows/contrib/4.4/jsonlite_1.8.0.zip",
		"/bin/big-sur-arm64/contrib/4.4/data.table_1.14.0.tgz",
		"/unrelated/readme.txt",
	)
	root := tree.TransformToTree(files)

	a := &adapter{}
	pkgs, err := a.GetPackages("cran-local", types.CRAN, root)
	if err != nil {
		t.Fatalf("GetPackages: %v", err)
	}

	byName := make(map[string]bool)
	for _, p := range pkgs {
		byName[p.Name] = true
		if p.Name == "PACKAGES" || p.Name == "PACKAGES.gz" {
			t.Errorf("index file enumerated as package: %+v", p)
		}
	}
	if len(byName) != 2 {
		t.Fatalf("expected 2 packages, got %d: %v", len(byName), byName)
	}
	for _, name := range []string{"jsonlite", "data.table"} {
		if !byName[name] {
			t.Errorf("missing package %q", name)
		}
	}
}

func TestGetPackagesCRANEmpty(t *testing.T) {
	root := tree.TransformToTree(nil)
	a := &adapter{}
	pkgs, err := a.GetPackages("cran-local", types.CRAN, root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pkgs) != 0 {
		t.Errorf("expected 0 packages, got %v", pkgs)
	}
}

func TestGetPackagesCRANSkipsFoldersAndIndexes(t *testing.T) {
	files := []types.File{
		{Name: "contrib", Uri: "/src/contrib", Folder: true},
		{Name: "PACKAGES", Uri: "/src/contrib/PACKAGES", Folder: false},
		{Name: "jsonlite_1.8.0.tar.gz", Uri: "/src/contrib/jsonlite_1.8.0.tar.gz", Folder: false},
	}
	root := tree.TransformToTree(files)
	a := &adapter{}
	pkgs, err := a.GetPackages("cran-local", types.CRAN, root)
	if err != nil {
		t.Fatalf("GetPackages: %v", err)
	}
	if len(pkgs) != 1 || pkgs[0].Name != "jsonlite" {
		t.Errorf("got %v, want [jsonlite]", pkgs)
	}
}
