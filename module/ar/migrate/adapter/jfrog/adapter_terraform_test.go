package jfrog

import (
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/tree"
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

func makeFiles(uris ...string) []types.File {
	files := make([]types.File, len(uris))
	for i, uri := range uris {
		parts := splitPath(uri)
		name := parts[len(parts)-1]
		files[i] = types.File{Name: name, Uri: uri, Folder: false}
	}
	return files
}

func splitPath(uri string) []string {
	var parts []string
	cur := ""
	for _, c := range uri {
		if c == '/' {
			if cur != "" {
				parts = append(parts, cur)
				cur = ""
			}
		} else {
			cur += string(c)
		}
	}
	if cur != "" {
		parts = append(parts, cur)
	}
	return parts
}

func TestGetPackagesTerraformDirect(t *testing.T) {
	files := makeFiles(
		"/hashicorp/vpc/aws/1.0.0/vpc-1.0.0.tar.gz",
		"/hashicorp/vpc/aws/1.1.0/vpc-1.1.0.tar.gz",
		"/hashicorp/aws/2.0.0/terraform-provider-aws_2.0.0_linux_amd64.zip",
		"/hashicorp/aws/2.0.0/terraform-provider-aws_2.0.0_darwin_arm64.zip",
	)
	root := tree.TransformToTree(files)

	a := &adapter{}
	pkgs, err := a.GetPackages("terraform-local", types.TERRAFORM, root)
	if err != nil {
		t.Fatalf("GetPackages: %v", err)
	}

	pkgSet := make(map[string]bool)
	for _, p := range pkgs {
		pkgSet[p.Name] = true
	}

	expected := map[string]bool{
		"hashicorp/vpc/aws": true,
		"hashicorp/aws":     true,
	}
	if len(pkgSet) != len(expected) {
		t.Fatalf("got %d packages %v, want %d", len(pkgSet), pkgSet, len(expected))
	}
	for k := range expected {
		if !pkgSet[k] {
			t.Errorf("missing package %q", k)
		}
	}
}

func TestGetPackagesTerraformDeduplicates(t *testing.T) {
	// Two files for the same module package — should still emit one package.
	files := makeFiles(
		"/hashicorp/vpc/aws/1.0.0/vpc-1.0.0.tar.gz",
		"/hashicorp/vpc/aws/1.1.0/vpc-1.1.0.tar.gz",
	)
	root := tree.TransformToTree(files)

	a := &adapter{}
	pkgs, err := a.GetPackages("terraform-local", types.TERRAFORM, root)
	if err != nil {
		t.Fatalf("GetPackages: %v", err)
	}
	if len(pkgs) != 1 || pkgs[0].Name != "hashicorp/vpc/aws" {
		t.Errorf("got %v, want [hashicorp/vpc/aws]", pkgs)
	}
}

func TestGetPackagesTerraformEmpty(t *testing.T) {
	root := tree.TransformToTree(nil)
	a := &adapter{}
	pkgs, err := a.GetPackages("terraform-local", types.TERRAFORM, root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pkgs) != 0 {
		t.Errorf("expected 0 packages, got %v", pkgs)
	}
}

func TestGetVersionsTerraformModule(t *testing.T) {
	files := makeFiles(
		"/hashicorp/vpc/aws/1.0.0/vpc-1.0.0.tar.gz",
		"/hashicorp/vpc/aws/1.1.0/vpc-1.1.0.tar.gz",
		// Different module — should not appear.
		"/hashicorp/other/aws/1.0.0/other-1.0.0.tar.gz",
	)
	root := tree.TransformToTree(files)
	pkg := types.Package{Name: "hashicorp/vpc/aws"}

	a := &adapter{}
	versions, err := a.GetVersions(pkg, root, "terraform-local", "hashicorp/vpc/aws", types.TERRAFORM)
	if err != nil {
		t.Fatalf("GetVersions: %v", err)
	}

	vSet := make(map[string]bool)
	for _, v := range versions {
		vSet[v.Name] = true
	}
	expected := map[string]bool{"1.0.0": true, "1.1.0": true}
	if len(vSet) != len(expected) {
		t.Fatalf("got %v, want %v", vSet, expected)
	}
	for k := range expected {
		if !vSet[k] {
			t.Errorf("missing version %q", k)
		}
	}
}

func TestGetVersionsTerraformProvider(t *testing.T) {
	files := makeFiles(
		"/hashicorp/aws/2.0.0/terraform-provider-aws_2.0.0_linux_amd64.zip",
		"/hashicorp/aws/2.0.0/terraform-provider-aws_2.0.0_darwin_arm64.zip",
		// Different provider — should not appear.
		"/hashicorp/google/3.0.0/terraform-provider-google_3.0.0_linux_amd64.zip",
	)
	root := tree.TransformToTree(files)
	pkg := types.Package{Name: "hashicorp/aws"}

	a := &adapter{}
	versions, err := a.GetVersions(pkg, root, "terraform-local", "hashicorp/aws", types.TERRAFORM)
	if err != nil {
		t.Fatalf("GetVersions: %v", err)
	}

	if len(versions) != 1 || versions[0].Name != "2.0.0" {
		t.Errorf("got %v, want [2.0.0]", versions)
	}
}

func TestGetVersionsTerraformEmpty(t *testing.T) {
	root := tree.TransformToTree(nil)
	pkg := types.Package{Name: "hashicorp/vpc/aws"}

	a := &adapter{}
	versions, err := a.GetVersions(pkg, root, "terraform-local", "hashicorp/vpc/aws", types.TERRAFORM)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(versions) != 0 {
		t.Errorf("expected 0 versions, got %v", versions)
	}
}
