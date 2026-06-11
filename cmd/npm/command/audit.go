package command

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/harness/harness-cli/cmd/cmdutils"
	regcmd "github.com/harness/harness-cli/cmd/registry/command"
	"github.com/harness/harness-cli/config"
	ar_v3 "github.com/harness/harness-cli/internal/api/ar_v3"
	client2 "github.com/harness/harness-cli/util/client"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

type PackageFix struct {
	Name            string
	CurrentVersion  string
	FixVersion      string
	ScanStatus      string
	Vulnerabilities []VulnerabilityInfo
}

type VulnerabilityInfo struct {
	CveId     string
	CvssScore float64
	Severity  string
}

type SecurityFixInfo struct {
	PackageName      string
	CurrentVersion   string
	FixVersion       string
	FixAvailable     bool
	HasSecurityIssue bool
}

type auditContext = regcmd.AuditContext

func NewNpmAuditCmd(f *cmdutils.Factory) *cobra.Command {
	var fix bool
	var packageJsonPath string

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Audit npm dependencies for vulnerabilities",
		Long: `Audit npm dependencies against firewall policies and security vulnerabilities.
		
This command will:
1. Parse your package.json file
2. Evaluate all dependencies against firewall policies
3. Identify vulnerable packages with available fixes
4. Optionally update package.json with fix versions (--fix flag)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate packageJsonPath for package.json file
			if err := validatePackageJsonPath(packageJsonPath); err != nil {
				return err
			}
			return runNpmAudit(f, packageJsonPath, fix)
		},
	}

	cmd.Flags().BoolVar(&fix, "fix", false, "Automatically update package.json with fix versions for vulnerable packages")
	cmd.Flags().StringVarP(&packageJsonPath, "file", "f", "package.json", "Path to package.json file")

	return cmd
}

func validatePackageJsonPath(filePath string) error {
	// Check if path exists
	fileInfo, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return fmt.Errorf("file not found: %s", filePath)
	}
	if err != nil {
		return fmt.Errorf("failed to access file: %w", err)
	}

	// Check if it's a directory
	if fileInfo.IsDir() {
		return fmt.Errorf("path is a directory, not a file: %s", filePath)
	}

	// Check if filename is package.json
	fileName := filePath
	if idx := strings.LastIndex(filePath, "/"); idx != -1 {
		fileName = filePath[idx+1:]
	}
	if fileName != "package.json" {
		return fmt.Errorf("file must be named 'package.json', got: %s", fileName)
	}

	return nil
}

func runNpmAudit(f *cmdutils.Factory, packageJsonPath string, fix bool) error {
	progress := p.NewConsoleReporter()

	progress.Start("Starting npm audit")
	log.Info().Str("file", packageJsonPath).Bool("fix", fix).Msg("Starting npm audit")

	// Detect registry
	progress.Step("Detecting HAR registry")
	regInfo, err := detectNpmRegistry()
	if err != nil {
		progress.Error(fmt.Sprintf("Failed to detect registry: %s", err))
		return err
	}
	progress.Success(fmt.Sprintf("Found registry: %s", regInfo.RegistryIdentifier))

	//Setting values assigned during configuration
	org := regInfo.OrgID
	project := regInfo.ProjectID

	if org == "" || project == "" {
		progress.Error("Organization and Project identifiers are required")
		return fmt.Errorf("organization and project identifiers are required. Set via config or environment variables")
	}

	// resolve registry UUID
	progress.Step("Resolving registry details")
	registryUUID, err := resolveRegistryUUID(f, regInfo.RegistryIdentifier, org, project, progress)
	if err != nil {
		return err
	}
	progress.Success(fmt.Sprintf("Registry UUID: %s", registryUUID.String()))

	// Parse package.json
	progress.Start(fmt.Sprintf("Parsing %s", packageJsonPath))
	dependencies, err := regcmd.ParseLockFile(packageJsonPath)
	if err != nil {
		progress.Error("Failed to parse package.json")
		return fmt.Errorf("failed to parse package.json: %w", err)
	}

	if len(dependencies) == 0 {
		progress.Success("No dependencies found in package.json")
		return nil
	}
	progress.Success(fmt.Sprintf("Found %d dependencies", len(dependencies)))

	// Step 5: Run firewall audit
	ctx := &auditContext{
		F:            f,
		RegistryUUID: registryUUID,
		Org:          org,
		Project:      project,
		P:            progress,
	}

	scanResults, err := regcmd.ProcessBatches(ctx, dependencies, regInfo.RegistryIdentifier)
	if err != nil {
		return err
	}

	if err := regcmd.DisplayResults(scanResults, progress); err != nil {
		return err
	}

	// Step : Identify vulnerable packages
	vulnerablePackages := filterVulnerablePackages(scanResults)

	if fix && len(vulnerablePackages) > 0 {
		fmt.Println()
		progress.Step("Analyzing vulnerable packages for fix versions")
		if err := regcmd.DisplayResults(vulnerablePackages, progress); err != nil {
			return err
		}

		fmt.Println()
		progress.Start("Fetching security fix information from scan details")
		fixes, manualFix, err := evaluateFixVersions(f, vulnerablePackages)
		if err != nil {
			progress.Error(fmt.Sprintf("Failed to fetch fix versions: %s", err))
			return err
		}

		if len(fixes) > 0 {
			progress.Success("Automatic Fixes (minor/patch updates):")
			displaySecurityFixes(fixes, progress)

			// Apply automatic fixes
			fmt.Println()
			progress.Start("Applying automatic fixes to package.json")

			// Backup package.json
			if err := backupPackageJson(packageJsonPath); err != nil {
				progress.Error(fmt.Sprintf("Failed to backup package.json: %s", err))
				return err
			}
			progress.Success(fmt.Sprintf("Backup created: %s.backup", packageJsonPath))

			// Update package.json
			if err := updatePackageJsonWithFixes(packageJsonPath, fixes, progress); err != nil {
				progress.Error(fmt.Sprintf("Failed to update package.json: %s", err))
				return err
			}

			// Display comparison
			displayFixComparisonFromSecurityFixes(fixes)
		}

		if len(manualFix) > 0 {
			fmt.Println()
			progress.Step("Manual Fixes Required for below packages (major version changes):")
			displaySecurityFixes(manualFix, progress)
			fmt.Println("⚠️  Major version updates may contain breaking changes.")
			fmt.Println("   Please review the changelog before updating manually.")
		}
	}
	if fix && len(vulnerablePackages) == 0 {
		fmt.Println()
		progress.Success("No vulnerable packages found. Nothing to fix!")
	}
	return nil
}

func detectNpmRegistry() (*registryInfo, error) {
	savedCfg, err := regcmd.LoadNpmRegistryConfig()
	if err == nil && savedCfg != nil && savedCfg.RegistryURL != "" {
		return &registryInfo{
			RegistryURL:        savedCfg.RegistryURL,
			RegistryIdentifier: savedCfg.RegistryIdentifier,
			OrgID:              savedCfg.OrgID,
			ProjectID:          savedCfg.ProjectID,
		}, nil

	}

	if err != nil {
		return nil, fmt.Errorf("no HAR registry found. Run 'hc registry configure npm' first,: %w", err)
	}

	return nil, fmt.Errorf("no HAR registry found. Run 'hc registry configure npm' first")
}

type registryInfo struct {
	RegistryURL        string
	RegistryIdentifier string
	OrgID              string
	ProjectID          string
}

func resolveRegistryUUID(f *cmdutils.Factory, registryIdentifier, org, project string, progress *p.ConsoleReporter) (uuid.UUID, error) {
	registryRef := client2.GetRef(config.Global.AccountID, org, project) + "/" + registryIdentifier
	registryResp, err := f.RegistryHttpClient().GetRegistryWithResponse(context.Background(), registryRef)
	if err != nil {
		progress.Error("Failed to fetch registry details")
		return uuid.Nil, fmt.Errorf("failed to fetch registry details: %w", err)
	}

	if registryResp.StatusCode() != 200 || registryResp.JSON200 == nil || registryResp.JSON200.Data.Uuid == nil {
		progress.Error(fmt.Sprintf("Registry '%s' not found", registryIdentifier))
		return uuid.Nil, fmt.Errorf("registry '%s' not found", registryIdentifier)
	}

	registryUUID, err := uuid.Parse(*registryResp.JSON200.Data.Uuid)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid registry UUID: %w", err)
	}

	return registryUUID, nil
}

type ScanResult = regcmd.ScanResult

func filterVulnerablePackages(results []ScanResult) []ScanResult {
	vulnerable := make([]ScanResult, 0)
	for _, r := range results {
		if r.ScanStatus == "BLOCKED" || r.ScanStatus == "WARN" {
			vulnerable = append(vulnerable, r)
		}
	}
	return vulnerable
}

func evaluateFixVersions(f *cmdutils.Factory, vulnerablePackages []ScanResult) ([]SecurityFixInfo, []SecurityFixInfo, error) {
	fixes := make([]SecurityFixInfo, 0)
	manualFix := make([]SecurityFixInfo, 0)

	for _, pkg := range vulnerablePackages {
		if pkg.ScanID == "" {
			log.Warn().Str("package", pkg.PackageName).Msg("No scan ID available, skipping")
			continue
		}

		// Fetch scan details
		scanParams := &ar_v3.GetArtifactScanDetailsParams{
			AccountIdentifier: config.Global.AccountID,
		}

		scanResponse, err := f.RegistryV3HttpClient().GetArtifactScanDetailsWithResponse(
			context.Background(),
			pkg.ScanID,
			scanParams,
		)
		if err != nil {
			log.Warn().Err(err).Str("package", pkg.PackageName).Msg("Failed to get scan details")
			continue
		}

		if scanResponse.StatusCode() != 200 || scanResponse.JSON200 == nil || scanResponse.JSON200.Data == nil {
			log.Warn().Str("package", pkg.PackageName).Int("status", scanResponse.StatusCode()).Msg("Invalid scan details response")
			continue
		}

		scanDetails := scanResponse.JSON200.Data

		// Check if there's a security violation
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

		// Extract fix version information if security violation exists
		if hasSecurityViolation && scanDetails.FixVersionDetails != nil {
			fixInfo := SecurityFixInfo{
				PackageName:      pkg.PackageName,
				CurrentVersion:   scanDetails.FixVersionDetails.CurrentVersion,
				FixAvailable:     scanDetails.FixVersionDetails.FixVersionAvailable,
				HasSecurityIssue: true,
			}

			if scanDetails.FixVersionDetails.FixVersion != nil {
				fixInfo.FixVersion = *scanDetails.FixVersionDetails.FixVersion
			}
			if !majorChange(fixInfo) {
				fixes = append(fixes, fixInfo)
			} else {
				manualFix = append(manualFix, fixInfo)
			}
			log.Info().Str("package", pkg.PackageName).Str("fixVersion", fixInfo.FixVersion).Msg("Found fix version")
		}
	}

	return fixes, manualFix, nil
}

func majorChange(fixInfo SecurityFixInfo) bool {
	currentMajor := extractMajorVersion(fixInfo.CurrentVersion)
	fixMajor := extractMajorVersion(fixInfo.FixVersion)

	// If we can't parse versions, consider it not a major change (safe default)
	if currentMajor == -1 || fixMajor == -1 {
		return false
	}

	return currentMajor != fixMajor
}

func extractMajorVersion(version string) int {
	// Remove any leading 'v' or '^' or '~' characters
	version = strings.TrimPrefix(version, "v")
	version = strings.TrimPrefix(version, "^")
	version = strings.TrimPrefix(version, "~")

	// Split by dot and get the first part
	parts := strings.Split(version, ".")
	if len(parts) == 0 {
		return -1
	}

	// Parse the major version number
	major := 0
	_, err := fmt.Sscanf(parts[0], "%d", &major)
	if err != nil {
		return -1
	}

	return major
}

func displaySecurityFixes(fixes []SecurityFixInfo, progress *p.ConsoleReporter) {

	fmt.Println()
	fmt.Println("Security Fix Information:")
	fmt.Println(strings.Repeat("=", 80))

	for _, fix := range fixes {
		fmt.Printf("📦 Package: %s\n", fix.PackageName)
		fmt.Printf("   Current Version:  %s\n", fix.CurrentVersion)
		if fix.FixVersion != "" {
			fmt.Printf("   Fix Version:      %s\n", fix.FixVersion)
		} else {
			fmt.Printf("   Fix Version:      (not available)\n")
		}
	}

	fmt.Println(strings.Repeat("=", 80))
	progress.Success(fmt.Sprintf("Found %d packages with available fixes", len(fixes)))
}

func backupPackageJson(filePath string) error {
	backupPath := filePath + ".backup"

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read package.json: %w", err)
	}

	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	log.Info().Str("backup", backupPath).Msg("Created backup of package.json")
	return nil
}

func updatePackageJsonWithFixes(filePath string, fixes []SecurityFixInfo, progress *p.ConsoleReporter) error {
	progress.Start("Updating package.json with fix versions")

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read package.json: %w", err)
	}

	var pkgJson map[string]interface{}
	if err := json.Unmarshal(data, &pkgJson); err != nil {
		return fmt.Errorf("failed to parse package.json: %w", err)
	}

	updated := 0
	for _, fix := range fixes {
		updated += updateDependencySectionWithFix(pkgJson, "dependencies", fix)
		updated += updateDependencySectionWithFix(pkgJson, "devDependencies", fix)
		updated += updateDependencySectionWithFix(pkgJson, "peerDependencies", fix)
		updated += updateDependencySectionWithFix(pkgJson, "optionalDependencies", fix)
	}

	// Write back to file with proper formatting
	updatedData, err := json.MarshalIndent(pkgJson, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal updated package.json: %w", err)
	}

	// Add newline at end of file (standard practice)
	updatedData = append(updatedData, '\n')

	if err := os.WriteFile(filePath, updatedData, 0644); err != nil {
		return fmt.Errorf("failed to write updated package.json: %w", err)
	}

	progress.Success(fmt.Sprintf("Updated %d package versions in package.json", updated))
	log.Info().Int("updated", updated).Msg("Updated package.json")
	return nil
}

func updateDependencySectionWithFix(pkgJson map[string]interface{}, section string, fix SecurityFixInfo) int {
	deps, ok := pkgJson[section].(map[string]interface{})
	if !ok {
		return 0
	}

	if _, exists := deps[fix.PackageName]; exists {
		deps[fix.PackageName] = fix.FixVersion
		log.Info().Str("package", fix.PackageName).Str("section", section).Str("version", fix.FixVersion).Msg("Updated package version")
		return 1
	}

	return 0
}

func displayFixComparisonFromSecurityFixes(fixes []SecurityFixInfo) {
	fmt.Println()
	fmt.Println("Package Updates Applied:")
	fmt.Println(strings.Repeat("=", 80))

	for _, fix := range fixes {
		fmt.Printf("\n📦 %s  ::   %s → %s\n", fix.PackageName, fix.CurrentVersion, fix.FixVersion)
	}

	fmt.Println(strings.Repeat("=", 80))
}
