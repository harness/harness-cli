package mock_jfrog

import (
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/adapter/jfrog"
	"github.com/harness/harness-cli/module/ar/migrate/tree"
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

const terraformRegistry = "terraform-local"

func TestGetPackagesTerraform(t *testing.T) {
	adapter := jfrog.NewAdapterWithClient(types.RegistryConfig{Type: types.MOCK_JFROG}, NewMockClient())

	files, err := adapter.GetFiles(terraformRegistry)
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	root := tree.TransformToTree(files)

	pkgs, err := adapter.GetPackages(terraformRegistry, types.TERRAFORM, root)
	if err != nil {
		t.Fatalf("GetPackages: %v", err)
	}

	pkgSet := make(map[string]bool)
	for _, p := range pkgs {
		pkgSet[p.Name] = true
	}

	// hashicorp/vpc/aws = module; hashicorp/aws = provider
	expected := map[string]bool{
		"hashicorp/vpc/aws": true,
		"hashicorp/aws":     true,
	}

	if len(pkgSet) != len(expected) {
		t.Fatalf("expected %d packages, got %d: %+v", len(expected), len(pkgSet), pkgs)
	}
	for pkg := range expected {
		if !pkgSet[pkg] {
			t.Errorf("missing expected package %q", pkg)
		}
	}
}

func TestGetVersionsTerraformModule(t *testing.T) {
	adapter := jfrog.NewAdapterWithClient(types.RegistryConfig{Type: types.MOCK_JFROG}, NewMockClient())

	files, err := adapter.GetFiles(terraformRegistry)
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	root := tree.TransformToTree(files)

	pkg := types.Package{Name: "hashicorp/vpc/aws"}
	versions, err := adapter.GetVersions(pkg, root, terraformRegistry, "hashicorp/vpc/aws", types.TERRAFORM)
	if err != nil {
		t.Fatalf("GetVersions: %v", err)
	}

	versionSet := make(map[string]bool)
	for _, v := range versions {
		versionSet[v.Name] = true
	}

	expected := map[string]bool{"1.0.0": true, "1.1.0": true}
	if len(versionSet) != len(expected) {
		t.Fatalf("expected %d versions, got %d: %+v", len(expected), len(versionSet), versions)
	}
	for v := range expected {
		if !versionSet[v] {
			t.Errorf("missing expected version %q", v)
		}
	}
}

func TestGetVersionsTerraformProvider(t *testing.T) {
	adapter := jfrog.NewAdapterWithClient(types.RegistryConfig{Type: types.MOCK_JFROG}, NewMockClient())

	files, err := adapter.GetFiles(terraformRegistry)
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	root := tree.TransformToTree(files)

	pkg := types.Package{Name: "hashicorp/aws"}
	versions, err := adapter.GetVersions(pkg, root, terraformRegistry, "hashicorp/aws", types.TERRAFORM)
	if err != nil {
		t.Fatalf("GetVersions: %v", err)
	}

	versionSet := make(map[string]bool)
	for _, v := range versions {
		versionSet[v.Name] = true
	}

	expected := map[string]bool{"2.0.0": true}
	if len(versionSet) != len(expected) {
		t.Fatalf("expected %d versions, got %d: %+v", len(expected), len(versionSet), versions)
	}
	for v := range expected {
		if !versionSet[v] {
			t.Errorf("missing expected version %q", v)
		}
	}
}
