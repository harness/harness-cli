package command

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	ar_v3 "github.com/harness/harness-cli/internal/api/ar_v3"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// pollInterval matches the previous async cadence (see original fw_audit poll loop).
const pollInterval = 2 * time.Second

// asyncMaxPolls caps the async wait per batch at ~4 minutes total.
const asyncMaxPolls = 120

// Evaluator abstracts the two firewall bulk-evaluation paths (sync vs async).
// Implementations return one ScanResult per input dependency, in the same order.
// A batch-level error means "the whole batch is unusable"; the caller decides
// whether to mark those deps UNKNOWN or fail the run.
type Evaluator interface {
	Evaluate(ctx context.Context, batch []Dependency, info batchInfo) (batchResult, error)
	MaxBatchSize() int
	Mode() string // "sync" or "async"
}

// progressReporter is the minimum surface asyncEvaluator uses to keep the
// original per-step chatter (initiate → wait → complete) visible to users on
// long polls. Kept small so tests can pass nil or a stub freely.
type progressReporter interface {
	Step(string)
	Success(string)
}

// batchResult carries per-artifact rows plus optional detail bodies for fw explain.
type batchResult struct {
	Results []ScanResult
	// Details is populated by syncEvaluator; nil for asyncEvaluator. Index-aligned
	// with Results. Used by fw explain for the single-artifact detail render.
	Details []*ar_v3.ArtifactScanDetailsV3
}

// v3ClientLike is the minimum surface the evaluators need from the generated
// ar_v3 client. Extracting an interface lets tests inject fakes without spinning
// a full HTTP server.
type v3ClientLike interface {
	BulkScanEvaluateSyncWithResponse(ctx context.Context, params *ar_v3.BulkScanEvaluateSyncParams, body ar_v3.BulkScanEvaluateSyncJSONRequestBody, reqEditors ...ar_v3.RequestEditorFn) (*ar_v3.BulkScanEvaluateSyncResp, error)
	InitiateBulkScanEvaluationWithResponse(ctx context.Context, params *ar_v3.InitiateBulkScanEvaluationParams, body ar_v3.InitiateBulkScanEvaluationJSONRequestBody, reqEditors ...ar_v3.RequestEditorFn) (*ar_v3.InitiateBulkScanEvaluationResp, error)
	GetBulkScanEvaluationStatusWithResponse(ctx context.Context, evaluationId string, params *ar_v3.GetBulkScanEvaluationStatusParams, reqEditors ...ar_v3.RequestEditorFn) (*ar_v3.GetBulkScanEvaluationStatusResp, error)
}

// Compile-time assertion: the generated client satisfies our interface.
var _ v3ClientLike = (*ar_v3.ClientWithResponses)(nil)

type evaluatorParams struct {
	registryUUID   uuid.UUID
	org            string
	project        string
	registryName   string
	skipCacheParam bool // sync only; sent as `skipCache` on the request body
}

// newEvaluator returns the sync (default) or async evaluator.
// `reporter` is only used by the async evaluator to preserve the original
// per-step chatter (initiate → poll → complete). Sync mode ignores it.
func newEvaluator(f *cmdutils.Factory, useAsync bool, p evaluatorParams, reporter progressReporter) Evaluator {
	if useAsync {
		return &asyncEvaluator{client: f.RegistryV3HttpClient(), params: p, reporter: reporter}
	}
	return &syncEvaluator{client: f.RegistryV3HttpClient(), params: p}
}

// ---------- syncEvaluator -----------------------------------------------------

type syncEvaluator struct {
	client v3ClientLike
	params evaluatorParams
}

func (e *syncEvaluator) MaxBatchSize() int { return 50 }
func (e *syncEvaluator) Mode() string      { return "sync" }

func (e *syncEvaluator) Evaluate(ctx context.Context, batch []Dependency, info batchInfo) (batchResult, error) {
	log.Info().Str("registry", info.registryName).Int("batch", info.batchIdx+1).
		Int("totalBatches", info.totalBatches).Int("batchSize", len(batch)).
		Msg("Sync bulk evaluation")

	artifacts := make([]ar_v3.ArtifactScanInput, 0, len(batch))
	for _, dep := range batch {
		artifacts = append(artifacts, ar_v3.ArtifactScanInput{
			PackageName: dep.Name,
			Version:     dep.Version,
		})
	}

	params := &ar_v3.BulkScanEvaluateSyncParams{
		AccountIdentifier: config.Global.AccountID,
		OrgIdentifier:     &e.params.org,
		ProjectIdentifier: &e.params.project,
	}
	skipCache := e.params.skipCacheParam

	resp, err := e.client.BulkScanEvaluateSyncWithResponse(ctx, params, ar_v3.BulkScanEvaluateSyncJSONRequestBody{
		RegistryId: e.params.registryUUID,
		Artifacts:  artifacts,
		SkipCache:  &skipCache,
	})
	if err != nil {
		return batchResult{}, fmt.Errorf("sync evaluate batch %d: %w", info.batchIdx+1, err)
	}
	if resp.StatusCode() != http.StatusOK {
		msg := fmt.Sprintf("sync evaluate batch %d: status %d", info.batchIdx+1, resp.StatusCode())
		if resp.JSONDefault != nil && resp.JSONDefault.Error.Message != nil {
			msg = *resp.JSONDefault.Error.Message
		}
		return batchResult{}, fmt.Errorf("%s", msg)
	}
	if resp.JSON200 == nil || resp.JSON200.Data == nil {
		return batchResult{}, fmt.Errorf("sync evaluate batch %d: empty response body", info.batchIdx+1)
	}

	// Server preserves input order but we align by (name, version) defensively.
	type key struct{ name, version string }
	byKey := make(map[key]*ar_v3.ArtifactScanDetailsV3, len(*resp.JSON200.Data))
	for i := range *resp.JSON200.Data {
		item := &(*resp.JSON200.Data)[i]
		byKey[key{item.PackageName, item.Version}] = item
	}

	results := make([]ScanResult, 0, len(batch))
	details := make([]*ar_v3.ArtifactScanDetailsV3, 0, len(batch))
	for _, dep := range batch {
		item := byKey[key{dep.Name, dep.Version}]
		if item == nil {
			results = append(results, ScanResult{
				PackageName: dep.Name,
				Version:     dep.Version,
				ScanStatus:  "UNKNOWN",
			})
			details = append(details, nil)
			continue
		}
		scanID := ""
		if item.Id != nil {
			scanID = item.Id.String()
		}
		results = append(results, ScanResult{
			PackageName: item.PackageName,
			Version:     item.Version,
			ScanID:      scanID,
			ScanStatus:  string(item.ScanStatus),
		})
		details = append(details, item)
	}

	return batchResult{Results: results, Details: details}, nil
}

// ---------- asyncEvaluator ----------------------------------------------------

type asyncEvaluator struct {
	client   v3ClientLike
	params   evaluatorParams
	reporter progressReporter // may be nil in tests; guarded via step/success helpers
}

func (e *asyncEvaluator) MaxBatchSize() int { return 50 }
func (e *asyncEvaluator) Mode() string      { return "async" }

func (e *asyncEvaluator) step(msg string) {
	if e.reporter != nil {
		e.reporter.Step(msg)
	}
}
func (e *asyncEvaluator) success(msg string) {
	if e.reporter != nil {
		e.reporter.Success(msg)
	}
}

func (e *asyncEvaluator) Evaluate(ctx context.Context, batch []Dependency, info batchInfo) (batchResult, error) {
	e.step(fmt.Sprintf("Processing batch %d/%d (%d packages) for registry: %s",
		info.batchIdx+1, info.totalBatches, len(batch), info.registryName))
	evalID, err := e.initiate(ctx, batch, info)
	if err != nil {
		return batchResult{}, err
	}
	e.success(fmt.Sprintf("Batch %d/%d evaluation initiated with ID: %s",
		info.batchIdx+1, info.totalBatches, evalID))

	e.step(fmt.Sprintf("Waiting for batch %d/%d evaluation to complete",
		info.batchIdx+1, info.totalBatches))
	statusResp, err := e.poll(ctx, evalID, info)
	if err != nil {
		return batchResult{}, err
	}
	return batchResult{Results: extractScanResults(statusResp, info.batchIdx)}, nil
}

func (e *asyncEvaluator) initiate(ctx context.Context, batch []Dependency, info batchInfo) (string, error) {
	artifacts := make([]ar_v3.ArtifactScanInput, 0, len(batch))
	for _, dep := range batch {
		artifacts = append(artifacts, ar_v3.ArtifactScanInput{
			PackageName: dep.Name,
			Version:     dep.Version,
		})
	}
	params := &ar_v3.InitiateBulkScanEvaluationParams{
		AccountIdentifier: config.Global.AccountID,
		OrgIdentifier:     &e.params.org,
		ProjectIdentifier: &e.params.project,
	}
	resp, err := e.client.InitiateBulkScanEvaluationWithResponse(ctx, params, ar_v3.InitiateBulkScanEvaluationJSONRequestBody{
		RegistryId: e.params.registryUUID,
		Artifacts:  artifacts,
	})
	if err != nil {
		return "", fmt.Errorf("initiate batch %d: %w", info.batchIdx+1, err)
	}
	if resp.StatusCode() != http.StatusAccepted {
		msg := fmt.Sprintf("initiate batch %d: status %d", info.batchIdx+1, resp.StatusCode())
		if resp.JSONDefault != nil && resp.JSONDefault.Error.Message != nil {
			msg = *resp.JSONDefault.Error.Message
		}
		return "", fmt.Errorf("%s", msg)
	}
	if resp.JSON202 == nil || resp.JSON202.Data == nil || resp.JSON202.Data.EvaluationId == nil {
		return "", fmt.Errorf("initiate batch %d: missing evaluationId", info.batchIdx+1)
	}
	return *resp.JSON202.Data.EvaluationId, nil
}

func (e *asyncEvaluator) poll(ctx context.Context, evaluationID string, info batchInfo) (*ar_v3.GetBulkScanEvaluationStatusResp, error) {
	params := &ar_v3.GetBulkScanEvaluationStatusParams{
		AccountIdentifier: config.Global.AccountID,
		OrgIdentifier:     &e.params.org,
		ProjectIdentifier: &e.params.project,
	}
	for i := 0; i < asyncMaxPolls; i++ {
		resp, err := e.client.GetBulkScanEvaluationStatusWithResponse(ctx, evaluationID, params)
		if err != nil {
			return nil, fmt.Errorf("poll batch %d: %w", info.batchIdx+1, err)
		}
		if resp.StatusCode() != http.StatusOK {
			msg := fmt.Sprintf("poll batch %d: status %d", info.batchIdx+1, resp.StatusCode())
			if resp.JSONDefault != nil && resp.JSONDefault.Error.Message != nil {
				msg = *resp.JSONDefault.Error.Message
			}
			return nil, fmt.Errorf("%s", msg)
		}
		if resp.JSON200 == nil || resp.JSON200.Data == nil || resp.JSON200.Data.Status == nil {
			return nil, fmt.Errorf("poll batch %d: malformed body", info.batchIdx+1)
		}
		status := *resp.JSON200.Data.Status
		log.Debug().Str("status", string(status)).Int("poll", i+1).
			Int("batch", info.batchIdx+1).Msg("Bulk evaluation status")
		switch status {
		case ar_v3.SUCCESS:
			return resp, nil
		case ar_v3.FAILURE:
			msg := fmt.Sprintf("batch %d evaluation FAILURE", info.batchIdx+1)
			if resp.JSON200.Data.Error != nil {
				msg = *resp.JSON200.Data.Error
			}
			return nil, fmt.Errorf("%s", msg)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
	return nil, fmt.Errorf("poll batch %d: timeout after %d polls", info.batchIdx+1, asyncMaxPolls)
}
