package command

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar_v3"
	"github.com/harness/harness-cli/util/common/download"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/spf13/cobra"
)

const expectedDownloadArgs = 3

type downloadDryRunEntry struct {
	SourcePath string `json:"source_path"`
	DestPath   string `json:"dest_path"`
	Size       string `json:"size"`
}

type downloadConflictJob struct {
	SourcePath string `json:"source_path"`
}

type downloadConflictEntry struct {
	DestPath string                `json:"dest_path"`
	Jobs     []downloadConflictJob `json:"jobs"`
}

// NewDownloadByRegexCmd creates a new cobra.Command for downloading artifacts by regex.
//
// Usage:
//
//	hc artifact download <registry> <path_regex> <dest_dir>
//
// All files in <registry> whose paths match <path_regex> (POSIX ERE) are downloaded
// into <dest_dir>, preserving the full registry path hierarchy.
// Works for all artifact types, not just generic.
func NewDownloadByRegexCmd(c *cmdutils.Factory) *cobra.Command {
	var pageSize int32
	var workers int
	var dryRun bool
	var flatten bool

	cmd := &cobra.Command{
		Use:   "download <registry> <path_regex> <dest_dir>",
		Short: "Download artifacts by path regex",
		Long: "Download all artifacts from a Harness Artifact Registry whose paths match a POSIX ERE regex. " +
			"Works for all artifact types. Files are saved under <dest_dir> preserving the full registry path hierarchy.",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != expectedDownloadArgs {
				return fmt.Errorf(
					"Error: Invalid number of arguments, accepts %d arg(s), received %d\nUsage:\n %s",
					expectedDownloadArgs, len(args), cmd.UseLine(),
				)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			registryName := args[0]
			pathRegex := args[1]
			destDir := args[2]

			// The search API declares page size bounds of 1..100 in its
			// OpenAPI spec, so guard here and return a clear usage error
			// instead of letting the server surface an opaque HTTP 400.
			if pageSize < 1 || pageSize > 100 {
				return fmt.Errorf("invalid --page-size %d: must be between 1 and 100", pageSize)
			}

			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			progress := p.NewConsoleReporter()
			progress.Start(fmt.Sprintf("Searching for files matching '%s' in registry '%s'", pathRegex, registryName))

			items, err := fetchAllMatchingFiles(ctx, c, registryName, pathRegex, int64(pageSize))
			if err != nil {
				progress.Error(fmt.Sprintf("Search failed: %v", err))
				return err
			}

			if len(items) == 0 {
				progress.Success("No files matched the given pattern")
				return nil
			}

			// Filter items to only those actually downloadable up front so both
			// dry-run and real-run report the same file set and conflict list.
			// Also drop items whose registry path escapes destDir — the real
			// download refuses to write those, so including them would make the
			// dry-run manifest lie about what the real run will do.
			// Skipped items are still surfaced in the progress log so users can
			// investigate why the server didn't return a URL, but they're
			// excluded from the summary count, manifest, and conflict check.
			downloadable := make([]ar_v3.FileMetadata, 0, len(items))
			skippedNoURL := 0
			skippedUnsafePath := 0
			for _, item := range items {
				if item.DownloadUrl == nil || *item.DownloadUrl == "" {
					skippedNoURL++
					continue
				}
				registryPathForCheck := item.Path
				if flatten {
					registryPathForCheck = filepath.Base(item.Path)
				}
				if !download.IsWithinDest(destDir, registryPathForCheck) {
					skippedUnsafePath++
					continue
				}
				downloadable = append(downloadable, item)
			}

			progress.Step(fmt.Sprintf("Found %d file(s)", len(downloadable)))

			if len(downloadable) == 0 {
				return fmt.Errorf("no downloadable files found: %d missing download URL, %d rejected as unsafe path",
					skippedNoURL, skippedUnsafePath)
			}

			if dryRun {
				progress.Step(fmt.Sprintf("Running dry run for '%s'", destDir))
				return writeDryRunOutput(downloadable, destDir, flatten)
			}

			progress.Step(fmt.Sprintf("Downloading to '%s'", destDir))

			if flatten {
				if conflicts := detectFlattenConflicts(downloadable, destDir); len(conflicts) > 0 {
					progress.Error(fmt.Sprintf("Conflicts detected - %d destination path(s) would be overwritten", len(conflicts)))
					return fmt.Errorf("download aborted due to conflicts. Run with --dry-run flag to see the full file and conflict list in detail")
				}
			}

			jobs := make([]download.FileDownloadJob, 0, len(downloadable))
			for _, item := range downloadable {
				registryPath := item.Path
				if flatten {
					registryPath = filepath.Base(item.Path)
				}
				jobs = append(jobs, download.NewURLDownloadJob(
					item.Path,
					registryPath,
					destDir,
					*item.DownloadUrl,
					0,
				))
			}

			engine := download.NewFileDownloadEngine(workers, p.NewConsoleReporter())
			results := engine.Execute(ctx, jobs)

			succeeded := download.GetSuccessfulDownloads(results)
			failed := len(results) - succeeded

			// Per-file failure detail is shown via progress so users can see
			// which specific downloads broke. The rest is a static summary block.
			if failed > 0 {
				printDownloadFailures(results, progress)
			}

			printDownloadSummary(len(items), succeeded, failed, skippedNoURL, skippedUnsafePath, destDir)

			progress.Success("Execution of download command complete")
			return nil
		},
	}

	cmd.Flags().Int32Var(&pageSize, "page-size", 20, "Number of files to fetch per page from the search API")
	cmd.Flags().IntVar(&workers, "workers", download.DefaultDownloadWorker, "Number of parallel download workers")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview files that would be downloaded without actually downloading them")
	cmd.Flags().BoolVar(&flatten, "flatten", false, "Save all files directly into <dest_dir> without preserving registry path hierarchy")

	return cmd
}

// fetchAllMatchingFiles paginates through SearchRegistryFilesV3 until hasMore is false.
func fetchAllMatchingFiles(ctx context.Context, c *cmdutils.Factory, registry, pathRegex string, pageSize int64) ([]ar_v3.FileMetadata, error) {
	var allItems []ar_v3.FileMetadata
	var page int64

	for {
		size := pageSize
		params := &ar_v3.SearchRegistryFilesV3Params{
			AccountIdentifier: config.Global.AccountID,
			Page:              &page,
			Size:              &size,
		}
		if org := config.Global.OrgID; org != "" {
			params.OrgIdentifier = &org
		}
		if project := config.Global.ProjectID; project != "" {
			params.ProjectIdentifier = &project
		}
		body := ar_v3.SearchRegistryFilesRequest{
			Registry: registry,
			Regex:    pathRegex,
		}

		resp, err := c.RegistryV3HttpClient().SearchRegistryFilesV3WithResponse(ctx, params, body)
		if err != nil {
			return nil, fmt.Errorf("search request failed: %w", err)
		}

		if resp.JSON200 == nil {
			msg := "unknown error"
			if resp.JSONDefault != nil && resp.JSONDefault.Error.Message != nil {
				msg = *resp.JSONDefault.Error.Message
			}
			return nil, fmt.Errorf("search failed (HTTP %d): %s", resp.StatusCode(), msg)
		}

		allItems = append(allItems, resp.JSON200.Items...)

		// Also guard against a server that returns HasMore=true with an empty
		// page — otherwise the same page would be re-requested forever.
		if !resp.JSON200.HasMore || len(resp.JSON200.Items) == 0 {
			break
		}
		page++
	}

	return allItems, nil
}

// detectFlattenConflicts detects files that would overwrite each other when flattened into destDir.
func detectFlattenConflicts(items []ar_v3.FileMetadata, destDir string) []downloadConflictEntry {
	destToSources := make(map[string][]string)
	for _, item := range items {
		destPath := filepath.Join(destDir, filepath.Base(item.Path))
		destToSources[destPath] = append(destToSources[destPath], item.Path)
	}
	var conflicts []downloadConflictEntry
	for destPath, sources := range destToSources {
		if len(sources) > 1 {
			jobs := make([]downloadConflictJob, 0, len(sources))
			for _, src := range sources {
				jobs = append(jobs, downloadConflictJob{SourcePath: src})
			}
			conflicts = append(conflicts, downloadConflictEntry{DestPath: destPath, Jobs: jobs})
		}
	}
	return conflicts
}

// writeDryRunOutput writes dry-run output and conflict file (if any) to the dry-run-output directory.
func writeDryRunOutput(items []ar_v3.FileMetadata, destDir string, flatten bool) error {
	outputDir := "dry-run-output"
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create dry-run output directory: %w", err)
	}
	timestamp := time.Now().Format("20060102_150405")

	entries := make([]downloadDryRunEntry, 0, len(items))
	for _, item := range items {
		var destPath string
		if flatten {
			destPath = filepath.Join(destDir, filepath.Base(item.Path))
		} else {
			destPath = filepath.Join(destDir, filepath.FromSlash(item.Path))
		}
		entries = append(entries, downloadDryRunEntry{
			SourcePath: item.Path,
			DestPath:   destPath,
			Size:       item.Size,
		})
	}

	outputPath := filepath.Join(outputDir, fmt.Sprintf("download-dryrun-output-%s.json", timestamp))
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal dry-run output: %w", err)
	}
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write dry-run output: %w", err)
	}

	fmt.Printf("\n=== Dry Run Complete ===\n")
	fmt.Printf("Total files found: %d\n", len(entries))
	fmt.Printf("Output written to: %s\n", outputPath)

	if flatten {
		conflicts := detectFlattenConflicts(items, destDir)
		if len(conflicts) > 0 {
			conflictPath := filepath.Join(outputDir, fmt.Sprintf("conflict-download-%s.json", timestamp))
			conflictData, err := json.MarshalIndent(conflicts, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal conflict output: %w", err)
			}
			if err := os.WriteFile(conflictPath, conflictData, 0644); err != nil {
				return fmt.Errorf("failed to write conflict output: %w", err)
			}
			fmt.Printf("%d conflict(s) detected. Conflict file written to: %s\n", len(conflicts), conflictPath)
		}
	}

	return nil
}

// printDownloadSummary prints the end-of-run summary block with counts and
// destination directory. Kept separate from the progress reporter so it
// renders as a plain static section, not as a status update.
func printDownloadSummary(total, succeeded, failed, skippedNoURL, skippedUnsafePath int, destDir string) {
	fmt.Printf("\n=== Download Execution Summary ===\n")
	fmt.Printf("Total files matched: %d\n", total)
	if failed > 0 {
		fmt.Printf("Failed: %d\n", failed)
	}
	if skippedNoURL > 0 {
		fmt.Printf("Skipped (no download URL): %d\n", skippedNoURL)
	}
	if skippedUnsafePath > 0 {
		fmt.Printf("Skipped (unsafe path — escapes destination): %d\n", skippedUnsafePath)
	}
	fmt.Printf("Successfully downloaded: %d\n", succeeded)
	fmt.Printf("Destination: %s\n", destDir)
}

// printDownloadFailures prints failed download entries via the progress reporter.
func printDownloadFailures(results []download.FileDownloadResult, progress *p.ConsoleReporter) {
	for _, r := range results {
		if !r.Success {
			progress.Error(fmt.Sprintf("failed: %s: %v", r.JobID, r.Error))
		}
	}
}
