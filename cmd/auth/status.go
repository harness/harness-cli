package auth

import (
	"fmt"

	"github.com/harness/harness-cli/config"

	"github.com/spf13/cobra"
)

func getStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "status",
		Short:        "Check authentication status",
		Long:         `Display current authentication status and details by checking the authentication with Harness API`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Checking authentication status...")

			// Check if we have the necessary credentials
			apiURL := config.Global.APIBaseURL
			if apiURL == "" {
				apiURL = "https://app.harness.io"
			}

			apiKey := config.Global.AuthToken
			accountID := config.Global.AccountID

			if apiKey == "" {
				return fmt.Errorf("not logged in: no API token found. Please run 'hc auth login' first")
			}

			if accountID == "" {
				return fmt.Errorf("not logged in: no account ID found. Please run 'hc auth login' first")
			}

			// Validate credentials using the shared validation function
			if err := validateCredentials(apiURL, apiKey, accountID); err != nil {
				return fmt.Errorf("authentication validation failed: %w", err)
			}

			// Display authentication information
			fmt.Println("Authentication Status: ✓ Authenticated")
			fmt.Println("API URL:     ", apiURL)
			fmt.Println("Account ID:  ", accountID)

			// Display organization and project if available
			if config.Global.OrgID != "" {
				fmt.Println("Org ID:      ", config.Global.OrgID)
			}
			if config.Global.ProjectID != "" {
				fmt.Println("Project ID:  ", config.Global.ProjectID)
			}

			return nil
		},
	}

	return cmd
}
