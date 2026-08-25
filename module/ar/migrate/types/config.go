package types

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pterm/pterm"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

type RegistryType string

var (
	HAR        RegistryType = "HAR"
	JFROG      RegistryType = "JFROG"
	MOCK_JFROG RegistryType = "MOCK_JFROG"
	NEXUS      RegistryType = "NEXUS"
	HARBOR     RegistryType = "HARBOR"
)

type ArtifactType string

var (
	DOCKER      ArtifactType = "DOCKER"
	HELM        ArtifactType = "HELM"
	HELM_LEGACY ArtifactType = "HELM_LEGACY"
	HELM_HTTP   ArtifactType = "HELM_HTTP"
	GENERIC     ArtifactType = "GENERIC"
	PYTHON      ArtifactType = "PYTHON"
	MAVEN       ArtifactType = "MAVEN"
	NPM         ArtifactType = "NPM"
	NUGET       ArtifactType = "NUGET"
	RPM         ArtifactType = "RPM"
	DEBIAN      ArtifactType = "DEBIAN"
	GO          ArtifactType = "GO"
	CONDA       ArtifactType = "CONDA"
	COMPOSER    ArtifactType = "COMPOSER"
	DART        ArtifactType = "DART"
	RAW         ArtifactType = "RAW"
	SWIFT       ArtifactType = "SWIFT"
	PUPPET      ArtifactType = "PUPPET"
	CRAN        ArtifactType = "CRAN"
	RUBY        ArtifactType = "RUBY"
	CONAN       ArtifactType = "CONAN"
	TERRAFORM   ArtifactType = "TERRAFORM"
)

// knownArtifactTypesList is the exhaustive, ordered list of valid ArtifactType
// values and the SINGLE SOURCE OF TRUTH for "which types exist": config
// validation, the validation error message, and the `migrate --help` text are
// all derived from it. Add new types here (and to the var block above)
// whenever a new ArtifactType is introduced.
var knownArtifactTypesList = []ArtifactType{
	DOCKER, HELM, HELM_LEGACY, HELM_HTTP, GENERIC, PYTHON, MAVEN, NPM, NUGET,
	RPM, DEBIAN, GO, CONDA, COMPOSER, DART, RAW, SWIFT, PUPPET, CRAN, RUBY, CONAN,
	TERRAFORM,
}

// knownArtifactTypes is the lookup set derived from knownArtifactTypesList.
var knownArtifactTypes = func() map[ArtifactType]struct{} {
	m := make(map[ArtifactType]struct{}, len(knownArtifactTypesList))
	for _, t := range knownArtifactTypesList {
		m[t] = struct{}{}
	}
	return m
}()

// KnownArtifactTypes returns the ordered list of all valid ArtifactType
// values. Safe to mutate by the caller (a fresh copy is returned each time).
func KnownArtifactTypes() []ArtifactType {
	out := make([]ArtifactType, len(knownArtifactTypesList))
	copy(out, knownArtifactTypesList)
	return out
}

// KnownArtifactTypesString returns the valid types as a single
// comma-separated string for error messages and help text.
func KnownArtifactTypesString() string {
	parts := make([]string, len(knownArtifactTypesList))
	for i, t := range knownArtifactTypesList {
		parts[i] = string(t)
	}
	return strings.Join(parts, ", ")
}

// IsKnownArtifactType reports whether t is a recognised ArtifactType value.
func IsKnownArtifactType(t ArtifactType) bool {
	_, ok := knownArtifactTypes[t]
	return ok
}

// Config represents the top-level configuration structure
type Config struct {
	Version     string `yaml:"version"`
	Concurrency int    `yaml:"concurrency"`
	Overwrite   bool   `yaml:"overwrite"`
	DryRun      bool   `yaml:"dryRun"`
	Summary     bool   `yaml:"summary"`
	// ResultFile is an optional path to a JSON-lines file that receives one
	// per-coordinate result record (mirroring FileStat) at the end of a
	// non-dry-run migration, so automation never has to parse human tables.
	ResultFile string            `yaml:"resultFile,omitempty"`
	Source     RegistryConfig    `yaml:"source"`
	Dest       RegistryConfig    `yaml:"destination"`
	Mappings   []RegistryMapping `yaml:"mappings"`
}

// RegistryConfig defines the source ar configuration
type RegistryConfig struct {
	Endpoint    string            `yaml:"endpoint"`
	Type        RegistryType      `yaml:"type"`
	Credentials CredentialsConfig `yaml:"credentials,omitempty"`
	Insecure    bool              `yaml:"insecure" default:"false"`
}

type DateFilterMatch string

const (
	DateFilterMatchAny DateFilterMatch = "ANY"
	DateFilterMatchAll DateFilterMatch = "ALL"
)

// DateFilter defines the time-based filtering criteria for a registry mapping
type DateFilter struct {
	Match           DateFilterMatch `yaml:"match"`
	CreatedAfter    *time.Time      `yaml:"createdAfter"`
	DownloadedAfter *time.Time      `yaml:"downloadedAfter"`
}

// RegistryMapping defines the mapping between source and destination registries
// Slashes are used to defined the scope. The format would be
// - "registry": Create registry at Account level
// - "org/registry": Create registry at Org level
// - "org/project/registry": Create registry at Project level
type RegistryMapping struct {
	ArtifactType        ArtifactType `yaml:"artifactType"`
	SourceRegistry      string       `yaml:"sourceRegistry"`
	DestinationRegistry string       `yaml:"destinationRegistry"`
	// IncludePatterns/ExcludePatterns are glob patterns (* and **) applied at
	// file level for file-level-filterable types (GENERIC, RAW, PYTHON, MAVEN,
	// NUGET, NPM, DART, GO) and at package-name level for
	// package-level-filterable types (DOCKER, HELM, HELM_LEGACY, HELM_HTTP,
	// RPM, CONDA, COMPOSER, SWIFT, CONAN). Setting them for any other type
	// (DEBIAN, TERRAFORM, PUPPET) is a config error — scope controls must
	// never be silent no-ops. Only one of the two may be set per mapping.
	IncludePatterns []string `yaml:"includePatterns"`
	ExcludePatterns []string `yaml:"excludePatterns"`
	//Optional
	SourcePackageHostname string      `yaml:"sourcePackageHostname"`
	DateFilter            *DateFilter `yaml:"dateFilter"`
	// PackageFilters is an opt-in allow-list restricting the migration to specific
	// packages, and optionally specific versions/files within them. When empty, all
	// packages migrate (behavior-preserving). Supported granularity varies by artifact
	// type — see ValidatePackageFilters.
	PackageFilters []PackageSelector `yaml:"packageFilters,omitempty"`
}

// CredentialsConfig defines the credential configuration
type CredentialsConfig struct {
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

// LoadConfig loads the configuration from a file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	// Expand environment variables in the file
	expandedData := expandEnvInYaml(string(data))

	var config Config
	if err := yaml.Unmarshal([]byte(expandedData), &config); err != nil {
		return nil, fmt.Errorf("error parsing config file: %w", err)
	}

	// Validate the configuration
	if err := validateConfig(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// expandEnvInYaml expands environment variables in YAML content
func expandEnvInYaml(content string) string {
	// Process ${VAR} style environment variables
	result := os.Expand(content, func(key string) string {
		return os.Getenv(key)
	})

	return result
}

// validateConfig performs basic validation on the configuration
func validateConfig(config *Config) error {
	// Check migration configuration
	if config.Concurrency <= 0 {
		return fmt.Errorf("concurrency must be greater than 0")
	}

	// Validate source and destination registry configurations
	if err := validateCredentials(config.Source); err != nil {
		return fmt.Errorf("invalid source credentials block provided in config: %w", err)
	}

	if err := validateCredentials(config.Dest); err != nil {
		return fmt.Errorf("invalid destination credentials block provided in config: %w", err)
	}

	// Validate registry mappings
	if len(config.Mappings) == 0 {
		return fmt.Errorf("at least one registry mapping must be defined")
	}
	// Validate each mapping
	for i, mapping := range config.Mappings {
		if mapping.SourceRegistry == "" {
			return fmt.Errorf("mapping %d: source registry cannot be empty", i)
		}
		if mapping.DestinationRegistry == "" {
			return fmt.Errorf("mapping %d: destination registry cannot be empty", i)
		}
		if !IsKnownArtifactType(mapping.ArtifactType) {
			return fmt.Errorf("mapping %d: unknown artifactType %q — valid values are: %s", i, mapping.ArtifactType, KnownArtifactTypesString())
		}
		// Date filtering for MAVEN relies on the source file listing rather than
		// maven-metadata.xml, so the metadata file may end up out of sync with
		// the filtered set of artifacts that actually get migrated.
		if mapping.ArtifactType == MAVEN && mapping.DateFilter != nil {
			msg := fmt.Sprintf("mapping %d: date filter is enabled for %s — "+
				"maven-metadata.xml may not be in sync with the migrated artifacts", i, MAVEN)
			log.Warn().Msg(msg)
			pterm.Warning.Println(msg)
		}
		if err := ValidatePackageFilters(mapping.PackageFilters, mapping.ArtifactType); err != nil {
			return fmt.Errorf("mapping %d: %w", i, err)
		}
		// Scope controls must never be silent no-ops: include/exclude patterns
		// only take effect for pattern-filterable types (file-level or
		// package-level); for every other type they would be ignored entirely.
		if (len(mapping.IncludePatterns) > 0 || len(mapping.ExcludePatterns) > 0) &&
			!IsPatternFilterable(mapping.ArtifactType) {
			return fmt.Errorf("mapping %d: includePatterns/excludePatterns are not supported for artifact type %s — "+
				"patterns are applied at file level for %s and at package level for %s; use packageFilters for scoping where supported",
				i, mapping.ArtifactType,
				"GENERIC, RAW, PYTHON, MAVEN, NUGET, NPM, DART, GO , RUBY",
				"DOCKER, HELM, HELM_LEGACY, HELM_HTTP, RPM, CONDA, COMPOSER, SWIFT, CONAN")
		}
		// includePatterns and excludePatterns are mutually exclusive: applying
		// both would silently discard excludePatterns (FilterFilesByPatterns uses
		// else-if). Catch this at config validation time so it never reaches the
		// migration step.
		if len(mapping.IncludePatterns) > 0 && len(mapping.ExcludePatterns) > 0 {
			return fmt.Errorf("mapping %d: includePatterns and excludePatterns are mutually exclusive — only one may be set per mapping", i)
		}
		// Index-seeded / metadata-driven types seed enumeration from a
		// repository index (PyPI's .pypi HTML, Conda's repodata, RPM's
		// repomd/primary) rather than from the raw file listing, so a
		// date-filtered run can silently omit in-scope content: the filter
		// narrows which files migrate, but the index decides what exists.
		// A full no-filter run with overwrite:false is the completeness path.
		if mapping.DateFilter != nil && isIndexSeededArtifact(mapping.ArtifactType) {
			msg := fmt.Sprintf("mapping %d: date filter is enabled for %s — "+
				"date-filtered runs of index-seeded types can omit in-scope content; "+
				"a full run with overwrite:false is the completeness path", i, mapping.ArtifactType)
			log.Warn().Msg(msg)
			pterm.Warning.Println(msg)
		}
	}

	return nil
}

// isIndexSeededArtifact reports whether the type's package/version enumeration
// is seeded from a repository index or metadata file (PyPI's .pypi HTML,
// Conda's repodata.json, RPM's repomd/primary.xml) rather than purely from the
// raw file listing — the set of types for which a date-filtered run can
// under-migrate in-scope content (see the date-filter warning in
// validateConfig).
func isIndexSeededArtifact(t ArtifactType) bool {
	switch t {
	case PYTHON, CONDA, RPM:
		return true
	default:
		return false
	}
}

func validateCredentials(registry RegistryConfig) error {
	// Check that the endpoint is not empty
	if registry.Endpoint == "" {
		return fmt.Errorf("registry endpoint cannot be empty")
	}

	// Validate registry type
	if registry.Type == "" {
		return fmt.Errorf("registry type cannot be empty")
	}

	// Check supported registry types
	switch registry.Type {
	case HAR, JFROG, NEXUS, HARBOR, MOCK_JFROG:
		// These are supported
	default:
		return fmt.Errorf("unsupported registry type: %s", registry.Type)
	}

	// Validate credentials
	// Authentication must be provided via either token or username
	hasUsername := registry.Credentials.Username != ""
	hasToken := registry.Credentials.Password != ""

	if !hasToken && !hasUsername {
		return fmt.Errorf("either token or username must be provided for authentication")
	}

	if hasUsername && registry.Credentials.Password == "" {
		return fmt.Errorf("password must be provided when using username authentication")
	}

	return nil
}
