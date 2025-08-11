package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/harness/harness-cli/cmd/ar"
	"github.com/harness/harness-cli/cmd/auth"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/util/templates"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func main() {
	var logFilePath string
	rootCmd := &cobra.Command{
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

	// Load auth config
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

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// AuthConfig represents authentication configuration
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

func loadAuthConfig() (*AuthConfig, error) {
	configPath := getAuthConfigPath()

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var config AuthConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("error unmarshaling auth config: %w", err)
	}

	return &config, nil
}

var (
	profileName   string
	profileOutput string
)

func addProfilingFlags(flags *pflag.FlagSet) {
	flags.StringVar(&profileName, "profile", "none",
		"Name of profile to capture. One of (none|cpu|heap|goroutine|threadcreate|block|mutex)")
	flags.StringVar(&profileOutput, "profile-output", "profile.pprof", "Name of the file to write the profile to")
}

func initProfiling() error {
	var (
		f   *os.File
		err error
	)
	switch profileName {
	case "none":
		return nil
	case "cpu":
		f, err = os.Create(profileOutput)
		if err != nil {
			return err
		}
		err = pprof.StartCPUProfile(f)
		if err != nil {
			return err
		}
	// Block and mutex profiles need a call to Set{Block,Mutex}ProfileRate to
	// output anything. We choose to sample all events.
	case "block":
		runtime.SetBlockProfileRate(1)
	case "mutex":
		runtime.SetMutexProfileFraction(1)
	default:
		// Check the profile name is valid.
		if profile := pprof.Lookup(profileName); profile == nil {
			return fmt.Errorf("unknown profile '%s'", profileName)
		}
	}

	// If the command is interrupted before the end (ctrl-c), flush the
	// profiling files
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		f.Close()
		flushProfiling()
		os.Exit(0)
	}()

	return nil
}

func flushProfiling() error {
	switch profileName {
	case "none":
		return nil
	case "cpu":
		pprof.StopCPUProfile()
	case "heap":
		runtime.GC()
		fallthrough
	default:
		profile := pprof.Lookup(profileName)
		if profile == nil {
			return nil
		}
		f, err := os.Create(profileOutput)
		if err != nil {
			return err
		}
		defer f.Close()
		return profile.WriteTo(f, 0)
	}
	return nil
}
