package command

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/util/common/errors"
	"github.com/harness/harness-cli/util/common/fileutil"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/spf13/cobra"
)

const (
	RubyFileExtension = ".gem"
)

type rubyUploadResponse struct {
	Status   string `json:"status"`
	Name     string `json:"name"`
	Version  string `json:"version"`
	Platform string `json:"platform"`
	SHA256   string `json:"sha256"`
}

func NewPushRubyCmd(c *cmdutils.Factory) *cobra.Command {
	const expectedNumberOfArgument = 2
	var postMetadata string
	cmd := &cobra.Command{
		Use:   "ruby <registry_name> <file_path>",
		Short: "Push Ruby gem",
		Long:  "Push a Ruby gem (.gem) to Harness Artifact Registry",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != expectedNumberOfArgument {
				return fmt.Errorf(
					"Error: Invalid number of argument,  accepts %d arg(s), received %d  \nUsage :\n %s",
					expectedNumberOfArgument, len(args), cmd.UseLine(),
				)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			registryName := args[0]
			filePath := args[1]

			progress := p.NewConsoleReporter()

			fileName := filepath.Base(filePath)

			fileInfo, err := os.Stat(filePath)
			if err != nil {
				return errors.NewValidationError("file_path", fmt.Sprintf("failed to access package file: %v", err))
			}
			if fileInfo.IsDir() {
				return errors.NewValidationError("file_path", "package file path must be a file, not a directory")
			}

			valid, err := fileutil.IsFilenameAcceptable(fileName, RubyFileExtension)
			if !valid {
				progress.Error("Invalid file name")
				return errors.NewValidationError("file_path", fmt.Sprintf("failed to validate package file name: %v", err))
			}

			file, err := os.Open(filePath)
			if err != nil {
				progress.Error("Failed to open package file")
				return err
			}
			defer file.Close()

			progress.Step("Uploading package to registry")

			pkgClient := c.PkgHttpClientWithProgress(progress, fileInfo.Size(), "ruby")

			// Do not send X-Checksum-* headers: the Ruby push handler writes
			// server-generated sidecar files (version_info.json/yaml) in the same
			// request, and the backend would incorrectly validate those against the
			// gem's digests. Native gem push does not send checksum headers either.
			resp, err := pkgClient.UploadRubyPackageWithBodyWithResponse(
				context.Background(),
				config.Global.AccountID,
				registryName,
				"application/octet-stream",
				file,
			)
			if err != nil {
				progress.Error("Failed to upload package")
				return err
			}
			if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusCreated {
				progress.Error("Upload failed")
				return fmt.Errorf("failed to push package: %s \n response: %s", resp.Status(), resp.Body)
			}

			successMsg := fmt.Sprintf("Successfully uploaded package %s", filePath)
			var uploadedPkgName, uploadedPkgVersion string
			if len(resp.Body) > 0 {
				var uploadResp rubyUploadResponse
				if err := json.Unmarshal(resp.Body, &uploadResp); err == nil && uploadResp.Name != "" && uploadResp.Version != "" {
					uploadedPkgName = uploadResp.Name
					uploadedPkgVersion = uploadResp.Version
					if uploadResp.Platform != "" {
						successMsg = fmt.Sprintf("Successfully uploaded %s@%s (%s)", uploadResp.Name, uploadResp.Version, uploadResp.Platform)
					} else {
						successMsg = fmt.Sprintf("Successfully uploaded %s@%s", uploadResp.Name, uploadResp.Version)
					}
				}
			}

			progress.Success(successMsg)
			applyPostPushMetadata(c, postMetadata, registryName, uploadedPkgName, uploadedPkgVersion)
			return nil
		},
	}

	cmd.Flags().StringVar(&postMetadata, "metadata", "", "Metadata key-value pairs to attach after push (format: key:value,key2:value2)")
	return cmd
}
