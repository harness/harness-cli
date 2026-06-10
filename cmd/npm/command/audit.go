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
	var registryName string
	var packageJsonPath string

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Audit npm dependencies for vulnerabilities",
		Long: `Audit npm dependencies against firewall policies and security vulnerabilities.
		
This command will:
1. Parse your package.json file
2. Evaluate all dependencies against firewall policies
3. Identify vulnerable packages with available fixes
4. Optionally update package.json with fix versions (--fix flag)
5. Optionally run npm install to apply updates (--install flag)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNpmAudit(f, registryName, packageJsonPath, fix)
		},
	}

	cmd.Flags().BoolVar(&fix, "fix", false, "Automatically update package.json with fix versions for vulnerable packages")
	cmd.Flags().StringVarP(&packageJsonPath, "file", "f", "package.json", "Path to package.json file")
	cmd.Flags().StringVar(&registryName, "registry", "", "Registry name (optional, will auto-detect from config)")

	return cmd
}

func runNpmAudit(f *cmdutils.Factory, registryName string, packageJsonPath string, fix bool) error {
	progress := p.NewConsoleReporter()

	// Validate package.json exists
	if _, err := os.Stat(packageJsonPath); os.IsNotExist(err) {
		progress.Error(fmt.Sprintf("package.json not found at: %s", packageJsonPath))
		return fmt.Errorf("package.json not found at: %s", packageJsonPath)
	}

	progress.Start("Starting npm audit")
	log.Info().Str("file", packageJsonPath).Bool("fix", fix).Msg("Starting npm audit")

	// Step 1: Detect registry
	progress.Step("Detecting HAR registry")
	regInfo, err := detectNpmRegistry(registryName)
	if err != nil {
		progress.Error(fmt.Sprintf("Failed to detect registry: %s", err))
		return err
	}
	progress.Success(fmt.Sprintf("Found registry: %s", regInfo.RegistryIdentifier))

	// Step 2: Resolve org/project
	org := config.Global.OrgID
	project := config.Global.ProjectID
	if envOrg := os.Getenv("ORG_IDENTIFIER"); envOrg != "" {
		org = envOrg
	}
	if envProj := os.Getenv("PROJECT_IDENTIFIER"); envProj != "" {
		project = envProj
	}

	// Fallback to saved npm config
	if org == "" || project == "" {
		savedCfg, _ := regcmd.LoadNpmRegistryConfig()
		if savedCfg != nil {
			if org == "" {
				org = savedCfg.OrgID
			}
			if project == "" {
				project = savedCfg.ProjectID
			}
		}
	}

	if org == "" || project == "" {
		progress.Error("Organization and Project identifiers are required")
		return fmt.Errorf("organization and project identifiers are required. Set via config or environment variables")
	}

	// Step 3: Resolve registry UUID
	progress.Step("Resolving registry details")
	registryUUID, err := resolveRegistryUUID(f, regInfo.RegistryIdentifier, org, project, progress)
	if err != nil {
		return err
	}
	progress.Success(fmt.Sprintf("Registry UUID: %s", registryUUID.String()))

	// Step 4: Parse package.json
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
			//TODO Apply fix here
		}

		if len(manualFix) > 0 {
			fmt.Println()
			progress.Step("Manual Fixes Required for below packages (major version changes):")
			displaySecurityFixes(manualFix, progress)
			fmt.Println("⚠️  Major version updates may contain breaking changes.")
			fmt.Println("   Please review the changelog before updating manually.")
		}
	}

	// Step 8: If fix flag is set, fetch fix versions and update package.json
	/*TODO
	if fix && len(vulnerablePackages) > 0 {
		fmt.Println()
		progress.Start("Fetching fix version details for vulnerable packages")

		fixes, err := fetchFixVersions(ctx, vulnerablePackages)
		if err != nil {
			progress.Error(fmt.Sprintf("Failed to fetch fix versions: %s", err))
			return err
		}

		fixablePackages := filterFixablePackages(fixes)
		if len(fixablePackages) == 0 {
			progress.Step("No fix versions available for vulnerable packages")
			return nil
		}

		progress.Success(fmt.Sprintf("Found fix versions for %d packages", len(fixablePackages)))

		// Create backup
		if err := backupPackageJson(packageJsonPath); err != nil {
			progress.Error(fmt.Sprintf("Failed to create backup: %s", err))
			return err
		}
		progress.Success(fmt.Sprintf("Created backup: %s.backup", packageJsonPath))

		// Update package.json
		if err := updatePackageJson(packageJsonPath, fixablePackages, progress); err != nil {
			progress.Error(fmt.Sprintf("Failed to update package.json: %s", err))
			return err
		}

		// Display before/after comparison
		displayFixComparison(fixablePackages)

		// Optionally run npm install
		if runInstall {
			fmt.Println()
			progress.Start("Running npm install to apply updates")
			if err := runNpmInstall(progress); err != nil {
				progress.Error(fmt.Sprintf("npm install failed: %s", err))
				return err
			}
			progress.Success("npm install completed successfully")
		} else {
			fmt.Println()
			progress.Step("Run 'npm install' to apply the updates")
		}
	} else if fix && len(vulnerablePackages) == 0 {
		fmt.Println()
		progress.Success("No vulnerable packages found. Nothing to fix!")
	}

	*/

	return nil
}

func detectNpmRegistry(explicitRegistry string) (*registryInfo, error) {
	savedCfg, err := regcmd.LoadNpmRegistryConfig()
	if err == nil && savedCfg != nil && savedCfg.RegistryURL != "" {
		if explicitRegistry == "" || explicitRegistry == savedCfg.RegistryIdentifier {
			return &registryInfo{
				RegistryURL:        savedCfg.RegistryURL,
				RegistryIdentifier: savedCfg.RegistryIdentifier,
			}, nil
		}
	}

	if explicitRegistry != "" {
		return nil, fmt.Errorf("registry '%s' not found in saved config", explicitRegistry)
	}
	return nil, fmt.Errorf("no HAR registry found. Run 'hc registry configure npm' first")
}

type registryInfo struct {
	RegistryURL        string
	RegistryIdentifier string
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

func filterFixablePackages(fixes []PackageFix) []PackageFix {
	fixable := make([]PackageFix, 0)
	for _, fix := range fixes {
		if fix.FixVersion != "" {
			fixable = append(fixable, fix)
		}
	}
	return fixable
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

func updatePackageJson(filePath string, fixes []PackageFix, progress *p.ConsoleReporter) error {
	progress.Start("Updating package.json with fix versions")

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read package.json: %w", err)
	}

	var pkgJson map[string]interface{}
	if err := json.Unmarshal(data, &pkgJson); err != nil {
		return fmt.Errorf("failed to parse package.json: %w", err)
	}

	// Update dependencies
	updated := 0
	for _, fix := range fixes {
		updated += updateDependencySection(pkgJson, "dependencies", fix)
		updated += updateDependencySection(pkgJson, "devDependencies", fix)
		updated += updateDependencySection(pkgJson, "peerDependencies", fix)
		updated += updateDependencySection(pkgJson, "optionalDependencies", fix)
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

func updateDependencySection(pkgJson map[string]interface{}, section string, fix PackageFix) int {
	deps, ok := pkgJson[section].(map[string]interface{})
	if !ok {
		return 0
	}

	if _, exists := deps[fix.Name]; exists {
		deps[fix.Name] = fix.FixVersion
		log.Info().Str("package", fix.Name).Str("section", section).Str("version", fix.FixVersion).Msg("Updated package version")
		return 1
	}

	return 0
}

func displayFixComparison(fixes []PackageFix) {
	fmt.Println()
	fmt.Println("Package Updates Applied:")
	fmt.Println(strings.Repeat("=", 80))

	for _, fix := range fixes {
		fmt.Printf("\n📦 %s\n", fix.Name)
		fmt.Printf("   %s → %s\n", fix.CurrentVersion, fix.FixVersion)

		if len(fix.Vulnerabilities) > 0 {
			fmt.Printf("   Fixes %d vulnerabilities:\n", len(fix.Vulnerabilities))
			for _, vuln := range fix.Vulnerabilities {
				fmt.Printf("     - %s (CVSS: %.1f, Severity: %s)\n", vuln.CveId, vuln.CvssScore, vuln.Severity)
			}
		}
	}

	fmt.Println()
	fmt.Println(strings.Repeat("=", 80))
}

func runNpmInstall(progress *p.ConsoleReporter) error {
	log.Info().Msg("Running npm install")

	// Note: This would typically execute npm install
	// Implementation depends on whether you want to use:
	// 1. Direct exec.Command("npm", "install")
	// 2. The existing pkgmgr.ExecuteWithFirewall infrastructure
	// 3. A simple os/exec call

	// For now, we'll indicate that the user should run it manually
	// To implement actual execution, uncomment and use:
	/*
		cmd := exec.Command("npm", "install")
		cmd.Dir = "."
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("npm install failed: %w", err)
		}
	*/

	return nil
}
