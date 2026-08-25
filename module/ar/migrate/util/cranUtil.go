package util

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/harness/harness-cli/module/ar/migrate/types"
)

const (
	cranContribDir         = "contrib"
	cranArchiveDir         = "Archive"
	cranSrcContribRepoPath = "src/contrib"
	cranSourceExt          = ".tar.gz"
	cranWindowsExt         = ".zip"
	cranMacOSExt           = ".tgz"
	cranOSWindows          = "windows"
	cranOSMacOS            = "macosx"
)

var (
	// cranRVersionRegex matches the "major.minor" R version used in contrib directory names (e.g. 4.4).
	cranRVersionRegex = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)
	// cranMacFlavorRegex validates a macOS build flavor directory such as "big-sur-arm64".
	cranMacFlavorRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	// cranPackageNameRegex validates an R package name (letters, digits and dots; must start with a
	// letter). R package names never contain underscores, which is what lets us split the file
	// name on the first underscore.
	cranPackageNameRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9.]*$`)

	// nonMacOSBinRoots excludes known non-mac roots (e.g. "linux") from being misidentified
	// as a mac build flavor by the path-shape heuristic below.
	nonMacOSBinRoots = []string{"linux"}
)

// CranPathInfo classifies a CRAN repo path and its HAR upload target.
type CranPathInfo struct {
	Type      string // "source" or "binary"
	OS        string // "" / "windows" / "macosx"
	Arch      string // macOS flavor when present
	RVersion  string // major.minor for binaries
	RepoPath  string // HAR contrib dir, e.g. "src/contrib" or "bin/macosx/big-sur-arm64/contrib/4.4"
	FileName  string
	Extension string
}

// DestUploadPath returns the flat HAR upload path; HAR recreates Archive/ itself.
func (p *CranPathInfo) DestUploadPath() string {
	if p == nil {
		return ""
	}
	return path.Join(p.RepoPath, p.FileName)
}

// IsCranIndexFile reports whether path is a PACKAGES index file (HAR regenerates these).
func IsCranIndexFile(filePath string) bool {
	name := path.Base(filePath)
	switch name {
	case "PACKAGES", "PACKAGES.gz", "PACKAGES.rds":
		return true
	default:
		return false
	}
}

// ParseCranUploadPath classifies a repo-relative CRAN path (live, Archive/, or Artifactory variants).
func ParseCranUploadPath(filePath string) (*CranPathInfo, error) {
	clean := strings.Trim(filePath, "/")
	if clean == "" {
		return nil, fmt.Errorf("ParseCranUploadPath: empty upload path")
	}
	segments := strings.Split(clean, "/")
	fileName := segments[len(segments)-1]

	switch {
	case segments[0] == "src":
		return parseCranSourcePath(segments, fileName)
	case segments[0] == "bin" && len(segments) >= 2 && segments[1] == cranOSWindows:
		return parseCranWindowsPath(segments, fileName)
	case segments[0] == "bin" && len(segments) >= 2 && segments[1] == cranOSMacOS:
		return parseCranMacOSPath(segments, fileName)
	case isArtifactoryMacOSBinLayout(segments):
		// Artifactory macOS layout: bin/<flavor>/contrib/<r-ver>/... (flavor is not "macosx").
		return parseCranArtifactoryMacOSPath(segments, fileName)
	default:
		return nil, fmt.Errorf("ParseCranUploadPath: unsupported R repository path %q", filePath)
	}
}

func parseCranSourcePath(segments []string, fileName string) (*CranPathInfo, error) {
	// Live:              src/contrib/<file>
	// Classic Archive:   src/contrib/Archive/<pkg>/<file>
	// Artifactory Archive: src/contrib/Archive/<pkg>/<version>/<file>
	switch {
	case len(segments) == 3 && segments[1] == cranContribDir:
		// live
	case len(segments) == 5 && segments[1] == cranContribDir && segments[2] == cranArchiveDir:
		if err := validateCranArchiveDirs(segments[3], "", fileName, cranSourceExt); err != nil {
			return nil, err
		}
	case len(segments) == 6 && segments[1] == cranContribDir && segments[2] == cranArchiveDir:
		if err := validateCranArchiveDirs(segments[3], segments[4], fileName, cranSourceExt); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("parseCranSourcePath: source packages must be under src/contrib/<file> "+
			"or src/contrib/Archive/<pkg>/[<version>/]<file>")
	}
	if !strings.HasSuffix(strings.ToLower(fileName), cranSourceExt) {
		return nil, fmt.Errorf("parseCranSourcePath: source packages must be .tar.gz, got %q", fileName)
	}
	return &CranPathInfo{
		Type:      "source",
		RepoPath:  cranSrcContribRepoPath,
		FileName:  fileName,
		Extension: cranSourceExt,
	}, nil
}

func parseCranWindowsPath(segments []string, fileName string) (*CranPathInfo, error) {
	// Live:                bin/windows/contrib/<r-ver>/<file>
	// Classic Archive:     bin/windows/contrib/<r-ver>/Archive/<pkg>/<file>
	// Artifactory Archive: bin/windows/contrib/<r-ver>/Archive/<pkg>/<version>/<file>
	var rVersion string
	switch {
	case len(segments) == 5 && segments[2] == cranContribDir:
		rVersion = segments[3]
	case len(segments) == 7 && segments[2] == cranContribDir && segments[4] == cranArchiveDir:
		rVersion = segments[3]
		if err := validateCranArchiveDirs(segments[5], "", fileName, cranWindowsExt); err != nil {
			return nil, err
		}
	case len(segments) == 8 && segments[2] == cranContribDir && segments[4] == cranArchiveDir:
		rVersion = segments[3]
		if err := validateCranArchiveDirs(segments[5], segments[6], fileName, cranWindowsExt); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf(
			"parseCranWindowsPath: windows binaries must be under bin/windows/contrib/<r-version>/<file> "+
				"or .../Archive/<pkg>/[<version>/]<file>")
	}
	if !cranRVersionRegex.MatchString(rVersion) {
		return nil, fmt.Errorf("parseCranWindowsPath: invalid R version %q", rVersion)
	}
	if !strings.HasSuffix(strings.ToLower(fileName), cranWindowsExt) {
		return nil, fmt.Errorf("parseCranWindowsPath: windows binaries must be .zip, got %q", fileName)
	}
	return &CranPathInfo{
		Type:      "binary",
		OS:        cranOSWindows,
		RVersion:  rVersion,
		RepoPath:  strings.Join([]string{"bin", cranOSWindows, cranContribDir, rVersion}, "/"),
		FileName:  fileName,
		Extension: cranWindowsExt,
	}, nil
}

func parseCranMacOSPath(segments []string, fileName string) (*CranPathInfo, error) {
	// Classic CRAN/HAR: bin/macosx[/<flavor>]/contrib/<r-ver>/...
	var flavor, rVersion string
	var repoSegments []string

	switch {
	case len(segments) == 6 && segments[3] == cranContribDir:
		flavor = segments[2]
		rVersion = segments[4]
		repoSegments = segments[:5]
		if err := validateMacFlavor(flavor); err != nil {
			return nil, err
		}
	case len(segments) == 5 && segments[2] == cranContribDir:
		rVersion = segments[3]
		repoSegments = segments[:4]
	case len(segments) == 8 && segments[3] == cranContribDir && segments[5] == cranArchiveDir:
		flavor = segments[2]
		rVersion = segments[4]
		repoSegments = segments[:5]
		if err := validateMacFlavor(flavor); err != nil {
			return nil, err
		}
		if err := validateCranArchiveDirs(segments[6], "", fileName, cranMacOSExt); err != nil {
			return nil, err
		}
	case len(segments) == 9 && segments[3] == cranContribDir && segments[5] == cranArchiveDir:
		flavor = segments[2]
		rVersion = segments[4]
		repoSegments = segments[:5]
		if err := validateMacFlavor(flavor); err != nil {
			return nil, err
		}
		if err := validateCranArchiveDirs(segments[6], segments[7], fileName, cranMacOSExt); err != nil {
			return nil, err
		}
	case len(segments) == 7 && segments[2] == cranContribDir && segments[4] == cranArchiveDir:
		rVersion = segments[3]
		repoSegments = segments[:4]
		if err := validateCranArchiveDirs(segments[5], "", fileName, cranMacOSExt); err != nil {
			return nil, err
		}
	case len(segments) == 8 && segments[2] == cranContribDir && segments[4] == cranArchiveDir:
		rVersion = segments[3]
		repoSegments = segments[:4]
		if err := validateCranArchiveDirs(segments[5], segments[6], fileName, cranMacOSExt); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("parseCranMacOSPath: unsupported macOS binary path")
	}
	return newMacOSPathInfo(flavor, rVersion, repoSegments, fileName)
}

// parseCranArtifactoryMacOSPath handles Artifactory mac layout (no "macosx" segment); rewrites to HAR path.
func parseCranArtifactoryMacOSPath(segments []string, fileName string) (*CranPathInfo, error) {
	flavor := segments[1]
	if err := validateMacFlavor(flavor); err != nil {
		return nil, fmt.Errorf("parseCranArtifactoryMacOSPath: %w", err)
	}

	var rVersion string
	switch {
	case len(segments) == 5 && segments[2] == cranContribDir:
		rVersion = segments[3]
	case len(segments) == 7 && segments[2] == cranContribDir && segments[4] == cranArchiveDir:
		rVersion = segments[3]
		if err := validateCranArchiveDirs(segments[5], "", fileName, cranMacOSExt); err != nil {
			return nil, err
		}
	case len(segments) == 8 && segments[2] == cranContribDir && segments[4] == cranArchiveDir:
		rVersion = segments[3]
		if err := validateCranArchiveDirs(segments[5], segments[6], fileName, cranMacOSExt); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("parseCranArtifactoryMacOSPath: unsupported Artifactory macOS path")
	}

	harRepo := strings.Join([]string{"bin", cranOSMacOS, flavor, cranContribDir, rVersion}, "/")
	return newMacOSPathInfo(flavor, rVersion, strings.Split(harRepo, "/"), fileName)
}

// newMacOSPathInfo validates the R version and extension shared by both macOS path variants,
// then builds the resulting CranPathInfo.
func newMacOSPathInfo(flavor, rVersion string, repoSegments []string, fileName string) (*CranPathInfo, error) {
	if !cranRVersionRegex.MatchString(rVersion) {
		return nil, fmt.Errorf("newMacOSPathInfo: invalid R version %q", rVersion)
	}
	if !strings.HasSuffix(strings.ToLower(fileName), cranMacOSExt) {
		return nil, fmt.Errorf("newMacOSPathInfo: macOS binaries must be .tgz, got %q", fileName)
	}
	return &CranPathInfo{
		Type:      "binary",
		OS:        cranOSMacOS,
		Arch:      flavor,
		RVersion:  rVersion,
		RepoPath:  strings.Join(repoSegments, "/"),
		FileName:  fileName,
		Extension: cranMacOSExt,
	}, nil
}

func validateMacFlavor(flavor string) error {
	if !cranMacFlavorRegex.MatchString(flavor) || flavor == cranContribDir || flavor == cranOSWindows {
		return fmt.Errorf("invalid macOS flavor %q", flavor)
	}
	return nil
}

// isArtifactoryMacOSBinLayout is true for bin/<flavor>/contrib/... mac builds (not linux/windows).
func isArtifactoryMacOSBinLayout(segments []string) bool {
	if len(segments) < 5 || segments[0] != "bin" {
		return false
	}
	flavor := segments[1]
	if flavor == cranOSWindows || flavor == cranOSMacOS || isNonMacOSBinRoot(flavor) {
		return false
	}
	if segments[2] != cranContribDir {
		return false
	}
	return validateMacFlavor(flavor) == nil
}

func isNonMacOSBinRoot(name string) bool {
	for _, root := range nonMacOSBinRoots {
		if name == root || strings.HasPrefix(name, root+"-") {
			return true
		}
	}
	return false
}

// validateCranArchiveDirs ensures Archive/<pkg>/[version/] matches the package (and optional
// version) embedded in the archive leaf. versionDir may be empty for classic CRAN Archive paths.
func validateCranArchiveDirs(pkgDir, versionDir, fileName, extension string) error {
	if !cranPackageNameRegex.MatchString(pkgDir) {
		return fmt.Errorf("validateCranArchiveDirs: invalid Archive package dir %q", pkgDir)
	}
	name, version, err := ParseCranPackageFileName(fileName, extension)
	if err != nil {
		return err
	}
	if name != pkgDir {
		return fmt.Errorf("validateCranArchiveDirs: Archive dir %q does not match package %q in %q",
			pkgDir, name, fileName)
	}
	if versionDir != "" && versionDir != version {
		return fmt.Errorf("validateCranArchiveDirs: Archive version dir %q does not match version %q in %q",
			versionDir, version, fileName)
	}
	return nil
}

// ParseCranPackageFileName splits an R archive leaf "<name>_<version>.<ext>" into package name and
// version. R package names never contain underscores, so the first underscore is the delimiter.
func ParseCranPackageFileName(fileName, extension string) (name, version string, err error) {
	base := strings.TrimSuffix(fileName, extension)
	if base == fileName {
		if idx := strings.LastIndex(strings.ToLower(fileName), extension); idx >= 0 {
			base = fileName[:idx]
		}
	}
	underscore := strings.IndexByte(base, '_')
	if underscore <= 0 || underscore == len(base)-1 {
		return "", "", fmt.Errorf("ParseCranPackageFileName: %q is not <name>_<version>%s", fileName, extension)
	}
	name = base[:underscore]
	version = base[underscore+1:]
	if !cranPackageNameRegex.MatchString(name) {
		return "", "", fmt.Errorf("ParseCranPackageFileName: invalid R package name %q", name)
	}
	if version == "" {
		return "", "", fmt.Errorf("ParseCranPackageFileName: missing version in %q", fileName)
	}
	return name, version, nil
}

// BuildCranPackageFilesMap groups CRAN archive files by package name in one pass.
func BuildCranPackageFilesMap(files []types.File) map[string][]types.File {
	out := make(map[string][]types.File)
	for _, file := range files {
		if file.Folder || IsCranIndexFile(file.Uri) {
			continue
		}
		pkgName, _, ok := ParseCranFileNameWithPath(file.Uri)
		if !ok {
			continue
		}
		out[pkgName] = append(out[pkgName], file)
	}
	return out
}

// ParseCranFileNameWithPath returns package name and version from a CRAN archive path.
func ParseCranFileNameWithPath(filePath string) (string, string, bool) {
	if IsCranIndexFile(filePath) {
		return "", "", false
	}
	pathInfo, err := ParseCranUploadPath(filePath)
	if err != nil {
		return "", "", false
	}
	name, version, err := ParseCranPackageFileName(pathInfo.FileName, pathInfo.Extension)
	if err != nil {
		return "", "", false
	}
	return name, version, true
}

// CranHarUploadPath returns the flat HAR destination path for a source-registry CRAN file URI.
func CranHarUploadPath(filePath string) (string, bool) {
	pathInfo, err := ParseCranUploadPath(filePath)
	if err != nil {
		return "", false
	}
	return pathInfo.DestUploadPath(), true
}
