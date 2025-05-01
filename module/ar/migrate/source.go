package migrate

import (
	"encoding/json"
	"fmt"
	"harness/module/ar/migrate/types"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// JFrogSourceRegistry implements the SourceRegistry interface for JFrog Artifactory
type JFrogSourceRegistry struct {
	config     types.SourceConfig
	httpClient *http.Client
}

// NewJFrogSourceRegistry creates a new JFrog source ar client
func NewJFrogSourceRegistry(cfg types.SourceConfig) (*JFrogSourceRegistry, error) {
	if cfg.Type != "JFROG" {
		return nil, fmt.Errorf("%w: expected JFROG, got %s", ErrUnsupportedRegistryType, cfg.Type)
	}

	return &JFrogSourceRegistry{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// ListArtifacts lists all artifacts in a JFrog ar
func (j *JFrogSourceRegistry) ListArtifacts(registry string) ([]Artifact, error) {
	// Check if the requested ar is in the filtered registries
	registryFound := false
	for _, r := range j.config.Filters.Registries {
		if r == registry {
			registryFound = true
			break
		}
	}

	if !registryFound && len(j.config.Filters.Registries) > 0 {
		return nil, fmt.Errorf("%w: %s not in filtered registries", ErrRegistryNotFound, registry)
	}

	// Build JFrog AQL query to list artifacts
	// This is a simplified example - real implementation would need to handle pagination and more complex filtering
	aqlQuery := map[string]interface{}{
		"repo": registry,
	}

	// Add filters for artifact types if specified
	if len(j.config.Filters.ArtifactTypes) > 0 {
		// This is a simplified approach - actual implementation would depend on
		// how artifact types are represented in JFrog Artifactory
		typeFilters := make([]interface{}, 0, len(j.config.Filters.ArtifactTypes))
		for _, t := range j.config.Filters.ArtifactTypes {
			typeFilters = append(typeFilters, map[string]string{
				"$match": t,
			})
		}
		aqlQuery["type"] = map[string]interface{}{
			"$or": typeFilters,
		}
	}

	// Add name pattern filters
	if len(j.config.Filters.ArtifactNamePatterns.Include) > 0 || len(j.config.Filters.ArtifactNamePatterns.Exclude) > 0 {
		nameQuery := make(map[string]interface{})

		if len(j.config.Filters.ArtifactNamePatterns.Include) > 0 {
			includeFilters := make([]interface{}, 0, len(j.config.Filters.ArtifactNamePatterns.Include))
			for _, pattern := range j.config.Filters.ArtifactNamePatterns.Include {
				includeFilters = append(includeFilters, map[string]string{
					"$match": pattern,
				})
			}
			nameQuery["$or"] = includeFilters
		}

		if len(j.config.Filters.ArtifactNamePatterns.Exclude) > 0 {
			excludeFilters := make([]interface{}, 0, len(j.config.Filters.ArtifactNamePatterns.Exclude))
			for _, pattern := range j.config.Filters.ArtifactNamePatterns.Exclude {
				excludeFilters = append(excludeFilters, map[string]string{
					"$nmatch": pattern,
				})
			}
			nameQuery["$and"] = excludeFilters
		}

		aqlQuery["name"] = nameQuery
	}

	// Execute the AQL query against JFrog API
	artifacts, err := j.executeAQLQuery(aqlQuery)
	if err != nil {
		return nil, fmt.Errorf("error listing artifacts: %w", err)
	}

	return artifacts, nil
}

// executeAQLQuery executes an AQL query against JFrog Artifactory
func (j *JFrogSourceRegistry) executeAQLQuery(query map[string]interface{}) ([]Artifact, error) {
	baseURL, err := url.Parse(j.config.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint URL: %w", err)
	}

	// Construct the AQL API URL
	aqlURL := *baseURL
	aqlURL.Path = path.Join(aqlURL.Path, "clients/search/aql")

	// Convert query to AQL format
	aqlBytes, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("error marshaling AQL query: %w", err)
	}

	// Create request
	req, err := http.NewRequest("POST", aqlURL.String(), strings.NewReader(string(aqlBytes)))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	// Set authentication headers
	req.SetBasicAuth(j.config.Credentials.Username, j.config.Credentials.Password)
	req.Header.Set("Content-Type", "text/plain")

	// Execute request
	resp, err := j.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrInvalidCredentials
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("JFrog API error (%d): %s", resp.StatusCode, body)
	}

	// Parse response
	var jfrogResponse struct {
		Results []struct {
			Repo    string `json:"repo"`
			Path    string `json:"path"`
			Name    string `json:"name"`
			Type    string `json:"type"`
			Size    int64  `json:"size"`
			Created string `json:"created"`
			// Add more fields as needed
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&jfrogResponse); err != nil {
		return nil, fmt.Errorf("error parsing JFrog response: %w", err)
	}

	// Convert to our Artifact type
	artifacts := make([]Artifact, 0, len(jfrogResponse.Results))
	for _, item := range jfrogResponse.Results {
		// Parse artifact type and version from name
		// This is a simplified approach - real implementation would need more sophisticated parsing
		parts := strings.Split(item.Name, "-")
		version := parts[len(parts)-1]

		if strings.HasSuffix(version, ".jar") {
			version = strings.TrimSuffix(version, ".jar")
		} else if strings.HasSuffix(version, ".tgz") {
			version = strings.TrimSuffix(version, ".tgz")
		} else if strings.HasSuffix(version, ".zip") {
			version = strings.TrimSuffix(version, ".zip")
		}

		artifact := Artifact{
			Name:     strings.Join(parts[:len(parts)-1], "-"),
			Version:  version,
			Type:     item.Type,
			Registry: item.Repo,
			Size:     item.Size,
			Properties: map[string]string{
				"path": item.Path,
			},
		}

		artifacts = append(artifacts, artifact)
	}

	return artifacts, nil
}

// DownloadArtifact downloads an artifact from JFrog Artifactory
func (j *JFrogSourceRegistry) DownloadArtifact(artifact Artifact) ([]byte, error) {
	baseURL, err := url.Parse(j.config.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint URL: %w", err)
	}

	// Construct the artifact download URL
	downloadURL := *baseURL

	// Determine the artifact path
	// This is a simplified example - real implementation would need to handle different artifact types differently
	artifactPath := path.Join(
		artifact.Registry,
		artifact.Properties["path"],
		fmt.Sprintf("%s-%s", artifact.Name, artifact.Version),
	)

	// Add file extension based on artifact type
	switch strings.ToLower(artifact.Type) {
	case "jar", "java":
		artifactPath += ".jar"
	case "tgz", "tar":
		artifactPath += ".tgz"
	case "zip":
		artifactPath += ".zip"
	case "python", "py":
		artifactPath += ".whl"
	}

	downloadURL.Path = path.Join(downloadURL.Path, artifactPath)

	// Create request
	req, err := http.NewRequest("GET", downloadURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	// Set authentication headers
	req.SetBasicAuth(j.config.Credentials.Username, j.config.Credentials.Password)

	// Execute request
	resp, err := j.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrInvalidCredentials
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrArtifactNotFound
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("JFrog API error (%d): %s", resp.StatusCode, body)
	}

	// Read the entire response body
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	return data, nil
}
