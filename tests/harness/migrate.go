package harness

import (
	"context"
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

// WriteConfig renders a temp migration config (MOCK_JFROG source -> HAR
// destination, single mapping) for the given spec and returns its path. The
// destination token is left as ${HAR_TOKEN} so it is expanded when the config
// is loaded.
func WriteConfig(t *testing.T, creds Creds, spec Spec) string {
	t.Helper()

	cfg := types.Config{
		Version:     "1.0.0",
		Concurrency: 1,
		Overwrite:   spec.Overwrite,
		Source: types.RegistryConfig{
			Endpoint: spec.sourceEndpoint(),
			Type:     types.MOCK_JFROG,
			Credentials: types.CredentialsConfig{
				Username: "dummy",
				Password: "dummy",
			},
			Insecure: spec.Insecure,
		},
		Dest: types.RegistryConfig{
			Endpoint: creds.PkgURL,
			Type:     types.HAR,
			Credentials: types.CredentialsConfig{
				Username: "harness",
				Password: "${HAR_TOKEN}",
			},
		},
		Mappings: []types.RegistryMapping{
			{
				ArtifactType:          types.ArtifactType(spec.ArtifactType),
				SourceRegistry:        spec.SourceRegistry,
				DestinationRegistry:   spec.DestRegistry,
				IncludePatterns:       spec.IncludePatterns,
				ExcludePatterns:       spec.ExcludePatterns,
				SourcePackageHostname: spec.SourcePackageHostname,
			},
		},
	}

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
		t.Fatalf("e2e config must have exactly one mapping, got %d", len(cfg.Mappings))
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

func runMigrateSubprocess(t *testing.T, bin, cfgPath string, creds Creds) {
	t.Helper()

	t.Logf("running migration via hc binary: %s", bin)

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

	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "HAR_TOKEN="+creds.APIKey)

	out, err := cmd.CombinedOutput()
	t.Logf("hc registry migrate output:\n%s", string(out))
	if err != nil {
		t.Fatalf("migration command failed: %v", err)
	}
}
