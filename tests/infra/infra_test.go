//go:build e2e

// Package infra contains end-to-end tests for cross-cutting migration
// infrastructure: registry scoping, auth, config knobs (concurrency, dryRun,
// summary, overwrite), multi-mapping configs, token expansion, and the CLI
// exit-code contract. They use a small, deterministic source (raw/generic) so
// the assertions stay focused on infrastructure rather than a specific format.
package infra

import (
	"testing"

	"github.com/harness/harness-cli/tests/harness"
)

// genericSpec returns a minimal GENERIC spec with a unique destination.
func genericSpec(t *testing.T, prefix string) harness.Spec {
	t.Helper()
	return harness.Spec{
		ArtifactType:   "GENERIC",
		PackageType:    "GENERIC",
		SourceRegistry: "generic-local",
		DestRegistry:   harness.UniqueRegistry(t, prefix),
		ExpectedRawURIs: []string{
			"default/default/data/config.json",
			"default/default/bin/run.sh",
		},
	}
}

// TestScopeProject migrates into a project-scoped destination registry
// (account/org/project/identifier) — the default and recommended scope.
func TestScopeProject(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)
	harness.RunAtScope(t, bin, creds, genericSpec(t, "scopeproj"), harness.ScopeProject)
}

// TestScopeAccount migrates into an account-only registry (no org/project). It
// verifies the CLI and management API accept an account-scoped registry ref.
func TestScopeAccount(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)
	harness.RunAtScope(t, bin, creds, genericSpec(t, "scopeacct"), harness.ScopeAccount)
}

// TestScopeOrg migrates into an org-scoped registry (account/org, no project).
func TestScopeOrg(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)
	harness.RunAtScope(t, bin, creds, genericSpec(t, "scopeorg"), harness.ScopeOrg)
}

// TestMultipleMappings migrates two source registries (RAW + GENERIC) of two
// artifact types in a single config with two mappings, and reconciles each
// destination independently.
func TestMultipleMappings(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	rawSpec := harness.Spec{
		ArtifactType:    "RAW",
		PackageType:     "GENERIC",
		SourceRegistry:  "raw-local",
		DestRegistry:    harness.UniqueRegistry(t, "multiraw"),
		IncludePatterns: []string{"configs/**", "assets/**"},
		ExpectedRawURIs: []string{"configs/v1/config.yaml", "assets/images/logo.png"},
	}
	genSpec := genericSpec(t, "multigen")

	harness.RunMulti(t, bin, creds, rawSpec, genSpec)
}

// TestConcurrencyN migrates a multi-version package with concurrency > 1 and
// verifies all versions still land intact (no corruption / lost updates).
func TestConcurrencyN(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.Run(t, bin, creds, harness.Spec{
		ArtifactType:   "PYTHON",
		PackageType:    "PYTHON",
		SourceRegistry: "python-local",
		DestRegistry:   harness.UniqueRegistry(t, "concurrency"),
		Concurrency:    4,
		ExpectedFiles: []harness.ExpectedFile{
			{Pkg: "requests", Version: "2.28.0"},
			{Pkg: "requests", Version: "2.29.0"},
		},
	})
}

// TestDryRun runs with dryRun enabled and asserts the CLI produces its output
// files while uploading nothing to the destination.
func TestDryRun(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.RunDryRun(t, bin, creds, harness.Spec{
		ArtifactType:       "RAW",
		PackageType:        "GENERIC",
		SourceRegistry:     "raw-local",
		DestRegistry:       harness.UniqueRegistry(t, "dryrun"),
		IncludePatterns:    []string{"configs/**"},
		NotExpectedRawURIs: []string{"configs/v1/config.yaml"},
	})
}

// TestSummary runs with summary output enabled and verifies the migration still
// completes and reconciles (summary changes only the console output, not the
// upload behavior).
func TestSummary(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	spec := genericSpec(t, "summary")
	spec.Summary = true
	harness.Run(t, bin, creds, spec)
}

// TestTokenExpansion exercises the default subprocess path, where the config's
// destination password is ${HAR_TOKEN} and must be expanded from the
// environment for uploads to authenticate.
func TestTokenExpansion(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)
	harness.Run(t, bin, creds, genericSpec(t, "tokenexp"))
}

// TestInvalidPAT asserts that provisioning with an invalid token is rejected by
// the management API (auth failure on registry create).
func TestInvalidPAT(t *testing.T) {
	creds := harness.RequireEnv(t)
	harness.ExpectAuthFailureOnCreate(t, creds, harness.UniqueRegistry(t, "badpat"), "GENERIC")
}

// TestExitZeroOnPartialFailure documents the CLI contract: a per-file failure
// (a shallow RAW path rejected with 400) does NOT fail the process; the CLI
// still exits 0. The reconcile gate is what distinguishes the landed deep file
// from the rejected shallow one.
func TestExitZeroOnPartialFailure(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.ApplyGlobalConfig(creds)
	harness.EnsureProject(t, creds)

	spec := harness.Spec{
		ArtifactType:       "RAW",
		PackageType:        "GENERIC",
		SourceRegistry:     "raw-local",
		DestRegistry:       harness.UniqueRegistry(t, "partialfail"),
		ExpectedRawURIs:    []string{"configs/v1/config.yaml"},
		NotExpectedRawURIs: []string{"docs/readme.txt"},
	}

	ref := harness.CreateRegistry(t, creds, spec.DestRegistry, spec.PackageType)
	t.Cleanup(func() { harness.DeleteRegistry(t, creds, ref) })

	cfgPath := harness.WriteConfig(t, creds, spec)
	code, _ := harness.RunMigrateResult(t, bin, cfgPath, creds)
	if code != 0 {
		t.Fatalf("expected CLI exit 0 on partial per-file failure, got %d", code)
	}

	harness.Reconcile(t, creds, spec)
	harness.ReconcileAbsent(t, creds, spec)
}
