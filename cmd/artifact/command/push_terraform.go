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
func NewPushTerraformCmd(c *cmdutils.Factory) *cobra.Command {
	var namespace, moduleName, moduleProvider, moduleVersion string

	cmd := &cobra.Command{
		Use:   "terraform <registry_name> <file_path>",
		Short: "Push Terraform module or provider",
		Long: "Push a Terraform module (.tar.gz/.tgz) or provider binary (.zip) to Harness Artifact Registry (HAR).\n\n" +
			"Modules require --namespace, --name, --provider and --version.\n" +
			"Providers require only --namespace; type/version/os/arch are parsed from the filename\n" +
			"(terraform-provider-{type}_{version}_{os}_{arch}.zip).",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			registryName := args[0]
			inputPath := args[1]

			progress := p.NewReporterAuto(config.Global.NoProgress)
			progress.Start("Validating input parameters")

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
				return pushTerraformModule(ctx, c, progress, registryName, archivePath, archiveInfo, namespace, moduleName, moduleProvider, moduleVersion)
			}

			files, err := utils.ResolveFilePath(inputPath)
			if err != nil {
				progress.Error("Failed to resolve file path")
				return err
			}
			var filePath string
			for _, f := range files {
				lower := strings.ToLower(f)
				if strings.HasSuffix(lower, terraformTarGzExt) || strings.HasSuffix(lower, terraformTgzExt) || strings.HasSuffix(lower, terraformZipExt) {
					filePath = f
					break
				}
			}
			if filePath == "" {
				progress.Error("No matching Terraform file found")
				return fmt.Errorf("no files with extensions [%s %s %s] matched pattern: %s",
					terraformTarGzExt, terraformTgzExt, terraformZipExt, inputPath)
			}

			fileInfo, err := os.Stat(filePath)
			if err != nil {
				progress.Error("Failed to access package file")
				return fmt.Errorf("failed to access package file: %w", err)
			}

			switch {
			case isTerraformModule(filePath):
				return pushTerraformModule(ctx, c, progress, registryName, filePath, fileInfo, namespace, moduleName, moduleProvider, moduleVersion)
			case isTerraformProvider(filePath):
				return pushTerraformProvider(ctx, c, progress, registryName, filePath, fileInfo, namespace)
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
	cmd.MarkFlagRequired("namespace")

	return cmd
}

// packageModuleDir validates a module source directory and packages it into
// a .tar.gz archive named "{ns}-{name}-{provider}-{ver}.tar.gz" in the OS
// temp dir, per the tech spec's §5.6.2 packaging steps. The caller owns
// removing the returned path.
func packageModuleDir(progress p.Reporter, dir, namespace, name, moduleProvider, version string) (string, error) {
	dir = filepath.Clean(dir)
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
		if terraformDirSkipNames[info.Name()] || strings.Contains(info.Name(), ".tfstate") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		// Only count .tf files at the root level (direct children of dir)
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".tf") && filepath.Dir(path) == dir {
			hasTf = true
		}
		return nil
	}); err != nil {
		progress.Error("Failed to scan module directory")
		return "", fmt.Errorf("failed to scan module directory: %w", err)
	}
	if !hasTf {
		progress.Error("Module directory must contain at least one .tf file at root level")
		return "", fmt.Errorf("module directory %q must contain at least one .tf file at the root level", dir)
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
		os.RemoveAll(tmpDir)
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

	gzWriter := gzip.NewWriter(out)
	tarWriter := tar.NewWriter(gzWriter)

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

		if err := func() error {
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
		}(); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to build module archive: %w", err)
	}

	if err := tarWriter.Close(); err != nil {
		return fmt.Errorf("failed to finalize tar: %w", err)
	}
	if err := gzWriter.Close(); err != nil {
		return fmt.Errorf("failed to finalize gzip: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("failed to close archive file: %w", err)
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
	c *cmdutils.Factory,
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

	file, err := os.Open(filePath)
	if err != nil {
		progress.Error("Failed to open package file")
		return fmt.Errorf("failed to open package file: %w", err)
	}
	defer file.Close()
	showBar := !config.Global.NoProgress && !p.IsCI()
	pkgClient := c.PkgHttpClientWithProgress(progress, fileInfo.Size(), fileInfo.Name(), showBar)
	progress.Step(fmt.Sprintf("Uploading module %s/%s/%s@%s", namespace, name, moduleProvider, version))
	resp, err := pkgClient.UploadTerraformModuleWithBodyWithResponse(
		ctx,
		config.Global.AccountID,
		registryName,
		namespace, name, moduleProvider, version,
		"application/octet-stream",
		file,
		func(_ context.Context, req *http.Request) error {
			utils.SetChecksumHeaders(req.Header, checksums)
			return nil
		},
	)
	if err != nil {
		progress.Error("Failed to upload module")
		return fmt.Errorf("failed to upload module: %w", err)
	}
	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusCreated && resp.StatusCode() != http.StatusNoContent {
		progress.Error("Upload failed")
		return fmt.Errorf("failed to upload module: %s\nresponse: %s", resp.Status(), resp.Body)
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
	c *cmdutils.Factory,
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

	file, err := os.Open(filePath)
	if err != nil {
		progress.Error("Failed to open package file")
		return fmt.Errorf("failed to open package file: %w", err)
	}
	defer file.Close()
	showBar := !config.Global.NoProgress && !p.IsCI()
	pkgClient := c.PkgHttpClientWithProgress(progress, fileInfo.Size(), fileInfo.Name(), showBar)
	progress.Step(fmt.Sprintf("Uploading provider %s/%s@%s (%s_%s)", namespace, typeName, version, osName, arch))
	resp, err := pkgClient.UploadTerraformProviderWithBodyWithResponse(
		ctx,
		config.Global.AccountID,
		registryName,
		namespace, typeName, version, filename,
		"application/octet-stream",
		file,
		func(_ context.Context, req *http.Request) error {
			utils.SetChecksumHeaders(req.Header, checksums)
			return nil
		},
	)
	if err != nil {
		progress.Error("Failed to upload provider")
		return fmt.Errorf("failed to upload provider: %w", err)
	}
	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusCreated && resp.StatusCode() != http.StatusNoContent {
		progress.Error("Upload failed")
		return fmt.Errorf("failed to upload provider: %s\nresponse: %s", resp.Status(), resp.Body)
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
