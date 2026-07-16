package harness

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/harness/harness-cli/module/ar/migrate/adapter"
	"github.com/harness/harness-cli/module/ar/migrate/engine"
	"github.com/harness/harness-cli/module/ar/migrate/migratable"
	"github.com/harness/harness-cli/module/ar/migrate/types"

	// Register adapter factories for in-process migration.
	_ "github.com/harness/harness-cli/module/ar/migrate/adapter/har"
	_ "github.com/harness/harness-cli/module/ar/migrate/adapter/jfrog"
	_ "github.com/harness/harness-cli/module/ar/migrate/adapter/mock_jfrog"
	"gopkg.in/yaml.v3"
)

// sourceConfig builds the source block for a spec. It defaults to the MOCK_JFROG
// source with dummy credentials (the offline suite); DOCKER/HELM live tests
// override SourceType/credentials to point at a real OCI source registry.
func sourceConfig(spec Spec) types.RegistryConfig {
	srcType := spec.SourceType
	if srcType == "" {
		srcType = types.MOCK_JFROG
	}
	username := spec.SourceUsername
	if username == "" {
		username = "dummy"
	}
	password := spec.SourcePassword
	if password == "" {
		password = "dummy"
	}
	return types.RegistryConfig{
		Endpoint: spec.sourceEndpoint(),
		Type:     srcType,
		Credentials: types.CredentialsConfig{
			Username: username,
			Password: password,
		},
		Insecure: spec.Insecure,
	}
}

// destConfig builds the HAR destination block. The token is left as
// ${HAR_TOKEN} so it is expanded from the environment when the config loads.
func destConfig(creds Creds) types.RegistryConfig {
	return types.RegistryConfig{
		Endpoint: creds.PkgURL,
		Type:     types.HAR,
		Credentials: types.CredentialsConfig{
			Username: "harness",
			Password: "${HAR_TOKEN}",
		},
	}
}

// mappingFor builds the registry mapping for a single spec.
func mappingFor(spec Spec) types.RegistryMapping {
	return types.RegistryMapping{
		ArtifactType:          types.ArtifactType(spec.ArtifactType),
		SourceRegistry:        spec.SourceRegistry,
		DestinationRegistry:   spec.DestRegistry,
		IncludePatterns:       spec.IncludePatterns,
		ExcludePatterns:       spec.ExcludePatterns,
		SourcePackageHostname: spec.SourcePackageHostname,
		DateFilter:            spec.DateFilter,
	}
}

// WriteConfig renders a temp migration config (MOCK_JFROG source -> HAR
// destination, single mapping) for the given spec and returns its path. The
// destination token is left as ${HAR_TOKEN} so it is expanded when the config
// is loaded.
func WriteConfig(t *testing.T, creds Creds, spec Spec) string {
	t.Helper()

	cfg := types.Config{
		Version:     "1.0.0",
		Concurrency: spec.concurrency(),
		Overwrite:   spec.Overwrite,
		DryRun:      spec.DryRun,
		Summary:     spec.Summary,
		Source:      sourceConfig(spec),
		Dest:        destConfig(creds),
		Mappings:    []types.RegistryMapping{mappingFor(spec)},
	}

	return marshalConfig(t, cfg)
}

// WriteConfigMulti renders a temp migration config with one mapping per spec
// (all sharing a single MOCK_JFROG source and HAR destination block). Top-level
// concurrency/overwrite/dryRun/summary are taken from the first spec. It is used
// by multi-mapping scenarios (2+ artifact types / source registries in one run).
func WriteConfigMulti(t *testing.T, creds Creds, specs []Spec) string {
	t.Helper()

	if len(specs) == 0 {
		t.Fatalf("WriteConfigMulti: at least one spec is required")
	}

	mappings := make([]types.RegistryMapping, 0, len(specs))
	for _, s := range specs {
		mappings = append(mappings, mappingFor(s))
	}

	cfg := types.Config{
		Version:     "1.0.0",
		Concurrency: specs[0].concurrency(),
		Overwrite:   specs[0].Overwrite,
		DryRun:      specs[0].DryRun,
		Summary:     specs[0].Summary,
		Source:      sourceConfig(specs[0]),
		Dest:        destConfig(creds),
		Mappings:    mappings,
	}

	return marshalConfig(t, cfg)
}

// marshalConfig writes a config to a temp file and returns its path.
func marshalConfig(t *testing.T, cfg types.Config) string {
	t.Helper()

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		t.Fatalf("failed to marshal migration config: %v", err)
	}

	path := filepath.Join(t.TempDir(), "migrate.yaml")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("failed to write migration config: %v", err)
	}
	t.Logf("migration config written to %s:\n%s", path, string(data))
	return path
}

// RunMigrate runs migration via the built hc binary from the current branch
// (./hc registry migrate), matching how users invoke the CLI.
//
// Set E2E_IN_PROCESS=1 to run the migratable engine in-process instead, with a
// test-only scoped registry lookup (workaround when prod getRegistry times out).
func RunMigrate(t *testing.T, bin, cfgPath string, creds Creds, spec Spec) {
	t.Helper()

	if os.Getenv("E2E_IN_PROCESS") != "" {
		runMigrateInProcess(t, cfgPath, creds, spec)
		return
	}
	runMigrateSubprocess(t, bin, cfgPath, creds)
}

func runMigrateInProcess(t *testing.T, cfgPath string, creds Creds, spec Spec) {
	t.Helper()

	transferStats := migrateInProcessCore(t, cfgPath, creds)

	t.Logf("migration transferred %d file(s)", len(transferStats.FileStats))
	for _, fs := range transferStats.FileStats {
		t.Logf("  %s: %s", fs.Status, fs.Name)
		if fs.Status == types.StatusFail {
			t.Fatalf("migration file failed: %s — %s", fs.Name, fs.Error)
		}
	}
	if len(transferStats.FileStats) == 0 {
		t.Fatalf("migration completed with zero files transferred")
	}
}

// migrateInProcessCore runs the migratable engine in-process for a single
// mapping and returns the transfer stats WITHOUT asserting per-file success or a
// non-empty result. Callers decide how to interpret the stats. Only genuine
// setup/engine failures fatal the test. A test-only scoped destination adapter
// is used so the pre-step registry lookup stays within the e2e scope.
func migrateInProcessCore(t *testing.T, cfgPath string, creds Creds) types.TransferStats {
	t.Helper()

	ApplyGlobalConfig(creds)

	if err := os.Setenv("HAR_TOKEN", creds.APIKey); err != nil {
		t.Fatalf("failed to set HAR_TOKEN: %v", err)
	}

	cfg, err := types.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("failed to load migration config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	srcAdapter, err := adapter.GetAdapter(ctx, cfg.Source)
	if err != nil {
		t.Fatalf("failed to build source adapter: %v", err)
	}

	destAdapter, err := adapter.GetAdapter(ctx, cfg.Dest)
	if err != nil {
		t.Fatalf("failed to build destination adapter: %v", err)
	}
	destAdapter = newScopedDestAdapter(t, destAdapter, creds)

	if len(cfg.Mappings) != 1 {
		t.Fatalf("in-process migration requires exactly one mapping, got %d", len(cfg.Mappings))
	}
	mapping := cfg.Mappings[0]

	var transferStats types.TransferStats
	transferStats.FileStats = make([]types.FileStat, 0)

	job := migratable.NewRegistryJob(
		srcAdapter,
		destAdapter,
		mapping.SourceRegistry,
		mapping.SourcePackageHostname,
		mapping.DestinationRegistry,
		mapping.ArtifactType,
		&transferStats,
		&mapping,
		cfg,
		nil,
	)

	eng := engine.NewEngine(cfg.Concurrency, []engine.Job{job})
	if err := eng.Execute(ctx); err != nil {
		t.Fatalf("migration engine failed: %v", err)
	}

	return transferStats
}

// MigrateInProcessStats provisions nothing; it runs the in-process migration for
// the given spec and returns the raw per-file transfer stats so failure and
// idempotency scenarios can assert on Status (Success / Skip / Fail) directly —
// something the subprocess path cannot expose because the CLI exits 0 even on
// per-file failures.
func MigrateInProcessStats(t *testing.T, creds Creds, spec Spec) types.TransferStats {
	t.Helper()
	cfgPath := WriteConfig(t, creds, spec)
	return migrateInProcessCore(t, cfgPath, creds)
}

func runMigrateSubprocess(t *testing.T, bin, cfgPath string, creds Creds) {
	t.Helper()
	exitCode, _ := RunMigrateResult(t, bin, cfgPath, creds)
	if exitCode != 0 {
		t.Fatalf("migration command failed with exit code %d", exitCode)
	}
}

// migrateArgs builds the argument list for `hc registry migrate`, passing scope
// flags only when set so account/org-scoped runs work.
func migrateArgs(cfgPath string, creds Creds) []string {
	args := []string{
		"registry", "migrate",
		"-c", cfgPath,
		"--api-url", creds.APIURL,
		"--token", creds.APIKey,
		"--account", creds.AccountID,
		"--pkg-url", creds.PkgURL,
	}
	if creds.OrgID != "" {
		args = append(args, "--org", creds.OrgID)
	}
	if creds.ProjectID != "" {
		args = append(args, "--project", creds.ProjectID)
	}
	if os.Getenv("E2E_VERBOSE") != "" {
		args = append(args, "-v")
	}
	return args
}

// RunMigrateResult runs `hc registry migrate` and returns the process exit code
// and combined output WITHOUT failing the test. It lets scenarios assert the CLI
// contract directly — e.g. exit 0 on partial per-file failures, or a non-zero
// exit on auth/config errors.
func RunMigrateResult(t *testing.T, bin, cfgPath string, creds Creds) (exitCode int, output string) {
	t.Helper()
	return RunMigrateResultDir(t, bin, cfgPath, creds, "")
}

// RunMigrateResultDir behaves like RunMigrateResult but runs the command in
// workDir (when non-empty). Dry-run output files are written relative to the
// process working directory, so scenarios that inspect them pass a temp dir.
func RunMigrateResultDir(t *testing.T, bin, cfgPath string, creds Creds, workDir string) (exitCode int, output string) {
	t.Helper()

	t.Logf("running migration via hc binary: %s", bin)

	cmd := exec.Command(bin, migrateArgs(cfgPath, creds)...)
	cmd.Env = append(os.Environ(), "HAR_TOKEN="+creds.APIKey)
	if workDir != "" {
		cmd.Dir = workDir
	}

	out, err := cmd.CombinedOutput()
	output = string(out)
	t.Logf("hc registry migrate output:\n%s", output)

	if err == nil {
		return 0, output
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), output
	}
	// Failed to start / non-exit error: surface as a fatal test failure since it
	// is not a migration outcome the caller can meaningfully assert on.
	t.Fatalf("failed to run migration command: %v", err)
	return -1, output
}
