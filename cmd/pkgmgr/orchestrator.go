package pkgmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/harness/harness-cli/cmd/cmdutils"
	regcmd "github.com/harness/harness-cli/cmd/registry/command"
	"github.com/harness/harness-cli/config"
	ar_v3 "github.com/harness/harness-cli/internal/api/ar_v3"
	client2 "github.com/harness/harness-cli/util/client"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// ExecuteWithFirewall runs the common 4-phase flow for any package manager:
//  1. Detect HAR registry
//  2. Resolve registry UUID
//  3. Run native command
//  4. On 403: resolve deps → firewall evaluation
func ExecuteWithFirewall(
	client Client,
	f *cmdutils.Factory,
	command string,
	args []string,
	explicitRegistry string,
	progress p.Reporter,
) error {
	clientName := client.Name()

	// Phase 1: Detect HAR registry
	progress.Start("Detecting HAR registry")
	regInfo, err := client.DetectRegistry(explicitRegistry)
	if err != nil {
		progress.Error(fmt.Sprintf("Failed to detect HAR registry: %s", err))
		log.Error().Err(err).Msg("Failed to detect HAR registry")
		return fmt.Errorf("failed to detect HAR registry: %w", err)
	}
	progress.Success(fmt.Sprintf("Found HAR registry: %s", regInfo.RegistryIdentifier))

	// Resolve org/project
	org, project := resolveOrgProject(client)

	// Phase 2: Resolve registry UUID
	progress.Step("Resolving registry details")
	registryUUID, err := ResolveRegistryUUID(f, regInfo.RegistryIdentifier, org, project, progress)
	if err != nil {
		return err
	}
	progress.Success(fmt.Sprintf("Registry UUID: %s", registryUUID.String()))

	// Phase 3: Run native command
	progress.Start(fmt.Sprintf("Running %s %s", clientName, command))
	result, err := client.RunCommand(command, args)
	if err != nil && result == nil {
		progress.Error(fmt.Sprintf("%s %s failed: %s", clientName, command, err))
		return fmt.Errorf("%s %s failed: %w", clientName, command, err)
	}

	if result.Status == "SUCCESS" {
		progress.Success(fmt.Sprintf("%s %s completed successfully", clientName, command))
		return nil
	}

	// Install failed
	has403 := client.DetectFirewallError(result.Stderr)
	if !has403 {
		progress.Error(fmt.Sprintf("%s %s failed: %s", clientName, command, result.Err))
		return fmt.Errorf("%s %s failed", clientName, command)
	}

	// Phase 4: 403 detected — resolve deps and run firewall evaluation
	progress.Error(fmt.Sprintf("%s %s failed (firewall may have blocked packages)", clientName, command))
	fmt.Println()

	progress.Start("Resolving complete dependency list")
	depResult, err := client.ResolveDependencies(progress)
	if err != nil {
		progress.Error(fmt.Sprintf("Failed to resolve dependencies: %s", err))
		return fmt.Errorf("%s %s failed: dependency resolution error: %w", clientName, command, err)
	}
	if depResult.Cleanup != nil {
		defer depResult.Cleanup()
	}

	if len(depResult.Dependencies) == 0 {
		progress.Error("No dependencies found to evaluate")
		return fmt.Errorf("%s %s failed", clientName, command)
	}

	progress.Success(fmt.Sprintf("Resolved %d total dependencies (including transitive)", len(depResult.Dependencies)))

	// Save build info for inspection/validation
	saveBuildInfo(clientName, command, regInfo.RegistryIdentifier, depResult.Dependencies)

	artifacts := make([]ar_v3.ArtifactScanInput, 0, len(depResult.Dependencies))
	for _, dep := range depResult.Dependencies {
		artifacts = append(artifacts, ar_v3.ArtifactScanInput{
			PackageName: dep.Name,
			Version:     dep.Version,
		})
	}

	progress.Start("Fetching firewall evaluation info")
	if evalErr := RunFirewallExplain(f, registryUUID, artifacts, org, project, progress); evalErr != nil {
		log.Error().Err(evalErr).Msg("Firewall evaluation failed")
		progress.Error(fmt.Sprintf("Firewall evaluation failed: %s", evalErr))
	}

	return fmt.Errorf("%s %s failed", clientName, command)
}

// resolveOrgProject resolves org and project from env vars, global config, or client-specific saved config.
func resolveOrgProject(client Client) (string, string) {
	org := config.Global.OrgID
	project := config.Global.ProjectID

	if envOrg := os.Getenv("ORG_IDENTIFIER"); envOrg != "" {
		org = envOrg
	}
	if envProj := os.Getenv("PROJECT_IDENTIFIER"); envProj != "" {
		project = envProj
	}

	// Fallback: ask the client for saved org/project
	if org == "" || project == "" {
		fallbackOrg, fallbackProject := client.FallbackOrgProject()
		if org == "" {
			org = fallbackOrg
		}
		if project == "" {
			project = fallbackProject
		}
	}

	return org, project
}

// ResolveRegistryUUID looks up the registry UUID from the registry identifier.
func ResolveRegistryUUID(f *cmdutils.Factory, registryIdentifier, org, project string, progress p.Reporter) (uuid.UUID, error) {
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
	if registryUUID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("registry '%s' returned nil UUID", registryIdentifier)
	}

	return registryUUID, nil
}

// buildInfo is the structure saved to .harness/build-info.json for validation.
type buildInfo struct {
	Client       string              `json:"client"`
	Command      string              `json:"command"`
	Registry     string              `json:"registry"`
	Timestamp    string              `json:"timestamp"`
	Dependencies []regcmd.Dependency `json:"dependencies"`
}

// saveBuildInfo writes the resolved dependency list to .harness/build-info.json.
func saveBuildInfo(clientName, command, registry string, deps []regcmd.Dependency) {
	info := buildInfo{
		Client:       clientName,
		Command:      command,
		Registry:     registry,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Dependencies: deps,
	}

	dir := filepath.Join(".harness")
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Warn().Err(err).Msg("Failed to create .harness directory for build info")
		return
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		log.Warn().Err(err).Msg("Failed to marshal build info")
		return
	}

	path := filepath.Join(dir, "build-info.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Warn().Err(err).Msg("Failed to write build info")
		return
	}

	log.Info().Str("path", path).Int("deps", len(deps)).Msg("Build info saved")
}
