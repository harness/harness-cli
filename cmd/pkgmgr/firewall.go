package pkgmgr

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	ar_v3 "github.com/harness/harness-cli/internal/api/ar_v3"
	"github.com/harness/harness-cli/util/common/printer"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

const (
	maxBulkScanBatchSize = 50
	maxRetries           = 3
	retryInterval        = 30 * time.Second
)

// RunFirewallExplain evaluates artifacts against firewall policies and displays results.
// Automatically batches into chunks of 50 (API limit).
// Returns the number of scan results and any error.
func RunFirewallExplain(f *cmdutils.Factory, registryUUID uuid.UUID, artifacts []ar_v3.ArtifactScanInput, org, project string, progress p.Reporter) (int, error) {
	if len(artifacts) == 0 {
		return 0, nil
	}

	var allScans []ar_v3.BulkScanResultItem
	totalBatches := (len(artifacts) + maxBulkScanBatchSize - 1) / maxBulkScanBatchSize

	for batchIdx := 0; batchIdx < totalBatches; batchIdx++ {
		start := batchIdx * maxBulkScanBatchSize
		end := start + maxBulkScanBatchSize
		if end > len(artifacts) {
			end = len(artifacts)
		}
		batch := artifacts[start:end]

		if totalBatches > 1 {
			progress.Step(fmt.Sprintf("Evaluating batch %d/%d (%d packages)", batchIdx+1, totalBatches, len(batch)))
		}

		var scans []ar_v3.BulkScanResultItem
		var err error
		for attempt := 1; attempt <= maxRetries; attempt++ {
			scans, err = runBulkEvaluation(f, registryUUID, batch, org, project, progress)
			if err == nil {
				break
			}
			if attempt < maxRetries {
				progress.Step(fmt.Sprintf("Evaluation failed (attempt %d/%d), retrying in %ds: %s", attempt, maxRetries, int(retryInterval.Seconds()), err))
				time.Sleep(retryInterval)
			}
		}
		if err != nil {
			return 0, fmt.Errorf("evaluation failed after %d attempts: %w", maxRetries, err)
		}
		allScans = append(allScans, scans...)
	}

	progress.Success(fmt.Sprintf("Firewall evaluation completed (%d packages)", len(allScans)))
	return len(allScans), DisplayBlockedScanResults(f, allScans, progress)
}

// runBulkEvaluation initiates a single bulk scan evaluation and polls until completion.
func runBulkEvaluation(f *cmdutils.Factory, registryUUID uuid.UUID, artifacts []ar_v3.ArtifactScanInput, org, project string, progress p.Reporter) ([]ar_v3.BulkScanResultItem, error) {
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
		return nil, fmt.Errorf("failed to initiate bulk evaluation: %w", err)
	}

	if initResp.StatusCode() != 202 {
		errMsg := "failed to initiate bulk evaluation"
		if initResp.JSONDefault != nil && initResp.JSONDefault.Error.Message != nil {
			errMsg = *initResp.JSONDefault.Error.Message
		}
		return nil, fmt.Errorf("%s (status %d)", errMsg, initResp.StatusCode())
	}

	if initResp.JSON202 == nil || initResp.JSON202.Data == nil || initResp.JSON202.Data.EvaluationId == nil {
		return nil, fmt.Errorf("invalid response from bulk evaluation API")
	}

	evaluationID := *initResp.JSON202.Data.EvaluationId
	progress.Step(fmt.Sprintf("Evaluation initiated: %s", evaluationID))

	statusParams := &ar_v3.GetBulkScanEvaluationStatusParams{
		AccountIdentifier: config.Global.AccountID,
		OrgIdentifier:     &org,
		ProjectIdentifier: &project,
	}

	maxPolls := 120
	pollRetries := 0
	for i := 0; i < maxPolls; i++ {
		statusResp, err := f.RegistryV3HttpClient().GetBulkScanEvaluationStatusWithResponse(
			context.Background(),
			evaluationID,
			statusParams,
		)
		if err != nil {
			pollRetries++
			if pollRetries <= maxRetries {
				progress.Step(fmt.Sprintf("Poll failed (attempt %d/%d), retrying in %ds: %s", pollRetries, maxRetries, int(retryInterval.Seconds()), err))
				time.Sleep(retryInterval)
				continue
			}
			return nil, fmt.Errorf("failed to get evaluation status after %d retries: %w", maxRetries, err)
		}

		if statusResp.StatusCode() != 200 || statusResp.JSON200 == nil ||
			statusResp.JSON200.Data == nil || statusResp.JSON200.Data.Status == nil {
			pollRetries++
			if pollRetries <= maxRetries {
				progress.Step(fmt.Sprintf("Unexpected poll response (attempt %d/%d), retrying in %ds", pollRetries, maxRetries, int(retryInterval.Seconds())))
				time.Sleep(retryInterval)
				continue
			}
			return nil, fmt.Errorf("unexpected response from evaluation status API after %d retries", maxRetries)
		}

		// Reset retries on successful poll
		pollRetries = 0
		status := *statusResp.JSON200.Data.Status

		if status == ar_v3.SUCCESS {
			if statusResp.JSON200.Data.Scans != nil {
				return *statusResp.JSON200.Data.Scans, nil
			}
			return nil, nil
		}

		if status == ar_v3.FAILURE {
			errMsg := "bulk evaluation failed"
			if statusResp.JSON200.Data.Error != nil {
				errMsg = *statusResp.JSON200.Data.Error
			}
			return nil, fmt.Errorf("%s", errMsg)
		}

		time.Sleep(2 * time.Second)
	}

	return nil, fmt.Errorf("timeout waiting for firewall evaluation to complete")
}

// DisplayBlockedScanResults shows detailed scan info for each blocked/warned package.
func DisplayBlockedScanResults(f *cmdutils.Factory, scans []ar_v3.BulkScanResultItem, progress p.Reporter) error {
	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("FIREWALL EVALUATION: %d package(s) evaluated\n", len(scans))
	fmt.Println(strings.Repeat("=", 60))

	for _, scan := range scans {
		pkgName := ""
		if scan.PackageName != nil {
			pkgName = *scan.PackageName
		}
		version := ""
		if scan.Version != nil {
			version = *scan.Version
		}
		scanStatus := ""
		if scan.ScanStatus != nil {
			scanStatus = string(*scan.ScanStatus)
		}

		fmt.Println()
		switch scanStatus {
		case "BLOCKED":
			progress.Error(fmt.Sprintf("BLOCKED  %s@%s", pkgName, version))
		case "WARN":
			progress.Step(fmt.Sprintf("WARN     %s@%s", pkgName, version))
		case "ALLOWED":
			progress.Success(fmt.Sprintf("ALLOWED  %s@%s", pkgName, version))
			continue
		default:
			fmt.Printf("  %s  %s@%s\n", scanStatus, pkgName, version)
			continue
		}

		if scan.ScanId == nil {
			continue
		}
		scanId := scan.ScanId.String()

		scanParams := &ar_v3.GetArtifactScanDetailsParams{
			AccountIdentifier: config.Global.AccountID,
		}

		scanResponse, err := f.RegistryV3HttpClient().GetArtifactScanDetailsWithResponse(
			context.Background(),
			scanId,
			scanParams,
		)
		if err != nil {
			log.Error().Err(err).Str("scanId", scanId).Msg("Failed to get scan details")
			continue
		}

		if scanResponse.StatusCode() == 200 && scanResponse.JSON200 != nil && scanResponse.JSON200.Data != nil {
			DisplayScanDetails(scanResponse.JSON200.Data)
		}
	}

	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	return nil
}

// DisplayScanDetails shows policy violations for a single scan result.
func DisplayScanDetails(scanDetails *ar_v3.ArtifactScanDetails) {
	if scanDetails.PolicySetFailureDetails == nil || len(*scanDetails.PolicySetFailureDetails) == 0 {
		return
	}

	for _, policySetFailure := range *scanDetails.PolicySetFailureDetails {
		fmt.Printf("    Policy Set: %s\n", policySetFailure.PolicySetName)

		for _, failure := range policySetFailure.PolicyFailureDetails {
			fmt.Printf("      Category: %s | Policy: %s\n", failure.Category, failure.PolicyName)

			switch failure.Category {
			case "Security":
				securityConfig, err := failure.AsSecurityPolicyFailureDetailConfig()
				if err == nil && len(securityConfig.Vulnerabilities) > 0 {
					var vulnData []map[string]interface{}
					for _, vuln := range securityConfig.Vulnerabilities {
						vulnData = append(vulnData, map[string]interface{}{
							"cveId":         vuln.CveId,
							"cvssScore":     fmt.Sprintf("%.1f", vuln.CvssScore),
							"cvssThreshold": fmt.Sprintf("%.1f", vuln.CvssThreshold),
						})
					}
					_ = printer.Print(vulnData, 0, 1, int64(len(vulnData)), false, [][]string{
						{"cveId", "CVE ID"},
						{"cvssScore", "CVSS Score"},
						{"cvssThreshold", "CVSS Threshold"},
					})
				}
				// Show fix version details for security failures
				if scanDetails.FixVersionDetails != nil {
					fmt.Printf("      Fix Available: %v\n", scanDetails.FixVersionDetails.FixVersionAvailable)
					fmt.Printf("      Current Version: %s\n", scanDetails.FixVersionDetails.CurrentVersion)
					if scanDetails.FixVersionDetails.FixVersion != nil {
						fmt.Printf("      Fix Version: %s\n", *scanDetails.FixVersionDetails.FixVersion)
					}
				}

			case "License":
				licenseConfig, err := failure.AsLicensePolicyFailureDetailConfig()
				if err == nil {
					fmt.Printf("      Blocked License: %s\n", licenseConfig.BlockedLicense)
					if len(licenseConfig.AllowedLicenses) > 0 {
						fmt.Printf("      Allowed Licenses: %s\n", strings.Join(licenseConfig.AllowedLicenses, ", "))
					}
				}

			case "PackageAge":
				packageAgeConfig, err := failure.AsPackageAgeViolationPolicyFailureDetailConfig()
				if err == nil {
					fmt.Printf("      Published On: %s\n", packageAgeConfig.PublishedOn)
					fmt.Printf("      Age Threshold: %s\n", packageAgeConfig.PackageAgeThreshold)
				}
			}
		}
	}
}
