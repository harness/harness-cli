package config

// GlobalFlags contains common flags used across commands
type GlobalFlags struct {
	// Common authentication and connection flags
	APIBaseURL string
	AuthToken  string
	ConfigPath string
	AccountID  string
	OrgID      string
	ProjectID  string
	Format     string

	// Command-specific configurations
	Registry RegistryConfig
}

// RegistryConfig holds ar-specific configurations
type RegistryConfig struct {
	// For migrate command
	Migrate MigrateConfig

	// For status command
	Status StatusConfig

	// For other ar commands as needed
	// ...
}

// MigrateConfig holds migrate command specific configurations
type MigrateConfig struct {
	DryRun      bool
	Concurrency int
}

// StatusConfig holds status command specific configurations
type StatusConfig struct {
	MigrationID  string
	PollInterval int
}

// Global is the shared instance of GlobalFlags
var Global = GlobalFlags{}
