package command

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	ar_v3 "github.com/harness/harness-cli/internal/api/ar_v3"
	"github.com/harness/harness-cli/util/common/printer"
	"github.com/harness/harness-cli/util/common/progress"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// Shared types for firewall audit functionality

type Dependency struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Source  string `json:"source"`
	Parent  string `json:"parent,omitempty"`
}

type ScanResult struct {
	PackageName string `json:"packageName"`
	Version     string `json:"version"`
	ScanID      string `json:"scanId"`
	ScanStatus  string `json:"scanStatus"`
}

type AuditContext struct {
	F                *cmdutils.Factory
	RegistryV3Client ar_v3.ClientWithResponsesInterface
	RegistryUUID     uuid.UUID
	Org              string
	Project          string
	P                *progress.ConsoleReporter
}

func (ctx *AuditContext) registryV3Client() ar_v3.ClientWithResponsesInterface {
	if ctx.RegistryV3Client != nil {
		return ctx.RegistryV3Client
	}
	return ctx.F.RegistryV3HttpClient()
}

type BatchInfo struct {
	BatchIdx     int
	TotalBatches int
	RegistryName string
}

// ProcessBatches evaluates dependencies in batches against firewall policies
func ProcessBatches(ctx *AuditContext, dependencies []Dependency, registryName string) ([]ScanResult, error) {
	const batchSize = 50
	totalBatches := (len(dependencies) + batchSize - 1) / batchSize
	allResults := make([]ScanResult, 0, len(dependencies))

	for batchIdx := 0; batchIdx < totalBatches; batchIdx++ {
		start := batchIdx * batchSize
		end := start + batchSize
		if end > len(dependencies) {
			end = len(dependencies)
		}
		batch := dependencies[start:end]

		info := BatchInfo{
			BatchIdx:     batchIdx,
			TotalBatches: totalBatches,
			RegistryName: registryName,
		}

		evaluationID, err := initiateBatchEvaluation(ctx, batch, info)
		if err != nil {
			return nil, err
		}

		statusResp, err := pollBatchEvaluation(ctx, evaluationID, info)
		if err != nil {
			return nil, err
		}

		batchResults := extractScanResults(statusResp, batchIdx)
		allResults = append(allResults, batchResults...)
	}

	return allResults, nil
}

func initiateBatchEvaluation(ctx *AuditContext, batch []Dependency, info BatchInfo) (string, error) {
	ctx.P.Step(fmt.Sprintf("Processing batch %d/%d (%d packages) for registry: %s", info.BatchIdx+1, info.TotalBatches, len(batch), info.RegistryName))
	log.Info().Str("registry", info.RegistryName).Int("batch", info.BatchIdx+1).Int("totalBatches", info.TotalBatches).Int("batchSize", len(batch)).Msg("Initiating bulk evaluation")

	artifacts := make([]ar_v3.ArtifactScanInput, 0, len(batch))
	for _, dep := range batch {
		artifacts = append(artifacts, ar_v3.ArtifactScanInput{
			PackageName: dep.Name,
			Version:     dep.Version,
		})
	}

	initParams := &ar_v3.InitiateBulkScanEvaluationParams{
		AccountIdentifier: config.Global.AccountID,
		OrgIdentifier:     &ctx.Org,
		ProjectIdentifier: &ctx.Project,
	}

	initResp, err := ctx.registryV3Client().InitiateBulkScanEvaluationWithResponse(
		context.Background(),
		initParams,
		ar_v3.InitiateBulkScanEvaluationJSONRequestBody{
			RegistryId: ctx.RegistryUUID,
			Artifacts:  artifacts,
		},
	)
	if err != nil {
		ctx.P.Error(fmt.Sprintf("Failed to initiate bulk evaluation for batch %d/%d", info.BatchIdx+1, info.TotalBatches))
		log.Error().Err(err).Int("batch", info.BatchIdx+1).Msg("Failed to initiate bulk evaluation")
		return "", fmt.Errorf("failed to initiate bulk evaluation for batch %d: %w", info.BatchIdx+1, err)
	}

	if initResp.StatusCode() != 202 {
		errMsg := fmt.Sprintf("Failed to initiate bulk evaluation for batch %d/%d", info.BatchIdx+1, info.TotalBatches)
		if initResp.JSONDefault != nil && initResp.JSONDefault.Error.Message != nil {
			errMsg = *initResp.JSONDefault.Error.Message
		}
		ctx.P.Error(errMsg)
		log.Error().Int("statusCode", initResp.StatusCode()).Int("batch", info.BatchIdx+1).Msg(errMsg)
		return "", fmt.Errorf("%s", errMsg)
	}

	if initResp.JSON202 == nil || initResp.JSON202.Data == nil || initResp.JSON202.Data.EvaluationId == nil {
		ctx.P.Error(fmt.Sprintf("Invalid response from bulk evaluation API for batch %d/%d", info.BatchIdx+1, info.TotalBatches))
		log.Error().Int("batch", info.BatchIdx+1).Msg("Invalid response from bulk evaluation API")
		return "", fmt.Errorf("invalid response from bulk evaluation API for batch %d", info.BatchIdx+1)
	}

	evaluationID := *initResp.JSON202.Data.EvaluationId
	ctx.P.Success(fmt.Sprintf("Batch %d/%d evaluation initiated with ID: %s", info.BatchIdx+1, info.TotalBatches, evaluationID))
	log.Info().Str("evaluationId", evaluationID).Int("batch", info.BatchIdx+1).Msg("Bulk evaluation initiated")

	return evaluationID, nil
}

func pollBatchEvaluation(ctx *AuditContext, evaluationID string, info BatchInfo) (*ar_v3.GetBulkScanEvaluationStatusResp, error) {
	ctx.P.Step(fmt.Sprintf("Waiting for batch %d/%d evaluation to complete", info.BatchIdx+1, info.TotalBatches))
	log.Info().Int("batch", info.BatchIdx+1).Msg("Polling bulk evaluation status")

	statusParams := &ar_v3.GetBulkScanEvaluationStatusParams{
		AccountIdentifier: config.Global.AccountID,
		OrgIdentifier:     &ctx.Org,
		ProjectIdentifier: &ctx.Project,
	}

	pollCount := 0
	maxPolls := 120

	for {
		pollCount++
		if pollCount > maxPolls {
			ctx.P.Error(fmt.Sprintf("Timeout waiting for batch %d/%d evaluation to complete", info.BatchIdx+1, info.TotalBatches))
			log.Error().Int("maxPolls", maxPolls).Int("batch", info.BatchIdx+1).Msg("Timeout waiting for bulk evaluation")
			return nil, fmt.Errorf("timeout waiting for batch %d evaluation to complete", info.BatchIdx+1)
		}

		statusResp, err := ctx.registryV3Client().GetBulkScanEvaluationStatusWithResponse(
			context.Background(),
			evaluationID,
			statusParams,
		)
		if err != nil {
			ctx.P.Error(fmt.Sprintf("Failed to get bulk evaluation status for batch %d/%d", info.BatchIdx+1, info.TotalBatches))
			log.Error().Err(err).Int("batch", info.BatchIdx+1).Msg("Failed to get bulk evaluation status")
			return nil, fmt.Errorf("failed to get bulk evaluation status for batch %d: %w", info.BatchIdx+1, err)
		}

		if statusResp.StatusCode() != 200 {
			errMsg := fmt.Sprintf("Failed to get bulk evaluation status for batch %d/%d", info.BatchIdx+1, info.TotalBatches)
			if statusResp.JSONDefault != nil && statusResp.JSONDefault.Error.Message != nil {
				errMsg = *statusResp.JSONDefault.Error.Message
			}
			ctx.P.Error(errMsg)
			log.Error().Int("statusCode", statusResp.StatusCode()).Int("batch", info.BatchIdx+1).Msg(errMsg)
			return nil, fmt.Errorf("%s", errMsg)
		}

		if statusResp.JSON200 == nil || statusResp.JSON200.Data == nil || statusResp.JSON200.Data.Status == nil {
			ctx.P.Error(fmt.Sprintf("Invalid response from bulk evaluation status API for batch %d/%d", info.BatchIdx+1, info.TotalBatches))
			log.Error().Int("batch", info.BatchIdx+1).Msg("Invalid response from bulk evaluation status API")
			return nil, fmt.Errorf("invalid response from bulk evaluation status API for batch %d", info.BatchIdx+1)
		}

		status := *statusResp.JSON200.Data.Status
		log.Debug().Str("status", string(status)).Int("poll", pollCount).Int("batch", info.BatchIdx+1).Msg("Bulk evaluation status")

		if status == ar_v3.SUCCESS {
			ctx.P.Success(fmt.Sprintf("Batch %d/%d evaluation completed successfully", info.BatchIdx+1, info.TotalBatches))
			log.Info().Int("batch", info.BatchIdx+1).Msg("Bulk evaluation completed successfully")
			return statusResp, nil
		}

		if status == ar_v3.FAILURE {
			errMsg := fmt.Sprintf("Batch %d/%d evaluation failed", info.BatchIdx+1, info.TotalBatches)
			if statusResp.JSON200.Data.Error != nil {
				errMsg = *statusResp.JSON200.Data.Error
			}
			ctx.P.Error(errMsg)
			log.Error().Str("error", errMsg).Int("batch", info.BatchIdx+1).Msg("Bulk evaluation failed")
			return nil, fmt.Errorf("%s", errMsg)
		}

		time.Sleep(2 * time.Second)
	}
}

func extractScanResults(statusResp *ar_v3.GetBulkScanEvaluationStatusResp, batchIdx int) []ScanResult {
	if statusResp == nil || statusResp.JSON200 == nil || statusResp.JSON200.Data == nil || statusResp.JSON200.Data.Scans == nil {
		return nil
	}

	scans := *statusResp.JSON200.Data.Scans
	log.Info().Int("count", len(scans)).Int("batch", batchIdx+1).Msg("Scan results received")

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
	return results
}

func DisplayResults(results []ScanResult, p *progress.ConsoleReporter) error {
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

	return printer.Print(results, 0, 1, int64(len(results)), false, [][]string{
		{"packageName", "Package Name"},
		{"version", "Version"},
		{"scanStatus", "Status"},
	})
}
