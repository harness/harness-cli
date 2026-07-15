package types

import (
	"errors"
	"io"
	"net/http"
	"sync"
	"time"
)

// Common errors
var (
	ErrUnsupportedRegistryType = errors.New("unsupported ar type")
	ErrArtifactNotFound        = errors.New("artifact not found")
	ErrRegistryNotFound        = errors.New("ar not found")
	ErrInvalidCredentials      = errors.New("invalid credentials")
	ErrArtifactAlreadyExists   = errors.New("artifact already exists")
)

type File struct {
	Name         string
	Registry     string
	Uri          string
	Folder       bool
	Size         int
	LastModified string
	SHA1         string
	SHA2         string
}

type TreeNode struct {
	Name     string
	Key      string
	Children []TreeNode
	IsLeaf   bool
	File     *File
}

type Package struct {
	Registry string
	Path     string
	Name     string
	Size     int
	URL      string
	Version  string
	Metadata map[string]string // Package-type specific metadata (e.g., Debian: distribution, component, etc.)
}

type Version struct {
	Registry string
	Pkg      string
	Name     string
	Path     string
	Size     int
}

type Artifact struct {
	Name       string
	Version    string
	Type       string
	Registry   string
	Size       int64
	Properties map[string]string
}

type Status string

const (
	StatusSuccess Status = "Success"
	StatusSkip    Status = "Skipped"
	StatusFail    Status = "Failed"
)

type FileStat struct {
	Name     string
	Registry string
	Uri      string
	Status   Status
	Size     int64
	Error    string
}

type TransferStats struct {
	mu        sync.Mutex
	FileStats []FileStat
}

// Add appends a single FileStat under the lock. Safe for concurrent use across
// many file jobs since TransferStats is always shared via a pointer.
func (s *TransferStats) Add(stat FileStat) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FileStats = append(s.FileStats, stat)
}

// Snapshot returns an independent copy of the current FileStats under the
// lock. Always returns a non-nil slice (len 0 for a fresh/nil TransferStats)
// so downstream marshalling/reporting never has to nil-check.
func (s *TransferStats) Snapshot() []FileStat {
	if s == nil {
		return make([]FileStat, 0)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]FileStat, len(s.FileStats))
	copy(out, s.FileStats)
	return out
}

type RegistryInfo struct {
	Type string
	URL  string
	Path string
}

const (
	ChartLayerMediaType = "application/vnd.cncf.helm.chart.layer.v1.tar+gzip"
	ConfigMediaType     = "application/vnd.cncf.helm.config.v1+json"
)

type HelmOCIConfig struct {
	APIVersion  string            `json:"apiVersion"`
	Created     time.Time         `json:"created"`
	Annotations map[string]string `json:"annotations"`
}

type PackageFiles struct {
	File         *File
	DownloadFile io.ReadCloser
	Header       *http.Header
}

// DryRunFileEntry represents a single file entry for dry-run output (from GetFiles)
type DryRunFileEntry struct {
	Registry     string `json:"registry"`
	Name         string `json:"name"`
	Uri          string `json:"uri"`
	Size         int    `json:"size"`
	LastModified string `json:"lastModified,omitempty"`
}

// DryRunDirectoryEntry represents the directory structure for dry-run output
type DryRunDirectoryEntry struct {
	Registry string                         `json:"registry"`
	Packages map[string]*DryRunPackageEntry `json:"packages"`
}

// DryRunPackageEntry represents a package in the directory structure
type DryRunPackageEntry struct {
	Name     string                         `json:"name"`
	Versions map[string]*DryRunVersionEntry `json:"versions"`
}

// DryRunVersionFileEntry represents a file with full details in the directory structure
type DryRunVersionFileEntry struct {
	Name         string `json:"name"`
	Registry     string `json:"registry"`
	Uri          string `json:"uri"`
	Size         int    `json:"size"`
	LastModified string `json:"lastModified,omitempty"`
}

// DryRunVersionEntry represents a version in the directory structure
type DryRunVersionEntry struct {
	Name  string                   `json:"name"`
	Files []DryRunVersionFileEntry `json:"files"`
}

// DownloadStat represents a single download stat entry for a searched file
type DownloadStat struct {
	Downloaded string `json:"downloaded"`
}

// SearchedFile represents a file entry returned by SearchFiles
type SearchedFile struct {
	Repo     string         `json:"repo"`
	Path     string         `json:"path"`
	Name     string         `json:"name"`
	Created  string         `json:"created"`
	Modified string         `json:"modified"`
	Stats    []DownloadStat `json:"stats"`
}

// DryRunStats holds the dry-run statistics.
//
// A single DryRunStats is shared across the registry/package/version/file jobs
// that the migration engine runs concurrently, so all access to the Files slice
// and the Directories map (and the nested maps/slices) must go through the
// methods below, which are guarded by mu.
type DryRunStats struct {
	mu          sync.Mutex
	Files       []DryRunFileEntry                // All files from GetFiles
	Directories map[string]*DryRunDirectoryEntry // Directory structure built incrementally
}

// AddFiles appends the given file entries to the shared Files slice.
func (s *DryRunStats) AddFiles(entries ...DryRunFileEntry) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Files = append(s.Files, entries...)
}

// ensureRegistryLocked returns the directory entry for registry, creating it if
// necessary. Callers must hold s.mu.
func (s *DryRunStats) ensureRegistryLocked(registry string) *DryRunDirectoryEntry {
	if s.Directories == nil {
		s.Directories = make(map[string]*DryRunDirectoryEntry)
	}
	dirEntry := s.Directories[registry]
	if dirEntry == nil {
		dirEntry = &DryRunDirectoryEntry{
			Registry: registry,
			Packages: make(map[string]*DryRunPackageEntry),
		}
		s.Directories[registry] = dirEntry
	}
	return dirEntry
}

// EnsureRegistry creates the directory entry for the given registry if it does
// not already exist.
func (s *DryRunStats) EnsureRegistry(registry string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureRegistryLocked(registry)
}

// EnsurePackage creates the registry and package entries if they do not already
// exist.
func (s *DryRunStats) EnsurePackage(registry, pkg string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	dirEntry := s.ensureRegistryLocked(registry)
	if dirEntry.Packages[pkg] == nil {
		dirEntry.Packages[pkg] = &DryRunPackageEntry{
			Name:     pkg,
			Versions: make(map[string]*DryRunVersionEntry),
		}
	}
}

// EnsureVersion creates the registry, package and version entries if they do
// not already exist.
func (s *DryRunStats) EnsureVersion(registry, pkg, version string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureVersionLocked(registry, pkg, version)
}

// ensureVersionLocked returns the version entry, creating the registry, package
// and version entries as needed. Callers must hold s.mu.
func (s *DryRunStats) ensureVersionLocked(registry, pkg, version string) *DryRunVersionEntry {
	dirEntry := s.ensureRegistryLocked(registry)
	pkgEntry := dirEntry.Packages[pkg]
	if pkgEntry == nil {
		pkgEntry = &DryRunPackageEntry{
			Name:     pkg,
			Versions: make(map[string]*DryRunVersionEntry),
		}
		dirEntry.Packages[pkg] = pkgEntry
	}
	versionEntry := pkgEntry.Versions[version]
	if versionEntry == nil {
		versionEntry = &DryRunVersionEntry{
			Name:  version,
			Files: make([]DryRunVersionFileEntry, 0),
		}
		pkgEntry.Versions[version] = versionEntry
	}
	return versionEntry
}

// AddVersionFile appends a file entry to the given registry/package/version,
// creating the intermediate entries if necessary.
func (s *DryRunStats) AddVersionFile(registry, pkg, version string, file DryRunVersionFileEntry) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	versionEntry := s.ensureVersionLocked(registry, pkg, version)
	versionEntry.Files = append(versionEntry.Files, file)
}
