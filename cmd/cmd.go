package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/harness/harness-cli/cmd/ar"
	"github.com/harness/harness-cli/cmd/auth"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/util/templates"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var rootCmd *cobra.Command

// Execute executes the root command.
func Execute() {
	initCmd()
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func initCmd() {
	var logFilePath string
	rootCmd = &cobra.Command{
		Use:   "hns",
		Short: "CLI tool for Harness",
		Long: templates.LongDesc(`
      Harness CLI is a tool to interact with Harness Resources.

      Find more information at:
            https://developer.harness.io/docs/`),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Skip loading config for auth commands
			if cmd.CommandPath() == "hns auth" ||
				cmd.CommandPath() == "hns auth login" ||
				cmd.CommandPath() == "hns auth logout" ||
				cmd.CommandPath() == "hns auth status" {
				return nil
			}

			// Check if authentication is needed
			if config.Global.APIBaseURL == "" || config.Global.AuthToken == "" || config.Global.AccountID == "" {
				// Only show auth error if we're not displaying help or completion
				if cmd.Name() != "help" && !cmd.IsAdditionalHelpTopicCommand() && cmd.Name() != "completion" {
					fmt.Println("Not logged in. Please run 'hns auth login' first.")
					os.Exit(1)
				}
			}

			// Configure logging based on flags
			if logFilePath != "" {
				// Ensure the directory exists
				logDir := filepath.Dir(logFilePath)
				if err := os.MkdirAll(logDir, 0755); err != nil {
					fmt.Printf("Warning: Could not create log directory: %v\n", err)
				}

				// Open the log file
				logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
				if err != nil {
					fmt.Printf("Warning: Could not open log file: %v\n", err)
				} else {
					// Set up log writer with timestamp format
					logWriter := zerolog.ConsoleWriter{
						Out:        logFile,
						TimeFormat: time.RFC3339,
						NoColor:    true,
					}
					log.Logger = log.Output(logWriter)
				}
			} else {
				// If no log file specified, disable logging
				log.Logger = log.Output(io.Discard)
				zerolog.SetGlobalLevel(zerolog.Disabled)
			}

			return initProfiling()
		},

		PersistentPostRunE: func(*cobra.Command, []string) error {
			if err := flushProfiling(); err != nil {
				return err
			}
			return nil
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
	rootCmd.PersistentFlags().StringVar(&config.Global.Format, "format", "table", "Format of the result")

	// Add log file path flag
	rootCmd.PersistentFlags().StringVar(&logFilePath, "log-file", "",
		"Path to store logs (if not provided, logging is disabled)")

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
	rootCmd.AddCommand(ar.GetRootCmd())

	flags := rootCmd.PersistentFlags()

	addProfilingFlags(flags)
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

}
