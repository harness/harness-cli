package command

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar_v2"
	"github.com/harness/harness-cli/internal/api/ar_v3"
	"github.com/spf13/cobra"
)

type errTransport struct{ err error }

func (e *errTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, e.err
}

func newErrClient(err error) *ar_v2.ClientWithResponses {
	c, _ := ar_v2.NewClientWithResponses("http://test", ar_v2.WithHTTPClient(&http.Client{
		Transport: &errTransport{err: err},
	}))
	return c
}

func TestApplyPostPushMetadata_NoOp(t *testing.T) {
	config.Global = config.GlobalFlags{AccountID: "test-account"}
	f := &cmdutils.Factory{
		RegistryV2HttpClient: func() *ar_v2.ClientWithResponses {
			return newMockClient(200, `{"status":"SUCCESS","data":{"metadata":[]}}`)
		},
	}

	// empty metadataStr — must not call API
	applyPostPushMetadata(f, "", "reg", "pkg", "1.0.0")

	// empty pkg — must not call API
	applyPostPushMetadata(f, "key:val", "reg", "", "1.0.0")
}

func TestApplyPostPushMetadata_InvalidFormat(t *testing.T) {
	config.Global = config.GlobalFlags{AccountID: "test-account"}
	f := &cmdutils.Factory{
		RegistryV2HttpClient: func() *ar_v2.ClientWithResponses {
			return newMockClient(200, `{"status":"SUCCESS","data":{"metadata":[]}}`)
		},
	}
	// Malformed metadata string — function should warn and return without error.
	applyPostPushMetadata(f, "noequalsnoseparator", "reg", "pkg", "1.0.0")
}

func TestApplyPostPushMetadata_Success(t *testing.T) {
	config.Global = config.GlobalFlags{AccountID: "test-account"}
	f := &cmdutils.Factory{
		RegistryV2HttpClient: func() *ar_v2.ClientWithResponses {
			return newMockClient(200, `{"status":"SUCCESS","data":{"metadata":[{"key":"env","value":"prod"}]}}`)
		},
	}
	applyPostPushMetadata(f, "env:prod", "my-reg", "my-pkg", "2.0.0")
}

func TestApplyPostPushMetadata_NoVersion(t *testing.T) {
	config.Global = config.GlobalFlags{AccountID: "test-account"}
	f := &cmdutils.Factory{
		RegistryV2HttpClient: func() *ar_v2.ClientWithResponses {
			return newMockClient(200, `{"status":"SUCCESS","data":{"metadata":[]}}`)
		},
	}
	// version empty — body.Version should not be set
	applyPostPushMetadata(f, "env:prod", "my-reg", "my-pkg", "")
}

func TestApplyPostPushMetadata_HTTPError(t *testing.T) {
	config.Global = config.GlobalFlags{AccountID: "test-account"}
	f := &cmdutils.Factory{
		RegistryV2HttpClient: func() *ar_v2.ClientWithResponses {
			return newErrClient(errors.New("network error"))
		},
	}
	// HTTP error — function should warn and return without panicking.
	applyPostPushMetadata(f, "env:prod", "my-reg", "my-pkg", "1.0.0")
}

func TestApplyPostPushMetadata_4xxResponse(t *testing.T) {
	config.Global = config.GlobalFlags{AccountID: "test-account"}
	for _, code := range []int{400, 404, 500} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			f := &cmdutils.Factory{
				RegistryV2HttpClient: func() *ar_v2.ClientWithResponses {
					return newMockClient(code, `{"message":"error"}`)
				},
			}
			applyPostPushMetadata(f, "env:prod", "my-reg", "my-pkg", "1.0.0")
		})
	}
}

func TestApplyPostPushMetadata_MultipleKeyValues(t *testing.T) {
	config.Global = config.GlobalFlags{AccountID: "test-account"}
	var capturedURL string
	transport := &captureTransport{statusCode: 200, body: `{"status":"SUCCESS","data":{"metadata":[]}}`, capturedURL: &capturedURL}
	httpClient := &http.Client{Transport: transport}
	client, _ := ar_v2.NewClientWithResponses("http://test", ar_v2.WithHTTPClient(httpClient))
	f := &cmdutils.Factory{
		RegistryV2HttpClient: func() *ar_v2.ClientWithResponses { return client },
	}
	applyPostPushMetadata(f, "k1:v1,k2:v2,k3:v3", "my-reg", "my-pkg", "1.0.0")
}

func newMockV3Client(statusCode int, body string) *ar_v3.ClientWithResponses {
	c, _ := ar_v3.NewClientWithResponses("http://test", ar_v3.WithHTTPClient(&http.Client{
		Transport: &captureTransport{statusCode: statusCode, body: body},
	}))
	return c
}

func newErrV3Client(err error) *ar_v3.ClientWithResponses {
	c, _ := ar_v3.NewClientWithResponses("http://test", ar_v3.WithHTTPClient(&http.Client{
		Transport: &errTransport{err: err},
	}))
	return c
}

func TestApplyPostPushAutoMetadata_NoOp(t *testing.T) {
	config.Global = config.GlobalFlags{AccountID: "test-account"}
	called := false
	f := &cmdutils.Factory{
		RegistryV3HttpClient: func() *ar_v3.ClientWithResponses {
			called = true
			return newMockV3Client(200, `{}`)
		},
	}
	applyPostPushAutoMetadata(f, "", "reg", "pkg", "1.0.0")
	applyPostPushAutoMetadata(f, "key:val", "reg", "", "1.0.0")
	if called {
		t.Fatal("expected no API call for empty metadata or empty package")
	}
}

func TestApplyPostPushAutoMetadata_InvalidFormat(t *testing.T) {
	config.Global = config.GlobalFlags{AccountID: "test-account"}
	called := false
	f := &cmdutils.Factory{
		RegistryV3HttpClient: func() *ar_v3.ClientWithResponses {
			called = true
			return newMockV3Client(200, `{}`)
		},
	}
	applyPostPushAutoMetadata(f, "noequalsnoseparator", "reg", "pkg", "1.0.0")
	if called {
		t.Fatal("expected no API call for invalid metadata format")
	}
}

func TestApplyPostPushAutoMetadata_Success(t *testing.T) {
	config.Global = config.GlobalFlags{AccountID: "test-account"}
	var capturedURL string
	var capturedBody []byte
	transport := &captureTransport{
		statusCode:  200,
		body:        `{}`,
		capturedURL: &capturedURL,
		capturedBody: &capturedBody,
	}
	client, _ := ar_v3.NewClientWithResponses("http://test", ar_v3.WithHTTPClient(&http.Client{Transport: transport}))
	f := &cmdutils.Factory{
		RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return client },
	}
	applyPostPushAutoMetadata(f, "pipelineId:p1,executionId:e1", "my-reg", "my-pkg", "2.0.0")

	if !strings.Contains(capturedURL, "/internal/metadata") {
		t.Fatalf("expected request to /internal/metadata, got %q", capturedURL)
	}
	var body ar_v3.AutoMetadataRequestInput
	if err := json.Unmarshal(capturedBody, &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body.RegistryName != "my-reg" {
		t.Fatalf("registryName = %q", body.RegistryName)
	}
	if body.Package == nil || *body.Package != "my-pkg" {
		t.Fatalf("package = %v", body.Package)
	}
	if body.Version == nil || *body.Version != "2.0.0" {
		t.Fatalf("version = %v", body.Version)
	}
	if len(body.Metadata) != 2 {
		t.Fatalf("metadata len = %d", len(body.Metadata))
	}
}

func TestApplyPostPushAutoMetadata_HTTPError(t *testing.T) {
	config.Global = config.GlobalFlags{AccountID: "test-account"}
	f := &cmdutils.Factory{
		RegistryV3HttpClient: func() *ar_v3.ClientWithResponses {
			return newErrV3Client(errors.New("network error"))
		},
	}
	applyPostPushAutoMetadata(f, "env:prod", "my-reg", "my-pkg", "1.0.0")
}

func TestApplyPostPushAutoMetadata_4xxResponse(t *testing.T) {
	config.Global = config.GlobalFlags{AccountID: "test-account"}
	for _, code := range []int{400, 404, 500} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			f := &cmdutils.Factory{
				RegistryV3HttpClient: func() *ar_v3.ClientWithResponses {
					return newMockV3Client(code, `{"message":"error"}`)
				},
			}
			applyPostPushAutoMetadata(f, "env:prod", "my-reg", "my-pkg", "1.0.0")
		})
	}
}

func TestRegisterAutoMetadataFlag_Hidden(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	var autoMetadata string
	registerAutoMetadataFlag(cmd, &autoMetadata)
	flag := cmd.Flags().Lookup("auto-metadata")
	if flag == nil {
		t.Fatal("expected auto-metadata flag to be registered")
	}
	if !flag.Hidden {
		t.Fatal("expected auto-metadata flag to be hidden")
	}
	if err := cmd.Flags().Set("auto-metadata", "pipelineId:p1"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	if autoMetadata != "pipelineId:p1" {
		t.Fatalf("autoMetadata = %q", autoMetadata)
	}
}

type captureTransport struct {
	statusCode   int
	body         string
	capturedURL  *string
	capturedBody *[]byte
}

func (c *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if c.capturedURL != nil {
		*c.capturedURL = req.URL.String()
	}
	if c.capturedBody != nil && req.Body != nil {
		b, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		*c.capturedBody = b
	}
	return &http.Response{
		StatusCode: c.statusCode,
		Body:       io.NopCloser(newStringReader(c.body)),
		Header:     make(http.Header),
	}, nil
}

func newStringReader(s string) *stringReader {
	return &stringReader{data: []byte(s)}
}

type stringReader struct {
	data []byte
	pos  int
}

func (r *stringReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
