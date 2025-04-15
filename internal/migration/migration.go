package migration

import (
	"context"
	"fmt"
	"harness/internal/api/ar"
	"harness/internal/config"
	"harness/internal/registry"
	"log"
	"sync"
)

// MigrationService handles the migration process
type MigrationService struct {
	config      *config.Config
	apiClient   *ar.ARClient
	source      registry.SourceRegistry
	destination registry.DestinationRegistry
}

// NewMigrationService creates a new migration service
func NewMigrationService(cfg *config.Config, apiClient *ar.ARClient) (*MigrationService, error) {
	sourceRegistry, err := registry.NewSourceRegistry(cfg.Source)
	if err != nil {
		return nil, fmt.Errorf("failed to create source registry client: %w", err)
	}

	destinationRegistry, err := registry.NewDestinationRegistry(cfg.Dest)
	if err != nil {
		return nil, fmt.Errorf("failed to create destination registry client: %w", err)
	}

	return &MigrationService{
		config:      cfg,
		apiClient:   apiClient,
		source:      sourceRegistry,
		destination: destinationRegistry,
	}, nil
}

// Run executes the migration process
func (m *MigrationService) Run(ctx context.Context) error {
	log.Println("Starting migration process")
	log.Printf("Source type: %s, Destination type: %s", m.config.Source.Type, m.config.Dest.Type)

	// Process each registry mapping
	for _, mapping := range m.config.Dest.Mappings {
		if err := m.processMapping(ctx, mapping); err != nil {
			log.Printf("Error processing mapping '%s' to '%s': %v",
				mapping.SourceRegistry, mapping.DestinationRegistry, err)

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

// processMapping handles a single registry mapping
func (m *MigrationService) processMapping(ctx context.Context, mapping config.RegistryMapping) error {
	log.Printf("Processing mapping from '%s' to '%s'", mapping.SourceRegistry, mapping.DestinationRegistry)

	// Create destination registry if needed
	log.Println("Ensuring destination registry exists")
	if err := m.ensureDestinationRegistry(mapping.DestinationRegistry); err != nil {
		return fmt.Errorf("failed to ensure destination registry: %w", err)
	}

	// List artifacts from source registry
	artifacts, err := m.source.ListArtifacts(mapping.SourceRegistry)
	if err != nil {
		return fmt.Errorf("failed to list artifacts from source registry: %w", err)
	}

	log.Printf("Found %d artifacts to migrate", len(artifacts))

	// Start migration tracking
	migReq := ar.MigrationRequest{
		RegistryID:        mapping.SourceRegistry,
		AccountIdentifier: m.config.Dest.AccountIdentifier,
		TotalImages:       len(artifacts),
	}

	migrationID, err := m.apiClient.StartMigration(migReq)
	if err != nil {
		return fmt.Errorf("failed to start migration tracking: %w", err)
	}

	log.Printf("Migration tracking started with ID: %s", migrationID)

	// Process artifacts
	return m.processArtifacts(ctx, migrationID, artifacts, mapping)
}

// ensureDestinationRegistry makes sure the destination registry exists
func (m *MigrationService) ensureDestinationRegistry(destRegistry string) error {
	// This would typically call the CreateRegistry API if the registry doesn't exist
	createReq := ar.RegistryRequest{
		Identifier:  destRegistry,
		PackageType: m.config.Dest.Type,
	}

	// Parse destination registry path
	// Format could be: "registry", "org/registry", or "org/project/registry"
	_, err := m.apiClient.CreateRegistry(createReq)
	if err != nil {
		return fmt.Errorf("failed to create destination registry: %w", err)
	}

	return nil
}

// processArtifacts handles the migration of artifacts
func (m *MigrationService) processArtifacts(
	ctx context.Context,
	migrationID string,
	artifacts []registry.Artifact,
	mapping config.RegistryMapping,
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

		go func(art registry.Artifact) {
			defer wg.Done()
			defer func() { <-sem }() // Release semaphore

			if err := m.processArtifact(ctx, migrationID, art, mapping); err != nil {
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
	artifact registry.Artifact,
	mapping config.RegistryMapping,
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

	// Download artifact
	log.Printf("Downloading artifact %s:%s", artifact.Name, artifact.Version)
	artifactBytes, err := m.source.DownloadArtifact(artifact)
	if err != nil {
		updateReq.Status = ar.StatusFailed
		updateReq.Error = fmt.Sprintf("Failed to download: %v", err)
		_ = m.apiClient.UpdateArtifactStatus(migrationID, updateReq)
		return fmt.Errorf("failed to download artifact: %w", err)
	}

	// Check context for cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Continue processing
	}

	// Upload artifact
	log.Printf("Uploading artifact %s:%s to destination", artifact.Name, artifact.Version)
	destArtifact := registry.Artifact{
		Name:       artifact.Name,
		Version:    artifact.Version,
		Type:       artifact.Type,
		Registry:   mapping.DestinationRegistry,
		Properties: artifact.Properties,
	}

	if err := m.destination.UploadArtifact(destArtifact, artifactBytes); err != nil {
		updateReq.Status = ar.StatusFailed
		updateReq.Error = fmt.Sprintf("Failed to upload: %v", err)
		_ = m.apiClient.UpdateArtifactStatus(migrationID, updateReq)
		return fmt.Errorf("failed to upload artifact: %w", err)
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
