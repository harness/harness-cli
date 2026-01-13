package command

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	v2client "github.com/harness/harness-cli/internal/api/ar_v2"
	p "github.com/harness/harness-cli/util/common/progress"
	"github.com/spf13/cobra"
)

func NewCopyArtifactCmd(c *cmdutils.Factory) *cobra.Command {
	var artifactType string
	const expectedArgumentCount = 2
	cmd := &cobra.Command{
		Use:   "copy <SRC_REGISTRY>/<PACKAGE_NAME>/<VERSION> <DEST_REGISTRY>",
		Short: "copy an artifact package  specific version",
		Long:  "copy an artifact package specific version to another registry",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != expectedArgumentCount {
				return fmt.Errorf(
					"Error: Invalid number of argument,  accepts %d arg(s), received %d  \nUsage :\n %s",
					expectedArgumentCount, len(args), cmd.UseLine(),
				)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			srcPackagePath := args[0]
			targetRegistryIdentifier := args[1]

			// Create progress reporter
			progress := p.NewConsoleReporter()

			progress.Start("Validating input parameters")

			srcDetail, err := parsePackagePath(srcPackagePath)
			if err != nil {
				progress.Error("Failed to validate input parameter")
				return err
			}
			srcRegistryIdentifier := srcDetail[0]
			srcArtifact := srcDetail[1]
			srcVersion := srcDetail[2]

			copyReqParam := &v2client.CopyRegistryPackageParams{
				AccountIdentifier:        config.Global.AccountID,
				SrcRegistryIdentifier:    srcRegistryIdentifier,
				TargetRegistryIdentifier: targetRegistryIdentifier,
				SrcArtifact:              srcArtifact,
				SrcVersion:               srcVersion,
			}

			if len(artifactType) > 0 {
				//setting additional param required for Hugging Face
				copyReqParam.ArtifactType = &artifactType
			}

			//validating all required parameter
			if err := validateCopyRegistryPackageParams(copyReqParam); err != nil {
				return err
			}
			progress.Success("Input parameters validated")
			// Initialize the package client

			progress.Step(fmt.Sprintf("copying package from %s to %s", srcPackagePath, targetRegistryIdentifier))

			resp, err := c.RegistryV2HttpClient().CopyRegistryPackageWithResponse(
				context.Background(),
				copyReqParam,
			)

			if err != nil {
				progress.Error("Failed to upload package")
				return err
			}
			// Check response
			if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusCreated {
				progress.Error("Copy failed")
				return fmt.Errorf("failed to copy package: %s \n response: %s", resp.Status(), resp.Body)
			}

			progress.Success(fmt.Sprintf("Successfully Copied package from  %s to %s", srcPackagePath, targetRegistryIdentifier))
			return nil
		},
	}

	cmd.Flags().StringVar(&artifactType, "artifact-type", "", "artifact type used e.g. model or dataset")
	return cmd
}

// parsing input of the form ,based on first and last slash
// <SRC_REGISTRY>/<ARTIFACT_PATH>/<VERSION>
func parsePackagePath(input string) ([]string, error) {
	if strings.TrimSpace(input) == "" {
		return nil, fmt.Errorf("input cannot be empty")
	}

	firstSlash := strings.Index(input, "/")
	lastSlash := strings.LastIndex(input, "/")

	// Must contain at least two slashes
	if firstSlash == -1 || lastSlash == -1 || firstSlash == lastSlash {
		return nil, fmt.Errorf(
			"invalid format: expected '<SRC_REGISTRY>/<ARTIFACT_PATH>/<VERSION>'",
		)
	}

	registry := input[:firstSlash]
	artifact := input[firstSlash+1 : lastSlash]
	version := input[lastSlash+1:]

	// Validate parts
	if strings.TrimSpace(registry) == "" {
		return nil, fmt.Errorf("invalid format: registry is empty")
	}
	if strings.TrimSpace(artifact) == "" {
		return nil, fmt.Errorf("invalid format: artifact path is empty")
	}
	if strings.TrimSpace(version) == "" {
		return nil, fmt.Errorf("invalid format: version is empty")
	}

	return []string{registry, artifact, version}, nil
}

func validateCopyRegistryPackageParams(r *v2client.CopyRegistryPackageParams) error {
	if r == nil {
		return fmt.Errorf("params cannot be nil")
	}
	if r.AccountIdentifier == "" {
		return fmt.Errorf("account_identifier is required")
	}
	if r.SrcRegistryIdentifier == "" {
		return fmt.Errorf("src_registry_identifier is required")
	}
	if r.TargetRegistryIdentifier == "" {
		return fmt.Errorf("target_registry_identifier is required")
	}
	if r.SrcArtifact == "" {
		return fmt.Errorf("src_artifact is required")
	}
	if r.SrcVersion == "" {
		return fmt.Errorf("src_version is required")
	}

	return nil
}
