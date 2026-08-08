package command

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar"
	"github.com/harness/harness-cli/internal/api/ar_v3"
	"github.com/harness/harness-cli/module/ar/migrate/util"
	client2 "github.com/harness/harness-cli/util/client"
	"github.com/harness/harness-cli/util/common/progress"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// stdinReader is a swappable test hook so the confirmation prompts can be
// driven deterministically. Production code uses os.Stdin.
var stdinReader io.Reader = os.Stdin

// stdinIsTerminal reports whether the confirmation prompt can actually be
// answered interactively. Piped or closed stdin is never a TTY, so a piped
// "y" can never authorize a real delete. Swappable test hook.
var stdinIsTerminal = func() bool {
	if f, ok := stdinReader.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

// deleteOutcome classifies the verified post-delete state of one coordinate.
type deleteOutcome string

const (
	outcomeSoftDeleted deleteOutcome = "SOFT_DELETED"
	outcomeHardDeleted deleteOutcome = "HARD_DELETED"
	outcomeUnchanged   deleteOutcome = "UNCHANGED"
	outcomeUnsupported deleteOutcome = "UNSUPPORTED"
)

// coordinateVerification is the per-coordinate outcome of a real delete,
// established by re-reading the coordinate after the server reports success.
type coordinateVerification struct {
	Coordinate string        `json:"coordinate"`
	Outcome    deleteOutcome `json:"outcome"`
	Detail     string        `json:"detail,omitempty"`
}

func NewDeleteArtifactCmd(c *cmdutils.Factory) *cobra.Command {
	var artifact, registry, version string
	var dryRun, force, yes bool

	cmd := &cobra.Command{
		Use:   "delete [artifact-name]",
		Short: "Delete an artifact or a specific version",
		Long: "Deletes an artifact and all its versions, or a specific version if --version flag is provided.\n\n" +
			"By default the command runs in dry-run mode and only previews what would be deleted; " +
			"a dry run never mutates anything and never prompts.\n" +
			"To execute a real delete pass --dry-run=false. A real delete requires confirmation: " +
			"an interactive prompt when stdin is a terminal, or --yes for non-interactive use.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := progress.NewConsoleReporter()
			artifact = args[0]

			// --yes confirms a real delete; combining it with the default
			// dry-run mode is a contradiction and almost certainly a mistake.
			if yes && dryRun {
				p.Error("Invalid flag combination: --yes cannot be used with --dry-run=true")
				return fmt.Errorf("--yes confirms a real delete and cannot be combined with --dry-run=true; drop --yes to preview, or pass --dry-run=false to delete")
			}

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

			if dryRun {
				if force {
					p.Error("Warning :: Force (hard) delete is enabled. A real run (--dry-run=false) with force is irreversible")
				}
				p.Success("Tip: run with --dry-run=false to execute the deletion after previewing")
			} else if err := confirmRealDelete(p, artifact, versions, registry, force, yes); err != nil {
				return err
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

			if resp.StatusCode() == http.StatusMethodNotAllowed {
				errMsg := "bulk delete is not supported by the server for this request (HTTP 405 Method Not Allowed); " +
					"this artifact type may not support deletion via this endpoint and nothing was deleted by this command. " +
					"Previously soft-deleted artifacts can be restored through the Harness HAR REST API restore endpoint"
				p.Error(errMsg)
				return fmt.Errorf("%s", errMsg)
			}

			if resp.StatusCode() != 200 {
				errMsg := fmt.Sprintf("Bulk delete failed with status %d", resp.StatusCode())
				if resp.JSONDefault != nil && resp.JSONDefault.Error.Message != nil {
					errMsg = *resp.JSONDefault.Error.Message
				}
				p.Error(errMsg)
				return fmt.Errorf("%s", errMsg)
			}

			var parsed bulkDeleteDryRunResponse
			if err := json.Unmarshal(resp.Body, &parsed); err != nil {
				log.Error().Err(err).Msg("Failed to parse bulk delete response: " + string(resp.Body))
				p.Error("Failed to parse bulk delete response")
				return fmt.Errorf("failed to parse bulk delete response: %w", err)
			}

			// The server's dryRun flag is authoritative: a dry-run response
			// means nothing was mutated, so only ever render the preview.
			if parsed.DryRun {
				printDryRunPreview(&parsed, impactType, p)
				return nil
			}

			return reportAndVerifyDelete(&parsed, c, registry, force, impactType, p)
		},
	}

	// Common flags
	cmd.Flags().StringVar(&registry, "registry", "", "name of the registry")
	cmd.Flags().BoolVar(&force, "force", false, "delete type hard/soft , hard when force = true , will delete permanently")
	cmd.Flags().StringVar(&version, "version", "", "specific version to delete (if not provided, deletes all versions)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", true, "Run Deletion in dry-run mode (no real deletion, generates version or package  list impacted)")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm a real delete non-interactively (only valid with --dry-run=false); required when stdin is not a terminal")

	cmd.MarkFlagRequired("registry")

	return cmd
}

// confirmRealDelete gates a mutating run (--dry-run=false). --yes confirms
// non-interactively; otherwise an interactive prompt is shown only when stdin
// is a terminal. A real delete can never be authorized by piped input.
func confirmRealDelete(p *progress.ConsoleReporter, artifact, versions, registry string, force, yes bool) error {
	if yes {
		p.Step("Delete confirmed via --yes flag")
		return nil
	}
	if !stdinIsTerminal() {
		p.Error("Confirmation required for a real delete")
		return fmt.Errorf("confirmation required: refusing a real delete (--dry-run=false) because stdin is not interactive; re-run with --yes to confirm, or use --dry-run=true to preview")
	}

	scope := fmt.Sprintf("artifacts matching '%s'", artifact)
	if versions != "" {
		scope = fmt.Sprintf("artifacts matching '%s' with versions matching '%s'", artifact, versions)
	}
	if force {
		p.Error("Warning :: Force (hard) delete is enabled. This action is irreversible")
		fmt.Printf("This will PERMANENTLY delete %s in registry '%s'. Do you want to proceed? (y/N): ", scope, registry)
	} else {
		fmt.Printf("This will soft delete %s in registry '%s'. Do you want to proceed? (y/N): ", scope, registry)
	}
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
	p.Step("User confirmed delete")
	return nil
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

// dryRunResultOutput is the machine-readable dry-run contract emitted with
// --format json. Mutated is always false so wrappers can assert the
// no-mutation guarantee.
type dryRunResultOutput struct {
	Mutated        bool     `json:"mutated"`
	DryRun         bool     `json:"dryRun"`
	Force          bool     `json:"force"`
	Registry       string   `json:"registry"`
	Pattern        string   `json:"pattern,omitempty"`
	VersionPattern string   `json:"versionPattern,omitempty"`
	Message        string   `json:"message,omitempty"`
	Total          int      `json:"total"`
	Success        int      `json:"success"`
	Failed         int      `json:"failed"`
	Impacted       []string `json:"impacted"`
	NotListed      int      `json:"notListed,omitempty"`
	FailedPackages []string `json:"failedPackages,omitempty"`
}

// deleteResultOutput is the machine-readable result of a real delete emitted
// with --format json. Mutated is true and every server-claimed coordinate
// carries a verified per-coordinate outcome.
type deleteResultOutput struct {
	Mutated        bool                     `json:"mutated"`
	Force          bool                     `json:"force"`
	Registry       string                   `json:"registry"`
	Message        string                   `json:"message,omitempty"`
	Total          int                      `json:"total"`
	Success        int                      `json:"success"`
	Failed         int                      `json:"failed"`
	FailedPackages []string                 `json:"failedPackages,omitempty"`
	Coordinates    []coordinateVerification `json:"coordinates"`
	NotVerified    int                      `json:"notVerified,omitempty"`
	OutcomeCounts  map[deleteOutcome]int    `json:"outcomeCounts"`
}

// printDryRunPreview renders the impact preview for a dry-run response. It
// never prompts and never triggers a follow-up mutation: dry-run output
// always ends with the machine-readable "mutated: false" guarantee.
func printDryRunPreview(parsed *bulkDeleteDryRunResponse, impactType string, p *progress.ConsoleReporter) {
	if config.Global.Format == "json" {
		out := dryRunResultOutput{
			Mutated:        false,
			DryRun:         true,
			Force:          parsed.Force,
			Registry:       parsed.Registry,
			Pattern:        parsed.Pattern,
			VersionPattern: parsed.VersionPattern,
			Message:        parsed.Message,
			Total:          parsed.Total,
			Success:        parsed.Success,
			Failed:         parsed.Failed,
			Impacted:       parsed.SuccessPackages,
			FailedPackages: parsed.FailedPackages,
		}
		if out.Impacted == nil {
			out.Impacted = []string{}
		}
		if extra := parsed.Success - len(parsed.SuccessPackages); extra > 0 {
			out.NotListed = extra
		}
		jsonBytes, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			p.Error("Failed to marshal JSON output")
			log.Error().Err(err).Msg("Failed to marshal dry-run JSON output")
			return
		}
		fmt.Println(string(jsonBytes))
		return
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
	} else {
		p.Step("Printing impacted packages/version")
		_ = printOutPut(parsed.SuccessPackages)
		if extra := parsed.Success - len(parsed.SuccessPackages); extra > 0 {
			fmt.Printf("... and %d more %s will be impacted (not listed above)\n", extra, impactType)
		}
		p.Step("Printing complete")
	}

	if len(parsed.FailedPackages) > 0 {
		p.Step(fmt.Sprintf("Printing faliure : "))
		for _, pkg := range parsed.FailedPackages {
			p.Step(fmt.Sprintf("%s \n", pkg))
		}
	}

	fmt.Println("mutated: false")
	p.Success("Dry run complete: no artifacts were deleted")
}

// reportAndVerifyDelete handles a real (dryRun=false) bulk delete response.
// Instead of trusting the server's success count it re-reads every reported
// coordinate and reports per-coordinate outcomes. The run fails (non-zero
// exit) when the server reported failures, when any coordinate is UNCHANGED
// (still present after the delete — the Conda/Terraform defect), or when any
// coordinate's outcome is UNSUPPORTED (could not be verified).
func reportAndVerifyDelete(
	parsed *bulkDeleteDryRunResponse,
	c *cmdutils.Factory,
	registry string,
	force bool,
	impactType string,
	p *progress.ConsoleReporter,
) error {
	deletedOutcome := outcomeSoftDeleted
	if force {
		deletedOutcome = outcomeHardDeleted
	}

	jsonMode := config.Global.Format == "json"

	if parsed.Message != "" && !jsonMode {
		fmt.Println(parsed.Message)
	}

	if parsed.Success == 0 && len(parsed.SuccessPackages) == 0 && parsed.Failed == 0 {
		fmt.Println("No package/Version found to be deleted matching given pattern")
		return nil
	}

	registryRef := client2.GetRef(config.Global.AccountID, config.Global.OrgID, config.Global.ProjectID, registry)
	if !jsonMode {
		p.Step("Verifying deleted coordinates")
	}
	verifications := make([]coordinateVerification, 0, len(parsed.SuccessPackages))
	for _, coordinate := range parsed.SuccessPackages {
		verifications = append(verifications, verifyCoordinate(c, registryRef, coordinate, deletedOutcome))
	}

	outcomeCounts := map[deleteOutcome]int{}
	for _, v := range verifications {
		outcomeCounts[v.Outcome]++
	}
	notVerified := parsed.Success - len(parsed.SuccessPackages)

	if config.Global.Format == "json" {
		out := deleteResultOutput{
			Mutated:        true,
			Force:          parsed.Force,
			Registry:       parsed.Registry,
			Message:        parsed.Message,
			Total:          parsed.Total,
			Success:        parsed.Success,
			Failed:         parsed.Failed,
			FailedPackages: parsed.FailedPackages,
			Coordinates:    verifications,
			OutcomeCounts:  outcomeCounts,
		}
		if out.Coordinates == nil {
			out.Coordinates = []coordinateVerification{}
		}
		if notVerified > 0 {
			out.NotVerified = notVerified
		}
		jsonBytes, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			p.Error("Failed to marshal JSON output")
			log.Error().Err(err).Msg("Failed to marshal delete result JSON output")
		} else {
			fmt.Println(string(jsonBytes))
		}
	} else {
		for _, v := range verifications {
			line := fmt.Sprintf("%s -> %s", v.Coordinate, v.Outcome)
			if v.Detail != "" {
				line = fmt.Sprintf("%s (%s)", line, v.Detail)
			}
			fmt.Println(line)
		}
		if notVerified > 0 {
			fmt.Printf("... and %d more %s reported deleted by the server (not listed above, not individually verified)\n",
				notVerified, impactType)
		}
		if len(parsed.FailedPackages) > 0 {
			p.Step("Printing failure : ")
			for _, pkg := range parsed.FailedPackages {
				p.Step(fmt.Sprintf("%s \n", pkg))
			}
		}
		fmt.Println("mutated: true")
		fmt.Printf("Outcomes: %s=%d %s=%d %s=%d %s=%d (server-reported success: %d, failed: %d)\n",
			outcomeSoftDeleted, outcomeCounts[outcomeSoftDeleted],
			outcomeHardDeleted, outcomeCounts[outcomeHardDeleted],
			outcomeUnchanged, outcomeCounts[outcomeUnchanged],
			outcomeUnsupported, outcomeCounts[outcomeUnsupported],
			parsed.Success, parsed.Failed)
	}

	if parsed.Failed > 0 {
		errMsg := fmt.Sprintf("bulk delete reported %d failed coordinate(s)", parsed.Failed)
		p.Error(errMsg)
		return fmt.Errorf("%s", errMsg)
	}
	if n := outcomeCounts[outcomeUnchanged]; n > 0 {
		errMsg := fmt.Sprintf("delete verification failed: %d coordinate(s) are still present after the delete (outcome %s); the server reported success but the artifacts were retained", n, outcomeUnchanged)
		p.Error(errMsg)
		return fmt.Errorf("%s", errMsg)
	}
	if n := outcomeCounts[outcomeUnsupported]; n > 0 {
		errMsg := fmt.Sprintf("delete verification inconclusive: %d coordinate(s) could not be re-read after the delete (outcome %s); refusing to claim success", n, outcomeUnsupported)
		p.Error(errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	if !jsonMode {
		p.Success("Bulk delete completed and verified")
	}
	return nil
}

// verifyCoordinate re-reads one deleted coordinate through the v1 read API
// (get artifact / get version summary). A coordinate that still resolves
// means the server claimed success while retaining the artifact and is
// reported as UNCHANGED. Coordinates the read API cannot answer for (405,
// transport errors, unexpected statuses) are UNSUPPORTED rather than being
// silently counted as deleted.
func verifyCoordinate(
	c *cmdutils.Factory,
	registryRef, coordinate string,
	deletedOutcome deleteOutcome,
) coordinateVerification {
	result := coordinateVerification{Coordinate: coordinate}
	if c.RegistryHttpClient == nil {
		result.Outcome = outcomeUnsupported
		result.Detail = "registry read client unavailable; cannot verify"
		return result
	}

	name, version := splitCoordinate(coordinate)
	status := 0
	var err error
	if version != "" {
		var resp *ar.GetArtifactVersionSummaryResp
		resp, err = c.RegistryHttpClient().GetArtifactVersionSummaryWithResponse(
			context.Background(), registryRef, name, version, &ar.GetArtifactVersionSummaryParams{})
		if resp != nil {
			status = resp.StatusCode()
		}
	} else {
		var resp *ar.GetArtifactSummaryResp
		resp, err = c.RegistryHttpClient().GetArtifactSummaryWithResponse(
			context.Background(), registryRef, name)
		if resp != nil {
			status = resp.StatusCode()
		}
	}

	switch {
	case err != nil:
		result.Outcome = outcomeUnsupported
		result.Detail = fmt.Sprintf("verification read failed: %v", err)
	case status == http.StatusOK:
		result.Outcome = outcomeUnchanged
		result.Detail = "coordinate still present after delete"
	case status == http.StatusNotFound:
		result.Outcome = deletedOutcome
	case status == http.StatusMethodNotAllowed:
		result.Outcome = outcomeUnsupported
		result.Detail = "existence read not supported for this coordinate (HTTP 405)"
	default:
		result.Outcome = outcomeUnsupported
		result.Detail = fmt.Sprintf("unexpected verification read status %d", status)
	}
	return result
}

// splitCoordinate splits a server-reported coordinate of the form
// "<name>@<version>" on the last '@'. A leading '@' (e.g. NPM "@scope/name")
// is part of the name, not a separator; coordinates without a trailing
// @version are package-level.
func splitCoordinate(coordinate string) (name, version string) {
	if idx := strings.LastIndex(coordinate, "@"); idx > 0 {
		return coordinate[:idx], coordinate[idx+1:]
	}
	return coordinate, ""
}

func printOutPut(filteredSlice []string) error {
	fmt.Println("Impacted package/Version")
	for _, pkg := range filteredSlice {
		fmt.Printf("%s \n", pkg)
	}
	return nil
}
