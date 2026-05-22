package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/util/credential"

	"github.com/spf13/cobra"
)

// getAuthConfigFilePath returns the path to the auth config file for logout
func getAuthConfigFilePath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Error getting home directory: %v\n", err)
		os.Exit(1)
	}

	return filepath.Join(homeDir, ".harness", "auth.json")
}

func getLogoutCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:          "logout",
		Short:        "Logout from Harness",
		Long:         `Remove saved Harness credentials by deleting the authentication configuration`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath := getAuthConfigFilePath()

			// Check if config file exists
			if _, err := os.Stat(configPath); os.IsNotExist(err) {
				if !force {
					return fmt.Errorf("not logged in: no authentication file found at %s", configPath)
				}
				fmt.Println("No authentication file found. Already logged out.")
				return nil
			}

			// Read config to get account ID for keychain cleanup
			if data, err := os.ReadFile(configPath); err == nil {
				var cfg struct {
					AccountID string `json:"account_id"`
				}
				if json.Unmarshal(data, &cfg) == nil && cfg.AccountID != "" {
					store := credential.NewStore(false)
					_ = store.Delete(cfg.AccountID)
				}
			}

			// Delete the config file
			if err := os.Remove(configPath); err != nil {
				return fmt.Errorf("error removing authentication file: %w", err)
			}

			// Clear the global config values
			config.Global.APIBaseURL = ""
			config.Global.AuthToken = ""
			config.Global.AccountID = ""
			config.Global.OrgID = ""
			config.Global.ProjectID = ""

			fmt.Println("Successfully logged out from Harness")
			return nil
		},
	}

	// Add flags
	cmd.Flags().BoolVar(&force, "force", false, "Force logout even if not currently logged in")

	return cmd
}
