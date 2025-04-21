package ar

import (
	"github.com/spf13/cobra"
	"harness/config"
	"log"
)

func getStatusCmd() *cobra.Command {
	// Create local variables for flag binding
	var localConfigPath string
	var localMigrationID string
	var localAPIBaseURL string
	var localAuthToken string
	var localPollInterval int

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Get the status of a migration",
		Run:   getMigrationStatus,
		PreRun: func(cmd *cobra.Command, args []string) {
			// Sync local flags to global config
			config.Global.ConfigPath = localConfigPath
			config.Global.Registry.Status.MigrationID = localMigrationID
			if localAPIBaseURL != "" {
				config.Global.APIBaseURL = localAPIBaseURL
			}
			if localAuthToken != "" {
				config.Global.AuthToken = localAuthToken
			}
			config.Global.Registry.Status.PollInterval = localPollInterval
		},
	}
	statusCmd.Flags().StringVarP(&localConfigPath, "config", "c", "config.yaml", "Path to configuration file")
	statusCmd.Flags().StringVarP(&localMigrationID, "id", "i", "", "Migration ID")
	statusCmd.Flags().StringVar(&localAPIBaseURL, "clients-url", "", "Base URL for the API (overrides config)")
	statusCmd.Flags().StringVar(&localAuthToken, "token", "", "Authentication token (overrides config)")
	statusCmd.Flags().IntVar(&localPollInterval, "poll", 0, "Poll interval in seconds (0 for single query)")
	return statusCmd
}

func getMigrationStatus(cmd *cobra.Command, args []string) {
	if config.Global.Registry.Status.MigrationID == "" {
		log.Fatalf("Migration ID is required")
	}

	// Load configuration
	//cfg, err := types.LoadConfig(config.Global.ConfigPath)
	//if err != nil {
	//	log.Fatalf("Failed to load configuration: %v", err)
	//}

	// Create API client
	// In a real implementation, you'd construct the API base URL from the config
	//apiClient := ar.NewHARClient("https://api.example.com", config.Global.AuthToken, "", "", "")

	// Create migration service
	//migrationSvc, err := ar2.NewMigrationService(cfg, apiClient)
	//if err != nil {
	//	log.Fatalf("Failed to create migration service: %v", err)
	//}

	// If poll interval is set, continuously poll for status
}
