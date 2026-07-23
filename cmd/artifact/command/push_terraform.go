package command

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/harness/harness-cli/cmd/artifact/command/utils"
	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/util"
	"github.com/harness/harness-cli/util/common/auth"
	"github.com/harness/harness-cli/util/common/httpclient"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/Masterminds/semver/v3"
	"github.com/spf13/cobra"
)

const (
	terraformTarGzExt      = ".tar.gz"
	terraformTgzExt        = ".tgz"
	terraformZipExt        = ".zip"
	terraformMaxModuleSize = 500 * 1024 * 1024 // 500MB
)

// terraformDirSkipNames are file/dir basenames excluded when packaging a
// module directory into a .tar.gz archive.
var terraformDirSkipNames = map[string]bool{
	".git":       true,
	".terraform": true,
	".DS_Store":  true,
}

// terraformProviderFilenameRegex matches terraform-provider-{type}_{version}_{os}_{arch}.zip
// per the Provider Network Mirror Protocol naming convention.
var terraformProviderFilenameRegex = regexp.MustCompile(
	`^terraform-provider-([a-zA-Z0-9-]+)_(\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?)_([a-z0-9]+)_([a-z0-9]+)\.zip$`,
)

// NewPushTerraformCmd creates a new cobra.Command for pushing Terraform modules and providers.
//
// Usage:
//
//	hc artifact push terraform <registry_name> <file_path> [flags]
//
// Modules must be uploaded as a pre-built .tar.gz (or .tgz) archive named after the
// module identity (--namespace, --name, --provider, --version are all required).
// Providers are uploaded as-is: a .zip file already named per the
// terraform-provider-{type}_{version}_{os}_{arch}.zip convention (only --namespace is
// required; type/version/os/arch are parsed straight from the filename).
//
// Unlike other push commands, this bypasses the generated package client
// (via *cmdutils.Factory) entirely and PUTs the raw archive directly
// (see putTerraformArchive), matching the generic_job.go pattern — the
// generated client encodes slashes in multi-segment paths, which breaks the
// terraform/v1/{modules,providers}/... URL shape. The factory param is kept
// for signature consistency with other NewPush*Cmd constructors.
func NewPushTerraformCmd(_ *cmdutils.Factory) *cobra.Command {
	var namespace, moduleName, moduleProvider, moduleVersion, pkgURL string

	cmd := &cobra.Command{
		Use:   "terraform <registry_name> <file_path>",
		Short: "Push Terraform module or provider",
		Long: "Push a Terraform module (.tar.gz/.tgz) or provider binary (.zip) to Harness Artifact Registry (HAR).\n\n" +
			"Modules require --namespace, --name, --provider and --version.\n" +
			"Providers require only --namespace; type/version/os/arch are parsed from the filename\n" +
			"(terraform-provider-{type}_{version}_{os}_{arch}.zip).",
		Args: cobra.ExactArgs(2),
		PreRun: func(cmd *cobra.Command, args []string) {
			if pkgURL != "" {
				config.Global.Registry.PkgURL = util.GetPkgUrl(pkgURL)
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			registryName := args[0]
			inputPath := args[1]

			progress := p.NewConsoleReporter()
			progress.Start("Validating input parameters")

			if namespace == "" {
				progress.Error("--namespace is required")
				return fmt.Errorf("--namespace is required")
			}

			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			pathInfo, err := os.Stat(inputPath)
			if err != nil {
				progress.Error("Failed to access package path")
				return fmt.Errorf("failed to access package path: %w", err)
			}

			if pathInfo.IsDir() {
				archivePath, packErr := packageModuleDir(progress, inputPath, namespace, moduleName, moduleProvider, moduleVersion)
				if packErr != nil {
					return packErr
				}
				defer os.RemoveAll(filepath.Dir(archivePath))

				archiveInfo, statErr := os.Stat(archivePath)
				if statErr != nil {
					progress.Error("Failed to access packaged module archive")
					return fmt.Errorf("failed to access packaged module archive: %w", statErr)
				}
				return pushTerraformModule(ctx, progress, registryName, archivePath, archiveInfo, namespace, moduleName, moduleProvider, moduleVersion)
			}

			files, err := utils.ResolveFilePath(inputPath, terraformTarGzExt, terraformTgzExt, terraformZipExt)
			if err != nil {
				progress.Error("Failed to resolve file path")
				return err
			}
			filePath := files[0]

			fileInfo, err := os.Stat(filePath)
			if err != nil {
				progress.Error("Failed to access package file")
				return fmt.Errorf("failed to access package file: %w", err)
			}

			switch {
			case isTerraformModule(filePath):
				return pushTerraformModule(ctx, progress, registryName, filePath, fileInfo, namespace, moduleName, moduleProvider, moduleVersion)
			case isTerraformProvider(filePath):
				return pushTerraformProvider(ctx, progress, registryName, filePath, fileInfo, namespace)
			default:
				progress.Error("Unsupported file type")
				return fmt.Errorf("package file must be a module (%s/%s) or provider (%s), got: %s",
					terraformTarGzExt, terraformTgzExt, terraformZipExt, filepath.Ext(filePath))
			}
		},
	}

	cmd.Flags().StringVar(&namespace, "namespace", "", "Terraform namespace (required)")
	cmd.Flags().StringVar(&moduleName, "name", "", "Module name (required for module uploads)")
	cmd.Flags().StringVar(&moduleProvider, "provider", "", "Module provider, e.g. aws (required for module uploads)")
	cmd.Flags().StringVar(&moduleVersion, "version", "", "Module version, SemVer 2.0.0 (required for module uploads)")
	cmd.Flags().StringVar(&pkgURL, "pkg-url", "", "Base URL for the Packages")

	return cmd
}

// packageModuleDir validates a module source directory and packages it into
// a .tar.gz archive named "{ns}-{name}-{provider}-{ver}.tar.gz" in the OS
// temp dir, per the tech spec's §5.6.2 packaging steps. The caller owns
// removing the returned path.
func packageModuleDir(progress p.Reporter, dir, namespace, name, moduleProvider, version string) (string, error) {
	if name == "" {
		progress.Error("--name is required to package a module directory")
		return "", fmt.Errorf("--name is required to package a module directory")
	}
	if moduleProvider == "" {
		progress.Error("--provider is required to package a module directory")
		return "", fmt.Errorf("--provider is required to package a module directory")
	}
	if version == "" {
		progress.Error("--version is required to package a module directory")
		return "", fmt.Errorf("--version is required to package a module directory")
	}
	if _, err := semver.NewVersion(version); err != nil {
		progress.Error("Invalid version, must be SemVer 2.0.0")
		return "", fmt.Errorf("invalid version %q, must be SemVer 2.0.0: %w", version, err)
	}

	progress.Step(fmt.Sprintf("Packaging module directory %s", dir))

	hasTf := false
	if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".tf") {
			hasTf = true
		}
		return nil
	}); err != nil {
		progress.Error("Failed to scan module directory")
		return "", fmt.Errorf("failed to scan module directory: %w", err)
	}
	if !hasTf {
		progress.Error("Module directory must contain at least one .tf file")
		return "", fmt.Errorf("module directory %q must contain at least one .tf file", dir)
	}

	tmpDir, err := os.MkdirTemp("", "hc-terraform-module-")
	if err != nil {
		progress.Error("Failed to create temp directory for packaging")
		return "", fmt.Errorf("failed to create temp directory for packaging: %w", err)
	}

	archiveName := fmt.Sprintf("%s-%s-%s-%s%s", namespace, name, moduleProvider, version, terraformTarGzExt)
	archivePath := filepath.Join(tmpDir, archiveName)

	if err := writeModuleArchive(archivePath, dir); err != nil {
		os.RemoveAll(tmpDir)
		progress.Error("Failed to package module directory")
		return "", err
	}

	info, err := os.Stat(archivePath)
	if err != nil {
		progress.Error("Failed to access packaged module archive")
		return "", fmt.Errorf("failed to access packaged module archive: %w", err)
	}
	if info.Size() > terraformMaxModuleSize {
		os.Remove(archivePath)
		progress.Error("Packaged module archive exceeds max size (500MB)")
		return "", fmt.Errorf("packaged module archive is %d bytes, exceeds max size of %d bytes", info.Size(), terraformMaxModuleSize)
	}

	progress.Success(fmt.Sprintf("Packaged module directory into %s", archivePath))
	return archivePath, nil
}

// writeModuleArchive walks dir and writes its contents (skipping VCS/state
// dirs) into a gzip-compressed tar at archivePath.
func writeModuleArchive(archivePath, dir string) error {
	out, err := os.Create(archivePath)
	if err != nil {
		return fmt.Errorf("failed to create archive file: %w", err)
	}
	defer out.Close()

	gzWriter := gzip.NewWriter(out)
	defer gzWriter.Close()
	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == dir {
			return nil
		}
		if terraformDirSkipNames[info.Name()] || strings.Contains(info.Name(), ".tfstate") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("failed to compute relative path for %s: %w", path, err)
		}

		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", path, err)
		}
		defer file.Close()

		if err := tarWriter.WriteHeader(&tar.Header{
			Name: filepath.ToSlash(relPath),
			Mode: 0o644,
			Size: info.Size(),
		}); err != nil {
			return fmt.Errorf("failed to write tar header for %s: %w", relPath, err)
		}
		if _, err := io.Copy(tarWriter, file); err != nil {
			return fmt.Errorf("failed to write %s to archive: %w", relPath, err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to build module archive: %w", err)
	}
	return nil
}

// isTerraformModule reports whether path is a module archive (.tar.gz or .tgz).
func isTerraformModule(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, terraformTarGzExt) || strings.HasSuffix(lower, terraformTgzExt)
}

// isTerraformProvider reports whether path is a provider archive (.zip).
func isTerraformProvider(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), terraformZipExt)
}

// pushTerraformModule uploads a pre-built module archive to
// PUT /terraform/v1/modules/{ns}/{name}/{provider}/{ver}.
func pushTerraformModule(
	ctx context.Context,
	progress p.Reporter,
	registryName, filePath string,
	fileInfo os.FileInfo,
	namespace, name, moduleProvider, version string,
) error {
	if name == "" {
		progress.Error("--name is required for module uploads")
		return fmt.Errorf("--name is required for module uploads")
	}
	if moduleProvider == "" {
		progress.Error("--provider is required for module uploads")
		return fmt.Errorf("--provider is required for module uploads")
	}
	if version == "" {
		progress.Error("--version is required for module uploads")
		return fmt.Errorf("--version is required for module uploads")
	}
	if _, err := semver.NewVersion(version); err != nil {
		progress.Error("Invalid version, must be SemVer 2.0.0")
		return fmt.Errorf("invalid version %q, must be SemVer 2.0.0: %w", version, err)
	}
	progress.Success("Input parameters validated")

	checksums, err := utils.ComputeFileChecksums(filePath)
	if err != nil {
		progress.Error("Failed to compute file checksums")
		return fmt.Errorf("failed to compute checksums for %s: %w", filePath, err)
	}

	url := fmt.Sprintf("%s/pkg/%s/%s/terraform/v1/modules/%s/%s/%s/%s",
		strings.TrimRight(config.Global.Registry.PkgURL, "/"),
		config.Global.AccountID,
		registryName,
		namespace, name, moduleProvider, version,
	)

	progress.Step(fmt.Sprintf("Uploading module %s/%s/%s@%s", namespace, name, moduleProvider, version))
	if err := putTerraformArchive(ctx, progress, url, filePath, fileInfo, checksums); err != nil {
		return err
	}

	progress.Success(fmt.Sprintf(
		"Successfully uploaded Terraform module '%s/%s/%s@%s' to registry '%s'",
		namespace, name, moduleProvider, version, registryName,
	))
	return nil
}

// pushTerraformProvider uploads a provider binary as-is to
// PUT /terraform/v1/providers/{ns}/{type}/{ver}/{filename}. type/version/os/arch
// are parsed from the filename, which must already follow the
// terraform-provider-{type}_{version}_{os}_{arch}.zip convention.
func pushTerraformProvider(
	ctx context.Context,
	progress p.Reporter,
	registryName, filePath string,
	fileInfo os.FileInfo,
	namespace string,
) error {
	filename := filepath.Base(filePath)
	typeName, version, osName, arch, err := parseProviderFilename(filename)
	if err != nil {
		progress.Error("Invalid provider filename")
		return err
	}
	if _, err := semver.NewVersion(version); err != nil {
		progress.Error("Invalid version in filename, must be SemVer 2.0.0")
		return fmt.Errorf("invalid version %q in filename, must be SemVer 2.0.0: %w", version, err)
	}
	progress.Success("Input parameters validated")

	checksums, err := utils.ComputeFileChecksums(filePath)
	if err != nil {
		progress.Error("Failed to compute file checksums")
		return fmt.Errorf("failed to compute checksums for %s: %w", filePath, err)
	}

	url := fmt.Sprintf("%s/pkg/%s/%s/terraform/v1/providers/%s/%s/%s/%s",
		strings.TrimRight(config.Global.Registry.PkgURL, "/"),
		config.Global.AccountID,
		registryName,
		namespace, typeName, version, filename,
	)

	progress.Step(fmt.Sprintf("Uploading provider %s/%s@%s (%s_%s)", namespace, typeName, version, osName, arch))
	if err := putTerraformArchive(ctx, progress, url, filePath, fileInfo, checksums); err != nil {
		return err
	}

	progress.Success(fmt.Sprintf(
		"Successfully uploaded Terraform provider '%s/%s@%s' (%s_%s) to registry '%s'",
		namespace, typeName, version, osName, arch, registryName,
	))
	return nil
}

// parseProviderFilename extracts type, version, os and arch from a provider
// filename following the terraform-provider-{type}_{version}_{os}_{arch}.zip
// convention mandated by the Provider Network Mirror Protocol.
func parseProviderFilename(filename string) (typeName, version, osName, arch string, err error) {
	m := terraformProviderFilenameRegex.FindStringSubmatch(filename)
	if m == nil {
		return "", "", "", "", fmt.Errorf(
			"filename %q does not match required convention terraform-provider-{type}_{version}_{os}_{arch}.zip",
			filename,
		)
	}
	return m[1], m[2], m[3], m[4], nil
}

// putTerraformArchive streams filePath to url via a raw PUT request, mirroring
// the pattern used by the generic push job to avoid the generated client
// encoding slashes in multi-segment paths.
func putTerraformArchive(
	ctx context.Context,
	progress p.Reporter,
	url, filePath string,
	fileInfo os.FileInfo,
	checksums utils.FileChecksums,
) error {
	file, err := os.Open(filePath)
	if err != nil {
		progress.Error("Failed to open package file")
		return fmt.Errorf("failed to open package file: %w", err)
	}
	defer file.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, file)
	if err != nil {
		progress.Error("Failed to create upload request")
		return fmt.Errorf("failed to create upload request: %w", err)
	}
	req.ContentLength = fileInfo.Size()
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("x-api-key", config.Global.AuthToken)
	if strings.HasPrefix(config.Global.AuthToken, auth.JWTTokenPrefix) {
		req.Header.Set("Authorization", config.Global.AuthToken)
	}
	utils.SetChecksumHeaders(req.Header, checksums)

	httpClient := httpclient.NewRetryClientWithProgress(progress, fileInfo.Size(), fileInfo.Name())
	resp, err := httpClient.Do(req)
	if err != nil {
		progress.Error("Failed to upload package")
		return fmt.Errorf("failed to upload package: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		progress.Error("Upload failed")
		return fmt.Errorf("failed to upload package: %s\nresponse: %s", resp.Status, string(body))
	}
	return nil
}
