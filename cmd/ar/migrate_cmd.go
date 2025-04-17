package ar

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"harness/clients/ar"
	"harness/config"
	ar2 "harness/module/ar"
	"harness/module/ar/types"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func getMigrateCmd() *cobra.Command {
	// Create local variables for flag binding
	var localConfigPath string
	var localAPIBaseURL string
	var localDryRun bool
	var localConcurrency int

	migrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "Start a migration based on configuration",
		Run:   runMigration,
		PreRun: func(cmd *cobra.Command, args []string) {
			// Sync local flags to global config
			config.Global.ConfigPath = localConfigPath
			if localAPIBaseURL != "" {
				config.Global.APIBaseURL = localAPIBaseURL
			}
			config.Global.Registry.Migrate.DryRun = localDryRun
			config.Global.Registry.Migrate.Concurrency = localConcurrency
		},
	}
	migrateCmd.Flags().StringVarP(&localConfigPath, "config", "c", "config.yaml", "Path to configuration file")
	migrateCmd.Flags().StringVar(&localAPIBaseURL, "ar-url", "", "Base URL for the API (overrides config)")
	migrateCmd.Flags().BoolVar(&localDryRun, "dry-run", false, "Perform a dry run (overrides config)")
	migrateCmd.Flags().IntVar(&localConcurrency, "concurrency", 5, "Number of concurrent operations (overrides config)")
	return migrateCmd
}

func runMigration(cmd *cobra.Command, args []string) {
	// Load configuration
	cfg, err := types.LoadConfig(config.Global.ConfigPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	if config.Global.Registry.Migrate.DryRun {
		cfg.Migration.DryRun = true
	}
	if config.Global.Registry.Migrate.Concurrency > 0 {
		cfg.Migration.Concurrency = config.Global.Registry.Migrate.Concurrency
	}

	// Create an API client for orchestration purpose. The registry clients will be initiated separately
	apiClient := ar.NewHARClient(config.Global.APIBaseURL, config.Global.AuthToken, config.Global.AccountID,
		config.Global.OrgID, config.Global.ProjectID)

	// Create a migration service
	migrationSvc, err := ar2.NewMigrationService(cfg, apiClient)
	if err != nil {
		log.Fatalf("Failed to create migration service: %v", err)
	}

	// Set up context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalChan
		fmt.Println("\nReceived interrupt signal, shutting down gracefully...")
		cancel()
	}()

	// Run the migration
	if err := migrationSvc.Run(ctx); err != nil {
		log.Fatalf("Migration failed: %v", err)
	}

	fmt.Println("Migration completed successfully")
}
