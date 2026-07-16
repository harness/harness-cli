//go:build e2e

// Package python contains the end-to-end migration test for Python artifacts:
// MOCK_JFROG python-local -> HAR PYTHON registry.
package python

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/harness/harness-cli/module/ar/migrate/adapter"
	"github.com/harness/harness-cli/module/ar/migrate/tree"
	"github.com/harness/harness-cli/module/ar/migrate/types"
	"github.com/harness/harness-cli/tests/harness"

	_ "github.com/harness/harness-cli/module/ar/migrate/adapter/mock_jfrog"
)

// TestMigratePython migrates the python-local "requests" sdists and verifies
// both versions are present in the destination. The destination package
// name/version are read by the migration from each sdist's PKG-INFO, which the
// fixtures set to requests / 2.28.0 and 2.29.0.
func TestMigratePython(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.Run(t, bin, creds, harness.Spec{
		ArtifactType:   "PYTHON",
		PackageType:    "PYTHON",
		SourceRegistry: "python-local",
		DestRegistry:   harness.UniqueRegistry(t, "python"),
		ExpectedFiles: []harness.ExpectedFile{
			{Pkg: "requests", Version: "2.28.0"},
			{Pkg: "requests", Version: "2.29.0"},
		},
	})
}

// TestMigratePythonValidateTicket11 reproduces ticket-11 against python-local
// (simple.html href="requests/"), migrates to HARNESS_PROJECT_ID, and leaves the
// destination registry for UI inspection.
//
// Run after exporting QA creds:
//
//	go test -tags=e2e ./tests/python/ -run TestMigratePythonValidateTicket11 -v -count=1
func TestMigratePythonValidateTicket11(t *testing.T) {
	t.Helper()

	creds := harness.RequireEnv(t)
	harness.ApplyGlobalConfig(creds)
	harness.EnsureProject(t, creds)

	const sourceRegistry = "python-local"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	srcAdapter, err := adapter.GetAdapter(ctx, types.RegistryConfig{
		Endpoint: "http://mock-jfrog.local",
		Type:     types.MOCK_JFROG,
		Credentials: types.CredentialsConfig{
			Username: "dummy",
			Password: "dummy",
		},
	})
	if err != nil {
		t.Fatalf("build source adapter: %v", err)
	}

	files, err := srcAdapter.GetFiles(sourceRegistry)
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	root := tree.TransformToTree(files)

	pkgs, err := srcAdapter.GetPackages(sourceRegistry, types.PYTHON, root)
	if err != nil {
		t.Fatalf("GetPackages: %v", err)
	}
	if len(pkgs) == 0 {
		t.Fatal("expected at least one PYTHON package from simple.html")
	}

	for _, pkg := range pkgs {
		indexPath := fmt.Sprintf(".pypi/%s/%s.html", pkg.Name, pkg.Name)
		t.Logf("enumerated package: name=%q indexPath=%q", pkg.Name, indexPath)

		if strings.Contains(indexPath, "//") {
			t.Logf("TICKET-11 OBSERVED: broken index path contains '//' (href trailing slash not trimmed)")
		}
		if strings.HasSuffix(pkg.Name, "/") {
			t.Logf("TICKET-11 OBSERVED: package name retains trailing slash from href %q", pkg.Name)
		}

		versions, err := srcAdapter.GetVersions(pkg, root, sourceRegistry, pkg.Name, types.PYTHON)
		if err != nil {
			t.Fatalf("GetVersions(%q): %v", pkg.Name, err)
		}
		t.Logf("  versions discovered: %d (index miss falls back to file tree for indexed repos)", len(versions))
		for _, v := range versions {
			t.Logf("    version=%s path=%s", v.Name, v.Path)
		}
	}

	spec := harness.Spec{
		ArtifactType:   "PYTHON",
		PackageType:    "PYTHON",
		SourceRegistry: sourceRegistry,
		// Fixed name so the registry is easy to find in the Harness UI after the test.
		DestRegistry: "python_ticket11_validate",
		ExpectedFiles: []harness.ExpectedFile{
			{Pkg: "requests", Version: "2.28.0"},
			{Pkg: "requests", Version: "2.29.0"},
		},
	}

	ref := harness.CreateRegistry(t, creds, spec.DestRegistry, spec.PackageType)
	t.Logf("validation run: project=%s/%s dest_registry=%s registry_ref=%s",
		creds.OrgID, creds.ProjectID, spec.DestRegistry, ref)
	t.Logf("registry left in HAR for manual inspection (not deleted after test)")

	stats := harness.MigrateInProcessStats(t, creds, spec)
	for _, fs := range stats.FileStats {
		t.Logf("  file stat: name=%q uri=%q size=%d status=%s", fs.Name, fs.Uri, fs.Size, fs.Status)
	}

	harness.Reconcile(t, creds, spec)
	t.Logf("HAR reconcile passed: requests@2.28.0 and requests@2.29.0 indexed from PKG-INFO, not from href %q",
		pkgs[0].Name)
}
