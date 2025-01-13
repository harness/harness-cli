package client

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
)

const (
	apiKeyHeaderKey         = "X-Api-Key"
	harnessAccountHeaderKey = "harness-account"
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
		apiKeyHeaderKey:         authToken,
		harnessAccountHeaderKey: account,
	})
	return &IacmClient{
		authToken: authToken,
		account:   account,
		resty:     r,
	}
}

type IacmError struct {
	Fault     bool   `json:"fault"`
	Id        string `json:"id"`
	Message   string `json:"message"`
	Name      string `json:"name"`
	Temporary bool   `json:"temporary"`
	Timeout   bool   `json:"timeout"`
}

func (e *IacmError) Error() string {
	return e.Message
}

type RemoteExecution struct {
	ID                   string              `json:"id"`
	Workspace            string              `json:"workspace"`
	Account              string              `json:"account"`
	Org                  string              `json:"org"`
	Project              string              `json:"project"`
	PipelineExecutionID  string              `json:"pipeline_execution_id"`
	PipelineExecutionURL string              `json:"pipeline_execution_url"`
	Sha256Checksum       string              `json:"sha256_checksum"`
	Created              int                 `json:"created"`
	Updated              int                 `json:"updated"`
	CustomArguments      map[string][]string `json:"custom_arguments"`
}

type CreateRemoteExecutionPayload struct {
	CustomArguments map[string][]string `json:"custom_arguments"`
}

type Workspace struct {
	Account          string                              `json:"account"`
	Org              string                              `json:"org"`
	Project          string                              `json:"project"`
	Identifier       string                              `json:"identifier"`
	RepositoryPath   string                              `json:"repository_path,omitempty"`
	DefaultPipelines map[string]*DefaultPipelineOverride `json:"default_pipelines"`
}

type PipelineExecutionError struct {
	Status          string
	Code            string
	Message         string
	DetailedMessage string
}

func (e *PipelineExecutionError) Error() string {
	return e.Message
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
		Get(fmt.Sprintf("/gateway/iacm/api/orgs/%s/projects/%s/workspaces/%s", org, project, workspace))
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, errorResult
	}
	return result, nil
}

func (c *IacmClient) CreateRemoteExecution(ctx context.Context, org, project, workspace string, customArguments map[string][]string) (*RemoteExecution, error) {
	result := &RemoteExecution{}
	body := &CreateRemoteExecutionPayload{
		CustomArguments: customArguments,
	}
	errorResult := &IacmError{}
	resp, err := c.resty.R().
		SetError(errorResult).
		SetResult(result).
		SetBody(body).
		Post(fmt.Sprintf("/gateway/iacm/api/orgs/%s/projects/%s/workspaces/%s/remote-executions", org, project, workspace))
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
		Post(fmt.Sprintf("/gateway/iacm/api/orgs/%s/projects/%s/workspaces/%s/remote-executions/%s/upload", org, project, workspace, id))
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
		Post(fmt.Sprintf("/gateway/iacm/api/orgs/%s/projects/%s/workspaces/%s/remote-executions/%s/execute", org, project, workspace, id))
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, errorResult
	}
	return result, nil
}

func (c *IacmClient) GetPipelineExecution(ctx context.Context, org, project, executionID string, stageNodeID string) (*PipelineExecutionDetail, error) {
	result := &ResponseDtoPipelineExecutionDetail{}
	errorResult := &PipelineExecutionError{}
	resp, err := c.resty.R().
		SetError(errorResult).
		SetResult(result).
		SetQueryParams(map[string]string{
			"accountIdentifier":     c.account,
			"orgIdentifier":         org,
			"projectIdentifier":     project,
			"stageNodeId":           stageNodeID,
			"renderFullBottomGraph": "true",
		}).
		Get(fmt.Sprintf("/pipeline/api/pipelines/execution/v2/%s", executionID))
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, errorResult
	}
	return result.Data, nil
}

func (c *IacmClient) GetLogToken(ctx context.Context) (string, error) {
	resp, err := c.resty.R().
		SetQueryParams(map[string]string{
			"accountID": c.account,
			"routingId": c.account,
		}).
		ForceContentType("text/plain").
		Get("/gateway/log-service/token")
	if err != nil {
		return "", err
	}
	if resp.IsError() {
		return "", errors.New(string(string(resp.Body())))
	}
	return string(resp.Body()), nil
}

type ResponseDtoPipelineExecutionDetail struct {
	Status        string                   `json:"status,omitempty"`
	Data          *PipelineExecutionDetail `json:"data,omitempty"`
	CorrelationId string                   `json:"correlationId,omitempty"`
}

type PipelineExecutionDetail struct {
	PipelineExecutionSummary *PipelineExecutionSummary `json:"pipelineExecutionSummary,omitempty"`
	ExecutionGraph           *ExecutionGraph           `json:"executionGraph,omitempty"`
}

type ExecutionGraph struct {
	RootNodeId           string                                `json:"rootNodeId,omitempty"`
	NodeMap              map[string]ExecutionNode              `json:"nodeMap,omitempty"`
	NodeAdjacencyListMap map[string]ExecutionNodeAdjacencyList `json:"nodeAdjacencyListMap,omitempty"`
}

type ExecutionNodeAdjacencyList struct {
	Children []string `json:"children,omitempty"`
	NextIds  []string `json:"nextIds,omitempty"`
}

type ExecutionNode struct {
	Uuid       string `json:"uuid,omitempty"`
	SetupId    string `json:"setupId,omitempty"`
	Name       string `json:"name,omitempty"`
	Identifier string `json:"identifier,omitempty"`
	StepType   string `json:"stepType,omitempty"`
	Status     string `json:"status,omitempty"`
	LogBaseKey string `json:"logBaseKey,omitempty"`
}

type ResponseMessage struct {
	Code           string            `json:"code,omitempty"`
	Level          string            `json:"level,omitempty"`
	Message        string            `json:"message,omitempty"`
	FailureTypes   []string          `json:"failureTypes,omitempty"`
	AdditionalInfo map[string]string `json:"additionalInfo,omitempty"`
}

type FailureInfoDto struct {
	Message          string            `json:"message,omitempty"`
	FailureTypeList  []string          `json:"failureTypeList,omitempty"`
	ResponseMessages []ResponseMessage `json:"responseMessages,omitempty"`
}

type PipelineExecutionSummary struct {
	PipelineIdentifier string                     `json:"pipelineIdentifier,omitempty"`
	OrgIdentifier      string                     `json:"orgIdentifier"`
	ProjectIdentifier  string                     `json:"projectIdentifier"`
	PlanExecutionId    string                     `json:"planExecutionId,omitempty"`
	Name               string                     `json:"name,omitempty"`
	YamlVersion        string                     `json:"yamlVersion,omitempty"`
	Status             string                     `json:"status,omitempty"`
	LayoutNodeMap      map[string]GraphLayoutNode `json:"layoutNodeMap,omitempty"`
	StartingNodeId     string                     `json:"startingNodeId,omitempty"`
}

type GraphLayoutNode struct {
	NodeType       string          `json:"nodeType,omitempty"`
	NodeGroup      string          `json:"nodeGroup,omitempty"`
	NodeIdentifier string          `json:"nodeIdentifier,omitempty"`
	Name           string          `json:"name,omitempty"`
	NodeUuid       string          `json:"nodeUuid,omitempty"`
	Status         string          `json:"status,omitempty"`
	EdgeLayoutList *EdgeLayoutList `json:"edgeLayoutList,omitempty"`
}

type EdgeLayoutList struct {
	CurrentNodeChildren []string `json:"currentNodeChildren,omitempty"`
	NextIds             []string `json:"nextIds,omitempty"`
}
