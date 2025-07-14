package jfrog

import (
	"encoding/json"
	"fmt"
	"io"
	http2 "net/http"
	"strings"

	"github.com/harness/harness-cli/module/ar/migrate/http"
	"github.com/harness/harness-cli/module/ar/migrate/http/auth/bearer"
	"github.com/harness/harness-cli/module/ar/migrate/lib"
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

// newClient constructs a jfrog client
func newClient(reg *types.RegistryConfig) *client {
	username, password := "", ""

	username = reg.Credentials.Username
	password = reg.Credentials.Password

	return &client{
		client: http.NewClient(
			&http2.Client{
				Transport: http.GetHTTPTransport(http.WithInsecure(true)),
			},
			bearer.NewAuthorizer(password),
		),
		url:      reg.Endpoint,
		insecure: true,
		username: username,
		password: password,
	}
}

type client struct {
	client   *http.Client
	url      string
	insecure bool
	username string
	password string
}

// JFrogPackage represents a file entry from JFrog Artifactory
type JFrogPackage struct {
	Registry string
	Path     string
	Name     string
	Size     int
}

type JFrogRepository struct {
	Key         string `json:"key"`
	Type        string `json:"type"`
	Url         string `json:"url"`
	Description string `json:"description"`
	PackageType string `json:"packageType"`
}

func (c *client) getRegistries() ([]JFrogRepository, error) {
	url := fmt.Sprintf("%s/artifactory/api/repositories", c.url)

	// Make GET request to fetch repositories
	var repositories []JFrogRepository
	err := c.client.Get(url, &repositories)
	if err != nil {
		return nil, fmt.Errorf("failed to get repositories: %w", err)
	}

	return repositories, nil
}

func (c *client) getRegistry(registry string) (JFrogRepository, error) {
	repositories, err := c.getRegistries()
	if err != nil {
		return JFrogRepository{}, fmt.Errorf("failed to get repositories: %w", err)
	}

	for _, repo := range repositories {
		if repo.Key == registry {
			return repo, nil
		}
	}

	return JFrogRepository{}, fmt.Errorf("registry %s not found", registry)
}

func (c *client) getFile(registry string, path string) (io.ReadCloser, http2.Header, error) {
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/")

	var url string
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		url = path
	} else {
		url = fmt.Sprintf("%s/artifactory/%s/%s", c.url, registry, path)
	}

	// Create GET request
	req, err := http2.NewRequest(http2.MethodGet, url, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request for file '%s': %w", path, err)
	}

	// Execute request with our client (which handles authentication)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to download file '%s': %w", path, err)
	}

	// Check for successful response
	if resp.StatusCode != http2.StatusOK {
		err := resp.Body.Close()
		if err != nil {
			return nil, nil, err
		} // Ensure we don't leak connection
		return nil, nil, fmt.Errorf("failed to download file '%s', status code: %d", path, resp.StatusCode)
	}

	// Return the body and headers, the caller must close the body when done
	return resp.Body, resp.Header, nil
}

// getFiles retrieves a list of files from the specified JFrog Artifactory registry
func (c *client) getFiles(registry string) ([]types.File, error) {
	repo, err := c.getRegistry(registry)
	if err != nil {
		return nil, fmt.Errorf("failed to get registry %s: %w", registry, err)
	}
	if repo.Type == "VIRTUAL" {
		return nil, fmt.Errorf("registry %s is a virtual repository", registry)
	}

	// Make GET request to fetch files
	url := fmt.Sprintf("%s/artifactory/api/storage/%s?list&deep=1", c.url, registry)

	// Define response structure for file list
	type fileListResponse struct {
		Files []struct {
			Uri          string `json:"uri"`
			Folder       bool   `json:"folder"`
			Size         int    `json:"size,omitempty"`
			LastModified string `json:"lastModified,omitempty"`
			SHA1         string `json:"sha1,omitempty"`
			SHA2         string `json:"sha2,omitempty"`
		} `json:"files"`
	}

	// Make GET request to fetch files
	var fileList fileListResponse
	err = c.client.Get(url, &fileList)
	if err != nil {
		return nil, fmt.Errorf("failed to get files from registry '%s': %w", registry, err)
	}

	// Convert response to JFrogPackage slice
	var result []types.File
	for _, file := range fileList.Files {
		// Skip folders
		if file.Folder {
			continue
		}

		f := types.File{
			Registry:     registry,
			Name:         getFileName(file.Uri),
			Uri:          file.Uri,
			Folder:       file.Folder,
			Size:         file.Size,
			LastModified: file.LastModified,
			SHA1:         file.SHA1,
			SHA2:         file.SHA2,
		}

		result = append(result, f)
	}

	return result, nil
}

func getFileName(uri string) string {
	// Normalize the URI by removing any leading/trailing slashes
	uri = strings.TrimPrefix(uri, "/")
	uri = strings.TrimSuffix(uri, "/")

	// Handle empty URI
	if uri == "" {
		return ""
	}

	// Split the URI by path separator
	parts := strings.Split(uri, "/")

	// Return the last part, which should be the filename
	return parts[len(parts)-1]
}

func buildCatalogURL(endpoint, repo string) string {
	return fmt.Sprintf("%s/artifactory/api/docker/%s/v2/_catalog?n=1000", endpoint, repo)
}

func (c *client) getCatalog(registry string) (repositories []string, err error) {
	url := buildCatalogURL(c.url, registry)
	for {
		repos, next, err := c.catalog(url)
		if err != nil {
			return nil, err
		}
		repositories = append(repositories, repos...)

		url = next
		// no next page, end the loop
		if len(url) == 0 {
			break
		}
		// relative URL
		if !strings.Contains(url, "://") {
			url = c.url + url
		}
	}
	return repositories, nil
}

func (c *client) catalog(url string) ([]string, string, error) {
	req, err := http2.NewRequest(http2.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	repositories := struct {
		Repositories []string `json:"repositories"`
	}{}
	if err := json.Unmarshal(body, &repositories); err != nil {
		return nil, "", err
	}
	return repositories.Repositories, next(resp.Header.Get("Link")), nil
}

// parse the next page link from the link header
func next(link string) string {
	links := lib.ParseLinks(link)
	for _, lk := range links {
		if lk.Rel == "next" {
			return lk.URL
		}
	}
	return ""
}
