package command

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	ar "github.com/harness/harness-cli/internal/api/ar"
	ar_v3 "github.com/harness/harness-cli/internal/api/ar_v3"
	client2 "github.com/harness/harness-cli/util/client"
	"github.com/harness/harness-cli/util/common/printer"
	"github.com/harness/harness-cli/util/common/progress"

	"github.com/spf13/cobra"
)

func NewFirewallExplainCmd(f *cmdutils.Factory) *cobra.Command {
	var registryName string
	var orgID string
	var projectID string

	cmd := &cobra.Command{
		Use:   "explain <name>@<version>",
		Short: "Explain firewall status for an artifact version",
		Long:  "Get detailed firewall and scan status information for a specific artifact version",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := progress.NewConsoleReporter()

			artifactWithVersion := args[0]

			// Use local flags if provided, otherwise use global config
			org := orgID
			if org == "" {
				org = config.Global.OrgID
			}
			project := projectID
			if project == "" {
				project = config.Global.ProjectID
			}

			// Handle scoped packages (e.g., @scope/package@version)
			var artifactName, version string
			lastAtIndex := strings.LastIndex(artifactWithVersion, "@")

			if lastAtIndex == -1 {
				return fmt.Errorf("invalid format: expected <name>@<version>, got %s", artifactWithVersion)
			}

			// If the string starts with @, we need at least 2 @ symbols
			if strings.HasPrefix(artifactWithVersion, "@") {
				if lastAtIndex == 0 {
					return fmt.Errorf("invalid format: expected <name>@<version>, got %s", artifactWithVersion)
				}
				artifactName = artifactWithVersion[:lastAtIndex]
				version = artifactWithVersion[lastAtIndex+1:]
			} else {
				artifactName = artifactWithVersion[:lastAtIndex]
				version = artifactWithVersion[lastAtIndex+1:]
			}

			if artifactName == "" || version == "" {
				return fmt.Errorf("invalid format: expected <name>@<version>, got %s", artifactWithVersion)
			}

			p.Start(fmt.Sprintf("Fetching firewall status for %s@%s", artifactName, version))

			params := &ar.GetArtifactVersionSummaryParams{}
			response, err := f.RegistryHttpClient().GetArtifactVersionSummaryWithResponse(
				context.Background(),
				client2.GetRef(config.Global.AccountID, org, project)+"/"+registryName,
				artifactName,
				version,
				params,
			)
			if err != nil {
				p.Error("Failed to get artifact version summary")
				return err
			}

			// Handle different response status codes
			switch response.StatusCode() {
			case 404:
				p.Error("Artifact version not found")
				return fmt.Errorf("artifact version '%s@%s' not found in registry '%s'", artifactName, version, registryName)
			case 400:
				p.Error("Bad request")
				return fmt.Errorf("invalid request parameters")
			case 401:
				p.Error("Authentication failed")
				return fmt.Errorf("authentication failed - check your token")
			case 403:
				p.Error("Access denied")
				return fmt.Errorf("access denied - insufficient permissions")
			case 500:
				p.Error("Server error")
				return fmt.Errorf("server error occurred")
			}

			if response.JSON200 == nil {
				p.Error("Unexpected response from API")
				return fmt.Errorf("unexpected response from API (status: %d)", response.StatusCode())
			}

			data := response.JSON200.Data

			firewallMode := ""
			if data.FirewallMode != nil {
				firewallMode = string(*data.FirewallMode)
			}

			scanStatus := ""
			if data.ScanStatus != nil {
				scanStatus = string(*data.ScanStatus)
			}

			scanId := ""
			if data.ScanId != nil {
				scanId = *data.ScanId
			}

			if config.Global.Format == "json" {
				result := map[string]interface{}{
					"registry":        registryName,
					"artifact":        artifactName,
					"version":         version,
					"firewallMode":    firewallMode,
					"scanStatus":      scanStatus,
					"scanId":          scanId,
					"firewallEnabled": firewallMode != "" && firewallMode != "ALLOW",
				}
				jsonBytes, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					p.Error("Failed to marshal JSON output")
					return err
				}
				fmt.Println(string(jsonBytes))
				return nil
			}

			p.Success(fmt.Sprintf("Retrieved firewall status for %s@%s in registry %s", artifactName, version, registryName))
			fmt.Println()

			// Check if firewall is enabled
			if firewallMode == "" || firewallMode == "ALLOW" {
				p.Step("Firewall is not enabled for the provided registry")
				fmt.Printf("   Firewall Mode: %s\n", getDisplayValue(firewallMode))
				return nil
			}

			// Firewall is enabled (WARN or BLOCK)
			p.Step("Firewall is enabled")
			fmt.Printf("   Firewall Mode: %s\n", firewallMode)
			fmt.Printf("   Scan Status:   %s\n", getDisplayValue(scanStatus))
			fmt.Printf("   Scan ID:       %s\n", getDisplayValue(scanId))

			// Check if artifact has been scanned
			if scanStatus == "" || scanId == "" {
				fmt.Println()
				p.Step("Artifact has not been scanned yet")
				return nil
			}

			// Show scan status result
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

			if scanId != "" {
				fmt.Println()
				p.Step("Fetching detailed scan information")

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
					return err
				}

				// Handle different response status codes for scan details
				switch scanResponse.StatusCode() {
				case 404:
					p.Error("Scan details not found")
					fmt.Printf("   Scan ID '%s' not found\n", scanId)
				case 400:
					p.Error("Bad request for scan details")
					fmt.Printf("   Invalid scan request parameters\n")
				case 401:
					p.Error("Authentication failed for scan details")
					fmt.Printf("   Authentication failed - check your token\n")
				case 403:
					p.Error("Access denied for scan details")
					fmt.Printf("   Access denied - insufficient permissions\n")
				case 500:
					p.Error("Server error while fetching scan details")
					fmt.Printf("   Server error occurred\n")
				case 200:
					if scanResponse.JSON200 != nil && scanResponse.JSON200.Data != nil {
						err = displayScanDetails(scanResponse.JSON200.Data)
						if err != nil {
							p.Error("Failed to display scan details")
							return err
						}
					}
				default:
					p.Error("Unexpected response from scan details API")
					fmt.Printf("   Unexpected response (status: %d)\n", scanResponse.StatusCode())
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&registryName, "registry", "", "Registry identifier (required)")
	cmd.MarkFlagRequired("registry")
	cmd.Flags().StringVar(&orgID, "org", "", "Organization identifier (defaults to global config)")
	cmd.Flags().StringVar(&projectID, "project", "", "Project identifier (defaults to global config)")

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
	fmt.Println("Scan Details:")
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

	if scanDetails.FixVersionDetails != nil {
		fmt.Println()
		fmt.Println("Fix Version Information:")
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
