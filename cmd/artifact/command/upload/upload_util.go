package upload

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/harness/harness-cli/util/common/upload"

	"github.com/bmatcuk/doublestar/v4"
)

const dryRunOutputDir = "dry-run-output"

// destPather is satisfied by job types that expose their registry destination
// path (e.g. RawUploadJob, GenericUploadJob). Checked at runtime so that the
// FileUploadJob interface itself stays minimal.
type destPather interface {
	GetDestPath() string
}

// dryRunEntry describes one file that would be uploaded.
type dryRunEntry struct {
	JobID     string `json:"job_id"`
	LocalPath string `json:"local_path"`
	DestPath  string `json:"dest_path"`
	SizeBytes int64  `json:"size_bytes"`
}

// conflictJobRef identifies one job that maps to a conflicting dest path.
type conflictJobRef struct {
	JobID     string `json:"job_id"`
	LocalPath string `json:"local_path"`
}

// conflictEntry groups all jobs that share the same dest_path.
type conflictEntry struct {
	DestPath string           `json:"dest_path"`
	Jobs     []conflictJobRef `json:"jobs"`
}

// findDestPathConflicts returns one conflictEntry per dest_path that is
// targeted by more than one job.
func findDestPathConflicts(jobs []upload.FileUploadJob) []conflictEntry {
	buckets := make(map[string][]conflictJobRef)
	order := make([]string, 0)

	for _, j := range jobs {
		destPath := ""
		if dp, ok := j.(destPather); ok {
			destPath = dp.GetDestPath()
		}
		if destPath == "" {
			continue
		}
		if _, exists := buckets[destPath]; !exists {
			order = append(order, destPath)
		}
		buckets[destPath] = append(buckets[destPath], conflictJobRef{
			JobID:     j.GetID(),
			LocalPath: j.GetFilePath(),
		})
	}

	var conflicts []conflictEntry
	for _, destPath := range order {
		if len(buckets[destPath]) > 1 {
			conflicts = append(conflicts, conflictEntry{
				DestPath: destPath,
				Jobs:     buckets[destPath],
			})
		}
	}
	return conflicts
}

// writeConflictReport writes the conflict list to a timestamped JSON file and
// returns the path it was written to.
func writeConflictReport(conflicts []conflictEntry) (string, error) {
	if err := os.MkdirAll(dryRunOutputDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create dry-run output directory: %w", err)
	}

	timestamp := time.Now().Format("20060102_150405")
	outPath := filepath.Join(dryRunOutputDir, fmt.Sprintf("conflict-upload-%s.json", timestamp))

	data, err := json.MarshalIndent(conflicts, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal conflict report: %w", err)
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return "", fmt.Errorf("failed to write conflict report: %w", err)
	}
	return outPath, nil
}

// validatePatterns checks that every include/exclude glob is syntactically valid
func validatePatterns(includes, excludes []string) error {
	for _, p := range includes {
		if _, err := doublestar.Match(p, ""); err != nil {
			return fmt.Errorf("include pattern %q is not a valid glob: %w", p, err)
		}
	}
	for _, p := range excludes {
		if _, err := doublestar.Match(p, ""); err != nil {
			return fmt.Errorf("exclude pattern %q is not a valid glob: %w", p, err)
		}
	}
	return nil
}

// applyIncludeExcludeFilter filters jobs according to include/exclude glob patterns.
// Patterns are guaranteed valid already verified before
func applyIncludeExcludeFilter(jobs []upload.FileUploadJob, includes, excludes []string) []upload.FileUploadJob {
	fmt.Printf("  Total Files found ::     %d\n", len(jobs))

	if len(includes) > 0 {
		var kept []upload.FileUploadJob
		for _, j := range jobs {
			p := filepath.ToSlash(j.GetID())
			for _, pattern := range includes {
				if matched, _ := doublestar.Match(pattern, p); matched {
					kept = append(kept, j)
					break
				}
			}
		}
		jobs = kept
		fmt.Printf("  Files After Applying include filter(s):  %d\n", len(jobs))
	}

	if len(excludes) > 0 {
		var kept []upload.FileUploadJob
		for _, j := range jobs {
			p := filepath.ToSlash(j.GetID())
			excluded := false
			for _, pattern := range excludes {
				if matched, _ := doublestar.Match(pattern, p); matched {
					excluded = true
					break
				}
			}
			if !excluded {
				kept = append(kept, j)
			}
		}
		jobs = kept
		fmt.Printf("  Files After Applying exclude filter(s):  %d\n", len(jobs))
	}

	return jobs
}

func runPreUpload(jobs []upload.FileUploadJob, dryRun bool) (bool, error) {
	conflicts := findDestPathConflicts(jobs)

	if len(conflicts) > 0 {
		if dryRun {
			_ = writeDryRunOutput(jobs)
			reportPath, err := writeConflictReport(conflicts)
			if err != nil {
				return false, fmt.Errorf("conflict found at %d destination(s); also failed to write conflict report: %w", len(conflicts), err)
			}
			fmt.Printf("\nConflict report written to: %s\n", reportPath)
			return false, fmt.Errorf("conflict found at %d destination(s), see conflict report at %s", len(conflicts), reportPath)
		}
		return false, fmt.Errorf("conflict found at %d destination(s), run with --dry-run to see details", len(conflicts))
	}

	if dryRun {
		if err := writeDryRunOutput(jobs); err != nil {
			return false, err
		}
		return true, nil
	}

	return false, nil
}

// writeDryRunOutput write  planned upload list to a timestamped JSON
// file under dry-run-output/, mirroring the pattern used by migration.go.
func writeDryRunOutput(jobs []upload.FileUploadJob) error {
	if err := os.MkdirAll(dryRunOutputDir, 0o755); err != nil {
		return fmt.Errorf("failed to create dry-run output directory: %w", err)
	}

	entries := make([]dryRunEntry, 0, len(jobs))
	for _, j := range jobs {
		destPath := ""
		if dp, ok := j.(destPather); ok {
			destPath = dp.GetDestPath()
		}
		entries = append(entries, dryRunEntry{
			JobID:     j.GetID(),
			LocalPath: j.GetFilePath(),
			DestPath:  destPath,
			SizeBytes: j.GetFileSize(),
		})
	}

	timestamp := time.Now().Format("20060102_150405")
	outPath := filepath.Join(dryRunOutputDir, fmt.Sprintf("upload-dryrun-output-%s.json", timestamp))

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal dry-run output: %w", err)
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write dry-run output file: %w", err)
	}

	fmt.Printf("\n=== Dry Run Complete ===\n")
	fmt.Printf("Total files that would be uploaded: %d\n", len(entries))
	fmt.Printf("File list written to: %s\n", outPath)

	return nil
}

func splitPatternRoot(pattern string) (root, relPattern string) {
	// Normalise to forward slashes for consistent splitting.
	norm := filepath.ToSlash(pattern)
	parts := strings.Split(norm, "/")

	var rootParts []string
	for i, p := range parts {
		if containsWildcard(p) {
			relPattern = strings.Join(parts[i:], "/")
			break
		}
		rootParts = append(rootParts, p)
	}

	if len(rootParts) == 0 {
		root = "."
	} else {
		root = strings.Join(rootParts, string(filepath.Separator))
	}
	return root, relPattern
}
