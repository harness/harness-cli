package command

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
	"gopkg.in/yaml.v3"
)

type Dependency struct {
	Name    string
	Version string
	Source  string
}

type ScanResult struct {
	PackageName string `json:"packageName"`
	Version     string `json:"version"`
	ScanID      string `json:"scanId"`
	ScanStatus  string `json:"scanStatus"`
}

func NewFirewallAuditCmd(f *cmdutils.Factory) *cobra.Command {
	var registryName string
	var filePath string
	var orgID string
	var projectID string

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Audit dependencies from lock files",
		Long:  "Parse and evaluate dependencies from package-lock.json, pnpm-lock.yaml, or yarn.lock files against firewall policies",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := progress.NewConsoleReporter()

			if filePath == "" {
				log.Error().Msg("--file flag is required")
				return fmt.Errorf("--file flag is required")
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

			// Fetch registry details FIRST to get package type
			p.Step(fmt.Sprintf("Fetching registry details for: %s", registryName))
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

			// Get package type from registry
			packageType := string(registryResp.JSON200.Data.PackageType)

			p.Success(fmt.Sprintf("Found registry: %s (type: %s)", registryUUID.String(), packageType))
			log.Info().Str("registryUUID", registryUUID.String()).Str("packageType", packageType).Msg("Registry details retrieved")

			// Validate file type matches registry package type
			fileName := filepath.Base(filePath)
			if err := validateFileForPackageType(fileName, packageType); err != nil {
				p.Error(err.Error())
				log.Error().Err(err).Str("file", fileName).Str("packageType", packageType).Msg("File type mismatch")
				return err
			}

			p.Start(fmt.Sprintf("Parsing dependency file: %s", fileName))
			log.Info().Str("file", filePath).Msg("Parsing dependency file")

			dependencies, err := parseLockFile(filePath)
			if err != nil {
				p.Error("Failed to parse dependency file")
				log.Error().Err(err).Msg("Failed to parse dependency file")
				return fmt.Errorf("failed to parse dependency file: %w", err)
			}

			if len(dependencies) == 0 {
				p.Success("No dependencies found in the file")
				log.Info().Msg("No dependencies found in the file")
				return nil
			}

			sort.Slice(dependencies, func(i, j int) bool {
				return dependencies[i].Name < dependencies[j].Name
			})

			p.Success(fmt.Sprintf("Found %d dependencies in %s", len(dependencies), fileName))

			p.Step(fmt.Sprintf("Initiating bulk scan evaluation for registry: %s", registryName))
			log.Info().Str("registry", registryName).Msg("Initiating bulk scan evaluation")

			artifacts := make([]ar_v3.ArtifactScanInput, 0, len(dependencies))
			for _, dep := range dependencies {
				artifacts = append(artifacts, ar_v3.ArtifactScanInput{
					PackageName: dep.Name,
					Version:     dep.Version,
				})
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
				p.Error("Failed to initiate bulk scan evaluation")
				log.Error().Err(err).Msg("Failed to initiate bulk scan evaluation")
				return fmt.Errorf("failed to initiate bulk scan evaluation: %w", err)
			}

			if initResp.StatusCode() != 202 {
				errMsg := "Failed to initiate bulk scan evaluation"
				if initResp.JSONDefault != nil && initResp.JSONDefault.Error.Message != nil {
					errMsg = *initResp.JSONDefault.Error.Message
				}
				p.Error(errMsg)
				log.Error().Int("statusCode", initResp.StatusCode()).Msg(errMsg)
				return fmt.Errorf(errMsg)
			}

			if initResp.JSON202 == nil || initResp.JSON202.Data == nil || initResp.JSON202.Data.EvaluationId == nil {
				p.Error("Invalid response from bulk scan evaluation API")
				log.Error().Msg("Invalid response from bulk scan evaluation API")
				return fmt.Errorf("invalid response from bulk scan evaluation API")
			}

			evaluationID := *initResp.JSON202.Data.EvaluationId
			p.Success(fmt.Sprintf("Bulk scan evaluation initiated with ID: %s", evaluationID))
			log.Info().Str("evaluationId", evaluationID).Msg("Bulk scan evaluation initiated")

			p.Step("Waiting for bulk scan evaluation to complete")
			log.Info().Msg("Polling bulk scan evaluation status")

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
					p.Error("Timeout waiting for bulk scan evaluation to complete")
					log.Error().Int("maxPolls", maxPolls).Msg("Timeout waiting for bulk scan evaluation")
					return fmt.Errorf("timeout waiting for bulk scan evaluation to complete")
				}

				statusResp, err = f.RegistryV3HttpClient().GetBulkScanEvaluationStatusWithResponse(
					context.Background(),
					evaluationID,
					statusParams,
				)
				if err != nil {
					p.Error("Failed to get bulk scan evaluation status")
					log.Error().Err(err).Msg("Failed to get bulk scan evaluation status")
					return fmt.Errorf("failed to get bulk scan evaluation status: %w", err)
				}

				if statusResp.StatusCode() != 200 {
					errMsg := "Failed to get bulk scan evaluation status"
					if statusResp.JSONDefault != nil && statusResp.JSONDefault.Error.Message != nil {
						errMsg = *statusResp.JSONDefault.Error.Message
					}
					p.Error(errMsg)
					log.Error().Int("statusCode", statusResp.StatusCode()).Msg(errMsg)
					return fmt.Errorf(errMsg)
				}

				if statusResp.JSON200 == nil || statusResp.JSON200.Data == nil || statusResp.JSON200.Data.Status == nil {
					p.Error("Invalid response from bulk scan evaluation status API")
					log.Error().Msg("Invalid response from bulk scan evaluation status API")
					return fmt.Errorf("invalid response from bulk scan evaluation status API")
				}

				status = *statusResp.JSON200.Data.Status
				log.Debug().Str("status", string(status)).Int("poll", pollCount).Msg("Bulk scan evaluation status")

				if status == ar_v3.BulkScanEvaluationStatusDataStatusSUCCESS {
					p.Success("Bulk scan evaluation completed successfully")
					log.Info().Msg("Bulk scan evaluation completed successfully")
					break
				}

				if status == ar_v3.BulkScanEvaluationStatusDataStatusFAILURE {
					errMsg := "Bulk scan evaluation failed"
					if statusResp.JSON200.Data.Error != nil {
						errMsg = *statusResp.JSON200.Data.Error
					}
					p.Error(errMsg)
					log.Error().Str("error", errMsg).Msg("Bulk scan evaluation failed")
					return fmt.Errorf(errMsg)
				}

				time.Sleep(2 * time.Second)
			}

			if statusResp.JSON200.Data.Scans == nil {
				p.Success("No scan results returned")
				log.Info().Msg("No scan results returned")
				return nil
			}

			scans := *statusResp.JSON200.Data.Scans
			log.Info().Int("count", len(scans)).Msg("Scan results received")

			results := make([]ScanResult, 0, len(scans))
			for _, scan := range scans {
				result := ScanResult{}
				if scan.PackageName != nil {
					result.PackageName = *scan.PackageName
				}
				if scan.Version != nil {
					result.Version = *scan.Version
				}
				if scan.ScanId != nil {
					result.ScanID = scan.ScanId.String()
				}
				if scan.ScanStatus != nil {
					result.ScanStatus = string(*scan.ScanStatus)
				}
				results = append(results, result)
			}

			sort.Slice(results, func(i, j int) bool {
				return results[i].PackageName < results[j].PackageName
			})

			if config.Global.Format == "json" {
				jsonBytes, err := json.MarshalIndent(results, "", "  ")
				if err != nil {
					p.Error("Failed to marshal JSON output")
					log.Error().Err(err).Msg("Failed to marshal JSON output")
					return err
				}
				fmt.Println(string(jsonBytes))
				return nil
			}

			fmt.Println()
			p.Success(fmt.Sprintf("Scan Results for %d dependencies:", len(results)))

			var blockedCount, warnCount, allowedCount, unknownCount int
			for _, r := range results {
				switch r.ScanStatus {
				case "BLOCKED":
					blockedCount++
				case "WARN":
					warnCount++
				case "ALLOWED":
					allowedCount++
				case "UNKNOWN":
					unknownCount++
				}
			}

			if blockedCount > 0 {
				p.Error(fmt.Sprintf("Blocked: %d", blockedCount))
			}
			if warnCount > 0 {
				p.Step(fmt.Sprintf("Warnings: %d", warnCount))
			}
			if allowedCount > 0 {
				p.Success(fmt.Sprintf("Allowed: %d", allowedCount))
			}
			if unknownCount > 0 {
				p.Step(fmt.Sprintf("Unknown: %d", unknownCount))
			}

			fmt.Println()

			err = printer.Print(results, 0, 1, int64(len(results)), false, [][]string{
				{"packageName", "Package Name"},
				{"version", "Version"},
				{"scanStatus", "Status"},
			})

			return err
		},
	}

	cmd.Flags().StringVar(&registryName, "registry", "", "Registry name")
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Path to lock file (package-lock.json, pnpm-lock.yaml, or yarn.lock)")
	cmd.Flags().StringVar(&orgID, "org", "", "Organization identifier (defaults to global config)")
	cmd.Flags().StringVar(&projectID, "project", "", "Project identifier (defaults to global config)")
	cmd.MarkFlagRequired("file")
	cmd.MarkFlagRequired("registry")

	return cmd
}

// validateFileForPackageType validates that the dependency file matches the registry package type
func validateFileForPackageType(fileName, packageType string) error {
	validFiles := map[string][]string{
		"NPM": {
			"package.json",
			"package-lock.json",
			"yarn.lock",
			"pnpm-lock.yaml",
		},
		"PYTHON": {
			"requirements.txt",
			"pyproject.toml",
			"Pipfile.lock",
			"poetry.lock",
		},
		"MAVEN": {
			"pom.xml",
			"build.gradle",
			"build.gradle.kts",
		},
	}

	validFilesList, ok := validFiles[packageType]
	if !ok {
		return fmt.Errorf("unsupported package type: %s (supported: NPM, PYTHON, MAVEN)", packageType)
	}

	for _, validFile := range validFilesList {
		if fileName == validFile || strings.HasSuffix(fileName, validFile) {
			return nil
		}
	}

	return fmt.Errorf("file '%s' is not compatible with package type '%s'. Valid files for %s: %s",
		fileName, packageType, packageType, strings.Join(validFilesList, ", "))
}

func parseLockFile(filePath string) ([]Dependency, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	fileName := filepath.Base(filePath)

	switch {
	case fileName == "package.json":
		return parsePackageJson(data)
	case strings.HasSuffix(fileName, "package-lock.json"):
		return parsePackageLock(data)
	case strings.HasSuffix(fileName, "pnpm-lock.yaml"):
		return parsePnpmLock(data)
	case strings.HasSuffix(fileName, "yarn.lock"):
		return parseYarnLock(data)
	case strings.HasSuffix(fileName, "requirements.txt"):
		return parseRequirementsTxt(data)
	case strings.HasSuffix(fileName, "pyproject.toml"):
		return parsePyProjectToml(data)
	case fileName == "Pipfile.lock":
		return parsePipfileLock(data)
	case fileName == "poetry.lock":
		return parsePoetryLock(data)
	case strings.HasSuffix(fileName, "pom.xml"):
		return parsePomXml(data)
	case strings.HasSuffix(fileName, "build.gradle") || strings.HasSuffix(fileName, "build.gradle.kts"):
		return parseBuildGradle(data)
	default:
		return nil, fmt.Errorf("unsupported dependency file format: %s (supported: package.json, package-lock.json, pnpm-lock.yaml, yarn.lock, requirements.txt, pyproject.toml, Pipfile.lock, poetry.lock, pom.xml, build.gradle)", fileName)
	}
}

func parsePackageJson(data []byte) ([]Dependency, error) {
	var pkgJson struct {
		Dependencies         map[string]string `json:"dependencies"`
		DevDependencies      map[string]string `json:"devDependencies"`
		PeerDependencies     map[string]string `json:"peerDependencies"`
		OptionalDependencies map[string]string `json:"optionalDependencies"`
	}

	if err := json.Unmarshal(data, &pkgJson); err != nil {
		return nil, fmt.Errorf("failed to parse package.json: %w", err)
	}

	deps := make([]Dependency, 0)
	seen := make(map[string]bool)

	cleanVersion := func(version string) string {
		version = strings.TrimSpace(version)
		version = strings.TrimPrefix(version, "^")
		version = strings.TrimPrefix(version, "~")
		version = strings.TrimPrefix(version, ">=")
		version = strings.TrimPrefix(version, ">")
		version = strings.TrimPrefix(version, "<=")
		version = strings.TrimPrefix(version, "<")
		version = strings.TrimPrefix(version, "=")
		if idx := strings.Index(version, " "); idx != -1 {
			version = version[:idx]
		}
		return version
	}

	for name, version := range pkgJson.Dependencies {
		if seen[name] {
			continue
		}
		seen[name] = true
		deps = append(deps, Dependency{
			Name:    name,
			Version: cleanVersion(version),
			Source:  "package.json",
		})
	}

	for name, version := range pkgJson.DevDependencies {
		if seen[name] {
			continue
		}
		seen[name] = true
		deps = append(deps, Dependency{
			Name:    name,
			Version: cleanVersion(version),
			Source:  "package.json",
		})
	}

	for name, version := range pkgJson.PeerDependencies {
		if seen[name] {
			continue
		}
		seen[name] = true
		deps = append(deps, Dependency{
			Name:    name,
			Version: cleanVersion(version),
			Source:  "package.json",
		})
	}

	for name, version := range pkgJson.OptionalDependencies {
		if seen[name] {
			continue
		}
		seen[name] = true
		deps = append(deps, Dependency{
			Name:    name,
			Version: cleanVersion(version),
			Source:  "package.json",
		})
	}

	return deps, nil
}

func parsePackageLock(data []byte) ([]Dependency, error) {
	var lockFile struct {
		LockfileVersion int `json:"lockfileVersion"`
		Dependencies    map[string]struct {
			Version string `json:"version"`
		} `json:"dependencies"`
		Packages map[string]struct {
			Version string `json:"version"`
		} `json:"packages"`
	}

	if err := json.Unmarshal(data, &lockFile); err != nil {
		return nil, fmt.Errorf("failed to parse package-lock.json: %w", err)
	}

	deps := make([]Dependency, 0)
	seen := make(map[string]bool)

	if lockFile.LockfileVersion >= 2 && len(lockFile.Packages) > 0 {
		for pkgPath, pkg := range lockFile.Packages {
			if pkgPath == "" {
				continue
			}
			name := strings.TrimPrefix(pkgPath, "node_modules/")
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			deps = append(deps, Dependency{
				Name:    name,
				Version: pkg.Version,
				Source:  "package-lock.json",
			})
		}
	} else if len(lockFile.Dependencies) > 0 {
		for name, dep := range lockFile.Dependencies {
			if seen[name] {
				continue
			}
			seen[name] = true
			deps = append(deps, Dependency{
				Name:    name,
				Version: dep.Version,
				Source:  "package-lock.json",
			})
		}
	}

	return deps, nil
}

func parsePnpmLock(data []byte) ([]Dependency, error) {
	var lockFile struct {
		Dependencies    map[string]interface{} `yaml:"dependencies"`
		DevDependencies map[string]interface{} `yaml:"devDependencies"`
		Packages        map[string]struct {
			Resolution struct {
				Integrity string `yaml:"integrity"`
			} `yaml:"resolution"`
		} `yaml:"packages"`
	}

	if err := yaml.Unmarshal(data, &lockFile); err != nil {
		return nil, fmt.Errorf("failed to parse pnpm-lock.yaml: %w", err)
	}

	deps := make([]Dependency, 0)
	seen := make(map[string]bool)

	for pkgPath := range lockFile.Packages {
		parts := strings.Split(pkgPath, "/")
		var name, version string

		if strings.HasPrefix(pkgPath, "@") && len(parts) >= 2 {
			name = parts[0] + "/" + parts[1]
			if len(parts) > 2 {
				version = strings.TrimPrefix(parts[2], "@")
			}
		} else if len(parts) >= 1 {
			nameParts := strings.Split(parts[0], "@")
			if len(nameParts) >= 2 {
				name = nameParts[0]
				version = nameParts[1]
			} else {
				name = parts[0]
			}
		}

		if name == "" || seen[name] {
			continue
		}
		seen[name] = true

		deps = append(deps, Dependency{
			Name:    name,
			Version: version,
			Source:  "pnpm-lock.yaml",
		})
	}

	return deps, nil
}

func parseYarnLock(data []byte) ([]Dependency, error) {
	lines := strings.Split(string(data), "\n")
	deps := make([]Dependency, 0)
	seen := make(map[string]bool)

	var currentPkg string
	var currentVersion string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if !strings.HasPrefix(line, " ") && strings.Contains(line, "@") && strings.HasSuffix(line, ":") {
			pkgLine := strings.TrimSuffix(line, ":")
			parts := strings.Split(pkgLine, ",")
			if len(parts) > 0 {
				firstPart := strings.TrimSpace(parts[0])
				lastAt := strings.LastIndex(firstPart, "@")
				if lastAt > 0 {
					currentPkg = strings.Trim(firstPart[:lastAt], "\"")
				} else {
					currentPkg = strings.Trim(firstPart, "\"")
				}
			}
			currentVersion = ""
		} else if strings.HasPrefix(line, "version ") && currentPkg != "" {
			currentVersion = strings.Trim(strings.TrimPrefix(line, "version "), "\"")
			if !seen[currentPkg] {
				seen[currentPkg] = true
				deps = append(deps, Dependency{
					Name:    currentPkg,
					Version: currentVersion,
					Source:  "yarn.lock",
				})
			}
			currentPkg = ""
		}
	}

	return deps, nil
}

func parseRequirementsTxt(data []byte) ([]Dependency, error) {
	lines := strings.Split(string(data), "\n")
	deps := make([]Dependency, 0)
	seen := make(map[string]bool)

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
			continue
		}

		if strings.HasPrefix(line, "git+") || strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") {
			continue
		}

		var name, version string

		if strings.Contains(line, "==") {
			parts := strings.SplitN(line, "==", 2)
			name = strings.TrimSpace(parts[0])
			version = strings.TrimSpace(parts[1])
		} else if strings.Contains(line, ">=") {
			parts := strings.SplitN(line, ">=", 2)
			name = strings.TrimSpace(parts[0])
			version = strings.TrimSpace(parts[1])
			if idx := strings.Index(version, ","); idx != -1 {
				version = version[:idx]
			}
		} else if strings.Contains(line, "<=") {
			parts := strings.SplitN(line, "<=", 2)
			name = strings.TrimSpace(parts[0])
			version = strings.TrimSpace(parts[1])
		} else if strings.Contains(line, "~=") {
			parts := strings.SplitN(line, "~=", 2)
			name = strings.TrimSpace(parts[0])
			version = strings.TrimSpace(parts[1])
		} else if strings.Contains(line, ">") {
			parts := strings.SplitN(line, ">", 2)
			name = strings.TrimSpace(parts[0])
			version = strings.TrimSpace(parts[1])
			if idx := strings.Index(version, ","); idx != -1 {
				version = version[:idx]
			}
		} else if strings.Contains(line, "<") {
			parts := strings.SplitN(line, "<", 2)
			name = strings.TrimSpace(parts[0])
			version = strings.TrimSpace(parts[1])
		} else {
			name = line
			version = "latest"
		}

		if idx := strings.Index(name, "["); idx != -1 {
			name = name[:idx]
		}

		name = strings.TrimSpace(name)
		version = strings.TrimSpace(version)

		if strings.Contains(version, ";") {
			version = strings.TrimSpace(strings.Split(version, ";")[0])
		}

		if name == "" || seen[name] {
			continue
		}
		seen[name] = true

		deps = append(deps, Dependency{
			Name:    name,
			Version: version,
			Source:  "requirements.txt",
		})
	}

	return deps, nil
}

func parsePyProjectToml(data []byte) ([]Dependency, error) {
	deps := make([]Dependency, 0)
	seen := make(map[string]bool)

	lines := strings.Split(string(data), "\n")
	inDependencies := false
	inOptionalDeps := false
	inPoetryDeps := false

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		if strings.HasPrefix(trimmedLine, "[") {
			inDependencies = false
			inOptionalDeps = false
			inPoetryDeps = false

			if trimmedLine == "[tool.poetry.dependencies]" {
				inPoetryDeps = true
			} else if strings.HasPrefix(trimmedLine, "[project.optional-dependencies") {
				inOptionalDeps = true
			}
			continue
		}

		if trimmedLine == "dependencies = [" {
			inDependencies = true
			continue
		}

		if trimmedLine == "]" {
			inDependencies = false
			inOptionalDeps = false
			continue
		}

		if inPoetryDeps {
			if trimmedLine == "" || strings.HasPrefix(trimmedLine, "#") {
				continue
			}
			if strings.HasPrefix(trimmedLine, "python") {
				continue
			}

			if strings.Contains(trimmedLine, "=") {
				parts := strings.SplitN(trimmedLine, "=", 2)
				name := strings.TrimSpace(parts[0])
				versionPart := strings.TrimSpace(parts[1])

				versionPart = strings.Trim(versionPart, "\"'^~>=<")
				if strings.HasPrefix(versionPart, "{") {
					if strings.Contains(versionPart, "version") {
						re := strings.Index(versionPart, "version")
						if re != -1 {
							sub := versionPart[re:]
							if eqIdx := strings.Index(sub, "\""); eqIdx != -1 {
								endIdx := strings.Index(sub[eqIdx+1:], "\"")
								if endIdx != -1 {
									versionPart = sub[eqIdx+1 : eqIdx+1+endIdx]
								}
							}
						}
					} else {
						versionPart = "latest"
					}
				}

				versionPart = strings.Trim(versionPart, "\"'^~>=<,}")

				if name != "" && !seen[name] {
					seen[name] = true
					deps = append(deps, Dependency{
						Name:    name,
						Version: versionPart,
						Source:  "pyproject.toml",
					})
				}
			}
			continue
		}

		if inDependencies || inOptionalDeps {
			if trimmedLine == "" || strings.HasPrefix(trimmedLine, "#") {
				continue
			}

			depLine := strings.Trim(trimmedLine, "\",")

			if !strings.Contains(depLine, "==") && !strings.Contains(depLine, ">=") && !strings.Contains(depLine, "~=") && !strings.Contains(depLine, ">") && !strings.Contains(depLine, "<") {
				if strings.Contains(depLine, "=") {
					continue
				}
			}

			var name, version string

			if strings.Contains(depLine, "==") {
				parts := strings.SplitN(depLine, "==", 2)
				name = strings.TrimSpace(parts[0])
				version = strings.TrimSpace(parts[1])
			} else if strings.Contains(depLine, ">=") {
				parts := strings.SplitN(depLine, ">=", 2)
				name = strings.TrimSpace(parts[0])
				version = strings.TrimSpace(parts[1])
				if idx := strings.Index(version, ","); idx != -1 {
					version = version[:idx]
				}
			} else if strings.Contains(depLine, "~=") {
				parts := strings.SplitN(depLine, "~=", 2)
				name = strings.TrimSpace(parts[0])
				version = strings.TrimSpace(parts[1])
			} else {
				name = depLine
				version = "latest"
			}

			if idx := strings.Index(name, "["); idx != -1 {
				name = name[:idx]
			}

			name = strings.TrimSpace(name)
			version = strings.TrimSpace(version)

			if strings.Contains(version, ";") {
				version = strings.Split(version, ";")[0]
			}
			version = strings.Trim(version, "\"',")

			if name == "" || seen[name] {
				continue
			}
			seen[name] = true

			deps = append(deps, Dependency{
				Name:    name,
				Version: version,
				Source:  "pyproject.toml",
			})
		}
	}

	return deps, nil
}

func parsePipfileLock(data []byte) ([]Dependency, error) {
	var lockFile struct {
		Default map[string]struct {
			Version string `json:"version"`
		} `json:"default"`
		Develop map[string]struct {
			Version string `json:"version"`
		} `json:"develop"`
	}

	if err := json.Unmarshal(data, &lockFile); err != nil {
		return nil, fmt.Errorf("failed to parse Pipfile.lock: %w", err)
	}

	deps := make([]Dependency, 0)
	seen := make(map[string]bool)

	for name, pkg := range lockFile.Default {
		if seen[name] {
			continue
		}
		seen[name] = true

		version := strings.TrimPrefix(pkg.Version, "==")
		deps = append(deps, Dependency{
			Name:    name,
			Version: version,
			Source:  "Pipfile.lock",
		})
	}

	for name, pkg := range lockFile.Develop {
		if seen[name] {
			continue
		}
		seen[name] = true

		version := strings.TrimPrefix(pkg.Version, "==")
		deps = append(deps, Dependency{
			Name:    name,
			Version: version,
			Source:  "Pipfile.lock",
		})
	}

	return deps, nil
}

func parsePoetryLock(data []byte) ([]Dependency, error) {
	deps := make([]Dependency, 0)
	seen := make(map[string]bool)

	lines := strings.Split(string(data), "\n")
	inPackage := false
	var currentName, currentVersion string

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		if trimmedLine == "[[package]]" {
			if currentName != "" && !seen[currentName] {
				seen[currentName] = true
				deps = append(deps, Dependency{
					Name:    currentName,
					Version: currentVersion,
					Source:  "poetry.lock",
				})
			}
			inPackage = true
			currentName = ""
			currentVersion = ""
			continue
		}

		if inPackage {
			if strings.HasPrefix(trimmedLine, "name = ") {
				currentName = strings.Trim(strings.TrimPrefix(trimmedLine, "name = "), "\"")
			} else if strings.HasPrefix(trimmedLine, "version = ") {
				currentVersion = strings.Trim(strings.TrimPrefix(trimmedLine, "version = "), "\"")
			} else if strings.HasPrefix(trimmedLine, "[") && trimmedLine != "[[package]]" {
				inPackage = false
			}
		}
	}

	if currentName != "" && !seen[currentName] {
		seen[currentName] = true
		deps = append(deps, Dependency{
			Name:    currentName,
			Version: currentVersion,
			Source:  "poetry.lock",
		})
	}

	return deps, nil
}

func parsePomXml(data []byte) ([]Dependency, error) {
	type PomDependency struct {
		GroupId    string `xml:"groupId"`
		ArtifactId string `xml:"artifactId"`
		Version    string `xml:"version"`
		Scope      string `xml:"scope"`
	}

	type PomProject struct {
		XMLName      xml.Name `xml:"project"`
		Dependencies struct {
			Dependency []PomDependency `xml:"dependency"`
		} `xml:"dependencies"`
		DependencyManagement struct {
			Dependencies struct {
				Dependency []PomDependency `xml:"dependency"`
			} `xml:"dependencies"`
		} `xml:"dependencyManagement"`
		Properties map[string]string `xml:"-"`
	}

	var pom PomProject
	if err := xml.Unmarshal(data, &pom); err != nil {
		return nil, fmt.Errorf("failed to parse pom.xml: %w", err)
	}

	pom.Properties = make(map[string]string)
	propsRegex := regexp.MustCompile(`<([a-zA-Z0-9._-]+)>([^<]+)</[a-zA-Z0-9._-]+>`)
	propsSection := regexp.MustCompile(`(?s)<properties>(.*?)</properties>`)
	if matches := propsSection.FindSubmatch(data); len(matches) > 1 {
		propMatches := propsRegex.FindAllSubmatch(matches[1], -1)
		for _, match := range propMatches {
			if len(match) >= 3 {
				pom.Properties[string(match[1])] = string(match[2])
			}
		}
	}

	deps := make([]Dependency, 0)
	seen := make(map[string]bool)

	resolveVersion := func(version string) string {
		if strings.HasPrefix(version, "${") && strings.HasSuffix(version, "}") {
			propName := version[2 : len(version)-1]
			if resolved, ok := pom.Properties[propName]; ok {
				return resolved
			}
		}
		return version
	}

	allDeps := append(pom.Dependencies.Dependency, pom.DependencyManagement.Dependencies.Dependency...)

	for _, dep := range allDeps {
		if dep.GroupId == "" || dep.ArtifactId == "" {
			continue
		}

		name := dep.GroupId + ":" + dep.ArtifactId
		version := resolveVersion(dep.Version)
		if version == "" {
			version = "latest"
		}

		if seen[name] {
			continue
		}
		seen[name] = true

		deps = append(deps, Dependency{
			Name:    name,
			Version: version,
			Source:  "pom.xml",
		})
	}

	return deps, nil
}

func parseBuildGradle(data []byte) ([]Dependency, error) {
	deps := make([]Dependency, 0)
	seen := make(map[string]bool)

	content := string(data)

	depPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?:implementation|api|compile|runtimeOnly|testImplementation|testCompile|compileOnly)\s*\(\s*['"]([^'"]+):([^'"]+):([^'"]+)['"]\s*\)`),
		regexp.MustCompile(`(?:implementation|api|compile|runtimeOnly|testImplementation|testCompile|compileOnly)\s*\(\s*group:\s*['"]([^'"]+)['"]\s*,\s*name:\s*['"]([^'"]+)['"]\s*,\s*version:\s*['"]([^'"]+)['"]\s*\)`),
		regexp.MustCompile(`(?:implementation|api|compile|runtimeOnly|testImplementation|testCompile|compileOnly)\s+['"]([^'"]+):([^'"]+):([^'"]+)['"]`),
	}

	for _, pattern := range depPatterns {
		matches := pattern.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			var name, version string
			if len(match) >= 4 {
				name = match[1] + ":" + match[2]
				version = match[3]
			}

			if name == "" || seen[name] {
				continue
			}
			seen[name] = true

			deps = append(deps, Dependency{
				Name:    name,
				Version: version,
				Source:  "build.gradle",
			})
		}
	}

	kotlinPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?:implementation|api|compile|runtimeOnly|testImplementation|testCompile|compileOnly)\s*\(\s*"([^"]+):([^"]+):([^"]+)"\s*\)`),
	}

	for _, pattern := range kotlinPatterns {
		matches := pattern.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			var name, version string
			if len(match) >= 4 {
				name = match[1] + ":" + match[2]
				version = match[3]
			}

			if name == "" || seen[name] {
				continue
			}
			seen[name] = true

			deps = append(deps, Dependency{
				Name:    name,
				Version: version,
				Source:  "build.gradle",
			})
		}
	}

	return deps, nil
}
