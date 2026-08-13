package command

import (
	"context"
	"errors"
	"net/http"
	"testing"

	ar_v3 "github.com/harness/harness-cli/internal/api/ar_v3"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// fakeV3Client is an in-memory stub for v3ClientLike. Tests wire up canned
// responses without touching the network.
type fakeV3Client struct {
	syncResp   *ar_v3.BulkScanEvaluateSyncResp
	syncErr    error
	syncCalls  int
	syncBodies []ar_v3.BulkScanEvaluateSyncJSONRequestBody

	initResp     *ar_v3.InitiateBulkScanEvaluationResp
	initErr      error
	pollResp     *ar_v3.GetBulkScanEvaluationStatusResp
	pollErr      error
	pollCalls    int
	pollSequence []*ar_v3.GetBulkScanEvaluationStatusResp // if set, returned in order
}

func (f *fakeV3Client) BulkScanEvaluateSyncWithResponse(ctx context.Context, params *ar_v3.BulkScanEvaluateSyncParams, body ar_v3.BulkScanEvaluateSyncJSONRequestBody, reqEditors ...ar_v3.RequestEditorFn) (*ar_v3.BulkScanEvaluateSyncResp, error) {
	f.syncCalls++
	f.syncBodies = append(f.syncBodies, body)
	return f.syncResp, f.syncErr
}

func (f *fakeV3Client) InitiateBulkScanEvaluationWithResponse(ctx context.Context, params *ar_v3.InitiateBulkScanEvaluationParams, body ar_v3.InitiateBulkScanEvaluationJSONRequestBody, reqEditors ...ar_v3.RequestEditorFn) (*ar_v3.InitiateBulkScanEvaluationResp, error) {
	return f.initResp, f.initErr
}

func (f *fakeV3Client) GetBulkScanEvaluationStatusWithResponse(ctx context.Context, evaluationId string, params *ar_v3.GetBulkScanEvaluationStatusParams, reqEditors ...ar_v3.RequestEditorFn) (*ar_v3.GetBulkScanEvaluationStatusResp, error) {
	f.pollCalls++
	if len(f.pollSequence) > 0 {
		i := f.pollCalls - 1
		if i >= len(f.pollSequence) {
			i = len(f.pollSequence) - 1
		}
		return f.pollSequence[i], nil
	}
	return f.pollResp, f.pollErr
}

func syncRespOK(items []ar_v3.ArtifactScanDetailsV3) *ar_v3.BulkScanEvaluateSyncResp {
	return &ar_v3.BulkScanEvaluateSyncResp{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &ar_v3.BulkScanEvaluateSyncResponse{
			Data: &items,
		},
	}
}

func packageTypePtr(t ar_v3.PackageType) *ar_v3.PackageType { return &t }

const testPackageTypeNPM ar_v3.PackageType = "NPM"

func TestChunkDependencies(t *testing.T) {
	tests := []struct {
		name      string
		deps      int
		batchSize int
		want      []int // expected batch sizes
	}{
		{"empty", 0, 20, []int{}},
		{"single-batch", 15, 20, []int{15}},
		{"exact-multiple", 40, 20, []int{20, 20}},
		{"remainder", 45, 20, []int{20, 20, 5}},
		{"one-per-batch", 3, 1, []int{1, 1, 1}},
		{"zero-batch-size-defaults", 25, 0, []int{20, 5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := make([]Dependency, tt.deps)
			for i := range deps {
				deps[i] = Dependency{Name: "p", Version: "1"}
			}
			batches := chunkDependencies(deps, tt.batchSize)
			got := make([]int, len(batches))
			for i, b := range batches {
				got[i] = len(b)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSyncEvaluator_HappyPath(t *testing.T) {
	regID := uuid.New()
	fake := &fakeV3Client{
		syncResp: syncRespOK([]ar_v3.ArtifactScanDetailsV3{
			{
				PackageName: "express",
				Version:     "4.18.2",
				ScanStatus:  ar_v3.ArtifactScanDetailsV3ScanStatusBLOCKED,
				PackageType: packageTypePtr(testPackageTypeNPM),
			},
			{
				PackageName: "lodash",
				Version:     "4.17.21",
				ScanStatus:  ar_v3.ArtifactScanDetailsV3ScanStatusALLOWED,
			},
		}),
	}
	e := &syncEvaluator{
		client: fake,
		params: evaluatorParams{registryUUID: regID, org: "o", project: "p", skipCacheParam: true},
	}
	batch := []Dependency{
		{Name: "express", Version: "4.18.2"},
		{Name: "lodash", Version: "4.17.21"},
	}
	res, err := e.Evaluate(context.Background(), batch, batchInfo{batchIdx: 0, totalBatches: 1, registryName: "r"})
	assert.NoError(t, err)
	assert.Len(t, res.Results, 2)
	assert.Equal(t, "BLOCKED", res.Results[0].ScanStatus)
	assert.Equal(t, "ALLOWED", res.Results[1].ScanStatus)
	assert.Equal(t, 1, fake.syncCalls)
	assert.NotNil(t, fake.syncBodies[0].SkipCache)
	assert.True(t, *fake.syncBodies[0].SkipCache)
	assert.Equal(t, regID, fake.syncBodies[0].RegistryId)
	// Details align with Results.
	assert.Len(t, res.Details, 2)
	assert.NotNil(t, res.Details[0])
	assert.Equal(t, "express", res.Details[0].PackageName)
}

func TestSyncEvaluator_MissingArtifactInResponse(t *testing.T) {
	// Server returns fewer items than requested — the missing one becomes UNKNOWN.
	fake := &fakeV3Client{
		syncResp: syncRespOK([]ar_v3.ArtifactScanDetailsV3{
			{PackageName: "express", Version: "4.18.2", ScanStatus: ar_v3.ArtifactScanDetailsV3ScanStatusALLOWED},
		}),
	}
	e := &syncEvaluator{client: fake, params: evaluatorParams{registryUUID: uuid.New()}}
	batch := []Dependency{
		{Name: "express", Version: "4.18.2"},
		{Name: "missing", Version: "1.0.0"},
	}
	res, err := e.Evaluate(context.Background(), batch, batchInfo{})
	assert.NoError(t, err)
	assert.Len(t, res.Results, 2)
	assert.Equal(t, "ALLOWED", res.Results[0].ScanStatus)
	assert.Equal(t, "UNKNOWN", res.Results[1].ScanStatus)
	assert.Equal(t, "missing", res.Results[1].PackageName)
	assert.Nil(t, res.Details[1])
}

func TestSyncEvaluator_HTTPError(t *testing.T) {
	fake := &fakeV3Client{syncErr: errors.New("boom")}
	e := &syncEvaluator{client: fake, params: evaluatorParams{registryUUID: uuid.New()}}
	batch := []Dependency{{Name: "x", Version: "1"}}
	res, err := e.Evaluate(context.Background(), batch, batchInfo{batchIdx: 0, totalBatches: 1})
	assert.Error(t, err)
	assert.Empty(t, res.Results)
}

func TestSyncEvaluator_Non200Status(t *testing.T) {
	fake := &fakeV3Client{
		syncResp: &ar_v3.BulkScanEvaluateSyncResp{
			HTTPResponse: &http.Response{StatusCode: http.StatusInternalServerError},
		},
	}
	e := &syncEvaluator{client: fake, params: evaluatorParams{registryUUID: uuid.New()}}
	res, err := e.Evaluate(context.Background(), []Dependency{{Name: "x", Version: "1"}}, batchInfo{})
	assert.Error(t, err)
	assert.Empty(t, res.Results)
}

func TestAsyncEvaluator_HappyPath(t *testing.T) {
	evalID := "eval-1"
	scanID := uuid.New()
	success := ar_v3.SUCCESS
	fake := &fakeV3Client{
		initResp: &ar_v3.InitiateBulkScanEvaluationResp{
			HTTPResponse: &http.Response{StatusCode: http.StatusAccepted},
			JSON202: &ar_v3.BulkScanEvaluationAccepted{
				Data: &ar_v3.BulkScanEvaluationAcceptedData{EvaluationId: &evalID},
			},
		},
		pollResp: &ar_v3.GetBulkScanEvaluationStatusResp{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &ar_v3.BulkScanEvaluationStatusResponse{
				Data: &ar_v3.BulkScanEvaluationStatusData{
					Status: &success,
					Scans: &[]ar_v3.BulkScanResultItem{
						{
							PackageName: stringPtr("express"),
							Version:     stringPtr("4.18.2"),
							ScanId:      &scanID,
							ScanStatus:  scanStatusPtr(ar_v3.BulkScanResultItemScanStatusBLOCKED),
						},
					},
				},
			},
		},
	}
	e := &asyncEvaluator{client: fake, params: evaluatorParams{registryUUID: uuid.New()}}
	res, err := e.Evaluate(context.Background(), []Dependency{{Name: "express", Version: "4.18.2"}}, batchInfo{})
	assert.NoError(t, err)
	assert.Len(t, res.Results, 1)
	assert.Equal(t, "BLOCKED", res.Results[0].ScanStatus)
}

func TestUnknownResults(t *testing.T) {
	batch := []Dependency{
		{Name: "a", Version: "1"},
		{Name: "b", Version: "2"},
	}
	got := unknownResults(batch)
	assert.Len(t, got, 2)
	for _, r := range got {
		assert.Equal(t, "UNKNOWN", r.ScanStatus)
	}
}
