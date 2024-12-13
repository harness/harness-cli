package client

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
)

type IacmClient struct {
	account   string
	authToken string
	resty     *resty.Client
}

func NewIacmClient(account string, host string, authToken string, debug bool) *IacmClient {
	r := resty.New()
	r.SetBaseURL(host)
	r.SetTimeout(10 * time.Second)
	r.SetRetryCount(3)
	r.SetRetryWaitTime(2 * time.Second)
	r.SetRetryMaxWaitTime(60 * time.Second)
	r.SetDebug(debug)
	r.SetHeaders(map[string]string{
		"x-api-key":       authToken,
		"harness-account": account,
	})
	return &IacmClient{
		authToken: authToken,
		account:   account,
		resty:     r,
	}
}

type IacmError struct {
	// Is the error a server-side fault?
	Fault bool `json:"fault"`
	// ID is a unique identifier for this particular occurrence of the problem.
	Id string `json:"id"`
	// Message is a human-readable explanation specific to this occurrence of the problem.
	Message string `json:"message"`
	// Name is the name of this class of errors.
	Name string `json:"name"`
	// Is the error temporary?
	Temporary bool `json:"temporary"`
	// Is the error a timeout?
	Timeout bool `json:"timeout"`
}

func (e *IacmError) Error() string {
	return e.Message
}

type RemoteExecution struct {
	ID                   string `json:"id"`
	Workspace            string `json:"workspace"`
	Account              string `json:"account"`
	Org                  string `json:"org"`
	Project              string `json:"project"`
	PipelineExecutionID  string `json:"pipeline_execution_id"`
	PipelineExecutionURL string `json:"pipeline_execution_url"`
	Sha256Checksum       string `json:"sha256_checksum"`
	Created              int    `json:"created"`
	Updated              int    `json:"updated"`
}

type Workspace struct {
	Account               string                              `json:"account"`
	Org                   string                              `json:"org"`
	Project               string                              `json:"project"`
	Identifier            string                              `json:"identifier"`
	Created               int                                 `json:"created,omitempty"`
	Updated               int                                 `json:"updated,omitempty"`
	Name                  string                              `json:"name"`
	Status                string                              `json:"status"`
	Description           *string                             `json:"description,omitempty"`
	Provisioner           string                              `json:"provisioner,omitempty"`
	ProvisionerVersion    string                              `json:"provisioner_version,omitempty"`
	ProvisionerData       map[string]string                   `json:"provisioner_data,omitempty"`
	ProviderConnector     string                              `json:"provider_connector,omitempty"`
	Repository            string                              `json:"repository,omitempty"`
	RepositoryBranch      string                              `json:"repository_branch,omitempty"`
	RepositoryCommit      string                              `json:"repository_commit,omitempty"`
	RepositorySha         string                              `json:"repository_sha,omitempty"`
	RepositoryConnector   string                              `json:"repository_connector,omitempty"`
	RepositoryPath        string                              `json:"repository_path,omitempty"`
	CostEstimationEnabled bool                                `json:"cost_estimation_enabled"`
	DefaultPipelines      map[string]*DefaultPipelineOverride `json:"default_pipelines"`
	BackendLocked         bool                                `json:"backend_locked,omitempty"`
	StateChecksum         string                              `json:"state_checksum,omitempty"`
}

type DefaultPipelineOverride struct {
	ProjectPipeline   *string `json:"project_pipeline,omitempty"`
	WorkspacePipeline *string `json:"workspace_pipeline,omitempty"`
}

func (c *IacmClient) GetWorkspace(ctx context.Context, org, project, workspace string) (*Workspace, error) {
	result := &Workspace{}
	errorResult := &IacmError{}
	resp, err := c.resty.R().
		SetError(errorResult).
		SetResult(result).
		Get(fmt.Sprintf("/iacm/api/orgs/%s/projects/%s/workspaces/%s", org, project, workspace))
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, errorResult
	}
	return result, nil
}

func (c *IacmClient) CreateRemoteExecution(ctx context.Context, org, project, workspace string) (*RemoteExecution, error) {
	result := &RemoteExecution{}
	errorResult := &IacmError{}
	resp, err := c.resty.R().
		SetError(errorResult).
		SetResult(result).
		Post(fmt.Sprintf("/iacm/api/orgs/%s/projects/%s/workspaces/%s/remote-executions", org, project, workspace))
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, errorResult
	}
	return result, nil
}

func (c *IacmClient) UploadRemoteExecution(ctx context.Context, org, project, workspace, id string, file []byte) (*RemoteExecution, error) {
	hasher := sha256.New()
	_, err := hasher.Write(file)
	if err != nil {
		return nil, err
	}
	sha256 := hasher.Sum(nil)
	result := &RemoteExecution{}
	errorResult := &IacmError{}

	resp, err := c.resty.R().
		SetError(errorResult).
		SetResult(result).
		SetBody(file).
		SetHeader("Content-Digest", fmt.Sprintf("sha256=%x", sha256)).
		Post(fmt.Sprintf("/iacm/api/orgs/%s/projects/%s/workspaces/%s/remote-executions/%s/upload", org, project, workspace, id))
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, errorResult
	}
	return result, nil
}

func (c *IacmClient) ExecuteRemoteExecution(ctx context.Context, org, project, workspace, id string) (*RemoteExecution, error) {
	result := &RemoteExecution{}
	errorResult := &IacmError{}
	resp, err := c.resty.R().
		SetResult(result).
		SetError(errorResult).
		Post(fmt.Sprintf("/iacm/api/orgs/%s/projects/%s/workspaces/%s/remote-executions/%s/execute", org, project, workspace, id))
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, errorResult
	}
	return result, nil
}
