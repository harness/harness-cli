package migrate

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/harness/harness-cli/internal/api/ar"
	"github.com/harness/harness-cli/module/ar/migrate/adapter"
	"github.com/harness/harness-cli/module/ar/migrate/engine"
	"github.com/harness/harness-cli/module/ar/migrate/migratable"
	"github.com/harness/harness-cli/module/ar/migrate/types"
	"github.com/harness/harness-cli/util/common/printer"

	"github.com/rs/zerolog/log"

	_ "github.com/harness/harness-cli/module/ar/migrate/adapter/har"
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

	return &MigrationService{
		config:      cfg,
		apiClient:   apiClient,
		source:      sourceAdapter,
		destination: destAdapter,
	}, nil
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
			mapping.DestinationRegistry, mapping.ArtifactType, &transferStats, &mapping, m.config)

		log.Info().Msgf("concurrency: %d, mapping: %+v", m.config.Concurrency, mapping)

		jobs = append(jobs, job)

	}

	eng := engine.NewEngine(m.config.Concurrency, jobs)
	err := eng.Execute(ctx)
	if err != nil {
		logger.Error().Err(err).Msgf("Engine execution saw following errors: %v", err)
	}
	logger.Info().Msg("Migration process completed")
	printer.Print(transferStats.FileStats, 0, 0, int64(len(transferStats.FileStats)), false, [][]string{
		{"Name", "Name"},
		{"Registry", "Registry"},
		{"Size", "Size"},
		{"Status", "Status"},
		{"Uri", "Uri"},
		{"Error", "Error"},
	})

	// Log the same data as JSON
	if jsonData, err := json.MarshalIndent(transferStats.FileStats, "", "  "); err == nil {
		logger.Info().
			RawJSON("file_stats", jsonData).
			Int("total_files", len(transferStats.FileStats)).
			Msg("Migration file statistics")
	} else {
		logger.Error().Err(err).Msg("Failed to marshal file stats to JSON")
	}

	return nil
}
