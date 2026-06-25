package harbor

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	httputil "github.com/harness/harness-cli/module/ar/migrate/http"
	"github.com/harness/harness-cli/module/ar/migrate/http/auth/basic"
	"github.com/harness/harness-cli/module/ar/migrate/lib"
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

const (
	harborAPIVersion = "v2.0"
	pageSize         = 100
)

// HarborProject represents a Harbor project
type HarborProject struct {
	Name string `json:"name"`
	ID   int    `json:"id"`
}

// HarborRepository represents a repository within a Harbor project
type HarborRepository struct {
	// Full name is "<project>/<repo>"
	Name          string `json:"name"`
	ArtifactCount int64  `json:"artifact_count"`
}

type client struct {
	client *httputil.Client
	url    string
}

func newClient(reg *types.RegistryConfig) *client {
	url := strings.TrimSuffix(reg.Endpoint, "/")
	return &client{
		client: httputil.NewClient(
			&http.Client{
				Transport: httputil.GetHTTPTransport(httputil.WithInsecure(reg.Insecure)),
			},
			basic.NewAuthorizer(reg.Credentials.Username, reg.Credentials.Password),
		),
		url: url,
	}
}

// health checks connectivity by hitting the Harbor health endpoint
func (c *client) health() error {
	url := fmt.Sprintf("%s/api/%s/health", c.url, harborAPIVersion)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// getProject retrieves project metadata by name
func (c *client) getProject(project string) (HarborProject, error) {
	url := fmt.Sprintf("%s/api/%s/projects/%s", c.url, harborAPIVersion, project)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return HarborProject{}, fmt.Errorf("create request: %w", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return HarborProject{}, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return HarborProject{}, fmt.Errorf("project %q not found", project)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return HarborProject{}, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	var p HarborProject
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return HarborProject{}, fmt.Errorf("decode response: %w", err)
	}
	return p, nil
}

// listRepositories returns all repositories for the given Harbor project, handling pagination
func (c *client) listRepositories(project string) ([]HarborRepository, error) {
	var all []HarborRepository
	page := 1
	for {
		url := fmt.Sprintf("%s/api/%s/projects/%s/repositories?page=%d&page_size=%d",
			c.url, harborAPIVersion, project, page, pageSize)
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		resp, err := c.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("execute request: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
		}

		var repos []HarborRepository
		if err := json.Unmarshal(body, &repos); err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}
		all = append(all, repos...)

		// If a Link header with rel=next exists, follow it; otherwise stop
		nextURL := nextPage(resp.Header.Get("Link"))
		if nextURL == "" || len(repos) < pageSize {
			break
		}
		page++
	}
	return all, nil
}

// nextPage extracts the next-page URL from a Link header, if present
func nextPage(linkHeader string) string {
	links := lib.ParseLinks(linkHeader)
	for _, lk := range links {
		if lk.Rel == "next" {
			return lk.URL
		}
	}
	return ""
}

// repoShortName strips the "<project>/" prefix Harbor prepends to repository names
func repoShortName(project, fullName string) string {
	prefix := project + "/"
	return strings.TrimPrefix(fullName, prefix)
}
