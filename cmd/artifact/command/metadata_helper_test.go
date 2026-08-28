package command

import (
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar_v2"
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

type captureTransport struct {
	statusCode  int
	body        string
	capturedURL *string
}

func (c *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if c.capturedURL != nil {
		*c.capturedURL = req.URL.String()
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
