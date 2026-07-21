package upload

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/harness/harness-cli/cmd/artifact/command/utils"
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/util/common/progress"
	"github.com/harness/harness-cli/util/common/upload"
)

// RawUploader implements Pusher for raw artifact uploads.
// path – files land exactly at <DestTemplate>/<FILE_PATH> under /files/.

type RawUploader struct {
	SrcPattern   string
	DestTemplate string // path prefix within the registry; may contain {N} placeholders
	RegistryName string
	PkgClient    *pkgclient.ClientWithResponses
}

// GetRegistryAndPath parses a raw target of the form "<registry>/<dest-path>".
func (u *RawUploader) GetRegistryAndPath(target string) (string, error) {
	idx := strings.IndexByte(target, '/')
	if idx < 0 {
		return "", fmt.Errorf("target must be in the form <registry>/<path>, got %q", target)
	}
	u.RegistryName = target[:idx]
	u.DestTemplate = target[idx+1:]
	return u.RegistryName, nil
}

func (u *RawUploader) GetFiles() ([]upload.FileUploadJob, UploadStats, error) {
	var stats UploadStats

	root, relPattern := splitPatternRoot(u.SrcPattern)

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, stats, fmt.Errorf("cannot resolve source root %q: %w", root, err)
	}

	// Literal path fast-path (no wildcards).
	if relPattern == "" {
		info, err := os.Stat(absRoot)
		if err != nil {
			return nil, stats, fmt.Errorf("cannot access %q: %w", u.SrcPattern, err)
		}
		if !info.Mode().IsRegular() {
			return nil, stats, fmt.Errorf("%q is not a regular file", u.SrcPattern)
		}
		basename := filepath.Base(absRoot)
		dest := resolveRawDestPath(u.DestTemplate, nil, basename)
		checksums, err := utils.ComputeFileChecksums(absRoot)
		if err != nil {
			return nil, stats, fmt.Errorf("checksum %s: %w", absRoot, err)
		}
		job := upload.NewRawUploadJob(basename, absRoot, dest, u.RegistryName, info.Size(), checksums, u.PkgClient)
		stats.FileCount = 1
		stats.TotalBytes = info.Size()
		return []upload.FileUploadJob{job}, stats, nil
	}

	// Compile the relative pattern into a regexp with capture groups.
	re, _, err := compileWildcardPattern(relPattern)
	if err != nil {
		return nil, stats, fmt.Errorf("invalid pattern %q: %w", u.SrcPattern, err)
	}

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
		dest := resolveRawDestPath(u.DestTemplate, captures, relPath)

		checksums, err := utils.ComputeFileChecksums(path)
		if err != nil {
			return fmt.Errorf("checksum %s: %w", path, err)
		}

		jobs = append(jobs, upload.NewRawUploadJob(relPath, path, dest, u.RegistryName, info.Size(), checksums, u.PkgClient))
		stats.FileCount++
		stats.TotalBytes += info.Size()
		return nil
	})
	if walkErr != nil {
		return nil, stats, fmt.Errorf("failed to walk %s: %w", absRoot, walkErr)
	}

	return jobs, stats, nil
}

// PushFiles runs the shared upload engine on the provided jobs.
func (u *RawUploader) PushFiles(ctx context.Context, jobs []upload.FileUploadJob) error {
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

func resolveRawDestPath(template string, captures []string, relPath string) string {
	dest := template
	for i, cap := range captures {
		dest = strings.ReplaceAll(dest, fmt.Sprintf("{%d}", i+1), cap)
	}
	dest = strings.TrimSuffix(dest, "/")

	// Preserve subdirectory structure when no captures are used.
	// When captures are present the user has explicit path control via {N},
	// so only the basename is appended to avoid duplication.
	filePathInDest := relPath
	if len(captures) > 0 {
		filePathInDest = filepath.Base(relPath)
	}
	return dest + "/" + filePathInDest
}
