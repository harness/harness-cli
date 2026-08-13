package command

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	ar_v3 "github.com/harness/harness-cli/internal/api/ar_v3"
	client2 "github.com/harness/harness-cli/util/client"
	"github.com/harness/harness-cli/util/common/printer"
	"github.com/harness/harness-cli/util/common/progress"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func NewFirewallExplainCmd(f *cmdutils.Factory) *cobra.Command {
	var registryName string
	var packageName string
	var version string
	var orgID string
	var projectID string
	var useAsync bool

	cmd := &cobra.Command{
		Use:   "explain",
		Short: "Explain firewall status for an artifact version",
		Long:  "Get detailed firewall and scan status information for a specific artifact version",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := progress.NewConsoleReporter()

			if packageName == "" {
				log.Error().Msg("--package flag is required")
				return fmt.Errorf("--package flag is required")
			}
			if version == "" {
				log.Error().Msg("--version flag is required")
				return fmt.Errorf("--version flag is required")
			}
			if registryName == "" {
				log.Error().Msg("--registry flag is required")
				return fmt.Errorf("--registry flag is required")
			}

			org := orgID
			if org == "" {
				org = config.Global.OrgID
			}
			project := projectID
			if project == "" {
				project = config.Global.ProjectID
			}

			registryUUID, err := lookupRegistryUUID(f, registryName, org, project, p)
			if err != nil {
				return err
			}

			if useAsync {
				return explainAsync(f, registryUUID, registryName, packageName, version, org, project, p)
			}
			return explainSync(f, registryUUID, registryName, packageName, version, org, project, p)
		},
	}

	cmd.Flags().StringVar(&registryName, "registry", "", "Registry name (required)")
	cmd.Flags().StringVar(&packageName, "package", "", "Package name (required)")
	cmd.Flags().StringVar(&version, "version", "", "Package version (required)")
	cmd.Flags().StringVar(&orgID, "org", "", "Organization identifier (defaults to global config)")
	cmd.Flags().StringVar(&projectID, "project", "", "Project identifier (defaults to global config)")
	cmd.Flags().BoolVar(&useAsync, "async", false, "Use the legacy async bulk-evaluate + poll flow instead of the default sync API")
	cmd.MarkFlagRequired("registry")
	cmd.MarkFlagRequired("package")
	cmd.MarkFlagRequired("version")

	return cmd
}

func lookupRegistryUUID(f *cmdutils.Factory, registryName, org, project string, p *progress.ConsoleReporter) (uuid.UUID, error) {
	p.Start(fmt.Sprintf("Fetching registry details for: %s", registryName))
	log.Info().Str("registry", registryName).Msg("Fetching registry details")

	registryRef := client2.GetRef(config.Global.AccountID, org, project) + "/" + registryName
	registryResp, err := f.RegistryHttpClient().GetRegistryWithResponse(context.Background(), registryRef)
	if err != nil {
		p.Error("Failed to fetch registry details")
		log.Error().Err(err).Msg("Failed to fetch registry details")
		return uuid.Nil, fmt.Errorf("failed to fetch registry details: %w", err)
	}
	if registryResp.StatusCode() != http.StatusOK {
		errMsg := fmt.Sprintf("Registry '%s' not found", registryName)
		if registryResp.JSON404 != nil && registryResp.JSON404.Message != "" {
			errMsg = registryResp.JSON404.Message
		}
		p.Error(errMsg)
		log.Error().Int("statusCode", registryResp.StatusCode()).Msg(errMsg)
		return uuid.Nil, fmt.Errorf("%s", errMsg)
	}
	if registryResp.JSON200 == nil || registryResp.JSON200.Data.Uuid == nil {
		p.Error("Registry UUID not found in response")
		return uuid.Nil, fmt.Errorf("registry UUID not found in response")
	}
	registryUUID, err := uuid.Parse(*registryResp.JSON200.Data.Uuid)
	if err != nil {
		p.Error("Invalid registry UUID format in response")
		return uuid.Nil, fmt.Errorf("invalid registry UUID format: %w", err)
	}
	p.Success(fmt.Sprintf("Found registry UUID: %s", registryUUID.String()))
	return registryUUID, nil
}

// explainSync uses the new synchronous bulk-evaluate API. One HTTP call
// returns the full policy failure detail body, so no follow-up GetArtifactScanDetails.
func explainSync(f *cmdutils.Factory, registryUUID uuid.UUID, registryName, packageName, version, org, project string, p *progress.ConsoleReporter) error {
	p.Step(fmt.Sprintf("Evaluating %s@%s", packageName, version))
	log.Info().Str("package", packageName).Str("version", version).Msg("Sync evaluation")

	skip := true
	params := &ar_v3.BulkScanEvaluateSyncParams{
		AccountIdentifier: config.Global.AccountID,
		OrgIdentifier:     &org,
		ProjectIdentifier: &project,
	}
	resp, err := f.RegistryV3HttpClient().BulkScanEvaluateSyncWithResponse(
		context.Background(),
		params,
		ar_v3.BulkScanEvaluateSyncJSONRequestBody{
			RegistryId: registryUUID,
			Artifacts: []ar_v3.ArtifactScanInput{
				{PackageName: packageName, Version: version},
			},
			SkipCache: &skip,
		},
	)
	if err != nil {
		p.Error("Failed to evaluate artifact")
		return fmt.Errorf("sync evaluate: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		errMsg := fmt.Sprintf("sync evaluate: status %d", resp.StatusCode())
		if resp.JSONDefault != nil && resp.JSONDefault.Error.Message != nil {
			errMsg = *resp.JSONDefault.Error.Message
		}
		p.Error(errMsg)
		return fmt.Errorf("%s", errMsg)
	}
	if resp.JSON200 == nil || resp.JSON200.Data == nil || len(*resp.JSON200.Data) == 0 {
		p.Success("No scan results returned")
		return nil
	}
	item := (*resp.JSON200.Data)[0]
	p.Success("Evaluation completed")

	scanStatus := string(item.ScanStatus)
	scanID := ""
	if item.Id != nil {
		scanID = item.Id.String()
	}

	printScanResultHeader(p, packageName, version, scanStatus, scanID)

	if config.Global.Format == "json" {
		return printExplainJSON(registryName, packageName, version, scanStatus, scanID)
	}

	// Reuse displayScanDetails via a shape adapter.
	adapted := v3DetailsToScanDetails(&item)
	if err := displayScanDetails(&adapted); err != nil {
		p.Error("Failed to display scan details")
		return err
	}
	return nil
}

// explainAsync preserves the pre-existing initiate + poll + GetArtifactScanDetails flow.
func explainAsync(f *cmdutils.Factory, registryUUID uuid.UUID, registryName, packageName, version, org, project string, p *progress.ConsoleReporter) error {
	p.Step(fmt.Sprintf("Initiating evaluation for %s@%s", packageName, version))

	artifacts := []ar_v3.ArtifactScanInput{
		{PackageName: packageName, Version: version},
	}
	initParams := &ar_v3.InitiateBulkScanEvaluationParams{
		AccountIdentifier: config.Global.AccountID,
		OrgIdentifier:     &org,
		ProjectIdentifier: &project,
	}
	initResp, err := f.RegistryV3HttpClient().InitiateBulkScanEvaluationWithResponse(
		context.Background(), initParams,
		ar_v3.InitiateBulkScanEvaluationJSONRequestBody{
			RegistryId: registryUUID,
			Artifacts:  artifacts,
		},
	)
	if err != nil {
		p.Error("Failed to initiate evaluation")
		return fmt.Errorf("initiate: %w", err)
	}
	if initResp.StatusCode() != http.StatusAccepted {
		errMsg := "Failed to initiate evaluation"
		if initResp.JSONDefault != nil && initResp.JSONDefault.Error.Message != nil {
			errMsg = *initResp.JSONDefault.Error.Message
		}
		p.Error(errMsg)
		return fmt.Errorf("%s", errMsg)
	}
	if initResp.JSON202 == nil || initResp.JSON202.Data == nil || initResp.JSON202.Data.EvaluationId == nil {
		p.Error("Invalid response from evaluation API")
		return fmt.Errorf("invalid response from evaluation API")
	}
	evaluationID := *initResp.JSON202.Data.EvaluationId
	p.Success(fmt.Sprintf("Evaluation initiated with ID: %s", evaluationID))

	p.Step("Waiting for evaluation to complete")
	statusParams := &ar_v3.GetBulkScanEvaluationStatusParams{
		AccountIdentifier: config.Global.AccountID,
		OrgIdentifier:     &org,
		ProjectIdentifier: &project,
	}
	var statusResp *ar_v3.GetBulkScanEvaluationStatusResp
	for i := 0; i < asyncMaxPolls; i++ {
		statusResp, err = f.RegistryV3HttpClient().GetBulkScanEvaluationStatusWithResponse(
			context.Background(), evaluationID, statusParams,
		)
		if err != nil {
			p.Error("Failed to get evaluation status")
			return fmt.Errorf("poll: %w", err)
		}
		if statusResp.StatusCode() != http.StatusOK {
			errMsg := "Failed to get evaluation status"
			if statusResp.JSONDefault != nil && statusResp.JSONDefault.Error.Message != nil {
				errMsg = *statusResp.JSONDefault.Error.Message
			}
			p.Error(errMsg)
			return fmt.Errorf("%s", errMsg)
		}
		if statusResp.JSON200 == nil || statusResp.JSON200.Data == nil || statusResp.JSON200.Data.Status == nil {
			return fmt.Errorf("invalid response from evaluation status API")
		}
		s := *statusResp.JSON200.Data.Status
		if s == ar_v3.SUCCESS {
			p.Success("Evaluation completed successfully")
			break
		}
		if s == ar_v3.FAILURE {
			errMsg := "Evaluation failed"
			if statusResp.JSON200.Data.Error != nil {
				errMsg = *statusResp.JSON200.Data.Error
			}
			p.Error(errMsg)
			return fmt.Errorf("%s", errMsg)
		}
		time.Sleep(pollInterval)
		if i == asyncMaxPolls-1 {
			p.Error("Timeout waiting for evaluation to complete")
			return fmt.Errorf("timeout waiting for evaluation to complete")
		}
	}

	if statusResp.JSON200.Data.Scans == nil || len(*statusResp.JSON200.Data.Scans) == 0 {
		p.Success("No scan results returned")
		return nil
	}
	scan := (*statusResp.JSON200.Data.Scans)[0]
	scanStatus := ""
	if scan.ScanStatus != nil {
		scanStatus = string(*scan.ScanStatus)
	}
	scanID := ""
	if scan.ScanId != nil {
		scanID = scan.ScanId.String()
	}

	printScanResultHeader(p, packageName, version, scanStatus, scanID)

	if config.Global.Format == "json" {
		return printExplainJSON(registryName, packageName, version, scanStatus, scanID)
	}

	if scanID != "" {
		fmt.Println()
		p.Step("Fetching detailed scan information")
		scanParams := &ar_v3.GetArtifactScanDetailsParams{
			AccountIdentifier: config.Global.AccountID,
		}
		scanResponse, err := f.RegistryV3HttpClient().GetArtifactScanDetailsWithResponse(
			context.Background(), scanID, scanParams,
		)
		if err != nil {
			p.Error("Failed to get scan details")
			return err
		}
		switch scanResponse.StatusCode() {
		case http.StatusOK:
			if scanResponse.JSON200 != nil && scanResponse.JSON200.Data != nil {
				if err := displayScanDetails(scanResponse.JSON200.Data); err != nil {
					p.Error("Failed to display scan details")
					return err
				}
			}
		case http.StatusNotFound:
			p.Error("Scan details not found")
			fmt.Printf("   Evaluation ID '%s' not found\n", scanID)
		case http.StatusBadRequest:
			p.Error("Bad request for scan details")
		case http.StatusUnauthorized:
			p.Error("Authentication failed for scan details")
		case http.StatusForbidden:
			p.Error("Access denied for scan details")
		case http.StatusInternalServerError:
			p.Error("Server error while fetching scan details")
		default:
			p.Error("Unexpected response from scan details API")
			fmt.Printf("   Unexpected response (status: %d)\n", scanResponse.StatusCode())
		}
	}
	return nil
}

// printScanResultHeader renders the block that fw explain has emitted since
// day one. Both sync and async paths use it so their text output is identical.
func printScanResultHeader(p *progress.ConsoleReporter, packageName, version, scanStatus, scanID string) {
	fmt.Println()
	p.Step("Scan Result")
	fmt.Printf("   Package:     %s\n", packageName)
	fmt.Printf("   Version:     %s\n", version)
	fmt.Printf("   Evaluation Status: %s\n", getDisplayValue(scanStatus))
	fmt.Printf("   Evaluation ID:     %s\n", getDisplayValue(scanID))

	switch scanStatus {
	case "BLOCKED":
		fmt.Println()
		p.Error("This artifact version is BLOCKED by the firewall")
	case "WARN":
		fmt.Println()
		p.Step("This artifact version has WARNINGS from the firewall")
	case "ALLOWED":
		fmt.Println()
		p.Success("This artifact version is ALLOWED by the firewall")
	}
}

func printExplainJSON(registryName, packageName, version, scanStatus, scanID string) error {
	result := map[string]interface{}{
		"registry":   registryName,
		"package":    packageName,
		"version":    version,
		"scanStatus": scanStatus,
		"scanId":     scanID,
	}
	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(jsonBytes))
	return nil
}

// v3DetailsToScanDetails adapts the sync response body into the shape
// displayScanDetails already knows how to render. Only the fields the
// renderer actually reads (LastEvaluatedAt, PolicySetFailureDetails,
// FixVersionDetails) are populated; other fields on ArtifactScanDetails
// are left zero-valued because they are never printed.
func v3DetailsToScanDetails(v3 *ar_v3.ArtifactScanDetailsV3) ar_v3.ArtifactScanDetails {
	adapted := ar_v3.ArtifactScanDetails{
		PackageName:             v3.PackageName,
		Version:                 v3.Version,
		LastEvaluatedAt:         v3.LastEvaluatedAt,
		PolicySetFailureDetails: v3.PolicySetFailureDetails,
		FixVersionDetails:       v3.FixVersionDetails,
	}
	if v3.PackageType != nil {
		adapted.PackageType = *v3.PackageType
	}
	if v3.RegistryName != nil {
		adapted.RegistryName = *v3.RegistryName
	}
	return adapted
}

func getDisplayValue(val string) string {
	if val == "" {
		return "(not set)"
	}
	return val
}

func formatTimestamp(timestampStr string) string {
	if timestampStr == "" {
		return "(not set)"
	}
	timestampMs, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return timestampStr
	}
	t := time.UnixMilli(timestampMs)
	return t.Format("2006-01-02 15:04:05 MST")
}

func displayScanDetails(scanDetails *ar_v3.ArtifactScanDetails) error {
	fmt.Println()
	fmt.Println("Evaluation Details:")
	fmt.Println(strings.Repeat("=", 60))

	if scanDetails.LastEvaluatedAt != nil && *scanDetails.LastEvaluatedAt != "" {
		fmt.Printf("Last Evaluated: %s\n", formatTimestamp(*scanDetails.LastEvaluatedAt))
	}

	hasSecurityViolation := false
	if scanDetails.PolicySetFailureDetails != nil {
		for _, policySetFailure := range *scanDetails.PolicySetFailureDetails {
			for _, failure := range policySetFailure.PolicyFailureDetails {
				if failure.Category == "Security" {
					hasSecurityViolation = true
					break
				}
			}
			if hasSecurityViolation {
				break
			}
		}
	}

	if hasSecurityViolation && scanDetails.FixVersionDetails != nil {
		fmt.Println()
		fmt.Println("Security Fix Information:")
		fmt.Printf("  Fix Available: %v\n", scanDetails.FixVersionDetails.FixVersionAvailable)
		fmt.Printf("  Current Version: %s\n", scanDetails.FixVersionDetails.CurrentVersion)
		if scanDetails.FixVersionDetails.FixVersion != nil {
			fmt.Printf("  Fix Version: %s\n", *scanDetails.FixVersionDetails.FixVersion)
		}
	}

	if scanDetails.PolicySetFailureDetails != nil && len(*scanDetails.PolicySetFailureDetails) > 0 {
		fmt.Println()
		fmt.Println("Policy Set Violations:")

		for psIdx, policySetFailure := range *scanDetails.PolicySetFailureDetails {
			fmt.Println()
			fmt.Printf("Policy Set %d: %s\n", psIdx+1, policySetFailure.PolicySetName)
			fmt.Printf("Policy Set Ref: %s\n", policySetFailure.PolicySetRef)
			fmt.Println(strings.Repeat("-", 60))

			if len(policySetFailure.PolicyFailureDetails) > 0 {
				for i, failure := range policySetFailure.PolicyFailureDetails {
					fmt.Printf("\n  %d.%d %s\n", psIdx+1, i+1, string(failure.Category))
					fmt.Printf("      Policy Name: %s\n", failure.PolicyName)
					fmt.Printf("      Policy Ref:  %s\n", failure.PolicyRef)

					switch failure.Category {
					case "Security":
						securityConfig, err := failure.AsSecurityPolicyFailureDetailConfig()
						if err == nil && len(securityConfig.Vulnerabilities) > 0 {
							fmt.Println("\n      Vulnerabilities:")
							var vulnData []map[string]interface{}
							for _, vuln := range securityConfig.Vulnerabilities {
								vulnData = append(vulnData, map[string]interface{}{
									"cveId":         vuln.CveId,
									"cvssScore":     fmt.Sprintf("%.1f", vuln.CvssScore),
									"cvssThreshold": fmt.Sprintf("%.1f", vuln.CvssThreshold),
								})
							}
							err := printer.Print(vulnData, 0, 1, int64(len(vulnData)), false, [][]string{
								{"cveId", "CVE ID"},
								{"cvssScore", "CVSS Score"},
								{"cvssThreshold", "CVSS Threshold"},
							})
							if err != nil {
								return err
							}
						}

					case "License":
						licenseConfig, err := failure.AsLicensePolicyFailureDetailConfig()
						if err == nil {
							fmt.Printf("\n      Blocked License: %s\n", licenseConfig.BlockedLicense)
							if len(licenseConfig.AllowedLicenses) > 0 {
								fmt.Printf("      Allowed Licenses: %s\n", strings.Join(licenseConfig.AllowedLicenses, ", "))
							}
						}

					case "PackageAge":
						packageAgeConfig, err := failure.AsPackageAgeViolationPolicyFailureDetailConfig()
						if err == nil {
							fmt.Printf("\n      Published On: %s\n", formatTimestamp(packageAgeConfig.PublishedOn))
							fmt.Printf("      Package Age Threshold: %s\n", packageAgeConfig.PackageAgeThreshold)
						}
					}
				}
			}
		}
	}

	return nil
}
