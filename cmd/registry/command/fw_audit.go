package command

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

			p.Start(fmt.Sprintf("Parsing lock file: %s", filepath.Base(filePath)))
			log.Info().Str("file", filePath).Msg("Parsing lock file")

			dependencies, err := parseLockFile(filePath)
			if err != nil {
				p.Error("Failed to parse lock file")
				log.Error().Err(err).Msg("Failed to parse lock file")
				return fmt.Errorf("failed to parse lock file: %w", err)
			}

			if len(dependencies) == 0 {
				p.Success("No dependencies found in the lock file")
				log.Info().Msg("No dependencies found in the lock file")
				return nil
			}

			sort.Slice(dependencies, func(i, j int) bool {
				return dependencies[i].Name < dependencies[j].Name
			})

			p.Success(fmt.Sprintf("Found %d dependencies in %s", len(dependencies), filepath.Base(filePath)))
			log.Info().Int("count", len(dependencies)).Msg("Dependencies found")

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

			p.Success(fmt.Sprintf("Found registry UUID: %s", registryUUID.String()))
			log.Info().Str("registryUUID", registryUUID.String()).Msg("Registry UUID retrieved")

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

func parseLockFile(filePath string) ([]Dependency, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	fileName := filepath.Base(filePath)

	switch {
	case strings.HasSuffix(fileName, "package-lock.json"):
		return parsePackageLock(data)
	case strings.HasSuffix(fileName, "pnpm-lock.yaml"):
		return parsePnpmLock(data)
	case strings.HasSuffix(fileName, "yarn.lock"):
		return parseYarnLock(data)
	default:
		return nil, fmt.Errorf("unsupported lock file format: %s (supported: package-lock.json, pnpm-lock.yaml, yarn.lock)", fileName)
	}
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
