package nexus

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	httputil "github.com/harness/harness-cli/module/ar/migrate/http"
	"github.com/harness/harness-cli/module/ar/migrate/http/auth/basic"
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

// newClient constructs a nexus client
func newClient(reg *types.RegistryConfig) *client {
	username, password := "", ""

	username = reg.Credentials.Username
	password = reg.Credentials.Password
	url := reg.Endpoint
	url = strings.TrimSuffix(url, "/")

	return &client{
		client: httputil.NewClient(
			&http.Client{
				Transport: httputil.GetHTTPTransport(httputil.WithInsecure(true)),
			},
			basic.NewAuthorizer(username, password),
		),
		url:      url,
		insecure: true,
		username: username,
		password: password,
	}
}

type client struct {
	client   *httputil.Client
	url      string
	insecure bool
	username string
	password string
}

// NexusRepository represents a repository from Nexus V3
type NexusRepository struct {
	Name   string `json:"name"`
	Format string `json:"format"`
	Type   string `json:"type"`
	URL    string `json:"url"`
	Online bool   `json:"online"`
}

// NexusAsset represents an asset from Nexus V3
type NexusAsset struct {
	ID         string                 `json:"id"`
	Repository string                 `json:"repository"`
	Format     string                 `json:"format"`
	Path       string                 `json:"path"`
	Checksum   map[string]string      `json:"checksum"`
	FileSize   int64                  `json:"fileSize"`
	Attributes map[string]interface{} `json:"attributes"`
}

// NexusComponent represents a component from Nexus V3
type NexusComponent struct {
	ID         string                 `json:"id"`
	Repository string                 `json:"repository"`
	Format     string                 `json:"format"`
	Group      string                 `json:"group"`
	Name       string                 `json:"name"`
	Version    string                 `json:"version"`
	Assets     []NexusAsset           `json:"assets"`
	Attributes map[string]interface{} `json:"attributes"`
}

// NexusSearchResponse represents the search response from Nexus V3
type NexusSearchResponse struct {
	Items             []NexusComponent `json:"items"`
	ContinuationToken string           `json:"continuationToken"`
}

// NexusRepositoryDetails represents detailed repository information from Nexus V3
type NexusRepositoryDetails struct {
	Name   string             `json:"name"`
	Docker *NexusDockerConfig `json:"docker,omitempty"`
}

// NexusDockerConfig represents Docker-specific configuration
type NexusDockerConfig struct {
	V1Enabled      bool   `json:"v1Enabled"`
	ForceBasicAuth bool   `json:"forceBasicAuth"`
	HttpPort       int    `json:"httpPort,omitempty"`
	HttpsPort      int    `json:"httpsPort,omitempty"`
	Subdomain      string `json:"subdomain,omitempty"`
}

// getRepositories retrieves all repositories from Nexus
func (c *client) getRepositories() ([]NexusRepository, error) {
	url := fmt.Sprintf("%s/service/rest/v1/repositories", strings.TrimSuffix(c.url, "/"))

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if c.username != "" && c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var repositories []NexusRepository
	if err := json.NewDecoder(resp.Body).Decode(&repositories); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return repositories, nil
}

// getRepository retrieves a specific repository by name
func (c *client) getRepository(name string) (NexusRepository, error) {
	repositories, err := c.getRepositories()
	if err != nil {
		return NexusRepository{}, err
	}

	for _, repo := range repositories {
		if repo.Name == name {
			return repo, nil
		}
	}

	return NexusRepository{}, fmt.Errorf("repository %s not found", name)
}

// getRepositoryDetails retrieves detailed repository configuration
func (c *client) getRepositoryDetails(name, packageType string) (*NexusRepositoryDetails, error) {
	url := fmt.Sprintf("%s/service/rest/v1/repositories/%s/hosted/%s", strings.TrimSuffix(c.url, "/"), packageType,
		name)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var repoDetails NexusRepositoryDetails
	if err := json.NewDecoder(resp.Body).Decode(&repoDetails); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &repoDetails, nil
}

// searchComponents searches for components in a repository
func (c *client) searchComponents(repository string, continuationToken string) (*NexusSearchResponse, error) {
	url := fmt.Sprintf("%s/service/rest/v1/search", strings.TrimSuffix(c.url, "/"))

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	q := req.URL.Query()
	q.Add("repository", repository)
	if continuationToken != "" {
		q.Add("continuationToken", continuationToken)
	}
	req.URL.RawQuery = q.Encode()

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var searchResponse NexusSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &searchResponse, nil
}

// getAsset downloads an asset by its download URL
func (c *client) getAsset(downloadURL string) (io.ReadCloser, http.Header, error) {
	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to execute request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return resp.Body, resp.Header, nil
}

// uploadAsset uploads an asset to Nexus
func (c *client) uploadAsset(repository string, file io.ReadCloser, filename string, path string) error {
	url := fmt.Sprintf("%s/service/rest/v1/components", strings.TrimSuffix(c.url, "/"))

	// Create multipart form data using standard library
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	// Process multipart form asynchronously
	go func() {
		defer pw.Close()
		defer writer.Close()

		// Add repository field
		if err := writer.WriteField("repository", repository); err != nil {
			pw.CloseWithError(err)
			return
		}

		// Add the file
		part, err := writer.CreateFormFile("asset", filename)
		if err != nil {
			pw.CloseWithError(err)
			return
		}

		// Copy the file content
		if _, err := io.Copy(part, file); err != nil {
			pw.CloseWithError(err)
			return
		}
	}()

	req, err := http.NewRequest("POST", url, pr)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	if c.username != "" && c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

// getFiles retrieves a list of files from the specified Nexus repository
func (c *client) getFiles(repository, packageType string) ([]types.File, error) {
	var allFiles []types.File
	continuationToken := ""

	for {
		searchResponse, err := c.searchComponents(repository, continuationToken)
		if err != nil {
			return nil, fmt.Errorf("failed to search components: %w", err)
		}

		for _, component := range searchResponse.Items {
			for _, asset := range component.Assets {
				file := types.File{
					Name:     getFileName(asset.Path),
					Registry: repository,
					Uri:      asset.Path,
					Folder:   false,
					Size:     int(asset.FileSize),
					SHA1:     asset.Checksum["sha1"],
					SHA2:     asset.Checksum["sha256"],
				}
				allFiles = append(allFiles, file)
			}
		}

		if searchResponse.ContinuationToken == "" {
			break
		}
		continuationToken = searchResponse.ContinuationToken
	}

	return allFiles, nil
}

// getFileName extracts filename from a path
func getFileName(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return path
}

// buildDownloadURL constructs the download URL for an asset
func (c *client) buildDownloadURL(repository, path string) string {
	return fmt.Sprintf("%s/repository/%s/%s", strings.TrimSuffix(c.url, "/"), repository, strings.TrimPrefix(path, "/"))
}

// getDockerPort attempts to discover the Docker HTTPS port for a repository
func (c *client) getDockerPort(repositoryName string, insecure bool) (int, error) {
	repoDetails, err := c.getRepositoryDetails(repositoryName, "docker")
	if err != nil {
		return 0, fmt.Errorf("failed to get repository details: %w", err)
	}

	// Check for Docker configuration
	if repoDetails.Docker == nil {
		return 0, fmt.Errorf("no Docker configuration found for repository %s", repositoryName)
	}

	if insecure {
		if repoDetails.Docker.HttpPort > 0 {
			return repoDetails.Docker.HttpPort, nil
		}
	}

	// Prefer HTTPS port, fallback to HTTP port
	if repoDetails.Docker.HttpsPort > 0 {
		return repoDetails.Docker.HttpsPort, nil
	}

	if repoDetails.Docker.HttpPort > 0 {
		return repoDetails.Docker.HttpPort, nil
	}

	return 0, fmt.Errorf("no Docker connector port configured for repository %s", repositoryName)
}
