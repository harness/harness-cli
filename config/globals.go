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

	// Request timeout in seconds (0 means no timeout)
	TimeoutSeconds int

	// Command-specific configurations
	Registry RegistryConfig
}

// RegistryConfig holds ar-specific configurations
type RegistryConfig struct {
	PkgURL string
	// For migrate command
	Migrate MigrateConfig

	// For status command
	Status StatusConfig

	// For other ar commands as needed
	// ...
}

// MigrateConfig holds migrate command specific configurations
type MigrateConfig struct {
	Concurrency int
	Overwrite   bool
	DryRun      bool
}

// StatusConfig holds status command specific configurations
type StatusConfig struct {
	MigrationID  string
	PollInterval int
}

// DefaultTimeoutSeconds is the default request timeout (0 means no timeout)
const DefaultTimeoutSeconds = 0

// Global is the shared instance of GlobalFlags
var Global = GlobalFlags{}
