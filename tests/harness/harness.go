// Package harness provides shared helpers for the end-to-end migration test
// suite under tests/. It builds the current branch's hc binary, provisions
// isolated HAR registries in a live (QA) environment, runs MOCK_JFROG -> HAR
// migration through that binary (hc registry migrate), and independently
// reconciles the files that were requested to be migrated against what is
// actually present in HAR.
//
// The package is intentionally not build-tagged so it is type-checked by a
// normal `go build ./...`; only the per-package *_test.go files carry the
// `//go:build e2e` tag so that `go test ./...` stays fast and non-mutating.
package harness

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

// Creds holds the live (QA) credentials and endpoints the suite needs. They are
// read from the environment by RequireEnv and are never written to disk.
type Creds struct {
	APIKey    string
	AccountID string
	OrgID     string
	ProjectID string
	APIURL    string // e.g. https://qa.harness.io
	PkgURL    string // e.g. https://pkg.qa.harness.io
}

// ExpectedFile identifies a single file expected to be present in the
// destination registry after migration, for package/version based artifact
// types (everything except RAW and the OCI types).
type ExpectedFile struct {
	Pkg      string
	Version  string
	FileName string
}

// ExpectedTag identifies an OCI image tag expected to be present in the
// destination registry after migration (DOCKER / HELM).
type ExpectedTag struct {
	Image string
	Tag   string
}

// Spec describes a single mapping to migrate and how to verify it.
type Spec struct {
	// Migration inputs.
	ArtifactType          string // migration artifactType, e.g. "RAW"
	PackageType           string // HAR registry packageType, e.g. "GENERIC"
	SourceRegistry        string // mock registry key, e.g. "raw-local"
	DestRegistry          string // HAR registry identifier (unique per run)
	IncludePatterns       []string
	ExcludePatterns       []string
	SourceEndpoint        string // defaults to defaultSourceEndpoint; OCI host for docker/helm
	SourcePackageHostname string
	Insecure              bool
	Overwrite             bool

	// Source registry type and credentials. Defaults preserve the mock source
	// (MOCK_JFROG with dummy creds) used by the offline suite. DOCKER/HELM live
	// tests set SourceType=JFROG and real credentials to migrate from an actual
	// OCI source registry.
	SourceType     types.RegistryType // defaults to MOCK_JFROG
	SourceUsername string             // defaults to "dummy"
	SourcePassword string             // defaults to "dummy"

	// Optional migration behavior. Zero values preserve the historical
	// single-mapping, concurrency=1, full-upload behavior so existing happy-path
	// tests are unaffected.
	Concurrency int               // config concurrency; defaults to 1 when <= 0
	DryRun      bool              // config dryRun: enumerate only, no uploads
	Summary     bool              // config summary: print stats table only
	DateFilter  *types.DateFilter // optional time-based filter on the mapping

	// Reconciliation expectations. Exactly one group is used depending on the
	// artifact type.
	ExpectedRawURIs []string       // RAW: file URIs expected present (HEAD)
	ExpectedFiles   []ExpectedFile // versioned types: (pkg, version, file)
	ExpectedTags    []ExpectedTag  // DOCKER/HELM: image tags

	// Negative reconciliation: assert these are ABSENT in the destination after
	// migration. Used by exclude-pattern, zero-file, and dest-mismatch
	// scenarios where a successful CLI exit must not be mistaken for a
	// successful migration.
	NotExpectedRawURIs []string
	NotExpectedFiles   []ExpectedFile
	NotExpectedTags    []ExpectedTag

	// ExpectZeroFiles asserts the migration selected zero files (e.g. filters
	// excluded everything). Reconcile must then confirm nothing landed.
	ExpectZeroFiles bool
}

// concurrency returns the configured concurrency, defaulting to 1.
func (s Spec) concurrency() int {
	if s.Concurrency > 0 {
		return s.Concurrency
	}
	return 1
}

const defaultSourceEndpoint = "http://mock-jfrog.local"

func (s Spec) sourceEndpoint() string {
	if s.SourceEndpoint != "" {
		return s.SourceEndpoint
	}
	return defaultSourceEndpoint
}

// ApplyGlobalConfig populates the process-wide config.Global so that adapters
// constructed in-process (for registry provisioning and reconciliation) talk to
// the same environment the built binary uses.
func ApplyGlobalConfig(creds Creds) {
	config.Global.APIBaseURL = creds.APIURL
	config.Global.AuthToken = creds.APIKey
	config.Global.AccountID = creds.AccountID
	config.Global.OrgID = creds.OrgID
	config.Global.ProjectID = creds.ProjectID
	config.Global.Registry.PkgURL = creds.PkgURL
}

// spaceRef returns the Harness scope path account[/org[/project]].
func (c Creds) spaceRef() string {
	parts := []string{c.AccountID}
	if c.OrgID != "" {
		parts = append(parts, c.OrgID)
		if c.ProjectID != "" {
			parts = append(parts, c.ProjectID)
		}
	}
	return strings.Join(parts, "/")
}

// registryRef returns the fully qualified registry reference
// account[/org[/project]]/identifier used as the registry_ref path param.
func (c Creds) registryRef(identifier string) string {
	return c.spaceRef() + "/" + identifier
}

// ScopeLevel selects the Harness scope at which a registry is provisioned.
type ScopeLevel string

const (
	ScopeAccount ScopeLevel = "account" // account only (no org/project)
	ScopeOrg     ScopeLevel = "org"     // account/org (no project)
	ScopeProject ScopeLevel = "project" // account/org/project (default)
)

// AtScope returns a copy of the credentials narrowed to the requested scope.
// Account scope clears both org and project; org scope clears only project.
// The returned Creds drive spaceRef/registryRef so registries are created and
// reconciled at the intended level.
func (c Creds) AtScope(scope ScopeLevel) Creds {
	switch scope {
	case ScopeAccount:
		c.OrgID = ""
		c.ProjectID = ""
	case ScopeOrg:
		c.ProjectID = ""
	}
	return c
}

// UniqueRegistry builds a run-unique, sanitized registry identifier so parallel
// branch runs never collide. An optional E2E_RUN_ID env var is folded in.
func UniqueRegistry(t *testing.T, prefix string) string {
	t.Helper()
	suffix := os.Getenv("E2E_RUN_ID")
	if suffix == "" {
		suffix = randHex(4)
	}
	id := fmt.Sprintf("e2e_%s_%s", prefix, suffix)
	return sanitizeIdentifier(id)
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "0000000000000000"[:n*2]
	}
	return hex.EncodeToString(b)
}

// sanitizeIdentifier lowercases and replaces any character outside [a-z0-9_-]
// with an underscore, matching HAR registry identifier constraints.
func sanitizeIdentifier(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// Run executes the full end-to-end flow for a single spec: provision the
// destination registry, run the migration with the built binary, reconcile the
// requested files against HAR, and clean up the registry afterwards.
func Run(t *testing.T, bin string, creds Creds, spec Spec) {
	t.Helper()

	ApplyGlobalConfig(creds)
	EnsureProject(t, creds)

	ref := CreateRegistry(t, creds, spec.DestRegistry, spec.PackageType)
	t.Cleanup(func() { DeleteRegistry(t, creds, ref) })

	cfgPath := WriteConfig(t, creds, spec)
	RunMigrate(t, bin, cfgPath, creds, spec)

	Reconcile(t, creds, spec)
}
