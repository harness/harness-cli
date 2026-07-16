package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/types"
)

// provisionDest creates the destination registry for a spec and schedules its
// cleanup, returning the fully qualified registry reference.
func provisionDest(t *testing.T, creds Creds, spec Spec) string {
	t.Helper()
	ref := CreateRegistry(t, creds, spec.DestRegistry, spec.PackageType)
	t.Cleanup(func() { DeleteRegistry(t, creds, ref) })
	return ref
}

// RunAtScope provisions the destination at the requested Harness scope
// (account/org/project), migrates via the built binary, and reconciles. It backs
// the infrastructure scope matrix (account-only, org-only, project-scoped).
func RunAtScope(t *testing.T, bin string, creds Creds, spec Spec, scope ScopeLevel) {
	t.Helper()

	scoped := creds.AtScope(scope)
	ApplyGlobalConfig(scoped)

	// A project-scoped registry needs its project to exist first; account/org
	// scopes provision directly under the account/org.
	if scope == ScopeProject {
		EnsureProject(t, scoped)
	}

	provisionDest(t, scoped, spec)

	cfgPath := WriteConfig(t, scoped, spec)
	RunMigrate(t, bin, cfgPath, scoped, spec)

	Reconcile(t, scoped, spec)
}

// RunExpectAbsent provisions the destination, migrates, and asserts the spec's
// NotExpected* groups are ABSENT afterward. It backs exclude-pattern and
// dest-mismatch scenarios, where the CLI may still exit 0 but nothing (or only a
// subset) should have landed.
func RunExpectAbsent(t *testing.T, bin string, creds Creds, spec Spec) {
	t.Helper()

	ApplyGlobalConfig(creds)
	EnsureProject(t, creds)
	provisionDest(t, creds, spec)

	cfgPath := WriteConfig(t, creds, spec)
	RunMigrate(t, bin, cfgPath, creds, spec)

	// Positive expectations (if any) must still hold — e.g. include+exclude
	// where some files are kept and others dropped.
	if len(spec.ExpectedFiles) > 0 || len(spec.ExpectedRawURIs) > 0 || len(spec.ExpectedTags) > 0 {
		Reconcile(t, creds, spec)
	}
	ReconcileAbsent(t, creds, spec)
}

// RunExpectZeroFiles provisions the destination, runs the migration in-process,
// and asserts that zero files were selected (filters removed everything). It
// also reconciles that the spec's NotExpected* artifacts are absent, so a
// no-op migration is never mistaken for success.
func RunExpectZeroFiles(t *testing.T, creds Creds, spec Spec) {
	t.Helper()

	ApplyGlobalConfig(creds)
	EnsureProject(t, creds)
	provisionDest(t, creds, spec)

	stats := MigrateInProcessStats(t, creds, spec)
	if n := len(stats.FileStats); n != 0 {
		t.Errorf("expected zero files migrated, got %d: %s", n, statusSummary(stats))
	}
	ReconcileAbsent(t, creds, spec)
}

// RunMulti provisions a destination per spec, migrates them all in a single
// multi-mapping config, and reconciles each. It backs the "multiple mappings in
// one config" infrastructure scenario.
func RunMulti(t *testing.T, bin string, creds Creds, specs ...Spec) {
	t.Helper()

	if len(specs) == 0 {
		t.Fatalf("RunMulti: at least one spec is required")
	}

	ApplyGlobalConfig(creds)
	EnsureProject(t, creds)
	for _, s := range specs {
		provisionDest(t, creds, s)
	}

	cfgPath := WriteConfigMulti(t, creds, specs)
	if code, _ := RunMigrateResult(t, bin, cfgPath, creds); code != 0 {
		t.Fatalf("multi-mapping migration exited %d", code)
	}

	for _, s := range specs {
		Reconcile(t, creds, s)
	}
}

// RunIdempotent provisions the destination and runs the migration in-process
// twice. The first run must land the artifacts; the second run's per-file
// statuses are asserted against the overwrite setting:
//   - Overwrite=false: the second run must Skip (no Fail), proving idempotency.
//   - Overwrite=true:  the second run must re-process (no Fail).
//
// Both runs are followed by a positive reconcile so a corrupt re-run is caught.
func RunIdempotent(t *testing.T, creds Creds, spec Spec) {
	t.Helper()

	ApplyGlobalConfig(creds)
	EnsureProject(t, creds)
	provisionDest(t, creds, spec)

	first := MigrateInProcessStats(t, creds, spec)
	if n := len(first.FileStats); n == 0 {
		t.Fatalf("idempotency: first run migrated zero files")
	}
	assertNoFailures(t, "first run", first)
	Reconcile(t, creds, spec)

	second := MigrateInProcessStats(t, creds, spec)
	assertNoFailures(t, "second run", second)
	if !spec.Overwrite {
		if countStatus(second, types.StatusSkip) == 0 {
			t.Errorf("idempotency (overwrite=false): expected some Skip statuses on re-run, got %s", statusSummary(second))
		}
	}
	Reconcile(t, creds, spec)
}

// RunDryRun runs the migration with dryRun enabled in an isolated working
// directory, asserts the CLI exited 0 and produced its file-list / directory
// structure output, and confirms nothing was uploaded (NotExpected* absent).
func RunDryRun(t *testing.T, bin string, creds Creds, spec Spec) {
	t.Helper()

	ApplyGlobalConfig(creds)
	EnsureProject(t, creds)
	provisionDest(t, creds, spec)

	spec.DryRun = true
	cfgPath := WriteConfig(t, creds, spec)

	workDir := t.TempDir()
	code, _ := RunMigrateResultDir(t, bin, cfgPath, creds, workDir)
	if code != 0 {
		t.Fatalf("dry-run migration exited %d", code)
	}

	outDir := filepath.Join(workDir, "dry-run-output")
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("dry-run: expected output dir %q: %v", outDir, err)
	}
	var haveFileList, haveDirStruct bool
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "file_list_") {
			haveFileList = true
		}
		if strings.HasPrefix(e.Name(), "directory_structure_") {
			haveDirStruct = true
		}
	}
	if !haveFileList || !haveDirStruct {
		t.Errorf("dry-run: missing output files in %q (file_list=%v dir_struct=%v)", outDir, haveFileList, haveDirStruct)
	}

	// Dry-run must not upload anything.
	if len(spec.NotExpectedFiles) > 0 || len(spec.NotExpectedRawURIs) > 0 || len(spec.NotExpectedTags) > 0 {
		ReconcileAbsent(t, creds, spec)
	}
}

// ExpectAuthFailureOnCreate asserts that the management API rejects a registry
// create performed with an invalid token. It backs the "invalid / expired PAT"
// scenario: with a bad credential, provisioning must not succeed.
func ExpectAuthFailureOnCreate(t *testing.T, creds Creds, identifier, packageType string) {
	t.Helper()

	bad := creds
	bad.APIKey = creds.APIKey + "_invalid_suffix"

	status, body := tryCreateRegistry(t, bad, identifier, packageType)
	switch status {
	case 401, 403:
		t.Logf("registry create rejected as expected with invalid token: status %d", status)
	case 201:
		t.Cleanup(func() { DeleteRegistry(t, creds, creds.registryRef(identifier)) })
		t.Errorf("registry create unexpectedly succeeded with an invalid token")
	default:
		t.Logf("registry create with invalid token returned status %d: %s (accepted as non-success)", status, body)
		if status >= 200 && status < 300 {
			t.Errorf("registry create with invalid token returned success status %d", status)
		}
	}
}

// FileStatByURI returns the first file stat whose Uri or Name contains substr,
// so failure tests can assert on a specific file's outcome.
func FileStatByURI(stats types.TransferStats, substr string) (types.FileStat, bool) {
	for _, fs := range stats.FileStats {
		if strings.Contains(fs.Uri, substr) || strings.Contains(fs.Name, substr) {
			return fs, true
		}
	}
	return types.FileStat{}, false
}

// CountStatus returns how many file stats have the given status (exported).
func CountStatus(stats types.TransferStats, status types.Status) int {
	return countStatus(stats, status)
}

// assertNoFailures fails the test if any file stat has StatusFail.
func assertNoFailures(t *testing.T, label string, stats types.TransferStats) {
	t.Helper()
	for _, fs := range stats.FileStats {
		if fs.Status == types.StatusFail {
			t.Errorf("%s: file %q failed: %s", label, fs.Name, fs.Error)
		}
	}
}

// countStatus returns how many file stats have the given status.
func countStatus(stats types.TransferStats, status types.Status) int {
	n := 0
	for _, fs := range stats.FileStats {
		if fs.Status == status {
			n++
		}
	}
	return n
}

// statusSummary renders a compact per-status count for logging.
func statusSummary(stats types.TransferStats) string {
	return fmt.Sprintf("[success=%d skip=%d fail=%d total=%d]",
		countStatus(stats, types.StatusSuccess),
		countStatus(stats, types.StatusSkip),
		countStatus(stats, types.StatusFail),
		len(stats.FileStats),
	)
}
