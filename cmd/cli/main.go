package main

import (
	"fmt"
	"github.com/spf13/cobra"
	"harness/cmd/auth"
	"harness/cmd/registry"
	"harness/internal/config"
	"os"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "harness",
		Short: "CLI tool for Harness",
		Long:  `Harness CLI is a tool to interact with Harness Resources`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Skip loading config for auth commands
			if cmd.CommandPath() == "harness auth" ||
				cmd.CommandPath() == "harness auth login" ||
				cmd.CommandPath() == "harness auth logout" ||
				cmd.CommandPath() == "harness auth status" {
				return
			}

			// Check if authentication is needed
			if config.Global.APIBaseURL == "" || config.Global.AuthToken == "" || config.Global.AccountID == "" {
				// Only show auth error if we're not displaying help or completion
				if cmd.Name() != "help" && !cmd.IsAdditionalHelpTopicCommand() && cmd.Name() != "completion" {
					fmt.Println("Not logged in. Please run 'harness auth login' first.")
					os.Exit(1)
				}
			}
		},
	}

	// Persistent flags available to all commands - bind them directly to global config
	rootCmd.PersistentFlags().StringVar(&config.Global.APIBaseURL, "api-url", "",
		"Base URL for the API (overrides saved config)")
	rootCmd.PersistentFlags().StringVar(&config.Global.AuthToken, "token", "",
		"Authentication token (overrides saved config)")
	rootCmd.PersistentFlags().StringVar(&config.Global.AccountID, "account", "", "Account (overrides saved config)")
	rootCmd.PersistentFlags().StringVar(&config.Global.OrgID, "org", "", "Org (overrides saved config)")
	rootCmd.PersistentFlags().StringVar(&config.Global.ProjectID, "project", "", "Project (overrides saved config)")

	authConfig, err := loadAuthConfig()
	if err == nil {
		// Use config values if not overridden by flags
		if config.Global.APIBaseURL == "" {
			config.Global.APIBaseURL = authConfig.BaseURL
		}
		if config.Global.AuthToken == "" {
			config.Global.AuthToken = authConfig.Token
		}
		if config.Global.AccountID == "" {
			config.Global.AccountID = authConfig.AccountID
		}
		if config.Global.OrgID == "" {
			config.Global.OrgID = authConfig.OrgID
		}
		if config.Global.ProjectID == "" {
			config.Global.ProjectID = authConfig.ProjectID
		}
	}

	// Add main command groups
	rootCmd.AddCommand(auth.GetRootCmd())
	rootCmd.AddCommand(registry.GetRootCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
