package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/harness/harness-cli/config"

	"github.com/spf13/cobra"
)

// accountResponse represents the response from the account API
type accountResponse struct {
	Account struct {
		Identifier string `json:"identifier"`
		Name       string `json:"name"`
		CreatedAt  int64  `json:"createdAt"`
	}
}

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
				return fmt.Errorf("not logged in: no API token found. Please run 'harness auth login' first")
			}

			if accountID == "" {
				return fmt.Errorf("not logged in: no account ID found. Please run 'harness auth login' first")
			}

			// Create HTTP client with reasonable timeout
			client := &http.Client{
				Timeout: 10 * time.Second,
			}

			// Build the request URL
			url := fmt.Sprintf("%s/ng/api/accounts/%s", apiURL, accountID)

			// Create the request
			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				return fmt.Errorf("error creating request: %w", err)
			}

			// Add headers
			req.Header.Add("x-api-key", apiKey)

			// Execute the request
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("error connecting to Harness API: %w", err)
			}
			defer resp.Body.Close()

			// Read response body
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("error reading response body: %w", err)
			}

			// Check status code
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("authentication failed with status %s. Response: %s",
					resp.Status, string(body))
			}

			// Parse the response
			var accountResp accountResponse
			if err := json.Unmarshal(body, &accountResp); err != nil {
				return fmt.Errorf("error parsing response: %w", err)
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
