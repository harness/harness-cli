package command

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar_v3"
	"github.com/harness/harness-cli/module/ar/migrate/util"
	"github.com/harness/harness-cli/util/common/progress"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// stdinReader is a swappable test hook so the confirmation prompts can be
// driven deterministically. Production code uses os.Stdin.
var stdinReader io.Reader = os.Stdin

func NewDeleteArtifactCmd(c *cmdutils.Factory) *cobra.Command {
	var artifact, registry, version string
	var dryRun, force bool

	cmd := &cobra.Command{
		Use:   "delete [artifact-name]",
		Short: "Delete an artifact or a specific version",
		Long:  "Deletes an artifact and all its versions, or a specific version if --version flag is provided",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := progress.NewConsoleReporter()
			artifact = args[0]

			p.Start(fmt.Sprintf("Ready for execution with delete mode force = %t", force))

			_, err := util.IsWildCardExpression(artifact)
			if err != nil {
				p.Error(fmt.Sprintf("Invalid package expression: %s", artifact))
				return err
			}
			p.Step(fmt.Sprintf("package expression validated :"))

			versions := version
			impactType := "Packages"
			if versions != "" {
				impactType = "Versions"
				_, err := util.IsWildCardExpression(versions)
				if err != nil {
					p.Error(fmt.Sprintf("Invalid version expression: %s", versions))
					return err
				}
				p.Step(fmt.Sprintf("version expression validated :"))
			}

			p.Step(fmt.Sprintf("Registry : %s", registry))
			p.Step(fmt.Sprintf("Dry-run mode : %t", dryRun))
			p.Step(fmt.Sprintf("Force delete : %t", force))
			p.Success("Input parameters validated")

			//if force is on, ignore all other flags
			if force {
				p.Error("Warning :: Force (hard) delete is enabled. This action is irreversible")
				p.Success("Tip: run with --dry-run first to preview impacted packages/versions.")
				fmt.Print("Are you sure you want to proceed with force delete ? (y/N): ")
				reader := bufio.NewReader(stdinReader)
				response, rErr := reader.ReadString('\n')
				if rErr != nil {
					p.Error("Failed to read confirmation input")
					return fmt.Errorf("failed to read confirmation: %w", rErr)
				}
				response = strings.TrimSpace(response)
				if response != "y" && response != "Y" {
					p.Error("Force delete cancelled by user")
					return fmt.Errorf("force delete cancelled by user")
				}
				p.Step("User confirmed force delete")
			}

			// Resolve org/project from global config
			org := config.Global.OrgID
			project := config.Global.ProjectID

			params := &ar_v3.BulkDeleteArtifactsParams{
				AccountIdentifier: config.Global.AccountID,
			}
			if org != "" {
				params.OrgIdentifier = &org
			}
			if project != "" {
				params.ProjectIdentifier = &project
			}

			resp, err := executeBulkDelete(c, params, artifact, versions, registry, force, dryRun, p)
			if err != nil {
				return err
			}

			if resp.StatusCode() != 200 {
				errMsg := fmt.Sprintf("Bulk delete failed with status %d", resp.StatusCode())
				if resp.JSONDefault != nil && resp.JSONDefault.Error.Message != nil {
					errMsg = *resp.JSONDefault.Error.Message
				}
				p.Error(errMsg)
				return fmt.Errorf("%s", errMsg)
			}

			if resp.JSON200 != nil {
				out, err := json.MarshalIndent(resp.JSON200, "", "  ")
				if err != nil {
					p.Error("Failed to marshal JSON output")
					log.Error().Err(err).Msg("Failed to marshal JSON output : " + string(out))
					return err
				} else {
					err := executeWithDryRunResponse(resp.Body, p, impactType, c, params, artifact, versions, registry, force)
					if err != nil {
						return err
					}
				}
			} else {
				log.Error().Err(err).Msg("resp.JSON200 in nil " + string(resp.Body))
			}

			p.Success("Bulk delete completed successfully")
			return nil
		},
	}

	// Common flags
	cmd.Flags().StringVar(&registry, "registry", "", "name of the registry")
	cmd.Flags().BoolVar(&force, "force", false, "delete type hard/soft , hard when force = true , will delete permanently")
	cmd.Flags().StringVar(&version, "version", "", "specific version to delete (if not provided, deletes all versions)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", true, "Run Deletion in dry-run mode (no real deletion, generates version or package  list impacted)")

	cmd.MarkFlagRequired("registry")

	return cmd
}

func executeBulkDelete(
	c *cmdutils.Factory,
	params *ar_v3.BulkDeleteArtifactsParams,
	artifact, versions, registry string,
	force, dryRun bool,
	p *progress.ConsoleReporter,
) (*ar_v3.BulkDeleteArtifactsResp, error) {
	body := ar_v3.BulkDeleteArtifactsJSONRequestBody{
		Packages: artifact,
		Versions: versions,
		Registry: registry,
		Force:    &force,
		DryRun:   &dryRun,
	}

	p.Step("executing  bulk delete ..")
	resp, err := c.RegistryV3HttpClient().BulkDeleteArtifactsWithResponse(
		context.Background(),
		params,
		body,
	)
	if err != nil {
		p.Error("bulk delete execution  failed")
		return nil, fmt.Errorf("bulk delete execution  failed: %w", err)
	}
	return resp, nil
}

// bulkDeleteDryRunResponse mirrors the JSON shape returned by the bulk delete API
// when invoked in dry-run mode.
type bulkDeleteDryRunResponse struct {
	DryRun          bool     `json:"dryRun"`
	Failed          int      `json:"failed"`
	FailedPackages  []string `json:"failedPackages"`
	Force           bool     `json:"force"`
	Message         string   `json:"message"`
	Pattern         string   `json:"pattern"`
	Registry        string   `json:"registry"`
	Success         int      `json:"success"`
	SuccessPackages []string `json:"successPackages"`
	Total           int      `json:"total"`
	VersionPattern  string   `json:"versionPattern"`
}

func executeWithDryRunResponse(
	body []byte,
	p *progress.ConsoleReporter,
	impactType string,
	c *cmdutils.Factory,
	params *ar_v3.BulkDeleteArtifactsParams,
	artifact, versions, registry string,
	force bool,
) error {
	var parsed bulkDeleteDryRunResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		log.Error().Err(err).Msg("Failed to parse bulk delete response: " + string(body))
		return fmt.Errorf("failed to parse bulk delete response: %w", err)
	}

	if parsed.Message != "" {
		fmt.Println(parsed.Message)
	}

	fmt.Printf("Registry        : %s\n", parsed.Registry)
	fmt.Printf("Version pattern : %s\n", parsed.VersionPattern)
	fmt.Printf("Dry-run         : %t\n", parsed.DryRun)
	fmt.Printf("Force           : %t\n", parsed.Force)
	fmt.Printf("Total impacted  : %d (success: %d, failed: %d)\n",
		parsed.Total, parsed.Success, parsed.Failed)

	if len(parsed.SuccessPackages) == 0 {
		fmt.Println("No package/Version found to be deleted matching given pattern")
		return nil
	}
	p.Step("Printing impacted packages/version")
	err := printOutPut(parsed.SuccessPackages)
	if err != nil {
		return err
	}

	extra := parsed.Success - len(parsed.SuccessPackages)
	if extra > 0 {

		extraMessage := fmt.Sprintf("... and %d more %s, will be impacted (not listed above)\n", extra, impactType)
		if !parsed.DryRun {
			// Already a real-run response; change to final message
			extraMessage = fmt.Sprintf("... and %d more %s, is deleted \n", extra, impactType)
		}
		fmt.Println(extraMessage)
	}

	p.Step("Printing complete")

	if len(parsed.FailedPackages) > 0 {

		p.Step(fmt.Sprintf("Printing faliure : "))
		for _, pkg := range parsed.FailedPackages {
			p.Step(fmt.Sprintf("%s \n", pkg))
		}
	}

	if !parsed.DryRun {
		// Already a real-run response; nothing more to do.
		return nil
	}

	fmt.Printf("Above %s will be soft deleted. Do you want to proceed? (y/N): ", impactType)
	reader := bufio.NewReader(stdinReader)
	response, rErr := reader.ReadString('\n')
	if rErr != nil {
		p.Error("Failed to read confirmation input")
		return fmt.Errorf("failed to read confirmation: %w", rErr)
	}
	response = strings.TrimSpace(response)
	if response != "y" && response != "Y" {
		p.Error("Bulk delete cancelled by user")
		return fmt.Errorf("bulk delete cancelled by user")
	}
	p.Step("User confirmed; executing actual bulk delete (dry-run=false)")

	//now running with dryRun = false , as user confirmed to delete
	resp, err := executeBulkDelete(c, params, artifact, versions, registry, force, false, p)
	if err != nil {
		return err
	}
	if resp.StatusCode() != 200 {
		errMsg := fmt.Sprintf("Bulk delete failed with status %d", resp.StatusCode())
		if resp.JSONDefault != nil && resp.JSONDefault.Error.Message != nil {
			errMsg = *resp.JSONDefault.Error.Message
		}
		p.Error(errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	var actual bulkDeleteDryRunResponse
	if uErr := json.Unmarshal(resp.Body, &actual); uErr != nil {
		log.Error().Err(uErr).Msg("Failed to parse actual bulk delete response: " + string(resp.Body))
		return fmt.Errorf("failed to parse actual bulk delete response: %w", uErr)
	}
	if actual.Message != "" {
		fmt.Println(actual.Message)
	}
	fmt.Printf("Deleted        : %d / %d (failed: %d)\n", actual.Success, actual.Total, actual.Failed)
	if len(actual.FailedPackages) > 0 {
		p.Step("Printing failure :")
		for _, pkg := range actual.FailedPackages {
			p.Step(fmt.Sprintf("%s \n", pkg))
		}
	}

	return nil
}

func printOutPut(filteredSlice []string) error {
	fmt.Println("Impacted package/Version")
	for _, pkg := range filteredSlice {
		fmt.Printf("%s \n", pkg)
	}
	return nil
}
