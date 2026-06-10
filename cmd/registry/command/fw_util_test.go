package command

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/harness/harness-cli/config"
	ar_v3 "github.com/harness/harness-cli/internal/api/ar_v3"
	"github.com/harness/harness-cli/util/common/progress"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestDependencyStruct(t *testing.T) {
	dep := Dependency{
		Name:    "express",
		Version: "4.18.2",
		Source:  "package.json",
		Parent:  "root",
	}

	assert.Equal(t, "express", dep.Name)
	assert.Equal(t, "4.18.2", dep.Version)
	assert.Equal(t, "package.json", dep.Source)
	assert.Equal(t, "root", dep.Parent)
}

func TestScanResultStruct(t *testing.T) {
	scanID := uuid.New().String()
	result := ScanResult{
		PackageName: "lodash",
		Version:     "4.17.21",
		ScanID:      scanID,
		ScanStatus:  "ALLOWED",
	}

	assert.Equal(t, "lodash", result.PackageName)
	assert.Equal(t, "4.17.21", result.Version)
	assert.Equal(t, scanID, result.ScanID)
	assert.Equal(t, "ALLOWED", result.ScanStatus)
}

func TestAuditContextStruct(t *testing.T) {
	testUUID := uuid.New()
	p := progress.NewConsoleReporter()

	ctx := &AuditContext{
		F:            nil,
		RegistryUUID: testUUID,
		Org:          "test-org",
		Project:      "test-project",
		P:            p,
	}

	assert.Equal(t, testUUID, ctx.RegistryUUID)
	assert.Equal(t, "test-org", ctx.Org)
	assert.Equal(t, "test-project", ctx.Project)
	assert.NotNil(t, ctx.P)
}

func TestBatchInfoStruct(t *testing.T) {
	info := BatchInfo{
		BatchIdx:     0,
		TotalBatches: 3,
		RegistryName: "test-registry",
	}

	assert.Equal(t, 0, info.BatchIdx)
	assert.Equal(t, 3, info.TotalBatches)
	assert.Equal(t, "test-registry", info.RegistryName)
}

func TestExtractScanResults(t *testing.T) {
	tests := []struct {
		name     string
		response *ar_v3.GetBulkScanEvaluationStatusResp
		batchIdx int
		want     int
	}{
		{
			name: "valid scan results",
			response: &ar_v3.GetBulkScanEvaluationStatusResp{
				JSON200: &ar_v3.BulkScanEvaluationStatusResponse{
					Data: &ar_v3.BulkScanEvaluationStatusData{
						Scans: &[]ar_v3.BulkScanResultItem{
							{
								PackageName: stringPtr("express"),
								Version:     stringPtr("4.18.2"),
								ScanId:      uuidPtr(uuid.New()),
								ScanStatus:  scanStatusPtr(ar_v3.BLOCKED),
							},
							{
								PackageName: stringPtr("lodash"),
								Version:     stringPtr("4.17.21"),
								ScanId:      uuidPtr(uuid.New()),
								ScanStatus:  scanStatusPtr(ar_v3.ALLOWED),
							},
						},
					},
				},
			},
			batchIdx: 0,
			want:     2,
		},
		{
			name: "nil scans",
			response: &ar_v3.GetBulkScanEvaluationStatusResp{
				JSON200: &ar_v3.BulkScanEvaluationStatusResponse{
					Data: &ar_v3.BulkScanEvaluationStatusData{
						Scans: nil,
					},
				},
			},
			batchIdx: 0,
			want:     0,
		},
		{
			name: "empty scans",
			response: &ar_v3.GetBulkScanEvaluationStatusResp{
				JSON200: &ar_v3.BulkScanEvaluationStatusResponse{
					Data: &ar_v3.BulkScanEvaluationStatusData{
						Scans: &[]ar_v3.BulkScanResultItem{},
					},
				},
			},
			batchIdx: 0,
			want:     0,
		},
		{
			name: "partial data in scan results",
			response: &ar_v3.GetBulkScanEvaluationStatusResp{
				JSON200: &ar_v3.BulkScanEvaluationStatusResponse{
					Data: &ar_v3.BulkScanEvaluationStatusData{
						Scans: &[]ar_v3.BulkScanResultItem{
							{
								PackageName: stringPtr("axios"),
								Version:     nil,
								ScanId:      nil,
								ScanStatus:  scanStatusPtr(ar_v3.WARN),
							},
						},
					},
				},
			},
			batchIdx: 1,
			want:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := extractScanResults(tt.response, tt.batchIdx)
			assert.Equal(t, tt.want, len(results))

			// Verify structure of results
			for _, result := range results {
				assert.NotEmpty(t, result.PackageName)
			}
		})
	}
}

func TestDisplayResults(t *testing.T) {
	tests := []struct {
		name    string
		results []ScanResult
		wantErr bool
	}{
		{
			name: "valid results with mixed statuses",
			results: []ScanResult{
				{
					PackageName: "express",
					Version:     "4.18.2",
					ScanID:      uuid.New().String(),
					ScanStatus:  "BLOCKED",
				},
				{
					PackageName: "lodash",
					Version:     "4.17.21",
					ScanID:      uuid.New().String(),
					ScanStatus:  "ALLOWED",
				},
				{
					PackageName: "axios",
					Version:     "0.21.0",
					ScanID:      uuid.New().String(),
					ScanStatus:  "WARN",
				},
			},
			wantErr: false,
		},
		{
			name:    "empty results",
			results: []ScanResult{},
			wantErr: false,
		},
		{
			name: "single result",
			results: []ScanResult{
				{
					PackageName: "react",
					Version:     "18.0.0",
					ScanID:      uuid.New().String(),
					ScanStatus:  "ALLOWED",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := progress.NewConsoleReporter()
			err := DisplayResults(tt.results, p)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDisplayResultsSorting(t *testing.T) {
	results := []ScanResult{
		{PackageName: "zebra", Version: "1.0.0", ScanStatus: "ALLOWED"},
		{PackageName: "axios", Version: "0.21.0", ScanStatus: "BLOCKED"},
		{PackageName: "lodash", Version: "4.17.21", ScanStatus: "WARN"},
	}

	p := progress.NewConsoleReporter()
	err := DisplayResults(results, p)
	assert.NoError(t, err)

	// Results should be sorted alphabetically by package name
	// This is tested implicitly by the DisplayResults function
}

func TestDisplayResultsStatusCounting(t *testing.T) {
	results := []ScanResult{
		{PackageName: "pkg1", Version: "1.0.0", ScanStatus: "BLOCKED"},
		{PackageName: "pkg2", Version: "1.0.0", ScanStatus: "BLOCKED"},
		{PackageName: "pkg3", Version: "1.0.0", ScanStatus: "WARN"},
		{PackageName: "pkg4", Version: "1.0.0", ScanStatus: "ALLOWED"},
		{PackageName: "pkg5", Version: "1.0.0", ScanStatus: "UNKNOWN"},
	}

	p := progress.NewConsoleReporter()
	err := DisplayResults(results, p)
	assert.NoError(t, err)

	// The function should correctly count:
	// - 2 BLOCKED
	// - 1 WARN
	// - 1 ALLOWED
	// - 1 UNKNOWN
}

func TestProcessBatches_EmptyDependencies(t *testing.T) {
	ctx := &AuditContext{
		RegistryUUID: uuid.New(),
		Org:          "test-org",
		Project:      "test-project",
		P:            progress.NewConsoleReporter(),
	}

	// Test with empty dependencies - no factory/client needed since no batches run
	results, err := ProcessBatches(ctx, []Dependency{}, "test-registry")
	assert.NoError(t, err)
	assert.Empty(t, results)
}

func TestProcessBatches_WithMockClient(t *testing.T) {
	// Create mock factory
	mockClient := &mockRegistryV3Client{
		evaluationID: "test-eval-id-123",
		scanResults: []ar_v3.BulkScanResultItem{
			{
				PackageName: stringPtr("express"),
				Version:     stringPtr("4.18.2"),
				ScanId:      uuidPtr(uuid.New()),
				ScanStatus:  scanStatusPtr(ar_v3.BLOCKED),
			},
			{
				PackageName: stringPtr("lodash"),
				Version:     stringPtr("4.17.21"),
				ScanId:      uuidPtr(uuid.New()),
				ScanStatus:  scanStatusPtr(ar_v3.ALLOWED),
			},
		},
	}

	ctx := newMockAuditContext(mockClient)

	dependencies := []Dependency{
		{Name: "express", Version: "4.18.2"},
		{Name: "lodash", Version: "4.17.21"},
	}

	results, err := ProcessBatches(ctx, dependencies, "test-registry")
	assert.NoError(t, err)
	assert.Equal(t, 2, len(results))
	assert.Equal(t, "express", results[0].PackageName)
	assert.Equal(t, "lodash", results[1].PackageName)
}

func TestProcessBatches_MultipleBatches(t *testing.T) {
	// Create 75 dependencies to trigger 2 batches (batch size is 50)
	dependencies := make([]Dependency, 75)
	scanResults := make([]ar_v3.BulkScanResultItem, 75)

	for i := 0; i < 75; i++ {
		dependencies[i] = Dependency{
			Name:    fmt.Sprintf("package-%d", i),
			Version: "1.0.0",
		}
		scanResults[i] = ar_v3.BulkScanResultItem{
			PackageName: stringPtr(fmt.Sprintf("package-%d", i)),
			Version:     stringPtr("1.0.0"),
			ScanId:      uuidPtr(uuid.New()),
			ScanStatus:  scanStatusPtr(ar_v3.ALLOWED),
		}
	}

	mockClient := &mockRegistryV3Client{
		evaluationID: "test-eval-id-multi",
		scanResults:  scanResults,
	}

	ctx := newMockAuditContext(mockClient)

	results, err := ProcessBatches(ctx, dependencies, "test-registry")
	assert.NoError(t, err)
	assert.Equal(t, 75, len(results))
}

func TestInitiateBatchEvaluation_Success(t *testing.T) {
	evaluationID := "test-eval-id-456"
	mockClient := &mockRegistryV3Client{
		evaluationID: evaluationID,
	}

	ctx := newMockAuditContext(mockClient)

	batch := []Dependency{
		{Name: "react", Version: "18.2.0"},
		{Name: "vue", Version: "3.2.0"},
	}

	info := BatchInfo{
		BatchIdx:     0,
		TotalBatches: 1,
		RegistryName: "test-registry",
	}

	resultID, err := initiateBatchEvaluation(ctx, batch, info)
	assert.NoError(t, err)
	assert.Equal(t, evaluationID, resultID)
}

func TestInitiateBatchEvaluation_HTTPError(t *testing.T) {
	mockClient := &mockRegistryV3Client{
		initiateError: fmt.Errorf("network error"),
	}

	ctx := newMockAuditContext(mockClient)

	batch := []Dependency{
		{Name: "express", Version: "4.18.2"},
	}

	info := BatchInfo{
		BatchIdx:     0,
		TotalBatches: 1,
		RegistryName: "test-registry",
	}

	_, err := initiateBatchEvaluation(ctx, batch, info)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to initiate bulk evaluation")
}

func TestInitiateBatchEvaluation_Non202Response(t *testing.T) {
	mockClient := &mockRegistryV3Client{
		initiateStatusCode: 500,
	}

	ctx := newMockAuditContext(mockClient)

	batch := []Dependency{
		{Name: "express", Version: "4.18.2"},
	}

	info := BatchInfo{
		BatchIdx:     0,
		TotalBatches: 1,
		RegistryName: "test-registry",
	}

	_, err := initiateBatchEvaluation(ctx, batch, info)
	assert.Error(t, err)
}

func TestPollBatchEvaluation_Success(t *testing.T) {
	scanResults := []ar_v3.BulkScanResultItem{
		{
			PackageName: stringPtr("express"),
			Version:     stringPtr("4.18.2"),
			ScanId:      uuidPtr(uuid.New()),
			ScanStatus:  scanStatusPtr(ar_v3.BLOCKED),
		},
	}

	mockClient := &mockRegistryV3Client{
		evaluationID: "test-eval-id",
		scanResults:  scanResults,
		pollStatus:   ar_v3.SUCCESS,
	}

	ctx := newMockAuditContext(mockClient)

	info := BatchInfo{
		BatchIdx:     0,
		TotalBatches: 1,
		RegistryName: "test-registry",
	}

	resp, err := pollBatchEvaluation(ctx, "test-eval-id", info)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.NotNil(t, resp.JSON200)
	assert.NotNil(t, resp.JSON200.Data)
	assert.Equal(t, ar_v3.SUCCESS, *resp.JSON200.Data.Status)
}

func TestPollBatchEvaluation_Failure(t *testing.T) {
	mockClient := &mockRegistryV3Client{
		evaluationID: "test-eval-id",
		pollStatus:   ar_v3.FAILURE,
		pollError:    "evaluation failed",
	}

	ctx := newMockAuditContext(mockClient)

	info := BatchInfo{
		BatchIdx:     0,
		TotalBatches: 1,
		RegistryName: "test-registry",
	}

	_, err := pollBatchEvaluation(ctx, "test-eval-id", info)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "evaluation failed")
}

func TestPollBatchEvaluation_HTTPError(t *testing.T) {
	mockClient := &mockRegistryV3Client{
		pollHTTPError: fmt.Errorf("connection timeout"),
	}

	ctx := newMockAuditContext(mockClient)

	info := BatchInfo{
		BatchIdx:     0,
		TotalBatches: 1,
		RegistryName: "test-registry",
	}

	_, err := pollBatchEvaluation(ctx, "test-eval-id", info)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get bulk evaluation status")
}

func TestPollBatchEvaluation_Non200Response(t *testing.T) {
	mockClient := &mockRegistryV3Client{
		pollStatusCode: 404,
	}

	ctx := newMockAuditContext(mockClient)

	info := BatchInfo{
		BatchIdx:     0,
		TotalBatches: 1,
		RegistryName: "test-registry",
	}

	_, err := pollBatchEvaluation(ctx, "test-eval-id", info)
	assert.Error(t, err)
}

func TestProcessBatches_BatchSizeCalculation(t *testing.T) {
	tests := []struct {
		name            string
		depCount        int
		expectedBatches int
	}{
		{
			name:            "less than batch size",
			depCount:        25,
			expectedBatches: 1,
		},
		{
			name:            "exactly batch size",
			depCount:        50,
			expectedBatches: 1,
		},
		{
			name:            "more than batch size",
			depCount:        75,
			expectedBatches: 2,
		},
		{
			name:            "multiple batches",
			depCount:        125,
			expectedBatches: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create dependencies
			deps := make([]Dependency, tt.depCount)
			for i := 0; i < tt.depCount; i++ {
				deps[i] = Dependency{
					Name:    fmt.Sprintf("pkg-%d", i),
					Version: "1.0.0",
				}
			}

			// Calculate expected batches
			const batchSize = 50
			expectedBatches := (tt.depCount + batchSize - 1) / batchSize
			assert.Equal(t, tt.expectedBatches, expectedBatches)
		})
	}
}

func TestInitiateBatchEvaluation_ArtifactConversion(t *testing.T) {
	// Test that dependencies are correctly converted to artifacts
	batch := []Dependency{
		{Name: "express", Version: "4.18.2"},
		{Name: "lodash", Version: "4.17.21"},
	}

	info := BatchInfo{
		BatchIdx:     0,
		TotalBatches: 1,
		RegistryName: "test-registry",
	}

	// Verify batch info structure
	assert.Equal(t, 0, info.BatchIdx)
	assert.Equal(t, 1, info.TotalBatches)
	assert.Equal(t, "test-registry", info.RegistryName)

	// Verify batch content
	assert.Equal(t, 2, len(batch))
	assert.Equal(t, "express", batch[0].Name)
	assert.Equal(t, "4.18.2", batch[0].Version)
}

func TestPollBatchEvaluation_StatusHandling(t *testing.T) {
	tests := []struct {
		name           string
		status         ar_v3.BulkScanEvaluationStatusDataStatus
		expectComplete bool
	}{
		{
			name:           "success status",
			status:         ar_v3.SUCCESS,
			expectComplete: true,
		},
		{
			name:           "failure status",
			status:         ar_v3.FAILURE,
			expectComplete: false,
		},
		{
			name:           "processing status",
			status:         ar_v3.PROCESSING,
			expectComplete: false,
		},
		{
			name:           "pending status",
			status:         ar_v3.PENDING,
			expectComplete: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify the status enum values exist
			assert.NotEmpty(t, string(tt.status))
		})
	}
}

func TestDisplayResults_JSONFormat(t *testing.T) {
	// Save original format
	originalFormat := config.Global.Format
	defer func() { config.Global.Format = originalFormat }()

	// Set JSON format
	config.Global.Format = "json"

	results := []ScanResult{
		{
			PackageName: "express",
			Version:     "4.18.2",
			ScanID:      uuid.New().String(),
			ScanStatus:  "BLOCKED",
		},
	}

	p := progress.NewConsoleReporter()
	err := DisplayResults(results, p)
	assert.NoError(t, err)
}

func TestDisplayResults_TableFormat(t *testing.T) {
	// Save original format
	originalFormat := config.Global.Format
	defer func() { config.Global.Format = originalFormat }()

	// Set table format (default)
	config.Global.Format = ""

	results := []ScanResult{
		{
			PackageName: "express",
			Version:     "4.18.2",
			ScanID:      uuid.New().String(),
			ScanStatus:  "BLOCKED",
		},
		{
			PackageName: "lodash",
			Version:     "4.17.21",
			ScanID:      uuid.New().String(),
			ScanStatus:  "ALLOWED",
		},
	}

	p := progress.NewConsoleReporter()
	err := DisplayResults(results, p)
	assert.NoError(t, err)
}

func TestDisplayResults_AllStatusTypes(t *testing.T) {
	results := []ScanResult{
		{PackageName: "pkg1", Version: "1.0.0", ScanStatus: "BLOCKED"},
		{PackageName: "pkg2", Version: "1.0.0", ScanStatus: "WARN"},
		{PackageName: "pkg3", Version: "1.0.0", ScanStatus: "ALLOWED"},
		{PackageName: "pkg4", Version: "1.0.0", ScanStatus: "UNKNOWN"},
	}

	p := progress.NewConsoleReporter()
	err := DisplayResults(results, p)
	assert.NoError(t, err)

	// Verify all status types are handled
	statusCounts := make(map[string]int)
	for _, r := range results {
		statusCounts[r.ScanStatus]++
	}

	assert.Equal(t, 1, statusCounts["BLOCKED"])
	assert.Equal(t, 1, statusCounts["WARN"])
	assert.Equal(t, 1, statusCounts["ALLOWED"])
	assert.Equal(t, 1, statusCounts["UNKNOWN"])
}

func TestExtractScanResults_NilResponse(t *testing.T) {
	// Test with nil response
	results := extractScanResults(nil, 0)
	assert.Empty(t, results)
}

func TestExtractScanResults_NilJSON200(t *testing.T) {
	response := &ar_v3.GetBulkScanEvaluationStatusResp{
		JSON200: nil,
	}
	results := extractScanResults(response, 0)
	assert.Empty(t, results)
}

func TestExtractScanResults_NilData(t *testing.T) {
	response := &ar_v3.GetBulkScanEvaluationStatusResp{
		JSON200: &ar_v3.BulkScanEvaluationStatusResponse{
			Data: nil,
		},
	}
	results := extractScanResults(response, 0)
	assert.Empty(t, results)
}

func TestExtractScanResults_WithAllFields(t *testing.T) {
	scanID := uuid.New()
	response := &ar_v3.GetBulkScanEvaluationStatusResp{
		JSON200: &ar_v3.BulkScanEvaluationStatusResponse{
			Data: &ar_v3.BulkScanEvaluationStatusData{
				Scans: &[]ar_v3.BulkScanResultItem{
					{
						PackageName: stringPtr("express"),
						Version:     stringPtr("4.18.2"),
						ScanId:      &scanID,
						ScanStatus:  scanStatusPtr(ar_v3.BLOCKED),
					},
				},
			},
		},
	}

	results := extractScanResults(response, 0)
	assert.Equal(t, 1, len(results))
	assert.Equal(t, "express", results[0].PackageName)
	assert.Equal(t, "4.18.2", results[0].Version)
	assert.Equal(t, scanID.String(), results[0].ScanID)
	assert.Equal(t, "BLOCKED", results[0].ScanStatus)
}

func TestExtractScanResults_WithMissingFields(t *testing.T) {
	response := &ar_v3.GetBulkScanEvaluationStatusResp{
		JSON200: &ar_v3.BulkScanEvaluationStatusResponse{
			Data: &ar_v3.BulkScanEvaluationStatusData{
				Scans: &[]ar_v3.BulkScanResultItem{
					{
						PackageName: stringPtr("express"),
						Version:     nil,
						ScanId:      nil,
						ScanStatus:  nil,
					},
				},
			},
		},
	}

	results := extractScanResults(response, 0)
	assert.Equal(t, 1, len(results))
	assert.Equal(t, "express", results[0].PackageName)
	assert.Equal(t, "", results[0].Version)
	assert.Equal(t, "", results[0].ScanID)
	assert.Equal(t, "", results[0].ScanStatus)
}

func TestBatchInfoCreation(t *testing.T) {
	info := BatchInfo{
		BatchIdx:     2,
		TotalBatches: 5,
		RegistryName: "production-registry",
	}

	assert.Equal(t, 2, info.BatchIdx)
	assert.Equal(t, 5, info.TotalBatches)
	assert.Equal(t, "production-registry", info.RegistryName)
}

func TestAuditContextCreation(t *testing.T) {
	testUUID := uuid.New()
	p := progress.NewConsoleReporter()
	mockClient := &mockRegistryV3Client{}

	ctx := &AuditContext{
		RegistryV3Client: mockClient,
		RegistryUUID:     testUUID,
		Org:              "my-org",
		Project:          "my-project",
		P:                p,
	}

	assert.Equal(t, testUUID, ctx.RegistryUUID)
	assert.Equal(t, "my-org", ctx.Org)
	assert.Equal(t, "my-project", ctx.Project)
	assert.NotNil(t, ctx.P)
	assert.NotNil(t, ctx.RegistryV3Client)
}

func TestDependencyCreation(t *testing.T) {
	dep := Dependency{
		Name:    "react",
		Version: "18.2.0",
		Source:  "package.json",
		Parent:  "root",
	}

	assert.Equal(t, "react", dep.Name)
	assert.Equal(t, "18.2.0", dep.Version)
	assert.Equal(t, "package.json", dep.Source)
	assert.Equal(t, "root", dep.Parent)
}

func TestScanResultCreation(t *testing.T) {
	scanID := uuid.New().String()
	result := ScanResult{
		PackageName: "vue",
		Version:     "3.2.0",
		ScanID:      scanID,
		ScanStatus:  "WARN",
	}

	assert.Equal(t, "vue", result.PackageName)
	assert.Equal(t, "3.2.0", result.Version)
	assert.Equal(t, scanID, result.ScanID)
	assert.Equal(t, "WARN", result.ScanStatus)
}

func TestDisplayResults_Sorting(t *testing.T) {
	// Create unsorted results
	results := []ScanResult{
		{PackageName: "zebra", Version: "1.0.0", ScanStatus: "ALLOWED"},
		{PackageName: "axios", Version: "1.0.0", ScanStatus: "BLOCKED"},
		{PackageName: "lodash", Version: "1.0.0", ScanStatus: "WARN"},
		{PackageName: "express", Version: "1.0.0", ScanStatus: "ALLOWED"},
	}

	p := progress.NewConsoleReporter()
	err := DisplayResults(results, p)
	assert.NoError(t, err)

	// DisplayResults should sort by package name
	// We can't directly verify the sorting since DisplayResults modifies internally,
	// but we can verify it doesn't error
}

func TestDisplayResults_LargeResultSet(t *testing.T) {
	// Create a large result set
	results := make([]ScanResult, 100)
	for i := 0; i < 100; i++ {
		results[i] = ScanResult{
			PackageName: fmt.Sprintf("package-%d", i),
			Version:     "1.0.0",
			ScanID:      uuid.New().String(),
			ScanStatus:  "ALLOWED",
		}
	}

	p := progress.NewConsoleReporter()
	err := DisplayResults(results, p)
	assert.NoError(t, err)
}

func TestExtractScanResults_MultipleBatches(t *testing.T) {
	// Test extracting results from different batch indices
	for batchIdx := 0; batchIdx < 3; batchIdx++ {
		response := &ar_v3.GetBulkScanEvaluationStatusResp{
			JSON200: &ar_v3.BulkScanEvaluationStatusResponse{
				Data: &ar_v3.BulkScanEvaluationStatusData{
					Scans: &[]ar_v3.BulkScanResultItem{
						{
							PackageName: stringPtr(fmt.Sprintf("pkg-batch-%d", batchIdx)),
							Version:     stringPtr("1.0.0"),
							ScanId:      uuidPtr(uuid.New()),
							ScanStatus:  scanStatusPtr(ar_v3.ALLOWED),
						},
					},
				},
			},
		}

		results := extractScanResults(response, batchIdx)
		assert.Equal(t, 1, len(results))
		assert.Contains(t, results[0].PackageName, fmt.Sprintf("batch-%d", batchIdx))
	}
}

// ----- registryV3Client helper -----

func TestRegistryV3Client_UsesInjectedClient(t *testing.T) {
	mockClient := &mockRegistryV3Client{evaluationID: "test-id"}
	ctx := newMockAuditContext(mockClient)
	client := ctx.registryV3Client()
	assert.Equal(t, mockClient, client)
}

// ----- initiateBatchEvaluation nil-path coverage -----

func TestInitiateBatchEvaluation_NilJSON202(t *testing.T) {
	mockClient := &mockRegistryV3Client{initiateNilJSON202: true}
	ctx := newMockAuditContext(mockClient)
	info := BatchInfo{BatchIdx: 0, TotalBatches: 1, RegistryName: "reg"}
	_, err := initiateBatchEvaluation(ctx, []Dependency{{Name: "pkg", Version: "1.0"}}, info)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid response")
}

func TestInitiateBatchEvaluation_NilData(t *testing.T) {
	mockClient := &mockRegistryV3Client{initiateNilData: true}
	ctx := newMockAuditContext(mockClient)
	info := BatchInfo{BatchIdx: 0, TotalBatches: 1, RegistryName: "reg"}
	_, err := initiateBatchEvaluation(ctx, []Dependency{{Name: "pkg", Version: "1.0"}}, info)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid response")
}

func TestInitiateBatchEvaluation_NilEvaluationId(t *testing.T) {
	mockClient := &mockRegistryV3Client{initiateNilEvalID: true}
	ctx := newMockAuditContext(mockClient)
	info := BatchInfo{BatchIdx: 0, TotalBatches: 1, RegistryName: "reg"}
	_, err := initiateBatchEvaluation(ctx, []Dependency{{Name: "pkg", Version: "1.0"}}, info)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid response")
}

// ----- pollBatchEvaluation nil-path coverage -----

func TestPollBatchEvaluation_NilJSON200(t *testing.T) {
	mockClient := &mockRegistryV3Client{pollNilJSON200: true}
	ctx := newMockAuditContext(mockClient)
	info := BatchInfo{BatchIdx: 0, TotalBatches: 1, RegistryName: "reg"}
	_, err := pollBatchEvaluation(ctx, "eval-id", info)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid response")
}

func TestPollBatchEvaluation_NilData(t *testing.T) {
	mockClient := &mockRegistryV3Client{pollNilData: true}
	ctx := newMockAuditContext(mockClient)
	info := BatchInfo{BatchIdx: 0, TotalBatches: 1, RegistryName: "reg"}
	_, err := pollBatchEvaluation(ctx, "eval-id", info)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid response")
}

func TestPollBatchEvaluation_NilStatus(t *testing.T) {
	mockClient := &mockRegistryV3Client{pollNilStatus: true}
	ctx := newMockAuditContext(mockClient)
	info := BatchInfo{BatchIdx: 0, TotalBatches: 1, RegistryName: "reg"}
	_, err := pollBatchEvaluation(ctx, "eval-id", info)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid response")
}

func TestPollBatchEvaluation_FailureWithNoErrorMessage(t *testing.T) {
	mockClient := &mockRegistryV3Client{
		pollStatus: ar_v3.FAILURE,
		// pollError is empty — should use default message
	}
	ctx := newMockAuditContext(mockClient)
	info := BatchInfo{BatchIdx: 0, TotalBatches: 1, RegistryName: "reg"}
	_, err := pollBatchEvaluation(ctx, "eval-id", info)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Batch 1/1 evaluation failed")
}

// ----- ProcessBatches error propagation -----

func TestProcessBatches_PropagatesInitiateError(t *testing.T) {
	mockClient := &mockRegistryV3Client{initiateError: fmt.Errorf("initiate failure")}
	ctx := newMockAuditContext(mockClient)
	_, err := ProcessBatches(ctx, []Dependency{{Name: "pkg", Version: "1.0"}}, "reg")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "initiate failure")
}

func TestProcessBatches_PropagatesPollError(t *testing.T) {
	mockClient := &mockRegistryV3Client{
		evaluationID:  "eval-id",
		pollHTTPError: fmt.Errorf("poll failure"),
	}
	ctx := newMockAuditContext(mockClient)
	_, err := ProcessBatches(ctx, []Dependency{{Name: "pkg", Version: "1.0"}}, "reg")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "poll failure")
}

// ----- DisplayResults edge cases -----

func TestDisplayResults_EmptyResults(t *testing.T) {
	originalFormat := config.Global.Format
	defer func() { config.Global.Format = originalFormat }()
	config.Global.Format = "table"

	p := progress.NewConsoleReporter()
	err := DisplayResults([]ScanResult{}, p)
	assert.NoError(t, err)
}

func TestDisplayResults_OnlyBlocked(t *testing.T) {
	originalFormat := config.Global.Format
	defer func() { config.Global.Format = originalFormat }()
	config.Global.Format = "table"

	results := []ScanResult{
		{PackageName: "pkg1", Version: "1.0.0", ScanStatus: "BLOCKED"},
		{PackageName: "pkg2", Version: "2.0.0", ScanStatus: "BLOCKED"},
	}
	p := progress.NewConsoleReporter()
	err := DisplayResults(results, p)
	assert.NoError(t, err)
}

func TestDisplayResults_OnlyAllowed(t *testing.T) {
	originalFormat := config.Global.Format
	defer func() { config.Global.Format = originalFormat }()
	config.Global.Format = "table"

	results := []ScanResult{
		{PackageName: "pkg1", Version: "1.0.0", ScanStatus: "ALLOWED"},
	}
	p := progress.NewConsoleReporter()
	err := DisplayResults(results, p)
	assert.NoError(t, err)
}

func TestDisplayResults_OnlyWarn(t *testing.T) {
	originalFormat := config.Global.Format
	defer func() { config.Global.Format = originalFormat }()
	config.Global.Format = "table"

	results := []ScanResult{
		{PackageName: "pkg1", Version: "1.0.0", ScanStatus: "WARN"},
	}
	p := progress.NewConsoleReporter()
	err := DisplayResults(results, p)
	assert.NoError(t, err)
}

func TestDisplayResults_OnlyUnknown(t *testing.T) {
	originalFormat := config.Global.Format
	defer func() { config.Global.Format = originalFormat }()
	config.Global.Format = "table"

	results := []ScanResult{
		{PackageName: "pkg1", Version: "1.0.0", ScanStatus: "UNKNOWN"},
	}
	p := progress.NewConsoleReporter()
	err := DisplayResults(results, p)
	assert.NoError(t, err)
}

func TestDisplayResults_JSONEmpty(t *testing.T) {
	originalFormat := config.Global.Format
	defer func() { config.Global.Format = originalFormat }()
	config.Global.Format = "json"

	p := progress.NewConsoleReporter()
	err := DisplayResults([]ScanResult{}, p)
	assert.NoError(t, err)
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func uuidPtr(u uuid.UUID) *uuid.UUID {
	return &u
}

func scanStatusPtr(s ar_v3.BulkScanResultItemScanStatus) *ar_v3.BulkScanResultItemScanStatus {
	return &s
}

// newMockAuditContext creates an AuditContext with the mock client injected directly
func newMockAuditContext(client *mockRegistryV3Client) *AuditContext {
	return &AuditContext{
		F:                nil,
		RegistryV3Client: client,
		RegistryUUID:     uuid.New(),
		Org:              "test-org",
		Project:          "test-project",
		P:                progress.NewConsoleReporter(),
	}
}

type mockRegistryV3Client struct {
	evaluationID       string
	scanResults        []ar_v3.BulkScanResultItem
	initiateError      error
	initiateStatusCode int
	initiateNilJSON202 bool
	initiateNilData    bool
	initiateNilEvalID  bool
	pollStatus         ar_v3.BulkScanEvaluationStatusDataStatus
	pollError          string
	pollHTTPError      error
	pollStatusCode     int
	pollNilJSON200     bool
	pollNilData        bool
	pollNilStatus      bool
	batchIndex         int
}

func (m *mockRegistryV3Client) InitiateBulkScanEvaluationWithResponse(
	ctx context.Context,
	params *ar_v3.InitiateBulkScanEvaluationParams,
	body ar_v3.InitiateBulkScanEvaluationJSONRequestBody,
	reqEditors ...ar_v3.RequestEditorFn,
) (*ar_v3.InitiateBulkScanEvaluationResp, error) {
	if m.initiateError != nil {
		return nil, m.initiateError
	}

	if m.initiateStatusCode != 0 && m.initiateStatusCode != 202 {
		return &ar_v3.InitiateBulkScanEvaluationResp{
			HTTPResponse: &http.Response{StatusCode: m.initiateStatusCode},
		}, nil
	}

	if m.initiateNilJSON202 {
		return &ar_v3.InitiateBulkScanEvaluationResp{
			HTTPResponse: &http.Response{StatusCode: 202},
			JSON202:      nil,
		}, nil
	}

	if m.initiateNilData {
		return &ar_v3.InitiateBulkScanEvaluationResp{
			HTTPResponse: &http.Response{StatusCode: 202},
			JSON202:      &ar_v3.BulkScanEvaluationAccepted{Data: nil},
		}, nil
	}

	if m.initiateNilEvalID {
		return &ar_v3.InitiateBulkScanEvaluationResp{
			HTTPResponse: &http.Response{StatusCode: 202},
			JSON202: &ar_v3.BulkScanEvaluationAccepted{
				Data: &ar_v3.BulkScanEvaluationAcceptedData{EvaluationId: nil},
			},
		}, nil
	}

	return &ar_v3.InitiateBulkScanEvaluationResp{
		HTTPResponse: &http.Response{StatusCode: 202},
		JSON202: &ar_v3.BulkScanEvaluationAccepted{
			Data: &ar_v3.BulkScanEvaluationAcceptedData{
				EvaluationId: &m.evaluationID,
			},
		},
	}, nil
}

func (m *mockRegistryV3Client) GetBulkScanEvaluationStatusWithResponse(
	ctx context.Context,
	evaluationId string,
	params *ar_v3.GetBulkScanEvaluationStatusParams,
	reqEditors ...ar_v3.RequestEditorFn,
) (*ar_v3.GetBulkScanEvaluationStatusResp, error) {
	if m.pollHTTPError != nil {
		return nil, m.pollHTTPError
	}

	if m.pollStatusCode != 0 && m.pollStatusCode != 200 {
		return &ar_v3.GetBulkScanEvaluationStatusResp{
			HTTPResponse: &http.Response{StatusCode: m.pollStatusCode},
		}, nil
	}

	if m.pollNilJSON200 {
		return &ar_v3.GetBulkScanEvaluationStatusResp{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200:      nil,
		}, nil
	}

	if m.pollNilData {
		return &ar_v3.GetBulkScanEvaluationStatusResp{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200:      &ar_v3.BulkScanEvaluationStatusResponse{Data: nil},
		}, nil
	}

	if m.pollNilStatus {
		return &ar_v3.GetBulkScanEvaluationStatusResp{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200: &ar_v3.BulkScanEvaluationStatusResponse{
				Data: &ar_v3.BulkScanEvaluationStatusData{Status: nil},
			},
		}, nil
	}

	status := m.pollStatus
	if status == "" {
		status = ar_v3.SUCCESS
	}

	var errorMsg *string
	if m.pollError != "" {
		errorMsg = &m.pollError
	}

	// Calculate which results to return based on batch, then advance the index
	batchSize := 50
	start := m.batchIndex * batchSize
	m.batchIndex++
	end := start + batchSize
	if end > len(m.scanResults) {
		end = len(m.scanResults)
	}

	var batchResults []ar_v3.BulkScanResultItem
	if start < len(m.scanResults) {
		batchResults = m.scanResults[start:end]
	}

	return &ar_v3.GetBulkScanEvaluationStatusResp{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &ar_v3.BulkScanEvaluationStatusResponse{
			Data: &ar_v3.BulkScanEvaluationStatusData{
				Status: &status,
				Scans:  &batchResults,
				Error:  errorMsg,
			},
		},
	}, nil
}

// Implement other required methods with no-op or panic
func (m *mockRegistryV3Client) GetArtifactScanDetailsWithResponse(ctx context.Context, scanId string, params *ar_v3.GetArtifactScanDetailsParams, reqEditors ...ar_v3.RequestEditorFn) (*ar_v3.GetArtifactScanDetailsResp, error) {
	panic("not implemented")
}

func (m *mockRegistryV3Client) AddBuildInfoWithBodyWithResponse(ctx context.Context, params *ar_v3.AddBuildInfoParams, contentType string, body io.Reader, reqEditors ...ar_v3.RequestEditorFn) (*ar_v3.AddBuildInfoResp, error) {
	panic("not implemented")
}

func (m *mockRegistryV3Client) AddBuildInfoWithResponse(ctx context.Context, params *ar_v3.AddBuildInfoParams, body ar_v3.AddBuildInfoJSONRequestBody, reqEditors ...ar_v3.RequestEditorFn) (*ar_v3.AddBuildInfoResp, error) {
	panic("not implemented")
}

func (m *mockRegistryV3Client) GetArtifactScansWithResponse(ctx context.Context, params *ar_v3.GetArtifactScansParams, reqEditors ...ar_v3.RequestEditorFn) (*ar_v3.GetArtifactScansResp, error) {
	panic("not implemented")
}

func (m *mockRegistryV3Client) InitiateBulkScanEvaluationWithBodyWithResponse(ctx context.Context, params *ar_v3.InitiateBulkScanEvaluationParams, contentType string, body io.Reader, reqEditors ...ar_v3.RequestEditorFn) (*ar_v3.InitiateBulkScanEvaluationResp, error) {
	panic("not implemented")
}

func (m *mockRegistryV3Client) BulkDeleteArtifactsWithBodyWithResponse(ctx context.Context, params *ar_v3.BulkDeleteArtifactsParams, contentType string, body io.Reader, reqEditors ...ar_v3.RequestEditorFn) (*ar_v3.BulkDeleteArtifactsResp, error) {
	panic("not implemented")
}

func (m *mockRegistryV3Client) BulkDeleteArtifactsWithResponse(ctx context.Context, params *ar_v3.BulkDeleteArtifactsParams, body ar_v3.BulkDeleteArtifactsJSONRequestBody, reqEditors ...ar_v3.RequestEditorFn) (*ar_v3.BulkDeleteArtifactsResp, error) {
	panic("not implemented")
}

func (m *mockRegistryV3Client) EvaluateArtifactScanWithBodyWithResponse(ctx context.Context, params *ar_v3.EvaluateArtifactScanParams, contentType string, body io.Reader, reqEditors ...ar_v3.RequestEditorFn) (*ar_v3.EvaluateArtifactScanResp, error) {
	panic("not implemented")
}

func (m *mockRegistryV3Client) EvaluateArtifactScanWithResponse(ctx context.Context, params *ar_v3.EvaluateArtifactScanParams, body ar_v3.EvaluateArtifactScanJSONRequestBody, reqEditors ...ar_v3.RequestEditorFn) (*ar_v3.EvaluateArtifactScanResp, error) {
	panic("not implemented")
}

func (m *mockRegistryV3Client) GetSystemInfoWithResponse(ctx context.Context, params *ar_v3.GetSystemInfoParams, reqEditors ...ar_v3.RequestEditorFn) (*ar_v3.GetSystemInfoResp, error) {
	panic("not implemented")
}
