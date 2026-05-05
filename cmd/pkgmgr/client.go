package pkgmgr

import (
	regcmd "github.com/harness/harness-cli/cmd/registry/command"
	p "github.com/harness/harness-cli/util/common/progress"
)

// RegistryInfo holds detected HAR registry details.
type RegistryInfo struct {
	RegistryURL        string
	RegistryIdentifier string
	AccountID          string
	AuthToken          string
}

// InstallResult holds the result of running a native package manager command.
type InstallResult struct {
	Status string // "SUCCESS" or "FAILURE"
	Stderr string
	Err    error
}

// DependencyResult holds resolved dependencies and an optional cleanup function.
type DependencyResult struct {
	Dependencies []regcmd.Dependency
	Cleanup      func()
}

// Client defines the interface that each package manager must implement.
type Client interface {
	// Name returns the client name, e.g. "npm", "maven", "pip", "nuget".
	Name() string

	// PackageType returns the registry package type, e.g. "npm", "maven", "pypi", "nuget".
	PackageType() string

	// DetectRegistry detects the HAR registry from saved config or native config files.
	// explicitRegistry is an optional user-provided registry identifier.
	DetectRegistry(explicitRegistry string) (*RegistryInfo, error)

	// RunCommand executes the native package manager command (e.g. "npm install").
	// command is the subcommand ("install", "ci", etc.), args are pass-through arguments.
	RunCommand(command string, args []string) (*InstallResult, error)

	// ResolveDependencies returns the full dependency list (including transitive).
	// Used for firewall evaluation after a 403 is detected.
	ResolveDependencies(progress p.Reporter) (*DependencyResult, error)

	// DetectFirewallError checks if stderr contains a 403/firewall block pattern.
	DetectFirewallError(stderr string) bool

	// FallbackOrgProject returns org/project from saved client config.
	// Used as fallback when global config and env vars don't have them.
	FallbackOrgProject() (org string, project string)
}
