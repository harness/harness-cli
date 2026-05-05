package pkgmgr

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar"
	ar_v3 "github.com/harness/harness-cli/internal/api/ar_v3"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
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
	count, err := RunFirewallExplain(f, uuid.New(), nil, "org", "project", progress)
	assert.NoError(t, err)
	assert.Equal(t, 0, count)
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
		err := DisplayBlockedScanResults(f, nil, progress)
		assert.NoError(t, err)
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
		scanStatus := ar_v3.ALLOWED

		scans := []ar_v3.BulkScanResultItem{
			{
				PackageName: &pkgName,
				Version:     &version,
				ScanStatus:  &scanStatus,
			},
		}

		progress := p.NewConsoleReporter()
		err := DisplayBlockedScanResults(f, scans, progress)
		assert.NoError(t, err)
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
		scanStatus := ar_v3.BLOCKED

		scans := []ar_v3.BulkScanResultItem{
			{
				PackageName: &pkgName,
				Version:     &version,
				ScanStatus:  &scanStatus,
				ScanId:      nil,
			},
		}

		progress := p.NewConsoleReporter()
		err := DisplayBlockedScanResults(f, scans, progress)
		assert.NoError(t, err)
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
		scanStatus := ar_v3.BLOCKED
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
		err := DisplayBlockedScanResults(f, scans, progress)
		assert.NoError(t, err)
	})
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
