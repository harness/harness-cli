package upload

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/harness/harness-cli/cmd/artifact/command/utils"
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/util/common/progress"
	"github.com/harness/harness-cli/util/common/upload"
)

// GenericUploader implements Pusher for generic artifact uploads using
//   - – matches any characters within a single path segment (no slash)
//     **         – matches any characters across zero or more path segments
//     (*)        – like * but captures the matched segment as a numbered group {1}, {2}, …
//     (**)       – like ** but captures the matched remainder as a numbered group
//     ?          – matches exactly one character

type GenericUploader struct {
	SrcPattern   string
	DestTemplate string // package path within the registry; may contain {N} placeholders
	RegistryName string
	Version      string
	DryRun       bool
	PkgClient    *pkgclient.ClientWithResponses
}

// GetRegistryAndPath parses a generic target of the form "<registry>/<dest-path>"

func (u *GenericUploader) GetRegistryAndPath(target string) (string, error) {
	idx := strings.IndexByte(target, '/')
	if idx < 0 {
		return "", fmt.Errorf("target must be in the form <registry>/<path>, got %q", target)
	}
	u.RegistryName = target[:idx]
	u.DestTemplate = target[idx+1:]
	return u.RegistryName, nil
}

// GetFiles expands SrcPattern and returns one GenericUploadJob per matched file.
func (u *GenericUploader) GetFiles() ([]upload.FileUploadJob, UploadStats, error) {
	var stats UploadStats

	version := u.Version
	if version == "" {
		version = "1.0.0"
	}

	// Determine the walk root (the longest non-wildcard directory prefix).
	root, relPattern := splitPatternRoot(u.SrcPattern)

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, stats, fmt.Errorf("cannot resolve source root %q: %w", root, err)
	}

	//  Handle the literal-path fast path (no wildcards)
	if relPattern == "" {
		info, err := os.Stat(absRoot)
		if err != nil {
			return nil, stats, fmt.Errorf("cannot access %q: %w", u.SrcPattern, err)
		}
		if !info.Mode().IsRegular() {
			return nil, stats, fmt.Errorf("%q is not a regular file", u.SrcPattern)
		}
		dest := resolveDestPath(u.DestTemplate, version, []string{}, filepath.Base(absRoot))
		checksums, err := utils.ComputeFileChecksums(absRoot)
		if err != nil {
			return nil, stats, fmt.Errorf("checksum %s: %w", absRoot, err)
		}
		job := upload.NewGenericUploadJob(
			filepath.Base(absRoot), absRoot, dest,
			u.RegistryName, "", "", info.Size(), checksums, u.PkgClient,
		)
		stats.FileCount = 1
		stats.TotalBytes = info.Size()
		return []upload.FileUploadJob{job}, stats, nil
	}

	//Compile the relative pattern into a regexp with capture groups
	re, _, err := compileWildcardPattern(relPattern)
	if err != nil {
		return nil, stats, fmt.Errorf("invalid pattern %q: %w", u.SrcPattern, err)
	}

	// Walk the root and collect matching files
	var jobs []upload.FileUploadJob

	walkErr := filepath.WalkDir(absRoot, func(path string, d os.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if path == absRoot {
			return nil
		}

		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		// Compute path relative to the walk root (forward slashes).
		relPath, err := filepath.Rel(absRoot, path)
		if err != nil {
			return fmt.Errorf("rel %s: %w", path, err)
		}
		relPath = filepath.ToSlash(relPath)

		// Test the relative path against the compiled pattern.
		matches := re.FindStringSubmatch(relPath)
		if matches == nil {
			return nil
		}

		// Extract capture groups (matches[0] is the full match).
		captures := matches[1:]

		dest := resolveDestPath(u.DestTemplate, version, captures, filepath.Base(relPath))

		checksums, err := utils.ComputeFileChecksums(path)
		if err != nil {
			return fmt.Errorf("checksum %s: %w", path, err)
		}

		jobs = append(jobs, upload.NewGenericUploadJob(
			relPath, path, dest,
			u.RegistryName, "", "", info.Size(), checksums, u.PkgClient,
		))
		stats.FileCount++
		stats.TotalBytes += info.Size()
		return nil
	})
	if walkErr != nil {
		return nil, stats, fmt.Errorf("failed to walk %s: %w", absRoot, walkErr)
	}

	return jobs, stats, nil
}

func (u *GenericUploader) PreUpload(jobs []upload.FileUploadJob) (bool, error) {
	return runPreUpload(jobs, u.DryRun)
}

// PushFiles runs the shared upload engine on the provided jobs and reports
func (u *GenericUploader) PushFiles(ctx context.Context, jobs []upload.FileUploadJob) error {
	engine := upload.NewFileUploadEngine(upload.DefaultUploadWorker, progress.NewConsoleReporter())
	results := engine.Execute(ctx, jobs)

	if upload.HasUploadErrors(results) {
		fmt.Println("\nFailed uploads:")
		for _, r := range results {
			if !r.Success {
				fmt.Printf("  - %s: %v\n", r.JobID, r.Error)
			}
		}
		failed := len(results) - upload.GetSuccessfulUploads(results)
		return fmt.Errorf("%d of %d file(s) failed to upload", failed, len(results))
	}
	return nil
}

// containsWildcard reports whether s contains any wildcard metacharacter.
func containsWildcard(s string) bool {
	return strings.ContainsAny(s, "*?([")
}

// compileWildcardPattern converts a  relative pattern to a Go regexp.
//
// Token precedence (checked left to right):
//
//	(**)  →  (.+)        captures the full remaining path (slashes included)
//	(*)   →  ([^/]+)     captures one path segment
//	**/   →  (?:.*/)?    matches zero-or-more directory levels (no capture)
//	**    →  .*          matches anything (no capture)
//	*     →  [^/]+       matches within one segment (no capture)
//	?     →  [^/]        matches exactly one char within a segment
//	else  →  QuoteMeta
func compileWildcardPattern(pattern string) (*regexp.Regexp, int, error) {
	var sb strings.Builder
	sb.WriteString("^")
	groupCount := 0
	i := 0
	n := len(pattern)

	for i < n {
		switch {
		// (**) – capturing recursive wildcard
		case i+4 <= n && pattern[i:i+4] == "(**)":
			sb.WriteString("(.+)")
			groupCount++
			i += 4

		// (*) – capturing single-segment wildcard
		case i+3 <= n && pattern[i:i+3] == "(*)":
			sb.WriteString("([^/]+)")
			groupCount++
			i += 3

		// **/ – non-capturing zero-or-more directory levels
		case i+3 <= n && pattern[i:i+3] == "**/":
			sb.WriteString("(?:.*/)?")
			i += 3

		// ** – non-capturing match-anything
		case i+2 <= n && pattern[i:i+2] == "**":
			sb.WriteString(".*")
			i += 2

		// * – non-capturing single-segment match
		case pattern[i] == '*':
			sb.WriteString("[^/]+")
			i++

		// ? – non-capturing single-char match (no slash)
		case pattern[i] == '?':
			sb.WriteString("[^/]")
			i++

		default:
			sb.WriteString(regexp.QuoteMeta(string(pattern[i])))
			i++
		}
	}

	sb.WriteString("$")
	re, err := regexp.Compile(sb.String())
	return re, groupCount, err
}

// resolveDestPath computes the final destination path for one matched file.
//
// The Harness generic registry requires paths of the form package/version/file.
//   - template  – the package-path template (may contain {1}, {2}, …)
//   - version   – inserted between the resolved template and the basename
//   - captures  – ordered capture group values substituted into template
//   - basename  – the file's base name, always appended after version
func resolveDestPath(template, version string, captures []string, basename string) string {
	dest := template
	for i, cap := range captures {
		dest = strings.ReplaceAll(dest, fmt.Sprintf("{%d}", i+1), cap)
	}
	dest = strings.TrimSuffix(dest, "/")
	return dest + "/" + version + "/" + basename
}
