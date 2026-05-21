package utils

import (
	"fmt"
	"net/http"

	"github.com/harness/harness-cli/internal/api/ar_pkg"
	p "github.com/harness/harness-cli/util/common/progress"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
)

func GetRetryClient(progress p.Reporter) ar_pkg.ClientOption {
	return ar_pkg.WithHTTPClient(NewRetryClient(progress))
}

// NewRetryClient creates a new retryable HTTP client with progress reporting hooks.
// It returns a standard *http.Client that can be used with pkgclient.WithHTTPClient().
func NewRetryClient(progress p.Reporter) *http.Client {
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 3
	retryClient.Logger = nil

	retryClient.RequestLogHook = func(
		_ retryablehttp.Logger,
		req *http.Request,
		retryNumber int,
	) {
		if retryNumber > 0 {
			//TODO format message
			progress.Step(fmt.Sprintf(
				"Retrying request: %s %s (attempt %d/%d)",
				req.Method,
				req.URL.String(),
				retryNumber,
				retryClient.RetryMax,
			))
		}
	}

	retryClient.ResponseLogHook = func(
		_ retryablehttp.Logger,
		resp *http.Response,
	) {
		if resp != nil {
			//TODO switch statusCode and print message as per response
			progress.Step(fmt.Sprintf(
				"Request failed: %s %s -> status %d",
				resp.Request.Method,
				resp.Request.URL.String(),
				resp.StatusCode,
			))
		}
	}

	return retryClient.StandardClient()
}
