package migrate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/harness/harness-cli/internal/api/ar"
	"github.com/harness/harness-cli/module/ar/migrate/adapter"
	"github.com/harness/harness-cli/module/ar/migrate/engine"
	"github.com/harness/harness-cli/module/ar/migrate/migratable"
	"github.com/harness/harness-cli/module/ar/migrate/types"
	"github.com/harness/harness-cli/util/common/printer"

	"github.com/pterm/pterm"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	_ "github.com/harness/harness-cli/module/ar/migrate/adapter/har"
	_ "github.com/harness/harness-cli/module/ar/migrate/adapter/harbor"
	_ "github.com/harness/harness-cli/module/ar/migrate/adapter/jfrog"
	_ "github.com/harness/harness-cli/module/ar/migrate/adapter/mock_jfrog"
	_ "github.com/harness/harness-cli/module/ar/migrate/adapter/nexus"
)

// MigrationService handles the migration process
type MigrationService struct {
	config      *types.Config
	apiClient   *ar.Client
	source      adapter.Adapter
	destination adapter.Adapter
	dryRunStats *types.DryRunStats
}

// NewMigrationService creates a new migration service
func NewMigrationService(ctx context.Context, cfg *types.Config, apiClient *ar.Client) (*MigrationService, error) {
	sourceAdapter, err := adapter.GetAdapter(ctx, cfg.Source)
	if err != nil {
		return nil, fmt.Errorf("failed to get source adapter: %v", err)
	}
	destAdapter, err := adapter.GetAdapter(ctx, cfg.Dest)
	if err != nil {
		return nil, fmt.Errorf("failed to get destination adapter: %v", err)
	}

	svc := &MigrationService{
		config:      cfg,
		apiClient:   apiClient,
		source:      sourceAdapter,
		destination: destAdapter,
	}

	if cfg.DryRun {
		svc.dryRunStats = &types.DryRunStats{
			Files:       make([]types.DryRunFileEntry, 0),
			Directories: make(map[string]*types.DryRunDirectoryEntry),
		}
	}

	return svc, nil
}

// Run executes the migration process
func (m *MigrationService) Run(ctx context.Context) error {
	logger := log.With().
		Str("source_type", string(m.config.Source.Type)).
		Str("destination_type", string(m.config.Dest.Type)).
		Logger()

	logger.Info().Msg("Starting migration process")

	var jobs []engine.Job
	var transferStats types.TransferStats
	transferStats.FileStats = make([]types.FileStat, 0)

	for _, mapping := range m.config.Mappings {
		mappingLogger := logger.With().
			Str("source_registry", mapping.SourceRegistry).
			Str("destination_registry", mapping.DestinationRegistry).
			Logger()

		mappingLogger.Info().Msg("Processing registry migration")

		job := migratable.NewRegistryJob(m.source, m.destination, mapping.SourceRegistry, mapping.SourcePackageHostname,
			mapping.DestinationRegistry, mapping.ArtifactType, &transferStats, &mapping, m.config, m.dryRunStats)

		log.Info().Msgf("concurrency: %d, mapping: %+v", m.config.Concurrency, mapping)

		jobs = append(jobs, job)

	}

	eng := engine.NewEngine(m.config.Concurrency, jobs)
	engineErr := eng.Execute(ctx)
	if engineErr != nil {
		logger.Error().Err(engineErr).Msgf("Engine execution saw following errors: %v", engineErr)
	}
	logger.Info().Msg("Migration process completed")

	// Handle dry-run output. Dry-run behavior is intentionally unchanged by the
	// exit-code contract below: a dry-run never mutates anything, so engine
	// errors there do not fail the process.
	if m.config.DryRun {
		return m.writeDryRunOutput(logger)
	}

	fileStats := transferStats.Snapshot()
	if m.config.Summary {
		printSummary(fileStats)
	} else {
		printer.Print(fileStats, 0, 0, int64(len(fileStats)), false, [][]string{
			{"name", "Name"},
			{"registry", "Registry"},
			{"size", "Size"},
			{"status", "Status"},
			{"uri", "Uri"},
			{"error", "Error"},
			{"reason", "Reason"},
		})

		// Log the same data as JSON
		if jsonData, err := json.MarshalIndent(fileStats, "", "  "); err == nil {
			logger.Info().
				RawJSON("file_stats", jsonData).
				Int("total_files", len(fileStats)).
				Msg("Migration file statistics")
		} else {
			logger.Error().Err(err).Msg("Failed to marshal file stats to JSON")
		}
	}

	// Machine-readable per-coordinate result file (opt-in) — written BEFORE the
	// exit-code decision so automation gets the full picture even on failure.
	if m.config.ResultFile != "" {
		if err := writeResultFile(m.config.ResultFile, fileStats); err != nil {
			logger.Error().Err(err).Str("path", m.config.ResultFile).Msg("Failed to write result file")
			engineErr = errors.Join(engineErr, fmt.Errorf("write result file: %w", err))
		} else {
			logger.Info().Str("path", m.config.ResultFile).Int("records", len(fileStats)).
				Msg("Wrote per-coordinate result file")
		}
	}

	// Exit-code contract: a migration with ANY failure fails the process — no
	// opt-out. Failures are (a) engine-level errors (enumeration aborts, job
	// panics) or (b) any per-coordinate StatusFail stat. Callers needing
	// partial-failure detail must consume the result file / read APIs rather
	// than rely on exit-0-with-failures.
	failed := 0
	for _, fs := range fileStats {
		if fs.Status == types.StatusFail {
			failed++
		}
	}
	if failed > 0 {
		engineErr = errors.Join(engineErr, fmt.Errorf("%d of %d artifact(s) failed to migrate", failed, len(fileStats)))
	}
	if engineErr != nil {
		pterm.Error.Println(fmt.Sprintf("Migration failed: %v", engineErr))
		return engineErr
	}

	return nil
}

// writeResultFile writes one JSON object per FileStat (JSON-lines) to path,
// creating parent directories as needed. The format mirrors the dry-run
// output convention: machine-first, one record per line, safe to stream-parse.
func writeResultFile(path string, fileStats []types.FileStat) error {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create result file directory: %w", err)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create result file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, fs := range fileStats {
		if err := enc.Encode(fs); err != nil {
			return fmt.Errorf("encode result record: %w", err)
		}
	}
	return nil
}

func printSummary(fileStats []types.FileStat) {
	counts := make(map[types.Status]int)
	for _, f := range fileStats {
		counts[f.Status]++
	}

	fmt.Println("\nMigration Summary of total files finalized for upload :")
	fmt.Printf("  %-10s %d\n", "Success :", counts[types.StatusSuccess])
	fmt.Printf("  %-10s %d\n", "Skipped :", counts[types.StatusSkip])
	fmt.Printf("  %-10s %d\n", "Failed  :", counts[types.StatusFail])
	fmt.Printf("  %-10s %d\n", "Total   :", len(fileStats))
}

// writeDryRunOutput writes the dry-run output files
func (m *MigrationService) writeDryRunOutput(logger zerolog.Logger) error {
	timestamp := time.Now().Format("20060102_150405")

	// Create output directory
	outputDir := "dry-run-output"
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		logger.Error().Err(err).Msg("Failed to create output directory")
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write file list
	fileListPath := filepath.Join(outputDir, fmt.Sprintf("file_list_%s.json", timestamp))
	fileListData, err := json.MarshalIndent(m.dryRunStats.Files, "", "  ")
	if err != nil {
		logger.Error().Err(err).Msg("Failed to marshal file list")
		return fmt.Errorf("failed to marshal file list: %w", err)
	}
	if err := os.WriteFile(fileListPath, fileListData, 0644); err != nil {
		logger.Error().Err(err).Msg("Failed to write file list")
		return fmt.Errorf("failed to write file list: %w", err)
	}
	logger.Info().Str("path", fileListPath).Int("total_files", len(m.dryRunStats.Files)).Msg("File list written")

	// Write directory structure
	dirStructPath := filepath.Join(outputDir, fmt.Sprintf("directory_structure_%s.json", timestamp))
	dirStructData, err := json.MarshalIndent(m.dryRunStats.Directories, "", "  ")
	if err != nil {
		logger.Error().Err(err).Msg("Failed to marshal directory structure")
		return fmt.Errorf("failed to marshal directory structure: %w", err)
	}
	if err := os.WriteFile(dirStructPath, dirStructData, 0644); err != nil {
		logger.Error().Err(err).Msg("Failed to write directory structure")
		return fmt.Errorf("failed to write directory structure: %w", err)
	}
	logger.Info().Str("path", dirStructPath).Int("total_registries", len(m.dryRunStats.Directories)).Msg("Directory structure written")

	// Compute summary from directory structure (filtered files)
	totalRegistries := len(m.dryRunStats.Directories)
	totalPackages := 0
	totalVersions := 0
	filteredFiles := 0
	for _, reg := range m.dryRunStats.Directories {
		if reg == nil {
			continue
		}
		totalPackages += len(reg.Packages)
		for _, pkg := range reg.Packages {
			if pkg == nil {
				continue
			}
			totalVersions += len(pkg.Versions)
			for _, ver := range pkg.Versions {
				if ver == nil {
					continue
				}
				filteredFiles += len(ver.Files)
			}
		}
	}
	totalSourceFiles := len(m.dryRunStats.Files)
	fmt.Printf("\nOutput files:\n")
	fmt.Printf("  File list          : %s\n", fileListPath)
	fmt.Printf("  Directory structure: %s\n", dirStructPath)

	fmt.Printf("\n==== Dry Run Summary ====\n")
	fmt.Printf("  %-30s %d\n", "Files found in source registry :", totalSourceFiles)
	fmt.Printf("  (see detail at %s)\n", fileListPath)
	fmt.Println()
	migratedCount := filteredFiles
	migratedLabel := "Files that passed all filters (To be migrated)   :"
	if filteredFiles == 0 && totalPackages > 0 {
		migratedCount = totalPackages
		migratedLabel = "Packages that passed all filters (To be migrated) :"
	}
	fmt.Printf("  %-50s %d\n", migratedLabel, migratedCount)
	fmt.Printf("  (see detail at %s)\n", dirStructPath)
	fmt.Printf("  %-30s %d\n", "Registries :", totalRegistries)
	fmt.Printf("  %-30s %d\n", "Packages   :", totalPackages)
	fmt.Printf("  %-30s %d\n", "Versions   :", totalVersions)

	return nil
}
