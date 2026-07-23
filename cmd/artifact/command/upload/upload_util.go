package upload

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/harness/harness-cli/util/common/upload"
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

// writeDryRunOutput serialises the planned upload list to a timestamped JSON
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

// ── Pattern helpers
// Examples:
//
//	"dist/(*)/*.zip"   →  "dist",  "(*)/*.zip"
//	"*.jar"            →  ".",     "*.jar"
//	"**/*.jar"         →  ".",     "**/*.jar"
//	"target/(**)"      →  "target","(**)"
//	"/abs/path/f.txt"  →  "/abs/path/f.txt", ""
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
