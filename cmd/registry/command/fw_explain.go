package command

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	ar_v3 "github.com/harness/harness-cli/internal/api/ar_v3"
	client2 "github.com/harness/harness-cli/util/client"
	"github.com/harness/harness-cli/util/common/printer"
	"github.com/harness/harness-cli/util/common/progress"
	"github.com/rs/zerolog/log"

	"github.com/spf13/cobra"
)

func NewFirewallExplainCmd(f *cmdutils.Factory) *cobra.Command {
	var registryName string
	var packageName string
	var version string
	var orgID string
	var projectID string

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

			p.Start(fmt.Sprintf("Fetching registry details for: %s", registryName))
			log.Info().Str("registry", registryName).Msg("Fetching registry details")

			registryRef := client2.GetRef(config.Global.AccountID, org, project) + "/" + registryName
			registryResp, err := f.RegistryHttpClient().GetRegistryWithResponse(context.Background(), registryRef)
			if err != nil {
				p.Error("Failed to fetch registry details")
				log.Error().Err(err).Msg("Failed to fetch registry details")
				return fmt.Errorf("failed to fetch registry details: %w", err)
			}

			if registryResp.StatusCode() != 200 {
				errMsg := fmt.Sprintf("Registry '%s' not found", registryName)
				if registryResp.JSON404 != nil && registryResp.JSON404.Message != "" {
					errMsg = registryResp.JSON404.Message
				}
				p.Error(errMsg)
				log.Error().Int("statusCode", registryResp.StatusCode()).Msg(errMsg)
				return fmt.Errorf(errMsg)
			}

			if registryResp.JSON200 == nil || registryResp.JSON200.Data.Uuid == nil {
				p.Error("Registry UUID not found in response")
				log.Error().Msg("Registry UUID not found in response")
				return fmt.Errorf("registry UUID not found in response")
			}

			registryUUID, err := uuid.Parse(*registryResp.JSON200.Data.Uuid)
			if err != nil {
				p.Error("Invalid registry UUID format in response")
				log.Error().Err(err).Str("uuid", *registryResp.JSON200.Data.Uuid).Msg("Invalid registry UUID format")
				return fmt.Errorf("invalid registry UUID format: %w", err)
			}

			p.Success(fmt.Sprintf("Found registry UUID: %s", registryUUID.String()))
			log.Info().Str("registryUUID", registryUUID.String()).Msg("Registry UUID retrieved")

			p.Step(fmt.Sprintf("Initiating evaluation for %s@%s", packageName, version))
			log.Info().Str("package", packageName).Str("version", version).Msg("Initiating evaluation")

			artifacts := []ar_v3.ArtifactScanInput{
				{
					PackageName: packageName,
					Version:     version,
				},
			}

			initParams := &ar_v3.InitiateBulkScanEvaluationParams{
				AccountIdentifier: config.Global.AccountID,
				OrgIdentifier:     &org,
				ProjectIdentifier: &project,
			}

			initResp, err := f.RegistryV3HttpClient().InitiateBulkScanEvaluationWithResponse(
				context.Background(),
				initParams,
				ar_v3.InitiateBulkScanEvaluationJSONRequestBody{
					RegistryId: registryUUID,
					Artifacts:  artifacts,
				},
			)
			if err != nil {
				p.Error("Failed to initiate evaluation")
				log.Error().Err(err).Msg("Failed to initiate evaluation")
				return fmt.Errorf("failed to initiate evaluation: %w", err)
			}

			if initResp.StatusCode() != 202 {
				errMsg := "Failed to initiate evaluation"
				if initResp.JSONDefault != nil && initResp.JSONDefault.Error.Message != nil {
					errMsg = *initResp.JSONDefault.Error.Message
				}
				p.Error(errMsg)
				log.Error().Int("statusCode", initResp.StatusCode()).Msg(errMsg)
				return fmt.Errorf(errMsg)
			}

			if initResp.JSON202 == nil || initResp.JSON202.Data == nil || initResp.JSON202.Data.EvaluationId == nil {
				p.Error("Invalid response from evaluation API")
				log.Error().Msg("Invalid response from evaluation API")
				return fmt.Errorf("invalid response from evaluation API")
			}

			evaluationID := *initResp.JSON202.Data.EvaluationId
			p.Success(fmt.Sprintf("Evaluation initiated with ID: %s", evaluationID))
			log.Info().Str("evaluationId", evaluationID).Msg("Evaluation initiated")

			p.Step("Waiting for evaluation to complete")
			log.Info().Msg("Polling evaluation status")

			statusParams := &ar_v3.GetBulkScanEvaluationStatusParams{
				AccountIdentifier: config.Global.AccountID,
				OrgIdentifier:     &org,
				ProjectIdentifier: &project,
			}

			var statusResp *ar_v3.GetBulkScanEvaluationStatusResp
			var status ar_v3.BulkScanEvaluationStatusDataStatus
			pollCount := 0
			maxPolls := 120

			for {
				pollCount++
				if pollCount > maxPolls {
					p.Error("Timeout waiting for evaluation to complete")
					log.Error().Int("maxPolls", maxPolls).Msg("Timeout waiting for evaluation")
					return fmt.Errorf("timeout waiting for evaluation to complete")
				}

				statusResp, err = f.RegistryV3HttpClient().GetBulkScanEvaluationStatusWithResponse(
					context.Background(),
					evaluationID,
					statusParams,
				)
				if err != nil {
					p.Error("Failed to get evaluation status")
					log.Error().Err(err).Msg("Failed to get evaluation status")
					return fmt.Errorf("failed to get evaluation status: %w", err)
				}

				if statusResp.StatusCode() != 200 {
					errMsg := "Failed to get evaluation status"
					if statusResp.JSONDefault != nil && statusResp.JSONDefault.Error.Message != nil {
						errMsg = *statusResp.JSONDefault.Error.Message
					}
					p.Error(errMsg)
					log.Error().Int("statusCode", statusResp.StatusCode()).Msg(errMsg)
					return fmt.Errorf(errMsg)
				}

				if statusResp.JSON200 == nil || statusResp.JSON200.Data == nil || statusResp.JSON200.Data.Status == nil {
					p.Error("Invalid response from evaluation status API")
					log.Error().Msg("Invalid response from evaluation status API")
					return fmt.Errorf("invalid response from evaluation status API")
				}

				status = *statusResp.JSON200.Data.Status
				log.Debug().Str("status", string(status)).Int("poll", pollCount).Msg("Evaluation status")

				if status == ar_v3.BulkScanEvaluationStatusDataStatusSUCCESS {
					p.Success("Evaluation completed successfully")
					log.Info().Msg("Evaluation completed successfully")
					break
				}

				if status == ar_v3.BulkScanEvaluationStatusDataStatusFAILURE {
					errMsg := "Evaluation failed"
					if statusResp.JSON200.Data.Error != nil {
						errMsg = *statusResp.JSON200.Data.Error
					}
					p.Error(errMsg)
					log.Error().Str("error", errMsg).Msg("Evaluation failed")
					return fmt.Errorf(errMsg)
				}

				time.Sleep(2 * time.Second)
			}

			if statusResp.JSON200.Data.Scans == nil || len(*statusResp.JSON200.Data.Scans) == 0 {
				p.Success("No scan results returned")
				log.Info().Msg("No scan results returned")
				return nil
			}

			scans := *statusResp.JSON200.Data.Scans
			scan := scans[0]

			scanStatus := ""
			if scan.ScanStatus != nil {
				scanStatus = string(*scan.ScanStatus)
			}

			scanId := ""
			if scan.ScanId != nil {
				scanId = scan.ScanId.String()
			}

			fmt.Println()
			p.Step("Scan Result")
			fmt.Printf("   Package:     %s\n", packageName)
			fmt.Printf("   Version:     %s\n", version)
			fmt.Printf("   Evaluation Status: %s\n", getDisplayValue(scanStatus))
			fmt.Printf("   Evaluation ID:     %s\n", getDisplayValue(scanId))

			if scanStatus == "BLOCKED" {
				fmt.Println()
				p.Error("This artifact version is BLOCKED by the firewall")
			} else if scanStatus == "WARN" {
				fmt.Println()
				p.Step("This artifact version has WARNINGS from the firewall")
			} else if scanStatus == "ALLOWED" {
				fmt.Println()
				p.Success("This artifact version is ALLOWED by the firewall")
			}

			if config.Global.Format == "json" {
				result := map[string]interface{}{
					"registry":   registryName,
					"package":    packageName,
					"version":    version,
					"scanStatus": scanStatus,
					"scanId":     scanId,
				}
				jsonBytes, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					p.Error("Failed to marshal JSON output")
					log.Error().Err(err).Msg("Failed to marshal JSON output")
					return err
				}
				fmt.Println(string(jsonBytes))
				return nil
			}

			if scanId != "" {
				fmt.Println()
				p.Step("Fetching detailed scan information")
				log.Info().Str("scanId", scanId).Msg("Fetching scan details")

				scanParams := &ar_v3.GetArtifactScanDetailsParams{
					AccountIdentifier: config.Global.AccountID,
				}

				scanResponse, err := f.RegistryV3HttpClient().GetArtifactScanDetailsWithResponse(
					context.Background(),
					scanId,
					scanParams,
				)
				if err != nil {
					p.Error("Failed to get scan details")
					log.Error().Err(err).Msg("Failed to get scan details")
					return err
				}

				switch scanResponse.StatusCode() {
				case 404:
					p.Error("Scan details not found")
					log.Error().Str("scanId", scanId).Msg("Scan details not found")
					fmt.Printf("   Evaluation ID '%s' not found\n", scanId)
				case 400:
					p.Error("Bad request for scan details")
					log.Error().Msg("Bad request for scan details")
					fmt.Printf("   Invalid scan request parameters\n")
				case 401:
					p.Error("Authentication failed for scan details")
					log.Error().Msg("Authentication failed for scan details")
					fmt.Printf("   Authentication failed - check your token\n")
				case 403:
					p.Error("Access denied for scan details")
					log.Error().Msg("Access denied for scan details")
					fmt.Printf("   Access denied - insufficient permissions\n")
				case 500:
					p.Error("Server error while fetching scan details")
					log.Error().Msg("Server error while fetching scan details")
					fmt.Printf("   Server error occurred\n")
				case 200:
					if scanResponse.JSON200 != nil && scanResponse.JSON200.Data != nil {
						log.Info().Msg("Scan details retrieved successfully")
						err = displayScanDetails(scanResponse.JSON200.Data)
						if err != nil {
							p.Error("Failed to display scan details")
							log.Error().Err(err).Msg("Failed to display scan details")
							return err
						}
					}
				default:
					p.Error("Unexpected response from scan details API")
					log.Error().Int("statusCode", scanResponse.StatusCode()).Msg("Unexpected response from scan details API")
					fmt.Printf("   Unexpected response (status: %d)\n", scanResponse.StatusCode())
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&registryName, "registry", "", "Registry name (required)")
	cmd.Flags().StringVar(&packageName, "package", "", "Package name (required)")
	cmd.Flags().StringVar(&version, "version", "", "Package version (required)")
	cmd.Flags().StringVar(&orgID, "org", "", "Organization identifier (defaults to global config)")
	cmd.Flags().StringVar(&projectID, "project", "", "Project identifier (defaults to global config)")
	cmd.MarkFlagRequired("registry")
	cmd.MarkFlagRequired("package")
	cmd.MarkFlagRequired("version")

	return cmd
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

	// Try to parse as milliseconds timestamp
	timestampMs, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return timestampStr
	}

	// Convert milliseconds to time
	t := time.UnixMilli(timestampMs)

	// Format as readable date and time
	return t.Format("2006-01-02 15:04:05 MST")
}

func displayScanDetails(scanDetails *ar_v3.ArtifactScanDetails) error {
	fmt.Println()
	fmt.Println("Evaluation Details:")
	fmt.Println(strings.Repeat("=", 60))

	if scanDetails.PolicySetName != nil && *scanDetails.PolicySetName != "" {
		fmt.Printf("Policy Set: %s\n", *scanDetails.PolicySetName)
	}
	if scanDetails.PolicySetRef != nil && *scanDetails.PolicySetRef != "" {
		fmt.Printf("Policy Set Ref: %s\n", *scanDetails.PolicySetRef)
	}
	if scanDetails.LastEvaluatedAt != nil && *scanDetails.LastEvaluatedAt != "" {
		fmt.Printf("Last Evaluated: %s\n", formatTimestamp(*scanDetails.LastEvaluatedAt))
	}

	hasSecurityViolation := false
	for _, failure := range scanDetails.PolicyFailureDetails {
		if failure.Category == "Security" {
			hasSecurityViolation = true
			break
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

	if len(scanDetails.PolicyFailureDetails) > 0 {
		fmt.Println()
		fmt.Println("Policy Violations:")
		fmt.Println()

		for i, failure := range scanDetails.PolicyFailureDetails {
			fmt.Printf("\n%d. %s\n", i+1, string(failure.Category))
			fmt.Println(strings.Repeat("-", 60))
			fmt.Printf("   Policy Name: %s\n", failure.PolicyName)
			fmt.Printf("   Policy Ref:  %s\n", failure.PolicyRef)

			switch failure.Category {
			case "Security":
				securityConfig, err := failure.AsSecurityPolicyFailureDetailConfig()
				if err == nil && len(securityConfig.Vulnerabilities) > 0 {
					fmt.Println("\n   Vulnerabilities:")
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
					fmt.Printf("\n   Blocked License: %s\n", licenseConfig.BlockedLicense)
					if len(licenseConfig.AllowedLicenses) > 0 {
						fmt.Printf("   Allowed Licenses: %s\n", strings.Join(licenseConfig.AllowedLicenses, ", "))
					}
				}

			case "PackageAge":
				packageAgeConfig, err := failure.AsPackageAgeViolationPolicyFailureDetailConfig()
				if err == nil {
					fmt.Printf("\n   Published On: %s\n", formatTimestamp(packageAgeConfig.PublishedOn))
					fmt.Printf("   Package Age Threshold: %s\n", packageAgeConfig.PackageAgeThreshold)
				}
			}
		}
	}

	return nil
}
