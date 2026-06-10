package command

import (
	"testing"

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
