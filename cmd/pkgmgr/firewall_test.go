package pkgmgr

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar"
	ar_v3 "github.com/harness/harness-cli/internal/api/ar_v3"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newMockV3Client(statusCode int, body string) *ar_v3.ClientWithResponses {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			header := make(http.Header)
			header.Set("Content-Type", "application/json")
			return &http.Response{
				StatusCode: statusCode,
				Body:       io.NopCloser(bytes.NewBufferString(body)),
				Header:     header,
			}, nil
		}),
	}
	client, _ := ar_v3.NewClientWithResponses("http://test", ar_v3.WithHTTPClient(httpClient))
	return client
}

func TestRunFirewallExplainEmptyArtifacts(t *testing.T) {
	config.Global = config.GlobalFlags{
		AccountID: "test-account",
	}
	f := &cmdutils.Factory{
		RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return newMockV3Client(200, "{}") },
	}

	progress := p.NewConsoleReporter()
	counts, err := RunFirewallExplain(f, uuid.New(), nil, "org", "project", progress)
	assert.NoError(t, err)
	assert.Equal(t, ScanStatusCounts{}, counts)
}

// Batch 1 succeeds, batch 2 fails: the tally from batch 1 must survive alongside the error.
func TestRunFirewallExplainPartialBatchFailure(t *testing.T) {
	maxRetries, retryInterval = 1, 0
	t.Cleanup(func() { maxRetries, retryInterval = 3, 30*time.Second })

	config.Global = config.GlobalFlags{AccountID: "test-account"}

	const statusBody = `{"data":{"status":"SUCCESS","scans":[
		{"packageName":"a","version":"1.0.0","scanStatus":"ALLOWED"},
		{"packageName":"b","version":"1.0.0","scanStatus":"BLOCKED"}]}}`

	initiates := 0
	v3Client, err := ar_v3.NewClientWithResponses("http://test", ar_v3.WithHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, status := statusBody, http.StatusOK
			if req.Method == http.MethodPost {
				initiates++
				if initiates == 1 {
					body = `{"data":{"evaluationId":"eval-1"}}`
					status = http.StatusAccepted
				} else {
					body = `{"error":{"message":"upstream proxy not found"}}`
					status = http.StatusNotFound
				}
			}
			return &http.Response{
				StatusCode: status,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		}),
	}))
	require.NoError(t, err)

	// 51 artifacts => two batches, so the second (failing) batch is reached.
	artifacts := make([]ar_v3.ArtifactScanInput, 51)
	f := &cmdutils.Factory{RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return v3Client }}

	counts, err := RunFirewallExplain(f, uuid.New(), artifacts, "org", "project", p.NewConsoleReporter())
	require.Error(t, err)
	assert.Equal(t, ScanStatusCounts{Allowed: 1, Blocked: 1}, counts)
}

func TestDisplayScanDetails(t *testing.T) {
	t.Run("nil policy details", func(t *testing.T) {
		details := &ar_v3.ArtifactScanDetails{
			PolicySetFailureDetails: nil,
		}
		DisplayScanDetails(details)
	})

	t.Run("empty policy details", func(t *testing.T) {
		empty := []ar_v3.PolicySetFailureDetail{}
		details := &ar_v3.ArtifactScanDetails{
			PolicySetFailureDetails: &empty,
		}
		DisplayScanDetails(details)
	})
}

func TestDisplayBlockedScanResults(t *testing.T) {
	t.Run("empty scans", func(t *testing.T) {
		config.Global = config.GlobalFlags{
			AccountID: "test-account",
		}
		f := &cmdutils.Factory{
			RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return newMockV3Client(200, "{}") },
		}

		progress := p.NewConsoleReporter()
		counts := DisplayBlockedScanResults(f, nil, progress, 0, false)
		assert.Equal(t, ScanStatusCounts{}, counts)
	})

	t.Run("scans with allowed status", func(t *testing.T) {
		config.Global = config.GlobalFlags{
			AccountID: "test-account",
		}
		f := &cmdutils.Factory{
			RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return newMockV3Client(200, "{}") },
		}

		pkgName := "lodash"
		version := "4.17.21"
		scanStatus := ar_v3.BulkScanResultItemScanStatusALLOWED

		scans := []ar_v3.BulkScanResultItem{
			{
				PackageName: &pkgName,
				Version:     &version,
				ScanStatus:  &scanStatus,
			},
		}

		progress := p.NewConsoleReporter()
		counts := DisplayBlockedScanResults(f, scans, progress, 1, false)
		assert.Equal(t, ScanStatusCounts{Allowed: 1}, counts)
	})

	t.Run("scans with blocked status and nil scan ID", func(t *testing.T) {
		config.Global = config.GlobalFlags{
			AccountID: "test-account",
		}
		f := &cmdutils.Factory{
			RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return newMockV3Client(200, "{}") },
		}

		pkgName := "bad-pkg"
		version := "1.0.0"
		scanStatus := ar_v3.BulkScanResultItemScanStatusBLOCKED

		scans := []ar_v3.BulkScanResultItem{
			{
				PackageName: &pkgName,
				Version:     &version,
				ScanStatus:  &scanStatus,
				ScanId:      nil,
			},
		}

		progress := p.NewConsoleReporter()
		counts := DisplayBlockedScanResults(f, scans, progress, 1, false)
		assert.Equal(t, ScanStatusCounts{Blocked: 1}, counts)
	})

	t.Run("scans with blocked status and scan ID", func(t *testing.T) {
		config.Global = config.GlobalFlags{
			AccountID: "test-account",
		}
		f := &cmdutils.Factory{
			RegistryV3HttpClient: func() *ar_v3.ClientWithResponses {
				return newMockV3Client(200, `{"data":{"policySetFailureDetails":[]}}`)
			},
		}

		pkgName := "bad-pkg"
		version := "1.0.0"
		scanStatus := ar_v3.BulkScanResultItemScanStatusBLOCKED
		scanID := uuid.New()

		scans := []ar_v3.BulkScanResultItem{
			{
				PackageName: &pkgName,
				Version:     &version,
				ScanStatus:  &scanStatus,
				ScanId:      &scanID,
			},
		}

		progress := p.NewConsoleReporter()
		counts := DisplayBlockedScanResults(f, scans, progress, 1, false)
		assert.Equal(t, ScanStatusCounts{Blocked: 1}, counts)
	})
}

func statusPtr(s ar_v3.BulkScanResultItemScanStatus) *ar_v3.BulkScanResultItemScanStatus {
	return &s
}

func TestCountScanStatuses(t *testing.T) {
	t.Run("mixed statuses", func(t *testing.T) {
		scans := []ar_v3.BulkScanResultItem{
			{ScanStatus: statusPtr(ar_v3.BulkScanResultItemScanStatusALLOWED)},
			{ScanStatus: statusPtr(ar_v3.BulkScanResultItemScanStatusALLOWED)},
			{ScanStatus: statusPtr(ar_v3.BulkScanResultItemScanStatusWARN)},
			{ScanStatus: statusPtr(ar_v3.BulkScanResultItemScanStatusBLOCKED)},
			{ScanStatus: statusPtr(ar_v3.BulkScanResultItemScanStatusBLOCKED)},
			{ScanStatus: statusPtr(ar_v3.BulkScanResultItemScanStatusBLOCKED)},
			{ScanStatus: statusPtr(ar_v3.BulkScanResultItemScanStatusUNKNOWN)},
			{ScanStatus: nil},
		}
		assert.Equal(t, ScanStatusCounts{Allowed: 2, Warn: 1, Blocked: 3, Unknown: 2}, countScanStatuses(scans))
	})

	t.Run("empty", func(t *testing.T) {
		assert.Equal(t, ScanStatusCounts{}, countScanStatuses(nil))
	})
}

func TestPrintFirewallEvaluationSummary(t *testing.T) {
	t.Run("prints requested counts", func(t *testing.T) {
		out := captureStdout(t, func() {
			printFirewallEvaluationSummary(ScanStatusCounts{Allowed: 1631, Warn: 12, Blocked: 3}, 1646, false)
		})
		assert.Contains(t, out, "FIREWALL EVALUATION SUMMARY")
		assert.Contains(t, out, "Evaluated: 1646")
		assert.Contains(t, out, "Allowed: 1631")
		assert.Contains(t, out, "Warn: 12")
		assert.Contains(t, out, "Blocked: 3")
		assert.NotContains(t, out, "Unknown:")
		assert.NotContains(t, out, "incomplete")
	})

	t.Run("prints partial evaluation line", func(t *testing.T) {
		out := captureStdout(t, func() {
			printFirewallEvaluationSummary(ScanStatusCounts{Allowed: 50, Blocked: 1}, 1646, true)
		})
		assert.Contains(t, out, "Evaluated: 51 of 1646 resolved dependencies")
		assert.Contains(t, out, "(incomplete — some batches failed)")
		assert.Contains(t, out, "Allowed: 50")
		assert.Contains(t, out, "Warn: 0")
		assert.Contains(t, out, "Blocked: 1")
	})

	t.Run("prints unknown when present", func(t *testing.T) {
		out := captureStdout(t, func() {
			printFirewallEvaluationSummary(ScanStatusCounts{Unknown: 2}, 2, false)
		})
		assert.Contains(t, out, "Unknown: 2")
	})
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stdout
	os.Stdout = w
	fn()
	require.NoError(t, w.Close())
	os.Stdout = orig
	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	require.NoError(t, r.Close())
	return buf.String()
}

func newMockARClient(statusCode int, body string) *ar.ClientWithResponses {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			header := make(http.Header)
			header.Set("Content-Type", "application/json")
			return &http.Response{
				StatusCode: statusCode,
				Body:       io.NopCloser(bytes.NewBufferString(body)),
				Header:     header,
			}, nil
		}),
	}
	client, _ := ar.NewClientWithResponses("http://test", ar.WithHTTPClient(httpClient))
	return client
}

func TestResolveRegistryUUID(t *testing.T) {
	t.Run("registry not found", func(t *testing.T) {
		config.Global = config.GlobalFlags{
			AccountID: "test-account",
		}

		f := &cmdutils.Factory{
			RegistryHttpClient: func() *ar.ClientWithResponses { return newMockARClient(404, `{"code":"NOT_FOUND"}`) },
		}

		progress := p.NewConsoleReporter()
		_, err := ResolveRegistryUUID(f, "nonexistent", "org", "proj", progress)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("successful resolution", func(t *testing.T) {
		config.Global = config.GlobalFlags{
			AccountID: "test-account",
		}

		testUUID := "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
		body := `{"status":"SUCCESS","data":{"identifier":"my-reg","uuid":"` + testUUID + `","packageType":"NPM","url":"https://example.com"}}`
		f := &cmdutils.Factory{
			RegistryHttpClient: func() *ar.ClientWithResponses { return newMockARClient(200, body) },
		}

		progress := p.NewConsoleReporter()
		regUUID, err := ResolveRegistryUUID(f, "my-reg", "org", "proj", progress)
		assert.NoError(t, err)
		assert.Equal(t, testUUID, regUUID.String())
	})
}
