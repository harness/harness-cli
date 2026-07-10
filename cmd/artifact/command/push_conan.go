package command

import (
	"context"
	"crypto/md5" //nolint:gosec // Conan revision checksum, not security.
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/harness/harness-cli/cmd/artifact/command/utils"
	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	conanutil "github.com/harness/harness-cli/module/ar/migrate/util"
	"github.com/harness/harness-cli/util/common/errors"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/spf13/cobra"
)

// conanExpectedNumberOfArgument is the positional arg count: <registry_name> <reference> <recipe_dir>.
// Canonical file names, tarball prefixes and the "_" placeholder live in the shared conanutil package.
const conanExpectedNumberOfArgument = 3

// Validation patterns, matching the server's Conan reference model.
var (
	conanNamePattern      = regexp.MustCompile(`^[a-z0-9_][a-z0-9_+.-]{1,100}$`) // name/version/user/channel
	conanRevisionPattern  = regexp.MustCompile(`^([a-f0-9]{32}|[a-f0-9]{40})$`)  // RREV/PREV
	conanPackageIDPattern = regexp.MustCompile(`^[a-f0-9]{40}$`)                 // PKGID
)

// NewPushConanCmd pushes a Conan package (v2 protocol): each recipe- and package-layer
// file is PUT to its revision path, with conanmanifest.txt last (finalization marker).
func NewPushConanCmd(c *cmdutils.Factory) *cobra.Command {
	var recipeRevision string
	var packageDir string
	var packageID string
	var packageRevision string

	cmd := &cobra.Command{
		Use:   "conan <registry_name> <reference> <recipe_dir>",
		Short: "Push Conan Artifacts",
		Long: "Push Conan Artifacts to Harness Artifact Registry.\n\n" +
			"<reference> is a Conan reference of the form name/version[@user/channel].\n" +
			"<recipe_dir> is a directory containing the exported recipe-layer files " +
			"(conanfile.py, conanmanifest.txt, conan_export.tgz, conan_sources.tgz).\n" +
			"Use --package-dir together with --package-id to also upload a package binary.",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != conanExpectedNumberOfArgument {
				return fmt.Errorf(
					"Error: Invalid number of argument,  accepts %d arg(s), received %d  \nUsage :\n %s",
					conanExpectedNumberOfArgument, len(args), cmd.UseLine(),
				)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			registryName := args[0]
			reference := args[1]
			recipeDir := args[2]

			progress := p.NewConsoleReporter()

			progress.Start("Validating input parameters")

			ref, err := parseConanReference(reference)
			if err != nil {
				progress.Error("Invalid Conan reference")
				return errors.NewValidationError("reference", err.Error())
			}

			// Unrecognised files (e.g. .DS_Store) are skipped to avoid a server 400.
			recipeFiles, recipeSkipped, err := collectConanLayerFiles(recipeDir, conanutil.IsConanRecipeFile)
			if err != nil {
				progress.Error("Failed to read recipe directory")
				return errors.NewValidationError("recipe_dir", err.Error())
			}
			if len(recipeSkipped) > 0 {
				progress.Step(fmt.Sprintf("Skipping %d non-Conan file(s) in recipe dir: %s",
					len(recipeSkipped), strings.Join(recipeSkipped, ", ")))
			}

			// Default RREV to the MD5 of conanmanifest.txt.
			if recipeRevision == "" {
				recipeRevision, err = conanRevisionFromManifest(recipeDir)
				if err != nil {
					progress.Error("Failed to derive recipe revision")
					return errors.NewValidationError("recipe-revision", err.Error())
				}
			}
			if !conanRevisionPattern.MatchString(recipeRevision) {
				progress.Error("Invalid recipe revision")
				return errors.NewValidationError("recipe-revision",
					fmt.Sprintf("recipe revision must be a 32-char MD5 or 40-char SHA, got: %s", recipeRevision))
			}

			// Validate package-layer inputs before uploading anything.
			var packageFiles []string
			if packageDir != "" {
				if packageID == "" {
					progress.Error("Missing package id")
					return errors.NewValidationError("package-id", "--package-id is required when --package-dir is set")
				}
				if !conanPackageIDPattern.MatchString(packageID) {
					progress.Error("Invalid package id")
					return errors.NewValidationError("package-id",
						fmt.Sprintf("package id must be a 40-char SHA-1, got: %s", packageID))
				}
				var packageSkipped []string
				packageFiles, packageSkipped, err = collectConanLayerFiles(packageDir, conanutil.IsConanPackageFile)
				if err != nil {
					progress.Error("Failed to read package directory")
					return errors.NewValidationError("package-dir", err.Error())
				}
				if len(packageSkipped) > 0 {
					progress.Step(fmt.Sprintf("Skipping %d non-Conan file(s) in package dir: %s",
						len(packageSkipped), strings.Join(packageSkipped, ", ")))
				}
				if packageRevision == "" {
					packageRevision, err = conanRevisionFromManifest(packageDir)
					if err != nil {
						progress.Error("Failed to derive package revision")
						return errors.NewValidationError("package-revision", err.Error())
					}
				}
				if !conanRevisionPattern.MatchString(packageRevision) {
					progress.Error("Invalid package revision")
					return errors.NewValidationError("package-revision",
						fmt.Sprintf("package revision must be a 32-char MD5 or 40-char SHA, got: %s", packageRevision))
				}
			}

			progress.Success(fmt.Sprintf("Validated Conan reference %s", ref.Display()))

			// Recipe layer (manifest last).
			progress.Step(fmt.Sprintf("Uploading recipe files (rrev %s)", recipeRevision))
			for _, filePath := range orderConanFiles(recipeFiles) {
				if err := uploadConanRecipeFile(c, registryName, ref, recipeRevision, filePath, progress); err != nil {
					return err
				}
			}

			// Package layer (manifest last), if requested.
			if packageDir != "" {
				progress.Step(fmt.Sprintf("Uploading package files (pkgid %s, prev %s)", packageID, packageRevision))
				for _, filePath := range orderConanFiles(packageFiles) {
					if err := uploadConanPackageFile(c, registryName, ref, recipeRevision, packageID,
						packageRevision, filePath, progress); err != nil {
						return err
					}
				}
			}

			progress.Success(fmt.Sprintf("Successfully uploaded Conan package %s to registry '%s'", ref.Display(), registryName))
			return nil
		},
	}

	cmd.Flags().StringVar(&recipeRevision, "recipe-revision", "", "Recipe revision (RREV). Defaults to the MD5 of conanmanifest.txt")
	cmd.Flags().StringVar(&packageDir, "package-dir", "", "Directory containing package-layer files (conaninfo.txt, conanmanifest.txt, conan_package.tgz)")
	cmd.Flags().StringVar(&packageID, "package-id", "", "Conan package id (PKGID); required with --package-dir")
	cmd.Flags().StringVar(&packageRevision, "package-revision", "", "Package revision (PREV). Defaults to the MD5 of the package's conanmanifest.txt")

	return cmd
}

// parseConanReference parses name/version[@user/channel], defaulting absent
// user/channel to the "_" placeholder.
func parseConanReference(reference string) (conanutil.ConanRef, error) {
	ref := conanutil.ConanRef{User: conanutil.ConanPlaceholder, Channel: conanutil.ConanPlaceholder}

	nameVersion := reference
	if at := strings.Index(reference, "@"); at >= 0 {
		nameVersion = reference[:at]
		userChannel := reference[at+1:]
		uc := strings.Split(userChannel, "/")
		if len(uc) != 2 || uc[0] == "" || uc[1] == "" {
			return conanutil.ConanRef{}, fmt.Errorf("user/channel must be of the form user/channel, got: %q", userChannel)
		}
		ref.User, ref.Channel = uc[0], uc[1]
	}

	nv := strings.Split(nameVersion, "/")
	if len(nv) != 2 || nv[0] == "" || nv[1] == "" {
		return conanutil.ConanRef{}, fmt.Errorf("reference must be of the form name/version[@user/channel], got: %q", reference)
	}
	ref.Name, ref.Version = nv[0], nv[1]

	if !conanNamePattern.MatchString(ref.Name) {
		return conanutil.ConanRef{}, fmt.Errorf("invalid Conan package name: %q", ref.Name)
	}
	if !conanNamePattern.MatchString(ref.Version) {
		return conanutil.ConanRef{}, fmt.Errorf("invalid Conan package version: %q", ref.Version)
	}
	if ref.User != conanutil.ConanPlaceholder && !conanNamePattern.MatchString(ref.User) {
		return conanutil.ConanRef{}, fmt.Errorf("invalid Conan user: %q", ref.User)
	}
	if ref.Channel != conanutil.ConanPlaceholder && !conanNamePattern.MatchString(ref.Channel) {
		return conanutil.ConanRef{}, fmt.Errorf("invalid Conan channel: %q", ref.Channel)
	}

	return ref, nil
}

// collectConanLayerFiles returns the valid top-level Conan files in dir and the names
// of any skipped ones. Requires conanmanifest.txt to be present; sub-dirs are ignored.
func collectConanLayerFiles(dir string, isValid func(string) bool) (files []string, skipped []string, err error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to access directory: %w", err)
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("path must be a directory: %s", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read directory: %w", err)
	}

	hasManifest := false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isValid(name) {
			skipped = append(skipped, name)
			continue
		}
		if name == conanutil.ConanManifestFile {
			hasManifest = true
		}
		files = append(files, filepath.Join(dir, name))
	}

	if !hasManifest {
		return nil, skipped, fmt.Errorf("%s not found in directory: %s", conanutil.ConanManifestFile, dir)
	}

	return files, skipped, nil
}

// orderConanFiles sorts files deterministically with conanmanifest.txt last.
func orderConanFiles(files []string) []string {
	ordered := make([]string, len(files))
	copy(ordered, files)
	sort.Slice(ordered, func(i, j int) bool {
		iManifest := filepath.Base(ordered[i]) == conanutil.ConanManifestFile
		jManifest := filepath.Base(ordered[j]) == conanutil.ConanManifestFile
		if iManifest != jManifest {
			return !iManifest // non-manifest files sort before the manifest
		}
		return ordered[i] < ordered[j]
	})
	return ordered
}

// conanRevisionFromManifest derives a revision as the hex MD5 of conanmanifest.txt.
func conanRevisionFromManifest(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, conanutil.ConanManifestFile))
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", conanutil.ConanManifestFile, err)
	}
	sum := md5.Sum(data) //nolint:gosec // Conan revision model.
	return hex.EncodeToString(sum[:]), nil
}

// uploadConanRecipeFile PUTs a single recipe-layer file to its RREV path.
func uploadConanRecipeFile(
	c *cmdutils.Factory,
	registryName string,
	ref conanutil.ConanRef,
	rrev string,
	filePath string,
	progress *p.ConsoleReporter,
) error {
	fileName := filepath.Base(filePath)
	file, checksums, size, err := openConanFile(filePath, progress)
	if err != nil {
		return err
	}
	defer file.Close()

	progress.Step(fmt.Sprintf("Uploading %s", fileName))
	client := c.PkgHttpClientWithProgress(progress, size, "conan")
	resp, err := client.UploadConanRecipeFileWithBodyWithResponse(
		context.Background(),
		config.Global.AccountID,
		registryName,
		ref.Name,
		ref.Version,
		ref.User,
		ref.Channel,
		rrev,
		fileName,
		"application/octet-stream",
		file,
		func(ctx context.Context, req *http.Request) error {
			req.Header.Set("X-Checksum-Sha1", checksums.SHA1)
			return nil
		},
	)
	if err != nil {
		progress.Error(fmt.Sprintf("Failed to upload %s", fileName))
		return err
	}
	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusCreated {
		progress.Error("Upload failed")
		return fmt.Errorf("failed to push recipe file %s: %s \n response: %s", fileName, resp.Status(), resp.Body)
	}
	return nil
}

// uploadConanPackageFile PUTs a single package-layer file to its PKGID/PREV path.
func uploadConanPackageFile(
	c *cmdutils.Factory,
	registryName string,
	ref conanutil.ConanRef,
	rrev string,
	pkgID string,
	prev string,
	filePath string,
	progress *p.ConsoleReporter,
) error {
	fileName := filepath.Base(filePath)
	file, checksums, size, err := openConanFile(filePath, progress)
	if err != nil {
		return err
	}
	defer file.Close()

	progress.Step(fmt.Sprintf("Uploading %s", fileName))
	client := c.PkgHttpClientWithProgress(progress, size, "conan")
	resp, err := client.UploadConanPackageFileWithBodyWithResponse(
		context.Background(),
		config.Global.AccountID,
		registryName,
		ref.Name,
		ref.Version,
		ref.User,
		ref.Channel,
		rrev,
		pkgID,
		prev,
		fileName,
		"application/octet-stream",
		file,
		func(ctx context.Context, req *http.Request) error {
			req.Header.Set("X-Checksum-Sha1", checksums.SHA1)
			return nil
		},
	)
	if err != nil {
		progress.Error(fmt.Sprintf("Failed to upload %s", fileName))
		return err
	}
	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusCreated {
		progress.Error("Upload failed")
		return fmt.Errorf("failed to push package file %s: %s \n response: %s", fileName, resp.Status(), resp.Body)
	}
	return nil
}

// openConanFile validates the file, computes its checksums, and returns an open handle and its size.
func openConanFile(filePath string, progress *p.ConsoleReporter) (*os.File, utils.FileChecksums, int64, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, utils.FileChecksums{}, 0, errors.NewValidationError("file_path", fmt.Sprintf("failed to access file: %v", err))
	}
	if info.IsDir() {
		return nil, utils.FileChecksums{}, 0, errors.NewValidationError("file_path", fmt.Sprintf("expected a file, got a directory: %s", filePath))
	}

	checksums, err := utils.ComputeFileChecksums(filePath)
	if err != nil {
		progress.Error("Failed to compute file checksums")
		return nil, utils.FileChecksums{}, 0, fmt.Errorf("failed to compute checksums for %s: %w", filePath, err)
	}

	file, err := os.Open(filePath)
	if err != nil {
		progress.Error("Failed to open file")
		return nil, utils.FileChecksums{}, 0, err
	}
	return file, checksums, info.Size(), nil
}
