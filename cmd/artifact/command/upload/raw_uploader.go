package upload

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/harness/harness-cli/cmd/artifact/command/utils"
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/util/common/progress"
	"github.com/harness/harness-cli/util/common/upload"

	"github.com/bmatcuk/doublestar/v4"
)

// RawUploader implements Pusher for raw artifact uploads.
// path – files land exactly at <DestTemplate>/<FILE_PATH> under /files/.

type RawUploader struct {
	SrcPattern   string
	DestTemplate string // path prefix within the registry
	RegistryName string
	PkgClient    *pkgclient.ClientWithResponses
}

// GetRegistryAndPath parses the target, which may be either:
//   - "<registry>/<dest-path>" – registry is the part before the first slash
func (u *RawUploader) GetRegistryAndPath(target string) (string, error) {
	idx := strings.IndexByte(target, '/')
	if idx < 0 {
		u.RegistryName = target
		u.DestTemplate = ""
	} else {
		u.RegistryName = target[:idx]
		u.DestTemplate = target[idx+1:]
	}
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
		dest := resolveRawDestPath(u.DestTemplate, basename)
		checksums, err := utils.ComputeFileChecksums(absRoot)
		if err != nil {
			return nil, stats, fmt.Errorf("checksum %s: %w", absRoot, err)
		}
		job := upload.NewRawUploadJob(basename, absRoot, dest, u.RegistryName, info.Size(), checksums, u.PkgClient)
		stats.FileCount = 1
		stats.TotalBytes = info.Size()
		return []upload.FileUploadJob{job}, stats, nil
	}

	// Use doublestar for glob matching: *, ?, [...], **.
	fsys := os.DirFS(absRoot)
	var jobs []upload.FileUploadJob

	walkErr := doublestar.GlobWalk(fsys, relPattern, func(relPath string, d fs.DirEntry) error {
		if d.IsDir() {
			return nil
		}
		//TODO to ignore other hidden file and git ignore etc too.

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", relPath, err)
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		absPath := filepath.Join(absRoot, filepath.FromSlash(relPath))
		dest := resolveRawDestPath(u.DestTemplate, relPath)

		checksums, err := utils.ComputeFileChecksums(absPath)
		if err != nil {
			return fmt.Errorf("checksum %s: %w", absPath, err)
		}

		jobs = append(jobs, upload.NewRawUploadJob(relPath, absPath, dest, u.RegistryName, info.Size(), checksums, u.PkgClient))
		stats.FileCount++
		stats.TotalBytes += info.Size()
		return nil
	})
	if walkErr != nil {
		return nil, stats, fmt.Errorf("invalid pattern or walk error for %q: %w", u.SrcPattern, walkErr)
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

// resolveRawDestPath computes the final destination path for a raw file upload.
// No version segment is inserted; the full relative path is always preserved
// so that source directory structure is mirrored in the registry.
// When template is empty the file lands at the registry root: /files/<relPath>.
func resolveRawDestPath(template, relPath string) string {
	prefix := strings.TrimSuffix(template, "/")
	if prefix == "" {
		return relPath
	}
	return prefix + "/" + relPath
}
