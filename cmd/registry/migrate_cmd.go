package registry

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"harness/internal/api/ar"
	"harness/internal/config"
	"harness/internal/migration"
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
	migrateCmd.Flags().StringVar(&localAPIBaseURL, "registry-url", "", "Base URL for the API (overrides config)")
	migrateCmd.Flags().BoolVar(&localDryRun, "dry-run", false, "Perform a dry run (overrides config)")
	migrateCmd.Flags().IntVar(&localConcurrency, "concurrency", 5, "Number of concurrent operations (overrides config)")
	return migrateCmd
}

func runMigration(cmd *cobra.Command, args []string) {
	// Load configuration
	cfg, err := config.LoadConfig(config.Global.ConfigPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Override configuration with command line flags
	if config.Global.APIBaseURL != "" {
		// This would need to be properly integrated with your config structure
	}
	if config.Global.AuthToken != "" {
		// This would need to be properly integrated with your config structure
	}
	if config.Global.Registry.Migrate.DryRun {
		cfg.Migration.DryRun = true
	}
	if config.Global.Registry.Migrate.Concurrency > 0 {
		cfg.Migration.Concurrency = config.Global.Registry.Migrate.Concurrency
	}

	// Create API client
	// In a real implementation, you'd construct the API base URL from the config
	apiClient := ar.NewHARClient("https://api.example.com", config.Global.AuthToken, "", "", "")

	// Create migration service
	migrationSvc, err := migration.NewMigrationService(cfg, apiClient)
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
