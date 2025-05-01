package migrate

import (
	"context"
	"fmt"
	"harness/clients/ar"
	"harness/config"
	"harness/module/ar/migrate/adapter"
	"harness/module/ar/migrate/types"
	"log"
	"strings"
	"sync"
)

// MigrationService handles the migration process
type MigrationService struct {
	config      *types.Config
	apiClient   *ar.Client
	source      adapter.Adapter
	destination adapter.Adapter
	handlers    map[string]ArtifactHandler // Cache for package type handlers
}

// NewMigrationService creates a new migration service
func NewMigrationService(cfg *types.Config, apiClient *ar.Client) (*MigrationService, error) {
	ctx := context.Background()

	// Get source adapter factory
	sourceFactory, err := adapter.GetFactory(cfg.Source.Type)
	if err != nil {
		return nil, fmt.Errorf("failed to get source adapter factory: %w", err)
	}

	// Create source adapter
	sourceAdapter, err := sourceFactory.Create(ctx, cfg.Source)
	if err != nil {
		return nil, fmt.Errorf("failed to create source adapter: %w", err)
	}

	// Get destination adapter factory
	destFactory, err := adapter.GetFactory(cfg.Dest.Type)
	if err != nil {
		return nil, fmt.Errorf("failed to get destination adapter factory: %w", err)
	}

	// Create destination adapter
	destAdapter, err := destFactory.Create(ctx, cfg.Dest)
	if err != nil {
		return nil, fmt.Errorf("failed to create destination adapter: %w", err)
	}

	return &MigrationService{
		config:      cfg,
		apiClient:   apiClient,
		source:      sourceAdapter,
		destination: destAdapter,
		handlers:    make(map[string]ArtifactHandler),
	}, nil
}

// Run executes the migration process
func (m *MigrationService) Run(ctx context.Context) error {
	log.Println("Starting migration process")
	log.Printf("Source type: %s, Destination type: %s", m.config.Source.Type, m.config.Dest.Type)

	for _, registry := range m.config.Filters.Registries {
		inputReg := types.InputRegistry{}
		inputReg.SourceRegistry = registry
		inputReg.DestinationRegistry = registry
		inputReg.ArtifactType = m.config.Filters.ArtifactType

		for _, mapping := range m.config.Mappings {
			if mapping.SourceRegistry == registry {
				inputReg.DestinationRegistry = mapping.DestinationRegistry
				inputReg.ArtifactNamePatterns.Include = mapping.ArtifactNamePatterns.Include
				inputReg.ArtifactNamePatterns.Exclude = mapping.ArtifactNamePatterns.Exclude
				break
			}
		}

		if err := m.processRegistry(ctx, inputReg); err != nil {
			log.Printf("Error processing mapping '%s' to '%s': %v",
				inputReg.SourceRegistry, inputReg.DestinationRegistry, err)

			if m.config.Migration.FailureMode == "stop" {
				return fmt.Errorf("migration stopped due to error in mapping: %w", err)
			}

			// If failure mode is "continue", we log the error and continue with the next mapping
			log.Println("Continuing with next mapping due to 'continue' failure mode")
			continue
		}
	}

	log.Println("Migration process completed")
	return nil
}

// processRegistry handles a single registry mapping
func (m *MigrationService) processRegistry(ctx context.Context, registry types.InputRegistry) error {
	log.Printf("Processing registry from '%s' to '%s'", registry.SourceRegistry, registry.DestinationRegistry)

	// Create a destination registry if needed
	log.Println("Ensuring destination registry exists")
	if err := m.ensureDestinationRegistry(registry.DestinationRegistry); err != nil {
		return fmt.Errorf("failed to ensure destination ar: %w", err)
	}

	// List artifacts from source ar
	srcArtifacts, err := m.source.ListArtifacts(registry.SourceRegistry, registry.ArtifactType)
	destArtifacts, err := m.destination.ListArtifacts(registry.DestinationRegistry, registry.ArtifactType)

	artifacts := minus(srcArtifacts, destArtifacts)

	if err != nil {
		return fmt.Errorf("failed to list artifacts from source ar: %w", err)
	}

	log.Printf("Found %d artifacts to migrate", len(artifacts))

	// Start migration tracking
	migReq := ar.MigrationRequest{
		RegistryID:        registry.SourceRegistry,
		AccountIdentifier: config.Global.AccountID,
		TotalImages:       len(artifacts),
	}

	migrationID, err := m.apiClient.StartMigration(migReq)
	if err != nil {
		return fmt.Errorf("failed to start migration tracking: %w", err)
	}

	log.Printf("Migration tracking started with ID: %s", migrationID)

	// Process artifacts
	return m.processArtifacts(ctx, migrationID, artifacts, registry)
}

func minus(iArtifacts, jArtifacts []types.Artifact) []types.Artifact {
	seenArtifacts := make(map[string]bool)

	for _, artifact := range jArtifacts {
		key := artifact.Name + ":" + artifact.Version
		seenArtifacts[key] = true
	}

	result := make([]types.Artifact, 0, len(iArtifacts))

	for _, artifact := range iArtifacts {
		key := artifact.Name + ":" + artifact.Version
		if !seenArtifacts[key] {
			result = append(result, artifact)
		}
	}

	return result
}

// ensureDestinationRegistry makes sure the destination ar exists
func (m *MigrationService) ensureDestinationRegistry(destRegistry string) error {
	// Use the adapter directly to create the registry
	_, err := m.destination.PrepareForPush(destRegistry, string(m.config.Dest.Type))
	if err != nil {
		return fmt.Errorf("failed to create destination ar: %w", err)
	}

	return nil
}

// processArtifacts handles the migration of artifacts
func (m *MigrationService) processArtifacts(
	ctx context.Context,
	migrationID string,
	artifacts []types.Artifact,
	inputRegistry types.InputRegistry,
) error {
	concurrency := m.config.Migration.Concurrency
	if concurrency <= 0 {
		concurrency = 5 // Default concurrency
	}

	// Create a semaphore to limit concurrent operations
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	// Create a channel for errors
	errCh := make(chan error, len(artifacts))

	log.Printf("Processing artifacts with concurrency: %d", concurrency)

	// Process each artifact
	for _, artifact := range artifacts {
		wg.Add(1)
		sem <- struct{}{} // Acquire semaphore

		go func(art Artifact) {
			defer wg.Done()
			defer func() { <-sem }() // Release semaphore

			if err := m.processArtifact(ctx, migrationID, art, inputRegistry); err != nil {
				log.Printf("Error processing artifact %s: %v", art.Name, err)
				errCh <- err

				if m.config.Migration.FailureMode == "stop" {
					// Signal context cancellation
					// Note: This doesn't immediately stop all goroutines, but they will
					// check ctx.Done() and exit gracefully
					return
				}
			}
		}(artifact)

		// Check if context is done (canceled)
		select {
		case <-ctx.Done():
			log.Println("Migration canceled")
			return ctx.Err()
		default:
			// Continue processing
		}
	}

	// Wait for all goroutines to finish
	wg.Wait()
	close(errCh)

	// Check if there were any errors
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("migration completed with %d errors", len(errs))
	}

	return nil
}

// processArtifact handles a single artifact migration
func (m *MigrationService) processArtifact(
	ctx context.Context,
	migrationID string,
	artifact Artifact,
	mapping types.RegistryOverride,
) error {
	// Update status to started
	updateReq := ar.ArtifactUpdateRequest{
		Package: artifact.Name,
		Version: artifact.Version,
		Status:  ar.StatusStarted,
	}

	if err := m.apiClient.UpdateArtifactStatus(migrationID, updateReq); err != nil {
		log.Printf("Failed to update artifact status to STARTED: %v", err)
		// Continue anyway
	}

	// Skip actual processing if in dry run mode
	if m.config.Migration.DryRun {
		log.Printf("DRY RUN: Would migrate artifact %s:%s", artifact.Name, artifact.Version)

		// Update status to completed in dry run mode
		updateReq.Status = ar.StatusCompleted
		if err := m.apiClient.UpdateArtifactStatus(migrationID, updateReq); err != nil {
			log.Printf("Failed to update artifact status to COMPLETED: %v", err)
		}

		return nil
	}

	// Get the appropriate handler for this artifact type
	handler, err := m.getArtifactHandler(artifact)
	if err != nil {
		updateReq.Status = ar.StatusFailed
		updateReq.Error = fmt.Sprintf("Failed to get handler: %v", err)
		_ = m.apiClient.UpdateArtifactStatus(migrationID, updateReq)
		return fmt.Errorf("failed to get artifact handler: %w", err)
	}

	// Check context for cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Continue processing
	}

	// Use the handler to copy the artifact
	log.Printf("Copying artifact %s:%s using %T handler", artifact.Name, artifact.Version, handler)
	// Create bridge implementations for our adapters
	sourceBridge := &adapterSourceBridge{adapter: m.source}
	destBridge := &adapterDestBridge{adapter: m.destination}
	if err := handler.CopyArtifact(sourceBridge, destBridge, artifact, mapping.DestinationRegistry); err != nil {
		updateReq.Status = ar.StatusFailed
		updateReq.Error = fmt.Sprintf("Failed to copy: %v", err)
		_ = m.apiClient.UpdateArtifactStatus(migrationID, updateReq)
		return fmt.Errorf("failed to copy artifact: %w", err)
	}

	// Update status to completed
	updateReq.Status = ar.StatusCompleted
	if err := m.apiClient.UpdateArtifactStatus(migrationID, updateReq); err != nil {
		log.Printf("Failed to update artifact status to COMPLETED: %v", err)
		// Continue anyway as the artifact was successfully migrated
	}

	log.Printf("Successfully migrated artifact %s:%s", artifact.Name, artifact.Version)
	return nil
}

// GetMigrationStatus retrieves the current status of a migration
func (m *MigrationService) GetMigrationStatus(migrationID string) (*ar.MigrationStatus, error) {
	return m.apiClient.GetMigrationStatus(migrationID)
}

// PrintStatus prints the current migration status
func (m *MigrationService) PrintStatus(status *ar.MigrationStatus) {
	fmt.Println("Migration Status:")
	fmt.Printf("ID: %s\n", status.ID)
	fmt.Printf("Registry: %s\n", status.Registry)
	fmt.Printf("Total Images: %d\n", status.TotalImages)
	fmt.Println("Status Counts:")
	fmt.Printf("  Not Started: %d\n", status.Status.NotStarted)
	fmt.Printf("  Started: %d\n", status.Status.Started)
	fmt.Printf("  Completed: %d\n", status.Status.Completed)
	fmt.Printf("  Failed: %d\n", status.Status.Failed)
	fmt.Printf("  Skipped: %d\n", status.Status.Skipped)
}

// getArtifactHandler returns the appropriate handler for the given artifact
func (m *MigrationService) getArtifactHandler(artifact Artifact) (*AdapterArtifactHandler, error) {
	// Determine the package type from artifact properties or file extension
	packageType := determinePackageType(artifact)

	// Check if we already have a handler for this package type
	if handler, ok := m.handlers[packageType]; ok {
		// Return the handler if it's an AdapterArtifactHandler
		if adapterHandler, ok := handler.(*AdapterArtifactHandler); ok {
			return adapterHandler, nil
		}
		// If it's not an AdapterArtifactHandler, we need to recreate it (unlikely scenario)
		log.Printf("Warning: Found non-adapter handler for %s, recreating", packageType)
	}

	// Create a new adapter handler for this package type
	handler, err := NewAdapterArtifactHandler(packageType, m.source, m.destination)
	if err != nil {
		return nil, fmt.Errorf("failed to create adapter handler for package type %s: %w", packageType, err)
	}

	// Cache the handler for future use
	m.handlers[packageType] = handler
	return handler, nil
}

// determinePackageType identifies the package type from artifact metadata
func determinePackageType(artifact Artifact) string {
	// First check if the artifact type is explicitly set
	if artifact.Type != "" {
		return strings.ToLower(artifact.Type)
	}

	// Check if there's a package type property
	if packageType, ok := artifact.Properties["packageType"]; ok {
		return strings.ToLower(packageType)
	}

	// Try to determine based on file extension or naming conventions
	if strings.HasSuffix(artifact.Name, ".whl") || strings.HasSuffix(artifact.Name, ".tar.gz") {
		return "python"
	} else if strings.HasSuffix(artifact.Name, ".jar") || strings.HasSuffix(artifact.Name, ".pom") {
		return "maven"
	} else if strings.HasSuffix(artifact.Name, ".tgz") || strings.HasPrefix(artifact.Name, "@") {
		return "npm"
	}

	// Default to generic if we can't determine the type
	return "generic"
}
