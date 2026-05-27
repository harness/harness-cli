package httpclient

import (
	"fmt"
	"io"
	"net/http"

	"github.com/harness/harness-cli/util/common"
	"github.com/harness/harness-cli/util/common/progress"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
	"github.com/pterm/pterm"
)

// creates a retryable HTTP client without progress reporting hooks.
// Use this when you don't have access to a progress reporter.
func NewRetryClientWithoutProgress() *http.Client {
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 3
	retryClient.Logger = nil
	// Create progress reporter
	p := progress.NewConsoleReporter()

	retryClient.RequestLogHook = func(
		_ retryablehttp.Logger,
		req *http.Request,
		retryNumber int,
	) {
		if retryNumber > 0 {
			fmt.Println()
			p.Step(fmt.Sprintf(
				"Retrying request: (attempt %d/%d)",
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
			result := "Request Failed"
			if resp.StatusCode == 201 || resp.StatusCode == 200 {
				result = "Request succeeded"
			}
			p.Step(fmt.Sprintf(
				"%s : -> status %d",
				result,
				resp.StatusCode,
			))
		}
	}

	return retryClient.StandardClient()
}

// creates a retryable HTTP client with progress reporting
func NewRetryClientWithProgress(prog progress.Reporter, fileSize int64, saveFilename string) *http.Client {
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 3
	retryClient.Logger = nil

	retryClient.RequestLogHook = func(
		_ retryablehttp.Logger,
		req *http.Request,
		retryNumber int,
	) {
		if retryNumber > 0 {
			prog.Step(fmt.Sprintf(
				"Retrying request: (attempt %d/%d)",
				retryNumber,
				retryClient.RetryMax,
			))
		}

		// Wrap the request body with progress bar for each attempt
		if req.Body != nil && fileSize > 0 {
			title := fmt.Sprintf("%s (%s)", saveFilename, common.GetSize(fileSize))
			bar := pterm.DefaultProgressbar.
				WithTitle(title).
				WithTotal(int(fileSize)).
				WithRemoveWhenDone(false)

			//TODO may be error handling required here
			//TODO check for defer too as used in old call
			pb, _ := bar.Start()

			wrapped := &progressReadCloser{
				reader:   req.Body,
				bar:      pb,
				progress: prog,
			}
			req.Body = wrapped
		}
	}

	retryClient.ResponseLogHook = func(
		_ retryablehttp.Logger,
		resp *http.Response,
	) {
		if resp != nil && resp.StatusCode >= 400 {
			//fmt.Println()
			prog.Error(fmt.Sprintf(
				"Request failed : -> status %d",
				resp.StatusCode,
			))
		}
		/*
			if resp != nil {
				result := "Request Failed"
				if resp.StatusCode == 201 || resp.StatusCode == 200 {
					result = "Request succeeded"
				}
				prog.Step(fmt.Sprintf(
					"%s : -> status %d",
					result,
					resp.StatusCode,
				))
			}

		*/
	}

	return retryClient.StandardClient()
}

// progress Read Closer wraps an io.ReadCloser with progress bar reporting
type progressReadCloser struct {
	reader   io.ReadCloser
	bar      *pterm.ProgressbarPrinter
	progress progress.Reporter
}

func (p *progressReadCloser) Read(buf []byte) (int, error) {
	n, err := p.reader.Read(buf)
	if n > 0 && p.bar != nil {
		p.bar.Add(n)
	}
	return n, err
}

func (p *progressReadCloser) Close() error {
	if p.bar != nil {
		p.bar.Stop()
		//pterm.Success.Println(p.bar.Title)
	}
	return p.reader.Close()
}
