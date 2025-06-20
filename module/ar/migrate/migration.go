package migrate

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
	"harness/internal/api/ar"
	"harness/module/ar/migrate/adapter"
	"harness/module/ar/migrate/engine"
	"harness/module/ar/migrate/migratable"
	"harness/module/ar/migrate/types"
	"harness/util/common/printer"

	_ "harness/module/ar/migrate/adapter/har"
	_ "harness/module/ar/migrate/adapter/jfrog"
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

		job := migratable.NewRegistryJob(m.source, m.destination, mapping.SourceRegistry,
			mapping.DestinationRegistry, mapping.ArtifactType, &transferStats)
		jobs = append(jobs, job)

	}

	eng := engine.NewEngine(m.config.Concurrency, jobs)
	err := eng.Execute(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("Engine execution failed")
		return fmt.Errorf("engine execution failed: %w", err)
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
	return nil
}
