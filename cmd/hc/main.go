package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/harness/harness-cli/cmd/artifact"
	"github.com/harness/harness-cli/cmd/auth"
	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/cmd/registry"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/style"
	"github.com/harness/harness-cli/internal/terminal"
	"github.com/harness/harness-cli/internal/tui"
	"github.com/harness/harness-cli/util/templates"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// version is set via ldflags during build
var version = "dev"

func main() {
	var (
		verbose     bool
		noColor     bool
		interactive bool
		jsonFlag    bool
	)
	factory := cmdutils.NewFactory()

	rootCmd := &cobra.Command{
		Use:           "hc",
		Short:         "CLI tool for Harness",
		SilenceUsage:  true,
		SilenceErrors: true, //prevent duplicate printing of errors
		Long: templates.LongDesc(`
      Harness CLI is a tool to interact with Harness Resources.

      Run without arguments in a terminal for an interactive experience,
      or use subcommands for scripted / CI workflows.

      Find more information at:
            https://developer.harness.io/docs/platform/automation/cli/reference/#v1.0.0-hc`),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// ── Initialise terminal & style ─────────────────────────────
			termInfo := terminal.Detect(noColor, interactive, jsonFlag)
			style.Init(termInfo.ColorEnabled)

			// Override format to JSON when --json is explicitly passed
			if termInfo.ForceJSON {
				config.Global.Format = "json"
			}

			// Skip loading config for auth commands, version, and upgrade
			if cmd.CommandPath() == "hc auth" ||
				cmd.CommandPath() == "hc auth login" ||
				cmd.CommandPath() == "hc auth logout" ||
				cmd.CommandPath() == "hc auth status" ||
				cmd.CommandPath() == "hc version" ||
				cmd.CommandPath() == "hc upgrade" {
				return nil
			}

			// Check if authentication is needed
			if config.Global.APIBaseURL == "" || config.Global.AuthToken == "" {
				// Only show auth error if we're not displaying help or completion
				if cmd.Name() != "help" && !cmd.IsAdditionalHelpTopicCommand() && cmd.Name() != "completion" {
					if termInfo.ColorEnabled {
						fmt.Fprintln(os.Stderr, style.Error.Render("Not logged in."))
						fmt.Fprintln(os.Stderr, style.Hint("Run 'hc auth login' to authenticate."))
					} else {
						fmt.Fprintln(os.Stderr, "Not logged in. Please run 'hc auth login' first.")
					}
					os.Exit(1)
				}
			}

			var err error
			if config.Global.AccountID == "" {
				config.Global.AccountID, err = auth.GetAccountIDFromToken(config.Global.AuthToken)
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
			}

			// Set up logging based on verbose flag
			if verbose {
				logWriter := zerolog.ConsoleWriter{
					Out:        os.Stderr,
					TimeFormat: time.RFC3339,
					NoColor:    noColor,
				}
				log.Logger = log.Output(logWriter)
			} else {
				// Disable logging when verbose is not enabled
				log.Logger = zerolog.Nop()
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
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging to console")

	// Interactive / display flags
	rootCmd.PersistentFlags().BoolVarP(&interactive, "interactive", "i", false,
		"Force interactive TUI mode (requires a terminal)")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false,
		"Disable colour output (also respects NO_COLOR env)")
	rootCmd.PersistentFlags().BoolVar(&jsonFlag, "json", false,
		"Output results as JSON (equivalent to --format=json)")

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

	// Check environment variables (override auth config, flags will override during Execute)
	if envVal := os.Getenv("HARNESS_API_URL"); envVal != "" {
		config.Global.APIBaseURL = envVal
	}
	if envVal := os.Getenv("HARNESS_API_KEY"); envVal != "" {
		config.Global.AuthToken = envVal
	}
	if envVal := os.Getenv("HARNESS_ORG_ID"); envVal != "" {
		config.Global.OrgID = envVal
	}
	if envVal := os.Getenv("HARNESS_PROJECT_ID"); envVal != "" {
		config.Global.ProjectID = envVal
	}

	// Add main command groups
	rootCmd.AddCommand(auth.GetRootCmd())
	rootCmd.AddCommand(registry.GetRootCmd(factory))
	rootCmd.AddCommand(artifact.GetRootCmd(factory))
	//rootCmd.AddCommand(project.GetRootCmd())
	//rootCmd.AddCommand(organisation.GetRootCmd())
	//rootCmd.AddCommand(api.GetRootCmd())
	rootCmd.AddCommand(versionCmd())
	rootCmd.AddCommand(upgradeCmd())

	flags := rootCmd.PersistentFlags()

	addProfilingFlags(flags)
	//zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	// ── Enhanced help text ───────────────────────────────────────────────
	// Apply styled help template when running in a colour-capable terminal
	termPreCheck := terminal.Detect(noColor, false, false)
	style.Init(termPreCheck.ColorEnabled)
	if helpTpl := tui.StyledHelpTemplate(); helpTpl != "" {
		rootCmd.SetUsageTemplate(helpTpl)
	}

	// ── Interactive entry point ──────────────────────────────────────────
	// If the user runs `hc` with no subcommand in a TTY (or passes -i),
	// launch the interactive main menu instead of printing help.
	if shouldLaunchInteractive(os.Args[1:], interactive) {
		style.Init(!noColor)
		runInteractiveMode(rootCmd, factory)
		return
	}

	if err := rootCmd.Execute(); err != nil {
		termInfo := terminal.Detect(noColor, false, false)
		if termInfo.IsTerminal && termInfo.ColorEnabled {
			fmt.Fprintln(os.Stderr, style.Error.Render("Error: "+err.Error()))
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}

// shouldLaunchInteractive returns true when the TUI main menu should be shown.
//   - explicit --interactive/-i flag → always (if TTY)
//   - no subcommand args and stdout is a TTY → auto-launch
func shouldLaunchInteractive(args []string, forceInteractive bool) bool {
	termInfo := terminal.Detect(false, forceInteractive, false)

	// Explicit --interactive / -i
	if forceInteractive && termInfo.IsTerminal {
		return true
	}

	// Auto-launch when no args (or only global flags) and TTY
	if !termInfo.IsTerminal {
		return false
	}

	// Strip known global flags to see if a subcommand was given
	stripped := stripGlobalFlags(args)
	return len(stripped) == 0
}

// stripGlobalFlags removes known flag tokens so we can detect bare `hc` invocations.
func stripGlobalFlags(args []string) []string {
	// Flags that take a value (next token is the value)
	valueFlags := map[string]bool{
		"--api-url": true, "--token": true, "--account": true,
		"--org": true, "--project": true, "--format": true,
		"--profile": true, "--profile-output": true,
	}
	// Boolean flags (no value)
	boolFlags := map[string]bool{
		"--verbose": true, "-v": true,
		"--interactive": true, "-i": true,
		"--no-color": true, "--json": true,
	}

	var rest []string
	skip := false
	for _, a := range args {
		if skip {
			skip = false
			continue
		}
		if boolFlags[a] {
			continue
		}
		if valueFlags[a] {
			skip = true
			continue
		}
		// Handle --flag=value form
		if strings.HasPrefix(a, "--") && strings.Contains(a, "=") {
			key := a[:strings.Index(a, "=")]
			if valueFlags[key] || boolFlags[key] {
				continue
			}
		}
		rest = append(rest, a)
	}
	return rest
}

// runInteractiveMode launches the Bubble Tea main menu and dispatches the
// chosen command through the existing Cobra tree.
func runInteractiveMode(rootCmd *cobra.Command, factory *cmdutils.Factory) {
	choice, err := tui.RunMenu()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if choice == "" {
		// User quit without choosing
		os.Exit(0)
	}

	// Special handling for interactive-specific flows
	switch choice {
	case "registry list":
		// Use the rich interactive table
		if err := tui.RunRegistryList(factory); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	case "registry delete":
		// Prompt for name, then confirm
		name, err := tui.PromptInput(
			"Registry name",
			"Enter the name of the registry to delete",
			"my-registry",
		)
		if err != nil || name == "" {
			fmt.Println(style.DimText.Render("Cancelled."))
			return
		}
		confirmed, err := tui.ConfirmDeletion("registry", name)
		if err != nil || !confirmed {
			fmt.Println(style.DimText.Render("Deletion cancelled."))
			return
		}
		// Run the actual delete via Cobra
		rootCmd.SetArgs([]string{"registry", "delete", name})
		if err := rootCmd.Execute(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(style.Success.Render("✓ Registry deleted successfully"))
		return
	}

	// For all other choices, dispatch through the Cobra command tree
	cmdArgs := strings.Fields(choice)
	rootCmd.SetArgs(cmdArgs)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// versionCmd returns the version command
func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version number of hc",
		Long:  "Print the version number of the Harness CLI (hc)",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("hc version %s\n", version)
			fmt.Printf("Built with %s\n", runtime.Version())
		},
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
