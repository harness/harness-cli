package registry

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar"
	ar2 "github.com/harness/harness-cli/module/ar/migrate"
	"github.com/harness/harness-cli/module/ar/migrate/types"
	"github.com/harness/harness-cli/util"

	"github.com/spf13/cobra"
)

func getMigrateCmd(*cmdutils.Factory) *cobra.Command {
	// Create local variables for flag binding
	var localConfigPath string
	var localPkgBaseURL string
	var localConcurrency int
	var overwrite bool
	var dryRun bool
	var summary bool

	migrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "Start a migration based on configuration",
		Long: fmt.Sprintf(`Migrate artifacts from a source registry to a Harness Artifact Registry.

This command reads a YAML configuration file that defines the source and destination
registries, credentials, and artifact mappings.

Example configuration file (config.yaml):

  version: 1.0.0
  concurrency: 5
  overwrite: false

  source:
    endpoint: https://source-registry.example.com
    type: JFROG                    # Supported: JFROG, NEXUS, HARBOR
    credentials:
      username: source_user
      password: source_password
    insecure: false

  destination:
    endpoint: https://pkg.harness.io
    type: HAR
    credentials:
      username: harness_user
      password: harness_api_key

  mappings:
    - artifactType: DOCKER
      sourceRegistry: docker-repo
      destinationRegistry: harness-docker-repo

    - artifactType: MAVEN
      sourceRegistry: maven-releases
      destinationRegistry: harness-maven
      # MAVEN identity is derived from file paths, so filter via files (see note below).
      packageFilters:
        - files: [my-artifact-1.0.0.jar, my-artifact-1.1.0.pom]

    - artifactType: PYTHON
      sourceRegistry: pypi-local
      destinationRegistry: harness-pypi
      # PYTHON has real package identity, so all three granularities are meaningful.
      packageFilters:
        - package: requests
          versions: [2.28.0, 2.29.0]
          files: [requests-2.28.0-py3-none-any.whl]

    - artifactType: NPM
      sourceRegistry: npm-local
      destinationRegistry: harness-npm

    - artifactType: HELM
      sourceRegistry: helm-charts
      destinationRegistry: harness-helm

Supported artifact types:
  %s

Note: HARBOR source supports OCI artifact types only (DOCKER, HELM).

Package Filtering (opt-in):
  The packageFilters field is an allow-list: when set, only the named packages (and
  optionally specific versions/files) are migrated; when omitted, everything migrates.

  Supported granularity by type:
    - Package-level only: DOCKER, HELM, HELM_LEGACY, HELM_HTTP, RPM, DEBIAN, CONDA, COMPOSER, SWIFT, CONAN
    - Package + version: GO
    - Package + version + file: GENERIC, RAW, MAVEN, PYTHON, NUGET, NPM, DART, PUPPET

  For synthetic-identity types (MAVEN, NPM, GENERIC, RAW), filter via the files field
  rather than package names, as package/version names are derived from file paths.

Environment variables can be used in the config file using ${VAR_NAME} syntax.

Usage example:
  hc registry migrate -c config.yaml`, types.GetKnownArtifactTypesList()),
		Run: runMigration,
		PreRun: func(cmd *cobra.Command, args []string) {
			// Sync local flags to global config
			config.Global.ConfigPath = localConfigPath
			if localPkgBaseURL != "" {
				config.Global.Registry.PkgURL = util.GetPkgUrl(localPkgBaseURL)
			}
			config.Global.Registry.Migrate.Concurrency = localConcurrency
			config.Global.Registry.Migrate.Overwrite = overwrite
			config.Global.Registry.Migrate.DryRun = dryRun
			config.Global.Registry.Migrate.Summary = summary
		},
	}
	migrateCmd.Flags().StringVarP(&localConfigPath, "config", "c", "config.yaml", "Path to configuration file")
	migrateCmd.Flags().StringVar(&localPkgBaseURL, "pkg-url", "", "Base URL for the API (overrides config)")
	migrateCmd.Flags().IntVar(&localConcurrency, "concurrency", 1, "Number of concurrent operations (overrides config)")
	migrateCmd.Flags().BoolVar(&overwrite, "overwrite", false, "Allow overwriting artifacts")
	migrateCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Run migration in dry-run mode (no uploads, generates file list and directory structure)")
	migrateCmd.Flags().BoolVar(&summary, "summary", false, "Print a status summary instead of the full per-file table")

	migrateCmd.MarkFlagRequired("config")

	return migrateCmd
}

func runMigration(cmd *cobra.Command, args []string) {
	// Load configuration
	cfg, err := types.LoadConfig(config.Global.ConfigPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	if config.Global.Registry.Migrate.Concurrency > 1 {
		cfg.Concurrency = config.Global.Registry.Migrate.Concurrency
	}

	if config.Global.Registry.Migrate.Overwrite {
		cfg.Overwrite = true
	}

	if config.Global.Registry.Migrate.DryRun {
		cfg.DryRun = true
	}

	if config.Global.Registry.Migrate.Summary {
		cfg.Summary = true
	}

	// Create an API client for orchestration purpose. The registry clients will be initiated separately
	apiClient, _ := ar.NewClient(config.Global.APIBaseURL)
	//, config.Global.AuthToken, config.Global.AccountID,
	//	config.Global.OrgID, config.Global.ProjectID)

	// Set up context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a migration service
	migrationSvc, err := ar2.NewMigrationService(ctx, cfg, apiClient)
	if err != nil {
		log.Fatalf("Failed to create migration service: %v", err)
	}

	// Set up signal handling for graceful shutdown
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalChan
		fmt.Println("\nReceived interrupt signal, shutting down gracefully...")
		cancel()
	}()

	// Run the migration
	if err := migrationSvc.Run(ctx); err != nil {
		log.Fatalf("Migration failed: %v", err)
	}

	fmt.Println("Migration completed successfully")
}
