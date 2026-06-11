package command

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/harness/harness-cli/cmd/artifact/command/utils"
	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/util"
	"github.com/harness/harness-cli/util/common/progress"
	"github.com/harness/harness-cli/util/common/upload"

	"github.com/inhies/go-bytesize"
	"github.com/spf13/cobra"
)

// NewPushGenericCmd creates a new cobra.Command for pushing generic artifacts to the registry.
//
// Usage:
//
//	hc artifact push generic <registry> <path> [<path> ...] --name <pkg> [--version v]
//
// Each <path> can be either a file or a directory:
//
//   - File:      uploaded as-is to <pkg>/<version>/<basename>.
//   - Directory: walked recursively. Every regular file under it is uploaded
//     to <pkg>/<version>/<dir-basename>/<relative-path>, preserving the
//     directory's identity and internal layout.
func NewPushGenericCmd(c *cmdutils.Factory) *cobra.Command {
	var packageName, packageVersion, description, pkgURL string
	var includeHidden bool

	cmd := &cobra.Command{
		Use:   "generic <registry> <path> [<path>...]",
		Short: "Push Generic Artifacts",
		Long: "Push one or more generic artifacts to a Harness Artifact Registry. " +
			"Each <path> may be a file or a directory; directories are walked recursively.",
		Args: cobra.MinimumNArgs(2),
		PreRun: func(cmd *cobra.Command, args []string) {
			if pkgURL != "" {
				config.Global.Registry.PkgURL = util.GetPkgUrl(pkgURL)
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			registryName := args[0]
			paths := args[1:]

			version := packageVersion
			if version == "" {
				version = "1.0.0"
			}

			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			pkgClient := c.PkgHttpClient()

			fmt.Printf("Scanning %d input(s) ...\n", len(paths))
			jobs, stats, err := collectGenericUploadJobs(paths, registryName, packageName, version, includeHidden, pkgClient)
			if err != nil {
				return err
			}
			if len(jobs) == 0 {
				return errors.New("no files to upload (use --include-hidden to include dotfiles inside directory inputs)")
			}

			fmt.Printf("Found %d file(s) (%s) to upload to %s/%s in registry '%s'\n",
				stats.fileCount, bytesize.New(float64(stats.totalBytes)),
				packageName, version, registryName)

			// TODO(bulk-checksum): Once the bulk-upload-by-checksum API ships, insert a
			// pre-flight pass here that:
			//   1. Computes a checksum (SHA-256) for each candidate file in `jobs`.
			//   2. Sends the {DestPath -> checksum} map to the bulk-check endpoint.
			//   3. Receives back the subset of files the server does NOT already have
			//      (or whose stored checksum differs).
			//   4. Filters `jobs` down to only that subset before calling engine.Execute,
			//      and prints a "skipping N unchanged files" summary.
			// This avoids re-uploading unchanged files and matches the semantics of
			// the future server-side dedup API.

			engine := upload.NewFileUploadEngine(upload.DefaultUploadWorker, progress.NewConsoleReporter())
			results := engine.Execute(ctx, jobs)

			if upload.HasUploadErrors(results) {
				printGenericUploadFailures(results)
				failed := len(results) - upload.GetSuccessfulUploads(results)
				return fmt.Errorf("%d of %d file(s) failed to upload", failed, len(results))
			}
			return nil
		},
	}

	// Add flags
	cmd.Flags().StringVarP(&packageName, "name", "n", "", "name for the artifact")
	cmd.Flags().StringVar(&packageVersion, "version", "", "version for the artifact (defaults to '1.0.0')")
	cmd.Flags().StringVarP(&description, "description", "d", "", "description of the artifact (default to empty)")
	cmd.Flags().StringVar(&pkgURL, "pkg-url", "", "Base URL for the Packages")
	cmd.Flags().BoolVar(&includeHidden, "include-hidden", false,
		"Include hidden files and directories (names starting with '.') when walking directory inputs")

	cmd.MarkFlagRequired("name")

	return cmd
}

// walkStats is the lightweight pre-flight summary shown to the user.
type walkStats struct {
	fileCount  int
	totalBytes int64
}

// collectGenericUploadJobs builds a job list from the user's positional inputs.
// Each path may be a file or a directory; directories are walked recursively.
// Hidden entries (basename starts with '.') are skipped unless includeHidden
// is set. Symlinks and special files (sockets, pipes, devices) are skipped
// regardless.
//
// Path layout:
//   - File   "./pkg.tgz"            -> <pkg>/<v>/pkg.tgz
//   - Dir    "./build" (rel)        -> <pkg>/<v>/build/<relPath>
//   - Dir    "/" or "."  (edge)     -> <pkg>/<v>/<relPath>
func collectGenericUploadJobs(paths []string, registry, packageName, version string, includeHidden bool, pkgClient *pkgclient.ClientWithResponses) ([]upload.FileUploadJob, walkStats, error) {
	var allJobs []upload.FileUploadJob
	var allStats walkStats

	for _, p := range paths {
		jobs, stats, err := collectFromPath(p, registry, packageName, version, includeHidden, pkgClient)
		if err != nil {
			return nil, allStats, err
		}
		allJobs = append(allJobs, jobs...)
		allStats.fileCount += stats.fileCount
		allStats.totalBytes += stats.totalBytes
	}
	return allJobs, allStats, nil
}

// collectFromPath returns jobs for one input path, dispatching on whether
// it's a file or a directory.
func collectFromPath(p, registry, packageName, version string, includeHidden bool, pkgClient *pkgclient.ClientWithResponses) ([]upload.FileUploadJob, walkStats, error) {
	var stats walkStats

	abs, err := filepath.Abs(p)
	if err != nil {
		return nil, stats, fmt.Errorf("cannot resolve %q: %w", p, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, stats, fmt.Errorf("cannot access %q: %w", p, err)
	}

	switch {
	case info.IsDir():
		return collectFromDir(abs, registry, packageName, version, includeHidden, pkgClient)
	case info.Mode().IsRegular():
		relName := filepath.Base(abs)
		destPath := fmt.Sprintf("%s/%s/%s", packageName, version, relName)

		// Compute checksums of the file for X-Checksum-* headers
		checksums, err := utils.ComputeFileChecksums(abs)
		if err != nil {
			return nil, stats, fmt.Errorf("failed to compute checksums for %s: %w", abs, err)
		}

		job := upload.NewGenericUploadJob(relName, abs, destPath, registry, packageName, version, info.Size(), checksums, pkgClient)
		stats.fileCount = 1
		stats.totalBytes = info.Size()
		return []upload.FileUploadJob{job}, stats, nil
	default:
		return nil, stats, fmt.Errorf("%s is not a regular file or directory", abs)
	}
}

// collectFromDir walks one directory and produces a job per regular file.
// The directory's basename is preserved as a prefix on the registry so
// "./build" -> "<pkg>/<v>/build/<rel>". Filesystem-root inputs (where no
// basename can be derived) gracefully fall back to no-prefix layout.
func collectFromDir(srcDir, registry, packageName, version string, includeHidden bool, pkgClient *pkgclient.ClientWithResponses) ([]upload.FileUploadJob, walkStats, error) {
	var jobs []upload.FileUploadJob
	var stats walkStats

	sourceBase := filepath.ToSlash(filepath.Base(srcDir))
	if sourceBase == "." || sourceBase == "/" {
		sourceBase = ""
	}

	err := filepath.WalkDir(srcDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == srcDir {
			return nil
		}

		base := filepath.Base(path)
		if !includeHidden && strings.HasPrefix(base, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			return nil
		}

		// Skip irregular files (devices, sockets, named pipes). Symlinks
		// classify as irregular and are therefore skipped here too.
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return fmt.Errorf("compute relative path for %s: %w", path, err)
		}
		relPath = filepath.ToSlash(relPath)
		if strings.HasPrefix(relPath, "../") || relPath == ".." {
			return fmt.Errorf("refusing to upload %s: relative path escapes source directory", path)
		}

		jobRelPath := relPath
		if sourceBase != "" {
			jobRelPath = sourceBase + "/" + relPath
		}
		destPath := fmt.Sprintf("%s/%s/%s", packageName, version, jobRelPath)

		// Compute checksums of the file for X-Checksum-* headers
		checksums, err := utils.ComputeFileChecksums(path)
		if err != nil {
			return fmt.Errorf("failed to compute checksums for %s: %w", path, err)
		}

		jobs = append(jobs, upload.NewGenericUploadJob(
			jobRelPath, path, destPath, registry, packageName, version, info.Size(), checksums, pkgClient,
		))
		stats.fileCount++
		stats.totalBytes += info.Size()
		return nil
	})
	if err != nil {
		return nil, stats, fmt.Errorf("failed to walk %s: %w", srcDir, err)
	}
	return jobs, stats, nil
}

// printGenericUploadFailures prints a compact list of failed uploads so users
// can target a re-run.
func printGenericUploadFailures(results []upload.FileUploadResult) {
	fmt.Println("\nFailed uploads:")
	for _, r := range results {
		if !r.Success {
			fmt.Printf("  - %s: %v\n", r.JobID, r.Error)
		}
	}
}
