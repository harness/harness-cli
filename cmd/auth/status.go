package auth

import (
	"fmt"
	"os"

	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/style"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func getStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "status",
		Short:        "Check authentication status",
		Long:         `Display current authentication status and details by checking the authentication with Harness API`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			isTTY := term.IsTerminal(int(os.Stdout.Fd()))

			// Check if we have the necessary credentials
			apiURL := config.Global.APIBaseURL
			if apiURL == "" {
				apiURL = "https://app.harness.io"
			}

			apiKey := config.Global.AuthToken
			accountID := config.Global.AccountID

			if apiKey == "" {
				if isTTY {
					return fmt.Errorf("not logged in\n\n%s", style.Hint("Run 'hc auth login' to authenticate."))
				}
				return fmt.Errorf("not logged in: no API token found. Please run 'hc auth login' first")
			}

			if accountID == "" {
				if isTTY {
					return fmt.Errorf("not logged in\n\n%s", style.Hint("Run 'hc auth login' to authenticate."))
				}
				return fmt.Errorf("not logged in: no account ID found. Please run 'hc auth login' first")
			}

			if isTTY {
				fmt.Print(style.DimText.Render("Checking authentication status... "))
			} else {
				fmt.Println("Checking authentication status...")
			}

			// Validate credentials using the shared validation function
			if err := validateCredentials(apiURL, apiKey, accountID); err != nil {
				if isTTY {
					fmt.Println(style.Error.Render("✗ failed"))
				}
				return fmt.Errorf("authentication validation failed: %w", err)
			}

			if isTTY {
				fmt.Println(style.Success.Render("✓ authenticated"))
				fmt.Println()
				fmt.Println(style.DimText.Render("  API URL:    ") + apiURL)
				fmt.Println(style.DimText.Render("  Account ID: ") + accountID)
				if config.Global.OrgID != "" {
					fmt.Println(style.DimText.Render("  Org ID:     ") + config.Global.OrgID)
				}
				if config.Global.ProjectID != "" {
					fmt.Println(style.DimText.Render("  Project ID: ") + config.Global.ProjectID)
				}
			} else {
				fmt.Println("Authentication Status: ✓ Authenticated")
				fmt.Println("API URL:     ", apiURL)
				fmt.Println("Account ID:  ", accountID)
				if config.Global.OrgID != "" {
					fmt.Println("Org ID:      ", config.Global.OrgID)
				}
				if config.Global.ProjectID != "" {
					fmt.Println("Project ID:  ", config.Global.ProjectID)
				}
			}

			return nil
		},
	}

	return cmd
}
