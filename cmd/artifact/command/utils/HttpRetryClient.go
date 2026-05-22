package utils

import (
	"fmt"
	"net/http"

	"github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/internal/api/ar_v3"
	p "github.com/harness/harness-cli/util/common/progress"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
)

func GetRetryClient(progress p.Reporter) ar_pkg.ClientOption {
	return ar_pkg.WithHTTPClient(NewRetryClient(progress))
}

// GetRetryClientV3 creates a retry client option for ar_v3 API client without progress reporting.
// Use this when you don't have access to a progress reporter (e.g., in factory methods).
func GetRetryClientV3() ar_v3.ClientOption {
	return ar_v3.WithHTTPClient(NewRetryClientWithoutProgress())
}

// GetRetryClientV3WithProgress creates a retry client option for ar_v3 API client with progress reporting.
func GetRetryClientV3WithProgress(progress p.Reporter) ar_v3.ClientOption {
	return ar_v3.WithHTTPClient(NewRetryClient(progress))
}

// NewRetryClientWithoutProgress creates a retryable HTTP client without progress reporting hooks.
// Use this when you don't have access to a progress reporter.
func NewRetryClientWithoutProgress() *http.Client {
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 3
	retryClient.Logger = nil
	return retryClient.StandardClient()
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
