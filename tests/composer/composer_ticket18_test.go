//go:build e2e

package composer

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/harness/harness-cli/module/ar/migrate/adapter"
	"github.com/harness/harness-cli/module/ar/migrate/tree"
	"github.com/harness/harness-cli/module/ar/migrate/types"
	"github.com/harness/harness-cli/tests/harness"

	_ "github.com/harness/harness-cli/module/ar/migrate/adapter/mock_jfrog"
)

// TestMigrateComposerLogicalPackageMissesVersions reproduces ticket #18 for a
// customer migrating acme/billing-sdk-style Composer packages from Artifactory.
//
// The mock composer-logical-local registry holds vendor/package at 1.0.0, 2.0.0,
// and 3.0.0, but GetPackages returns ONE logical package row. GetVersions does
// not scan the tree for sibling zips — it echoes that single row — so migration
// uploads only the first zip. HAR ends up with 1.0.0 only; 2.0.0 and 3.0.0 are
// missing despite a successful-looking migrate.
//
// This test is expected to FAIL until GetVersions scans for all versions (or
// GetPackages emits one job per zip again).
//
// Run after exporting QA creds and mock fixtures:
//
//	make mock-init
//	go test -tags=e2e ./tests/composer/ -run TestMigrateComposerLogicalPackageMissesVersions -v -count=1
func TestMigrateComposerLogicalPackageMissesVersions(t *testing.T) {
	t.Helper()

	creds := harness.RequireEnv(t)
	harness.ApplyGlobalConfig(creds)
	harness.EnsureProject(t, creds)

	const sourceRegistry = "composer-logical-local"
	const logicalPkg = "vendor/package"

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

	zipCount := 0
	for _, f := range files {
		if !f.Folder && strings.HasSuffix(f.Uri, ".zip") {
			zipCount++
		}
	}
	if zipCount < 3 {
		t.Fatalf("fixture: expected at least 3 composer zips in %s, got %d", sourceRegistry, zipCount)
	}
	t.Logf("source registry has %d version zip(s) for logical package %q", zipCount, logicalPkg)

	pkgs, err := srcAdapter.GetPackages(sourceRegistry, types.COMPOSER, root)
	if err != nil {
		t.Fatalf("GetPackages: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("ticket-18 setup: expected 1 logical package row, got %d: %+v", len(pkgs), pkgs)
	}
	t.Logf("GetPackages returned 1 logical row: name=%q url=%q", pkgs[0].Name, pkgs[0].URL)

	versions, err := srcAdapter.GetVersions(pkgs[0], root, sourceRegistry, logicalPkg, types.COMPOSER)
	if err != nil {
		t.Fatalf("GetVersions: %v", err)
	}
	t.Logf("GetVersions returned %d version(s) (want scan of all %d zips)", len(versions), zipCount)
	for _, v := range versions {
		t.Logf("  version name=%q path=%q", v.Name, v.Path)
	}
	if len(versions) != 1 {
		t.Fatalf("ticket-18 setup: GetVersions should currently return exactly 1 stub version, got %d", len(versions))
	}

	spec := harness.Spec{
		ArtifactType:    "COMPOSER",
		PackageType:     "COMPOSER",
		SourceRegistry:  sourceRegistry,
		DestRegistry:    "composer_ticket18_validate",
		IncludePatterns: []string{logicalPkg},
		ExpectedFiles: []harness.ExpectedFile{
			{Pkg: logicalPkg, Version: "1.0.0"},
			{Pkg: logicalPkg, Version: "2.0.0"},
			{Pkg: logicalPkg, Version: "3.0.0"},
		},
	}

	ref := harness.CreateRegistry(t, creds, spec.DestRegistry, spec.PackageType)
	t.Logf("validation run: project=%s/%s dest_registry=%s registry_ref=%s",
		creds.OrgID, creds.ProjectID, spec.DestRegistry, ref)
	t.Logf("registry left in HAR for manual inspection (not deleted after test)")

	stats := harness.MigrateInProcessStats(t, creds, spec)
	if len(stats.FileStats) != 1 {
		t.Fatalf("expected exactly 1 package upload job (first zip only), got %d file stats: %+v",
			len(stats.FileStats), stats.FileStats)
	}
	for _, fs := range stats.FileStats {
		t.Logf("  file stat: name=%q uri=%q size=%d status=%s", fs.Name, fs.Uri, fs.Size, fs.Status)
	}

	// Fails today: only vendor/package@1.0.0 lands in HAR; 2.0.0 and 3.0.0 missing.
	harness.Reconcile(t, creds, spec)
}
