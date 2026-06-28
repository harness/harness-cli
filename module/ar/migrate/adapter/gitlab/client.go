package gitlab

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/harness/harness-cli/module/ar/migrate/types"
	"github.com/rs/zerolog/log"
)

const (
	defaultPerPage = 100
	maxPerPage     = 100
)

// Client handles communication with GitLab Package Registry API
type Client struct {
	baseURL    string
	httpClient *http.Client
	username   string
	token      string
}

// GitLabPackage represents a package in GitLab
type GitLabPackage struct {
	ID              int64             `json:"id"`
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	PackageType     string            `json:"package_type"`
	CreatedAt       time.Time         `json:"created_at"`
	ProjectID       int64             `json:"project_id"`
	PackageFiles    []GitLabPackageFile `json:"package_files,omitempty"`
	Links           map[string]string `json:"_links,omitempty"`
}

// GitLabPackageFile represents a file within a package
type GitLabPackageFile struct {
	ID          int64     `json:"id"`
	PackageID   int64     `json:"package_id"`
	FileName    string    `json:"file_name"`
	Size        int       `json:"size"`
	FileMD5     string    `json:"file_md5"`
	FileSHA1    string    `json:"file_sha1"`
	FileSHA256  string    `json:"file_sha256"`
	CreatedAt   time.Time `json:"created_at"`
}

// GitLabProject represents a GitLab project
type GitLabProject struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	PathWithNamespace string `json:"path_with_namespace"`
	WebURL            string `json:"web_url"`
}

// newClient creates a new GitLab API client
func newClient(config *types.RegistryConfig) *Client {
	return &Client{
		baseURL:    strings.TrimSuffix(config.Endpoint, "/"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
		username:   config.Credentials.Username,
		token:      config.Credentials.Password,
	}
}

// doRequest performs an HTTP request with GitLab authentication
func (c *Client) doRequest(method, path string, body io.Reader) (*http.Response, error) {
	fullURL := c.baseURL + path

	req, err := http.NewRequest(method, fullURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// GitLab supports multiple authentication methods
	// Private-Token header is the most common for API access
	if c.token != "" {
		req.Header.Set("PRIVATE-TOKEN", c.token)
	}

	req.Header.Set("Content-Type", "application/json")

	log.Debug().
		Str("method", method).
		Str("url", fullURL).
		Msg("GitLab API request")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitLab API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	return resp, nil
}

// getProject fetches project information by path or ID
func (c *Client) getProject(projectPath string) (*GitLabProject, error) {
	// URL encode the project path (e.g., "group/project" -> "group%2Fproject")
	encodedPath := url.PathEscape(projectPath)

	resp, err := c.doRequest("GET", fmt.Sprintf("/api/v4/projects/%s", encodedPath), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var project GitLabProject
	if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
		return nil, fmt.Errorf("failed to decode project: %w", err)
	}

	return &project, nil
}

// listPackages lists all packages in a project with pagination
func (c *Client) listPackages(projectPath string, packageType string, page int) ([]GitLabPackage, bool, error) {
	encodedPath := url.PathEscape(projectPath)

	apiPath := fmt.Sprintf("/api/v4/projects/%s/packages?per_page=%d&page=%d",
		encodedPath, maxPerPage, page)

	// Filter by package type if specified
	if packageType != "" {
		apiPath += "&package_type=" + packageType
	}

	resp, err := c.doRequest("GET", apiPath, nil)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	var packages []GitLabPackage
	if err := json.NewDecoder(resp.Body).Decode(&packages); err != nil {
		return nil, false, fmt.Errorf("failed to decode packages: %w", err)
	}

	// Check if there are more pages
	hasMore := len(packages) == maxPerPage

	return packages, hasMore, nil
}

// getAllPackages retrieves all packages across all pages
func (c *Client) getAllPackages(projectPath string, packageType string) ([]GitLabPackage, error) {
	var allPackages []GitLabPackage
	page := 1

	for {
		packages, hasMore, err := c.listPackages(projectPath, packageType, page)
		if err != nil {
			return nil, err
		}

		allPackages = append(allPackages, packages...)

		if !hasMore {
			break
		}

		page++
	}

	log.Info().
		Str("project", projectPath).
		Str("packageType", packageType).
		Int("count", len(allPackages)).
		Msg("Retrieved all packages from GitLab")

	return allPackages, nil
}

// getPackageFiles retrieves all files for a specific package
func (c *Client) getPackageFiles(projectPath string, packageID int64) ([]GitLabPackageFile, error) {
	encodedPath := url.PathEscape(projectPath)

	resp, err := c.doRequest("GET",
		fmt.Sprintf("/api/v4/projects/%s/packages/%d/package_files", encodedPath, packageID),
		nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var files []GitLabPackageFile
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, fmt.Errorf("failed to decode package files: %w", err)
	}

	return files, nil
}

// downloadFile downloads a file from GitLab
func (c *Client) downloadFile(downloadURL string) (io.ReadCloser, http.Header, error) {
	// For GitLab, package files can be downloaded via their direct URLs
	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create download request: %w", err)
	}

	if c.token != "" {
		req.Header.Set("PRIVATE-TOKEN", c.token)
	}

	log.Debug().Str("url", downloadURL).Msg("Downloading file from GitLab")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("download failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, nil, fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	return resp.Body, resp.Header, nil
}

// getRegistry returns registry information for a project
func (c *Client) getRegistry(projectPath string) (types.RegistryInfo, error) {
	project, err := c.getProject(projectPath)
	if err != nil {
		return types.RegistryInfo{}, err
	}

	return types.RegistryInfo{
		Type: "gitlab",
		URL:  project.WebURL,
		Path: project.PathWithNamespace,
	}, nil
}

// getCatalog retrieves the OCI catalog for Container Registry (Docker images)
// Uses the Docker Registry V2 API that GitLab Container Registry implements
func (c *Client) getCatalog(projectPath string) ([]string, error) {
	// GitLab Container Registry uses the standard OCI Distribution API
	// Endpoint: /v2/_catalog
	// But we need to construct it properly for the project's registry

	// First get the project to get its ID
	project, err := c.getProject(projectPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	// For GitLab Container Registry, we need to use the registry endpoint
	// Format: registry.gitlab.com/v2/<project_path>/_catalog
	// or for self-hosted: <gitlab-host>/v2/<project_path>/_catalog

	registryHost := strings.Replace(c.baseURL, "https://", "https://registry.", 1)
	if !strings.Contains(c.baseURL, "gitlab.com") {
		// For self-hosted GitLab, registry is typically at the same host
		registryHost = c.baseURL
	}

	catalogURL := fmt.Sprintf("%s/v2/%s/_catalog", registryHost, url.PathEscape(projectPath))

	req, err := http.NewRequest("GET", catalogURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create catalog request: %w", err)
	}

	// Container Registry uses token authentication
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	log.Debug().Str("url", catalogURL).Msg("Fetching OCI catalog")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("catalog request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Debug().Int("status", resp.StatusCode).Str("body", string(bodyBytes)).Msg("Catalog request non-200")

		// Try alternative: get repositories via API
		return c.getRepositoriesViaAPI(project.ID)
	}

	var catalog struct {
		Repositories []string `json:"repositories"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&catalog); err != nil {
		return nil, fmt.Errorf("failed to decode catalog: %w", err)
	}

	log.Info().Strs("repositories", catalog.Repositories).Msg("Found Docker repositories in Container Registry")

	return catalog.Repositories, nil
}

// ContainerRepository represents a GitLab container repository with tags
type ContainerRepository struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Location string `json:"location"`
}

// ContainerTag represents a tag within a container repository
type ContainerTag struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Location  string    `json:"location"`
	Revision  string    `json:"revision"`
	ShortRevision string `json:"short_revision"`
	Digest    string    `json:"digest"`
	CreatedAt time.Time `json:"created_at"`
	TotalSize int64     `json:"total_size"`
}

// getRepositoriesViaAPI gets container repositories via GitLab API as a fallback
func (c *Client) getRepositoriesViaAPI(projectID int64) ([]string, error) {
	// GitLab API v4: /projects/:id/registry/repositories
	resp, err := c.doRequest("GET", fmt.Sprintf("/api/v4/projects/%d/registry/repositories", projectID), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get repositories via API: %w", err)
	}
	defer resp.Body.Close()

	var repos []ContainerRepository

	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		return nil, fmt.Errorf("failed to decode repositories: %w", err)
	}

	repositories := make([]string, 0, len(repos))
	for _, repo := range repos {
		// Use the path which includes the full image path
		repositories = append(repositories, repo.Path)
	}

	log.Info().Int("count", len(repositories)).Msg("Found Docker repositories via GitLab API")

	return repositories, nil
}

// getRepositoryTags gets all tags for a container repository
func (c *Client) getRepositoryTags(projectID int64, repositoryID int64) ([]ContainerTag, error) {
	// GitLab API v4: /projects/:id/registry/repositories/:repository_id/tags
	resp, err := c.doRequest("GET",
		fmt.Sprintf("/api/v4/projects/%d/registry/repositories/%d/tags", projectID, repositoryID), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get repository tags: %w", err)
	}
	defer resp.Body.Close()

	var tags []ContainerTag
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, fmt.Errorf("failed to decode tags: %w", err)
	}

	return tags, nil
}

// getContainerRepositoriesWithTags gets all container repositories with their tags
func (c *Client) getContainerRepositoriesWithTags(projectPath string) ([]ContainerRepository, map[int64][]ContainerTag, error) {
	// Get project first
	project, err := c.getProject(projectPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get project: %w", err)
	}

	// Get repositories
	resp, err := c.doRequest("GET", fmt.Sprintf("/api/v4/projects/%d/registry/repositories", project.ID), nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get repositories: %w", err)
	}
	defer resp.Body.Close()

	var repos []ContainerRepository
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		return nil, nil, fmt.Errorf("failed to decode repositories: %w", err)
	}

	// Get tags for each repository
	allTags := make(map[int64][]ContainerTag)
	for _, repo := range repos {
		tags, err := c.getRepositoryTags(project.ID, repo.ID)
		if err != nil {
			log.Warn().Err(err).Int64("repoID", repo.ID).Msg("Failed to get tags, skipping")
			continue
		}
		allTags[repo.ID] = tags
		log.Info().
			Str("repository", repo.Name).
			Int("tagCount", len(tags)).
			Msg("Retrieved tags for Docker repository")
	}

	return repos, allTags, nil
}

// mapGitLabPackageType maps GitLab package types to migration artifact types
func mapGitLabPackageType(gitlabType string) types.ArtifactType {
	switch strings.ToLower(gitlabType) {
	case "maven":
		return types.MAVEN
	case "npm":
		return types.NPM
	case "pypi":
		return types.PYTHON
	case "nuget":
		return types.NUGET
	case "composer":
		return types.COMPOSER
	case "conan":
		return types.GENERIC
	case "helm":
		return types.HELM
	case "debian":
		return types.DEBIAN
	case "generic":
		return types.GENERIC
	case "golang":
		return types.GO
	default:
		log.Warn().Str("gitlabType", gitlabType).Msg("Unknown GitLab package type, using GENERIC")
		return types.GENERIC
	}
}
