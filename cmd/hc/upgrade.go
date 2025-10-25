package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	githubAPIURL = "https://api.github.com/repos/iarvind/harness-cli/releases/latest"
	githubOwner  = "iarvind"
	githubRepo   = "harness-cli"
)

// GitHubRelease represents a GitHub release
type GitHubRelease struct {
	TagName    string        `json:"tag_name"`
	Name       string        `json:"name"`
	Draft      bool          `json:"draft"`
	Prerelease bool          `json:"prerelease"`
	Assets     []GitHubAsset `json:"assets"`
	Body       string        `json:"body"`
	HTMLURL    string        `json:"html_url"`
}

// GitHubAsset represents a release asset
type GitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

func upgradeCmd() *cobra.Command {
	var preRelease bool

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade hc to the latest version",
		Long:  "Check for the latest version of hc and upgrade if a newer version is available",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpgrade(preRelease)
		},
	}

	cmd.Flags().BoolVar(&preRelease, "pre-release", false, "Include pre-release versions")

	return cmd
}

func runUpgrade(includePreRelease bool) error {
	fmt.Println("Checking for updates...")

	// Get current version
	currentVersion := version
	if currentVersion == "dev" {
		fmt.Println("⚠️  Development version detected. Upgrade may overwrite your local build.")
		fmt.Print("Continue? (y/N): ")
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) != "y" {
			fmt.Println("Upgrade cancelled.")
			return nil
		}
	}

	// Get latest release
	release, err := getLatestRelease(includePreRelease)
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	latestVersion := release.TagName

	// Compare versions
	if currentVersion == latestVersion {
		fmt.Printf("✓ You are already using the latest version: %s\n", currentVersion)
		return nil
	}

	fmt.Printf("Current version: %s\n", currentVersion)
	fmt.Printf("Latest version:  %s\n", latestVersion)

	if release.Prerelease {
		fmt.Println("⚠️  This is a pre-release version")
	}

	// Get the appropriate asset for current OS and architecture
	asset, checksum, err := findAssetForPlatform(release, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return fmt.Errorf("no compatible release found: %w", err)
	}

	fmt.Printf("\nDownloading %s...\n", asset.Name)

	tmpDir, err := os.MkdirTemp("", "hc-upgrade-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, asset.Name)
	if err := downloadFile(archivePath, asset.BrowserDownloadURL); err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}

	// Verify checksum if available
	if checksum != nil {
		fmt.Println("Verifying checksum...")
		if err := verifyChecksum(archivePath, checksum.BrowserDownloadURL); err != nil {
			return fmt.Errorf("checksum verification failed: %w", err)
		}
		fmt.Println("✓ Checksum verified")
	}

	// Extract the binary
	fmt.Println("Extracting...")
	binaryPath, err := extractBinary(archivePath, tmpDir)
	if err != nil {
		return fmt.Errorf("failed to extract: %w", err)
	}

	// Replace the current binary
	fmt.Println("Installing...")
	if err := replaceBinary(binaryPath); err != nil {
		return fmt.Errorf("failed to install: %w", err)
	}

	fmt.Printf("\n✓ Successfully upgraded to %s\n", latestVersion)
	fmt.Printf("Release notes: %s\n", release.HTMLURL)

	return nil
}

func getLatestRelease(includePreRelease bool) (*GitHubRelease, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	var url string
	if includePreRelease {
		// Get all releases and find the latest (including pre-releases)
		url = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", githubOwner, githubRepo)
	} else {
		// Get the latest stable release
		url = githubAPIURL
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	if includePreRelease {
		var releases []GitHubRelease
		if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
			return nil, err
		}
		if len(releases) == 0 {
			return nil, fmt.Errorf("no releases found")
		}
		return &releases[0], nil
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
}

func findAssetForPlatform(release *GitHubRelease, goos, goarch string) (*GitHubAsset, *GitHubAsset, error) {
	// Map Go OS/Arch to release naming conventions
	osName := goos
	archName := goarch

	if goos == "darwin" {
		osName = "mac-os"
	}
	if goarch == "amd64" {
		archName = "x86_64"
	} else if goarch == "386" {
		archName = "i386"
	}

	// Expected file extension
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}

	// Look for matching asset (e.g., hc_v2.0.0_mac-os_arm64.tar.gz)
	var targetAsset *GitHubAsset
	var checksumAsset *GitHubAsset

	for i := range release.Assets {
		asset := &release.Assets[i]

		// Check for checksum file
		if asset.Name == "checksums.txt" {
			checksumAsset = asset
			continue
		}

		// Check if this asset matches our platform
		if strings.Contains(asset.Name, osName) &&
			strings.Contains(asset.Name, archName) &&
			strings.HasSuffix(asset.Name, ext) {
			targetAsset = asset
		}
	}

	if targetAsset == nil {
		return nil, nil, fmt.Errorf("no release found for %s/%s", goos, goarch)
	}

	return targetAsset, checksumAsset, nil
}

func downloadFile(filepath, url string) error {
	client := &http.Client{Timeout: 5 * time.Minute}

	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Create a progress indicator
	total := resp.ContentLength
	counter := &writeCounter{Total: total}

	_, err = io.Copy(out, io.TeeReader(resp.Body, counter))
	fmt.Println() // New line after progress

	return err
}

type writeCounter struct {
	Total      int64
	Downloaded int64
}

func (wc *writeCounter) Write(p []byte) (int, error) {
	n := len(p)
	wc.Downloaded += int64(n)
	wc.printProgress()
	return n, nil
}

func (wc *writeCounter) printProgress() {
	if wc.Total > 0 {
		percentage := float64(wc.Downloaded) / float64(wc.Total) * 100
		fmt.Printf("\rDownloading... %.1f%% (%d/%d bytes)", percentage, wc.Downloaded, wc.Total)
	} else {
		fmt.Printf("\rDownloading... %d bytes", wc.Downloaded)
	}
}

func verifyChecksum(filepath, checksumURL string) error {
	// Download checksums.txt
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(checksumURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download checksums: status %d", resp.StatusCode)
	}

	// Read checksums
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Find the checksum for our file
	filename := filepath[strings.LastIndex(filepath, "/")+1:]
	lines := strings.Split(string(body), "\n")
	var expectedChecksum string

	for _, line := range lines {
		if strings.Contains(line, filename) {
			parts := strings.Fields(line)
			if len(parts) >= 1 {
				expectedChecksum = parts[0]
				break
			}
		}
	}

	if expectedChecksum == "" {
		return fmt.Errorf("checksum not found for %s", filename)
	}

	// Calculate actual checksum
	actualChecksum, err := calculateSHA256(filepath)
	if err != nil {
		return err
	}

	if actualChecksum != expectedChecksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum)
	}

	return nil
}

func calculateSHA256(filepath string) (string, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func extractBinary(archivePath, destDir string) (string, error) {
	if strings.HasSuffix(archivePath, ".tar.gz") {
		return extractTarGz(archivePath, destDir)
	} else if strings.HasSuffix(archivePath, ".zip") {
		return extractZip(archivePath, destDir)
	}
	return "", fmt.Errorf("unsupported archive format")
}

func extractTarGz(archivePath, destDir string) (string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return "", err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		// Look for the binary
		if header.Typeflag == tar.TypeReg && (header.Name == "hc" || header.Name == "hc.exe") {
			target := filepath.Join(destDir, header.Name)

			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, 0755)
			if err != nil {
				return "", err
			}
			defer f.Close()

			if _, err := io.Copy(f, tr); err != nil {
				return "", err
			}

			return target, nil
		}
	}

	return "", fmt.Errorf("binary not found in archive")
}

func extractZip(archivePath, destDir string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer r.Close()

	for _, f := range r.File {
		// Look for the binary
		if f.Name == "hc.exe" || f.Name == "hc" {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			defer rc.Close()

			target := filepath.Join(destDir, f.Name)
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, 0755)
			if err != nil {
				return "", err
			}
			defer outFile.Close()

			if _, err := io.Copy(outFile, rc); err != nil {
				return "", err
			}

			return target, nil
		}
	}

	return "", fmt.Errorf("binary not found in archive")
}

func replaceBinary(newBinaryPath string) error {
	// Get the path of the current executable
	executable, err := os.Executable()
	if err != nil {
		return err
	}

	// Resolve symlinks
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return err
	}

	// Create a backup
	backupPath := executable + ".old"
	if err := os.Rename(executable, backupPath); err != nil {
		return fmt.Errorf("failed to backup current binary: %w", err)
	}

	// Copy new binary to the executable location
	if err := copyFile(newBinaryPath, executable); err != nil {
		// Restore backup on failure
		os.Rename(backupPath, executable)
		return fmt.Errorf("failed to install new binary: %w", err)
	}

	// Make it executable
	if err := os.Chmod(executable, 0755); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	// Remove backup
	os.Remove(backupPath)

	return nil
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	return destFile.Sync()
}
