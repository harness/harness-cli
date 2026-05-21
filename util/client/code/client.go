package code

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/harness/harness-cli/config"
)

const (
	apiKeyHeader     = "x-api-key"
	accountParam     = "accountIdentifier"
	basePath         = "/gateway/code/api/v1"
)

type Client struct {
	resty     *resty.Client
	accountID string
}

func NewClient() *Client {
	r := resty.New()
	r.SetBaseURL(config.Global.APIBaseURL + basePath)
	r.SetTimeout(30 * time.Second)
	if config.Global.TimeoutSeconds > 0 {
		r.SetTimeout(time.Duration(config.Global.TimeoutSeconds) * time.Second)
	}
	r.SetRetryCount(3)
	r.SetRetryWaitTime(2 * time.Second)
	r.SetRetryMaxWaitTime(30 * time.Second)
	r.AddRetryCondition(func(resp *resty.Response, err error) bool {
		if err != nil {
			return true
		}
		return resp.StatusCode() == http.StatusTooManyRequests ||
			resp.StatusCode() >= http.StatusInternalServerError
	})
	r.SetHeader(apiKeyHeader, config.Global.AuthToken)
	r.SetQueryParam(accountParam, config.Global.AccountID)

	return &Client{
		resty:     r,
		accountID: config.Global.AccountID,
	}
}

func repoPath(repoRef string) string {
	return fmt.Sprintf("/repos/%s/+", repoRef)
}

func (c *Client) ListPullRequests(repoRef string, opts ListPullRequestsOptions) ([]PullRequest, error) {
	var result []PullRequest

	req := c.resty.R().SetResult(&result)

	if len(opts.States) > 0 {
		for _, s := range opts.States {
			req.QueryParam.Add("state", s)
		}
	}
	if opts.SourceBranch != "" {
		req.SetQueryParam("source_branch", opts.SourceBranch)
	}
	if opts.TargetBranch != "" {
		req.SetQueryParam("target_branch", opts.TargetBranch)
	}
	if opts.Query != "" {
		req.SetQueryParam("query", opts.Query)
	}
	if opts.CreatedBy != "" {
		req.SetQueryParam("created_by", opts.CreatedBy)
	}
	if opts.Page > 0 {
		req.SetQueryParam("page", fmt.Sprintf("%d", opts.Page))
	}
	if opts.Limit > 0 {
		req.SetQueryParam("limit", fmt.Sprintf("%d", opts.Limit))
	}
	if opts.Sort != "" {
		req.SetQueryParam("sort", opts.Sort)
	}
	if opts.Order != "" {
		req.SetQueryParam("order", opts.Order)
	}

	resp, err := req.Get(repoPath(repoRef) + "/pullreq")
	if err != nil {
		return nil, fmt.Errorf("list pull requests: %w", err)
	}
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	return result, nil
}

func (c *Client) GetPullRequest(repoRef string, number int64) (*PullRequest, error) {
	var result PullRequest
	resp, err := c.resty.R().
		SetResult(&result).
		Get(fmt.Sprintf("%s/pullreq/%d", repoPath(repoRef), number))
	if err != nil {
		return nil, fmt.Errorf("get pull request: %w", err)
	}
	if err := checkResponse(resp); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) CreatePullRequest(repoRef string, input CreatePullRequestInput) (*PullRequest, error) {
	var result PullRequest
	resp, err := c.resty.R().
		SetBody(input).
		SetResult(&result).
		Post(repoPath(repoRef) + "/pullreq")
	if err != nil {
		return nil, fmt.Errorf("create pull request: %w", err)
	}
	if err := checkResponse(resp); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) MergePullRequest(repoRef string, number int64, input MergePullRequestInput) (*MergePullRequestResponse, error) {
	var result MergePullRequestResponse
	resp, err := c.resty.R().
		SetBody(input).
		SetResult(&result).
		Post(fmt.Sprintf("%s/pullreq/%d/merge", repoPath(repoRef), number))
	if err != nil {
		return nil, fmt.Errorf("merge pull request: %w", err)
	}
	if err := checkResponse(resp); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) CreateComment(repoRef string, number int64, input CreateCommentInput) (*Comment, error) {
	var result Comment
	resp, err := c.resty.R().
		SetBody(input).
		SetResult(&result).
		Post(fmt.Sprintf("%s/pullreq/%d/comments", repoPath(repoRef), number))
	if err != nil {
		return nil, fmt.Errorf("create comment: %w", err)
	}
	if err := checkResponse(resp); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetPullRequestChecks(repoRef string, number int64) (*ChecksResponse, error) {
	var result ChecksResponse
	resp, err := c.resty.R().
		SetResult(&result).
		Get(fmt.Sprintf("%s/pullreq/%d/checks", repoPath(repoRef), number))
	if err != nil {
		return nil, fmt.Errorf("get checks: %w", err)
	}
	if err := checkResponse(resp); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetPullRequestReviewers(repoRef string, number int64) ([]PullRequestReviewer, error) {
	var result []PullRequestReviewer
	resp, err := c.resty.R().
		SetResult(&result).
		Get(fmt.Sprintf("%s/pullreq/%d/reviewers", repoPath(repoRef), number))
	if err != nil {
		return nil, fmt.Errorf("get reviewers: %w", err)
	}
	if err := checkResponse(resp); err != nil {
		return nil, err
	}
	return result, nil
}

// APIError represents an error response from the Harness Code API.
type APIError struct {
	StatusCode int
	Method     string
	Path       string
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error %d %s %s: %s", e.StatusCode, e.Method, e.Path, e.Message)
}

func (e *APIError) ExitCode() int {
	switch {
	case e.StatusCode == http.StatusNotFound:
		return 4
	case e.StatusCode >= 400 && e.StatusCode < 500:
		return 2
	default:
		return 1
	}
}

func checkResponse(resp *resty.Response) error {
	if resp.StatusCode() >= 200 && resp.StatusCode() < 300 {
		return nil
	}
	body := strings.TrimSpace(resp.String())
	if body == "" {
		body = http.StatusText(resp.StatusCode())
	}
	return &APIError{
		StatusCode: resp.StatusCode(),
		Method:     resp.Request.Method,
		Path:       resp.Request.URL,
		Message:    body,
	}
}
