package migrate

import (
	"context"
	"fmt"
	"harness/clients/ar"
	"harness/module/ar/migrate/adapter"
	"harness/module/ar/migrate/engine"
	"harness/module/ar/migrate/migratable"
	"harness/module/ar/migrate/types"
	"log"
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
	log.Println("Starting migration process")
	log.Printf("Source type: %s, Destination type: %s", m.config.Source.Type, m.config.Dest.Type)

	var jobs []migratable.Job

	for _, mapping := range m.config.Mappings {
		input := types.InputMapping{}
		input.SourceRegistry = mapping.SourceRegistry
		input.DestinationRegistry = mapping.DestinationRegistry
		input.ArtifactType = mapping.ArtifactType
		input.ArtifactNamePatterns.Include = mapping.ArtifactNamePatterns.Include
		input.ArtifactNamePatterns.Exclude = mapping.ArtifactNamePatterns.Exclude
		log.Printf("Processing registry migration from '%s' to '%s'", mapping.SourceRegistry,
			mapping.DestinationRegistry)

		job := migratable.NewRegistryJob(m.source, m.destination, mapping.SourceRegistry,
			mapping.DestinationRegistry)
		jobs = append(jobs, job)

	}

	eng := engine.NewEngine(m.config.Concurrency, jobs)
	err := eng.Execute(ctx)
	if err != nil {
		return fmt.Errorf("engine execution failed: %w", err)
	}
	log.Println("Migration process completed")
	return nil
}
