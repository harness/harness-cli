package ar

import (
	"bytes"
	"encoding/json"
	"fmt"
	"harness/util/client"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client represents the client for Harness Artifact Registry API
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	prefix     string
	AuthToken  string
	AccountID  string
	OrgID      string
	ProjectID  string
}

// NewHARClient creates a new Harness Artifact Registry client
func NewHARClient(baseURL string, authToken string, accountID string, orgID string, projectID string) *Client {
	return &Client{
		BaseURL:    baseURL,
		prefix:     "gateway/har/api/v1",
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		AuthToken:  authToken,
		AccountID:  accountID,
		OrgID:      orgID,
		ProjectID:  projectID,
	}
}

// Registry Operations

// RegistryRequest represents a request to create or update a ar
type RegistryRequest struct {
	Identifier     string                 `json:"identifier"`
	PackageType    string                 `json:"packageType"`
	Description    string                 `json:"description,omitempty"`
	Labels         []string               `json:"labels,omitempty"`
	ParentRef      string                 `json:"parentRef,omitempty"`
	AllowedPattern []string               `json:"allowedPattern,omitempty"`
	BlockedPattern []string               `json:"blockedPattern,omitempty"`
	Config         map[string]interface{} `json:"config,omitempty"`
}

// Registry represents a Harness Artifact Registry
type Registry struct {
	Identifier  string                 `json:"identifier"`
	PackageType string                 `json:"packageType"`
	Description string                 `json:"description"`
	URL         string                 `json:"url"`
	Labels      []string               `json:"labels"`
	CreatedAt   string                 `json:"createdAt"`
	ModifiedAt  string                 `json:"modifiedAt"`
	Config      map[string]interface{} `json:"config"`
}

// RegistryResponse represents the API response for ar operations
type RegistryResponse struct {
	Status struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"status"`
	Data Registry `json:"data"`
}

// ListRegistryResponse represents the API response for listing registries
type ListRegistryResponse struct {
	Status string `json:"status"`
	Data   struct {
		Registries []struct {
			Identifier     string   `json:"identifier"`
			PackageType    string   `json:"packageType"`
			Type           string   `json:"type"`
			URL            string   `json:"url"`
			Description    string   `json:"description"`
			Labels         []string `json:"labels"`
			LastModified   string   `json:"lastModified"`
			ArtifactsCount int64    `json:"artifactsCount"`
			DownloadsCount int64    `json:"downloadsCount"`
			RegistrySize   string   `json:"registrySize"`
			Path           string   `json:"path"`
		} `json:"registries"`
		ItemCount int `json:"itemCount"`
		PageCount int `json:"pageCount"`
		PageIndex int `json:"pageIndex"`
		PageSize  int `json:"pageSize"`
	} `json:"data"`
}

// CreateRegistry creates a new ar
func (c *Client) CreateRegistry(req RegistryRequest) (*RegistryResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}
	endpoint := fmt.Sprintf("%s?space_ref=%s", "/ar", client.GetRef(c.AccountID, c.OrgID, c.ProjectID))

	resp, err := c.doRequest("POST", endpoint, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var registryResp RegistryResponse
	if err := json.NewDecoder(resp.Body).Decode(&registryResp); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return &registryResp, nil
}

// GetRegistry retrieves details of a ar
func (c *Client) GetRegistry(registryRef string) (*RegistryResponse, error) {
	endpoint := fmt.Sprintf("/ar/%s", url.PathEscape(registryRef))
	resp, err := c.doRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var registryResp RegistryResponse
	if err := json.NewDecoder(resp.Body).Decode(&registryResp); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return &registryResp, nil
}

// UpdateRegistry updates an existing ar
func (c *Client) UpdateRegistry(registryRef string, req RegistryRequest) (*RegistryResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	endpoint := fmt.Sprintf("/ar/%s", url.PathEscape(registryRef))
	resp, err := c.doRequest("PUT", endpoint, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var registryResp RegistryResponse
	if err := json.NewDecoder(resp.Body).Decode(&registryResp); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return &registryResp, nil
}

// DeleteRegistry deletes a ar
func (c *Client) DeleteRegistry(registryRef string) error {
	endpoint := fmt.Sprintf("/ar/%s", url.PathEscape(registryRef))
	resp, err := c.doRequest("DELETE", endpoint, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// ListRegistries lists all registries
func (c *Client) ListRegistries(
	packageTypes []string,
	registryType string,
	page, size int,
	searchTerm string,
	recursive bool,
) (*ListRegistryResponse, error) {
	var endpoint string
	endpoint = fmt.Sprintf("/spaces/%s/+/registries", client.GetRef(c.AccountID, c.OrgID, c.ProjectID))

	// Build query parameters
	params := url.Values{}
	if page >= 0 {
		params.Add("page", strconv.Itoa(page))
	}
	if size > 0 {
		params.Add("size", strconv.Itoa(size))
	}
	if len(packageTypes) > 0 {
		for _, pt := range packageTypes {
			params.Add("package_type", pt)
		}
	}
	if registryType != "" {
		params.Add("type", registryType)
	}
	if searchTerm != "" {
		params.Add("search_term", searchTerm)
	}
	if recursive {
		params.Add("recursive", "true")
	}

	// Add query parameters to endpoint
	if len(params) > 0 {
		endpoint = fmt.Sprintf("%s?%s", endpoint, params.Encode())
	}

	resp, err := c.doRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var listResp ListRegistryResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return &listResp, nil
}

// ArtifactMetadata represents metadata for an artifact
type ArtifactMetadata struct {
	Name           string `json:"name"`
	PackageType    string `json:"packageType,omitempty"`
	DownloadsCount int    `json:"downloadsCount"`
}

// ListArtifactsResponse represents the response for listing artifacts
type ListArtifactsResponse struct {
	Status string `json:"status"`
	Data   struct {
		Artifacts []ArtifactMetadata `json:"artifacts"`
		ItemCount int                `json:"itemCount"`
		PageCount int                `json:"pageCount"`
		PageIndex int                `json:"pageIndex"`
		PageSize  int                `json:"pageSize"`
	} `json:"data"`
}

// ArtifactDetail represents details of an artifact version
type ArtifactDetail struct {
	Name        string                 `json:"name"`
	Version     string                 `json:"version"`
	Size        string                 `json:"size,omitempty"`
	CreatedAt   string                 `json:"createdAt,omitempty"`
	ModifiedAt  string                 `json:"modifiedAt,omitempty"`
	Labels      []string               `json:"labels,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	DownloadURL string                 `json:"downloadUrl,omitempty"`
}

// ArtifactDetailResponse represents the response for artifact detail operations
type ArtifactDetailResponse struct {
	Status struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"status"`
	Data ArtifactDetail `json:"data"`
}

// ListArtifacts lists artifacts in a ar
func (c *Client) ListArtifacts(
	registryRef string,
	labels []string,
	page, size int,
	searchTerm string,
) (*ListArtifactsResponse, error) {
	endpoint := fmt.Sprintf("/registry/%s/+/artifacts", client.GetRef(c.AccountID, c.OrgID, c.ProjectID, registryRef))

	// Build query parameters
	params := url.Values{}
	if page > 0 {
		params.Add("page", strconv.Itoa(page))
	}
	if size > 0 {
		params.Add("size", strconv.Itoa(size))
	}
	if len(labels) > 0 {
		for _, label := range labels {
			params.Add("label", label)
		}
	}
	if searchTerm != "" {
		params.Add("search_term", searchTerm)
	}

	// Add query parameters to endpoint
	if len(params) > 0 {
		endpoint = fmt.Sprintf("%s?%s", endpoint, params.Encode())
	}

	resp, err := c.doRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var listResp ListArtifactsResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return &listResp, nil
}

// GetArtifactDetail retrieves details of an artifact version
func (c *Client) GetArtifactDetail(registryRef, artifact, version string) (*ArtifactDetailResponse, error) {
	endpoint := fmt.Sprintf("/ar/%s/artifact/%s/version/%s/details",
		url.PathEscape(registryRef), url.PathEscape(artifact), url.PathEscape(version))

	resp, err := c.doRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var detailResp ArtifactDetailResponse
	if err := json.NewDecoder(resp.Body).Decode(&detailResp); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return &detailResp, nil
}

// DeleteArtifactVersion deletes a specific artifact version
func (c *Client) DeleteArtifactVersion(registryRef, artifact, version string) error {
	endpoint := fmt.Sprintf("/ar/%s/artifact/%s/version/%s",
		url.PathEscape(registryRef), url.PathEscape(artifact), url.PathEscape(version))

	resp, err := c.doRequest("DELETE", endpoint, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// ArtifactLabelRequest represents the request to update artifact labels
type ArtifactLabelRequest struct {
	Labels []string `json:"labels"`
}

// ArtifactLabelResponse represents the response for updating artifact labels
type ArtifactLabelResponse struct {
	Status struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"status"`
	Data struct {
		Labels []string `json:"labels"`
	} `json:"data"`
}

// UpdateArtifactLabels updates the labels for an artifact
func (c *Client) UpdateArtifactLabels(registryRef, artifact string, labels []string) (
	*ArtifactLabelResponse,
	error,
) {
	req := ArtifactLabelRequest{Labels: labels}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	endpoint := fmt.Sprintf("/ar/%s/artifact/%s/labels",
		url.PathEscape(registryRef), url.PathEscape(artifact))

	resp, err := c.doRequest("PUT", endpoint, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var labelResp ArtifactLabelResponse
	if err := json.NewDecoder(resp.Body).Decode(&labelResp); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return &labelResp, nil
}

// ArtifactSummaryResponse represents the response for getting an artifact summary
type ArtifactSummaryResponse struct {
	Status struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"status"`
	Data struct {
		Name              string   `json:"name"`
		LatestVersion     string   `json:"latestVersion"`
		Description       string   `json:"description"`
		Labels            []string `json:"labels"`
		AvailableVersions int      `json:"availableVersions"`
		TotalDownloads    int64    `json:"totalDownloads"`
		ArtifactSize      string   `json:"artifactSize"`
	} `json:"data"`
}

// GetArtifactSummary gets a summary of an artifact
func (c *Client) GetArtifactSummary(registryRef, artifact string) (*ArtifactSummaryResponse, error) {
	endpoint := fmt.Sprintf("/ar/%s/artifact/%s/summary",
		url.PathEscape(registryRef), url.PathEscape(artifact))

	resp, err := c.doRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var summaryResp ArtifactSummaryResponse
	if err := json.NewDecoder(resp.Body).Decode(&summaryResp); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return &summaryResp, nil
}

// ClientSetupResponse represents the response for getting client setup details
type ClientSetupResponse struct {
	Status struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"status"`
	Data struct {
		MainHeader string `json:"mainHeader"`
		SecHeader  string `json:"secHeader"`
		Sections   []struct {
			Header  string `json:"header"`
			Content string `json:"content"`
		} `json:"sections"`
	} `json:"data"`
}

// GetClientSetupDetails gets client setup details for a ar
func (c *Client) GetClientSetupDetails(registryRef string, artifact, version string) (*ClientSetupResponse, error) {
	endpoint := fmt.Sprintf("/ar/%s/client-setup-details", url.PathEscape(registryRef))

	// Add optional parameters
	params := url.Values{}
	if artifact != "" {
		params.Add("artifact", artifact)
	}
	if version != "" {
		params.Add("version", version)
	}

	// Add query parameters to endpoint
	if len(params) > 0 {
		endpoint = fmt.Sprintf("%s?%s", endpoint, params.Encode())
	}

	resp, err := c.doRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var setupResp ClientSetupResponse
	if err := json.NewDecoder(resp.Body).Decode(&setupResp); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return &setupResp, nil
}

// Utility Methods

// doRequest performs an HTTP request to the API
func (c *Client) doRequest(method, path string, body []byte) (*http.Response, error) {
	requestURL, _ := url.JoinPath(c.BaseURL, c.prefix)
	requestURL += path
	var req *http.Request
	var err error

	fmt.Println(requestURL)

	if body != nil {
		req, err = http.NewRequest(method, requestURL, bytes.NewBuffer(body))
	} else {
		req, err = http.NewRequest(method, requestURL, nil)
	}

	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.AuthToken != "" {
		// Use x-clients-key header instead of Authorization
		req.Header.Set("x-api-key", c.AuthToken)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		var errorResp struct {
			Status struct {
				Success bool   `json:"success"`
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"status"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&errorResp); err != nil {
			return nil, fmt.Errorf("HTTP error: %s", resp.Status)
		}

		return nil, fmt.Errorf("API error (%s): %s - %s",
			resp.Status, errorResp.Status.Code, errorResp.Status.Message)
	}

	return resp, nil
}

// StartMigration initiates a new migration
func (c *Client) StartMigration(req MigrationRequest) (string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("error marshaling request: %w", err)
	}

	resp, err := c.doRequest("POST", "/clients/v1/migration/start", body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var migResp MigrationResponse
	if err := json.NewDecoder(resp.Body).Decode(&migResp); err != nil {
		return "", fmt.Errorf("error decoding response: %w", err)
	}

	return migResp.ID, nil
}

// GetMigrationStatus retrieves the status of a migration
func (c *Client) GetMigrationStatus(migrationID string) (*MigrationStatus, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/clients/v1/migration/%s/status", migrationID), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var status MigrationStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return &status, nil
}

// UpdateArtifactStatus updates the status of an artifact in a migration
func (c *Client) UpdateArtifactStatus(migrationID string, req ArtifactUpdateRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("error marshaling request: %w", err)
	}

	resp, err := c.doRequest("PUT", fmt.Sprintf("/clients/v1/migration/%s/update", migrationID), body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// GetArtifacts retrieves all artifacts in a ar
func (c *Client) GetArtifacts(registryID string) ([]map[string]interface{}, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/clients/v1/migration/artifacts/%s", registryID), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var artifacts []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&artifacts); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return artifacts, nil
}

// MigrationRequest represents the request body for starting a migration
type MigrationRequest struct {
	RegistryID        string `json:"registryID"`
	AccountIdentifier string `json:"accountIdentifier"`
	TotalImages       int    `json:"totalImages"`
}

// MigrationResponse represents the response from a migration API call
type MigrationResponse struct {
	ID string `json:"id"`
}

// MigrationStatus represents the status of a migration
type MigrationStatus struct {
	ID          string         `json:"id"`
	Registry    string         `json:"ar"`
	ParentRef   string         `json:"parentRef"`
	TotalImages int            `json:"totalImages"`
	Status      StatusCounters `json:"status"`
}

// StatusCounters represents the counters for different statuses
type StatusCounters struct {
	NotStarted int `json:"NOTSTARTED"`
	Started    int `json:"STARTED"`
	Completed  int `json:"COMPLETED"`
	Failed     int `json:"FAILED"`
	Skipped    int `json:"SKIPPED"`
}

// ArtifactStatus represents valid status values for artifact updates
type ArtifactStatus string

const (
	StatusStarted   ArtifactStatus = "STARTED"
	StatusCompleted ArtifactStatus = "COMPLETED"
	StatusFailed    ArtifactStatus = "FAILED"
	StatusSkipped   ArtifactStatus = "SKIPPED"
)

// ArtifactUpdateRequest represents the request body for updating an artifact status
type ArtifactUpdateRequest struct {
	Image   string         `json:"image,omitempty"`
	Package string         `json:"package,omitempty"`
	Version string         `json:"version,omitempty"`
	Status  ArtifactStatus `json:"status"`
	Error   string         `json:"error,omitempty"`
}

// CreateRegistryRequest represents the request to create a ar
type CreateRegistryRequest struct {
	Name              string `json:"name"`
	Type              string `json:"type"`
	AccountIdentifier string `json:"accountIdentifier"`
	OrganizationID    string `json:"organizationId,omitempty"`
	ProjectID         string `json:"projectId,omitempty"`
}
