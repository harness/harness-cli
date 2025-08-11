package auth

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/harness/harness-cli/config"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// AuthConfig represents authentication configuration for saving to disk
type AuthConfig struct {
	BaseURL   string `json:"base_url"`
	Token     string `json:"token"`
	AccountID string `json:"account_id"`
	OrgID     string `json:"org_id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

// getAuthConfigPath returns the path to the auth config file
func getAuthConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Error getting home directory: %v\n", err)
		os.Exit(1)
	}

	configDir := filepath.Join(homeDir, ".harness")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		fmt.Printf("Error creating config directory: %v\n", err)
		os.Exit(1)
	}

	return filepath.Join(configDir, "auth.json")
}

// saveAuthConfig saves the authentication configuration to disk
func saveAuthConfig(authConfig AuthConfig) error {
	configPath := getAuthConfigPath()

	// Marshal the config to JSON with indentation for readability
	data, err := json.MarshalIndent(authConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling auth config: %w", err)
	}

	// Write the data to the file
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("error writing auth config file: %w", err)
	}

	return nil
}

// readPassword reads a password from stdin without echoing it
func readPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	password, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println() // Add a newline after the password input
	if err != nil {
		return "", fmt.Errorf("error reading password: %w", err)
	}
	return strings.TrimSpace(string(password)), nil
}

// readInput reads a line of text from stdin
func readInput(prompt string) (string, error) {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("error reading input: %w", err)
	}
	return strings.TrimSpace(input), nil
}

func getLoginCmd() *cobra.Command {
	var (
		apiURL         string
		token          string
		accountID      string
		orgID          string
		projectID      string
		nonInteractive bool
	)

	cmd := &cobra.Command{
		Use:          "login",
		Short:        "Login to Harness",
		Long:         `Authenticate with Harness services and save credentials for future use`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Check if we need interactive mode
			needInteractive := !nonInteractive && (token == "" || accountID == "")

			// Interactive mode for missing required inputs
			if needInteractive {
				fmt.Println("Entering interactive login mode. Press Ctrl+C to cancel.")

				// Get API URL
				if apiURL == "" {
					defaultURL := "https://app.harness.io"
					input, err := readInput(fmt.Sprintf("API URL [%s]: ", defaultURL))
					if err != nil {
						return err
					}
					if input == "" {
						apiURL = defaultURL
					} else {
						apiURL = input
					}
				}

				// Get API Token
				if token == "" {
					input, err := readPassword("API Token: ")
					if err != nil {
						return err
					}
					if input == "" {
						return fmt.Errorf("API token is required")
					}
					token = input
				}

				// Get Account ID
				if accountID == "" {
					input, err := readInput("Account ID: ")
					if err != nil {
						return err
					}
					if input == "" {
						return fmt.Errorf("Account ID is required")
					}
					accountID = input
				}

				// Get optional Org ID
				if orgID == "" {
					input, err := readInput("Organization ID (optional): ")
					if err != nil {
						return err
					}
					orgID = input
				}

				// Get optional Project ID
				if projectID == "" {
					input, err := readInput("Project ID (optional): ")
					if err != nil {
						return err
					}
					projectID = input
				}
			}

			// Use default API URL if not provided
			if apiURL == "" {
				apiURL = "https://app.harness.io"
			}

			// Verify required fields are provided
			if token == "" {
				return fmt.Errorf("API token is required. Use --api-token flag or interactive mode")
			}
			if accountID == "" {
				return fmt.Errorf("Account ID is required. Use --account flag or interactive mode")
			}

			// Create auth config struct for saving to file
			authConfig := AuthConfig{
				BaseURL:   apiURL,
				Token:     token,
				AccountID: accountID,
				OrgID:     orgID,
				ProjectID: projectID,
			}

			// Save config to disk
			if err := saveAuthConfig(authConfig); err != nil {
				return fmt.Errorf("failed to save authentication config: %w", err)
			}

			// Update the global config for the current session as well
			config.Global.APIBaseURL = apiURL
			config.Global.AuthToken = token
			config.Global.AccountID = accountID
			config.Global.OrgID = orgID
			config.Global.ProjectID = projectID

			// Print confirmation message
			fmt.Println("Successfully logged into Harness")
			fmt.Println("API URL:     ", apiURL)
			fmt.Println("Account ID:  ", accountID)
			if orgID != "" {
				fmt.Println("Org ID:      ", orgID)
			}
			if projectID != "" {
				fmt.Println("Project ID:  ", projectID)
			}

			return nil
		},
	}

	// Add flags specific to login
	cmd.Flags().StringVar(&apiURL, "api-url", "", "Harness API URL (default: https://app.harness.io)")
	cmd.Flags().StringVar(&token, "api-token", "", "Authentication token")
	cmd.Flags().StringVar(&accountID, "account", "", "Account ID")
	cmd.Flags().StringVar(&orgID, "org", "", "Organization ID")
	cmd.Flags().StringVar(&projectID, "project", "", "Project ID")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Disable interactive prompts (requires all mandatory flags)")

	// Don't mark flags as required as we'll handle missing flags in interactive mode
	// We'll validate the values in the RunE function instead

	return cmd
}
