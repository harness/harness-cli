package har

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	http2 "net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar"
	"github.com/harness/harness-cli/module/ar/migrate/http"
	"github.com/harness/harness-cli/module/ar/migrate/http/auth/xApiKey"
	"github.com/harness/harness-cli/module/ar/migrate/types"
	"github.com/harness/harness-cli/util/common/auth"
	retryablehttp "github.com/hashicorp/go-retryablehttp"
)

func retryingHTTPClient() *http2.Client {
	rc := retryablehttp.NewClient()
	rc.RetryMax = 5
	rc.RetryWaitMin = 200 * time.Millisecond
	rc.RetryWaitMax = 1 * time.Minute
	rc.Backoff = retryablehttp.RateLimitLinearJitterBackoff
	rc.Logger = nil

	std := rc.StandardClient() // returns *http.Client using a retrying RoundTripper
	std.Timeout = 20 * time.Second
	return std
}

// newClient constructs a jfrog client
func newClient(reg *types.RegistryConfig) *client {
	username, token := "", ""

	username = reg.Credentials.Username
	token = reg.Credentials.Password

	arClient, _ := ar.NewClientWithResponses(config.Global.APIBaseURL+"/gateway/har/api/v1",
		ar.WithHTTPClient(retryingHTTPClient()),
		auth.GetXApiKeyOptionAR())
	return &client{
		client: http.NewClient(
			&http2.Client{
				Transport: http.GetHTTPTransport(http.WithInsecure(true)),
			},
			xApiKey.NewAuthorizer(token),
		),
		url:       reg.Endpoint,
		insecure:  true,
		username:  username,
		password:  token,
		apiClient: arClient,
	}
}

type client struct {
	apiClient *ar.ClientWithResponses
	client    *http.Client
	url       string
	insecure  bool
	username  string
	password  string
}

func (c *client) uploadGenericFile(registry, artifactName, version string, f *types.File, file io.ReadCloser) error {
	url := fmt.Sprintf("%s/generic/%s/%s/%s/%s", c.url, config.Global.AccountID, registry, artifactName, version)

	// Create a pipe to write the file contents
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	// Process multipart form asynchronously
	go func() {
		defer pw.Close()
		// Add the file
		part, err := writer.CreateFormFile("file", f.Name)
		if err != nil {
			pw.CloseWithError(err)
			return
		}

		// Copy the file content
		if _, err := io.Copy(part, file); err != nil {
			pw.CloseWithError(err)
			return
		}

		// Add filename field
		if err := writer.WriteField("filename", f.Name); err != nil {
			pw.CloseWithError(err)
			return
		}

		// Add description field
		if err := writer.WriteField("description", "Uploaded via harness-cli migration tool"); err != nil {
			pw.CloseWithError(err)
			return
		}

		// Close the writer
		if err := writer.Close(); err != nil {
			pw.CloseWithError(err)
			return
		}
	}()

	// Create request
	req, err := http2.NewRequest(http2.MethodPut, url, pr)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Execute request with our client (which handles authentication)
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload file '%s/%s': %w", artifactName, version, err)
	}
	defer resp.Body.Close()

	// Check for successful response
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to upload file '%s/%s', status code: %d, response: %s",
			artifactName, version, resp.StatusCode, string(body))
	}

	return nil
}

func (c *client) uploadMavenFile(
	registry string,
	name string,
	version string,
	f *types.File,
	file io.ReadCloser,
) error {
	fileUri := strings.TrimPrefix(f.Uri, "/")
	url := fmt.Sprintf("%s/maven/%s/%s/%s", c.url, config.Global.AccountID, registry, fileUri)
	// Create request
	req, err := http2.NewRequest(http2.MethodPut, url, file)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload file '%s': %w", fileUri, err)
	}
	defer resp.Body.Close()

	// Check for successful response
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to upload file '%s', status code: %d, response: %s",
			fileUri, resp.StatusCode, string(body))
	}

	return nil
}

func (c *client) uploadNugetFile(
	registry string,
	name string,
	version string,
	f *types.File,
	file io.ReadCloser,
) error {
	fileUri := strings.TrimPrefix(f.Uri, "/")
	url := fmt.Sprintf("%s/pkg/%s/%s/nuget/", c.url, config.Global.AccountID, registry)
	if strings.HasSuffix(url, ".snupkg") {
		url = fmt.Sprintf("%s/pkg/%s/%s/nuget/symbolpackage/", c.url, config.Global.AccountID, registry)
	}

	// Create a pipe to write the multipart form data
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	// Process multipart form asynchronously
	go func() {
		defer pw.Close()
		defer writer.Close()

		// Add the file as "content" field
		part, err := writer.CreateFormFile("package", f.Name)
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

	// Create request
	req, err := http2.NewRequest(http2.MethodPut, url, pr)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Execute request with our client (which handles authentication)
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload file '%s': %w", fileUri, err)
	}
	defer resp.Body.Close()

	// Check for successful response
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to upload file '%s', status code: %d, response: %s",
			fileUri, resp.StatusCode, string(body))
	}

	return nil
}

func (c *client) uploadNPMFile(
	registry string,
	name string,
	version string,
	f *types.File,
	file io.ReadCloser,
) error {
	url := fmt.Sprintf("%s/pkg/%s/%s/npm/%s", c.url, config.Global.AccountID, registry, name)

	// Create request
	req, err := http2.NewRequest(http2.MethodPut, url, file)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload file '%s': %w", url, err)
	}
	defer resp.Body.Close()

	// Check for successful response
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to upload file '%s', status code: %d, response: %s",
			url, resp.StatusCode, string(body))
	}

	return nil
}

func (c *client) uploadRPMFile(
	registry string,
	filename string,
	file io.ReadCloser,
) error {
	fileUri := strings.TrimPrefix(filename, "/")
	url := fmt.Sprintf("%s/pkg/%s/%s/rpm/%s", c.url, config.Global.AccountID, registry, fileUri)

	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	// Process multipart form asynchronously
	go func() {
		defer pw.Close()
		defer writer.Close()

		part, err := writer.CreateFormFile("file", filename)
		if err != nil {
			pw.CloseWithError(err)
			return
		}

		if _, err := io.Copy(part, file); err != nil {
			pw.CloseWithError(err)
			return
		}
	}()

	req, err := http2.NewRequest(http2.MethodPut, url, pr)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload file '%s': %w", fileUri, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to upload file '%s', status code: %d, response: %s",
			fileUri, resp.StatusCode, string(body))
	}

	return nil
}

func (c *client) AddNPMTag(registry string, name string, version string, tagUri string) error {
	url := fmt.Sprintf("%s/pkg/%s/%s/npm", c.url, config.Global.AccountID, registry)
	url = url + tagUri
	versionJSON, err := json.Marshal(version)
	if err != nil {
		return fmt.Errorf("failed to marshal version to json: %w", err)
	}

	req, err := http2.NewRequest(http2.MethodPut, url, bytes.NewReader(versionJSON))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload file '%s': %w", url, err)
	}
	defer resp.Body.Close()

	// Check for successful response
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to upload file '%s', status code: %d, response: %s",
			url, resp.StatusCode, string(body))
	}

	return nil
}

func (c *client) uploadPythonFile(
	registry string,
	name string,
	version string,
	f *types.File,
	file io.ReadCloser,
	metadata map[string]interface{},
) error {
	fileUri := strings.TrimPrefix(f.Uri, "/")
	url := fmt.Sprintf("%s/pkg/%s/%s/python/%s", c.url, config.Global.AccountID, registry, fileUri)

	// Create a pipe to write the multipart form data
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	// Process multipart form asynchronously
	go func() {
		defer pw.Close()
		defer writer.Close()

		// Add the file as "content" field
		part, err := writer.CreateFormFile("content", f.Name)
		if err != nil {
			pw.CloseWithError(err)
			return
		}

		// Copy the file content
		if _, err := io.Copy(part, file); err != nil {
			pw.CloseWithError(err)
			return
		}

		// Add metadata fields
		if metadata != nil {
			// Iterate through all metadata and add as form fields
			for key, val := range metadata {
				switch v := val.(type) {
				case []string:
					// Handle array values
					for _, item := range v {
						if err := writer.WriteField(key, item); err != nil {
							pw.CloseWithError(err)
							return
						}
					}
				default:
					// Handle simple values
					if val != nil && fmt.Sprintf("%v", val) != "" {
						if err := writer.WriteField(key, fmt.Sprintf("%v", val)); err != nil {
							pw.CloseWithError(err)
							return
						}
					}
				}
			}
		}
	}()

	// Create request
	req, err := http2.NewRequest(http2.MethodPost, url, pr)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Execute request with our client (which handles authentication)
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload file '%s': %w", fileUri, err)
	}
	defer resp.Body.Close()

	// Check for successful response
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to upload file '%s', status code: %d, response: %s",
			fileUri, resp.StatusCode, string(body))
	}

	return nil
}

func (c *client) artifactFileExists(
	ctx context.Context,
	registryRef, pkg, version, fileURI string,
	artifactType types.ArtifactType,
) (bool, error) {
	page := int64(0)
	size := int64(100)
	fileURI = strings.TrimPrefix(fileURI, "/")

	for {
		response, err := c.apiClient.GetArtifactFilesWithResponse(ctx, registryRef, pkg, version,
			&ar.GetArtifactFilesParams{
				Page:      &page,
				Size:      &size,
				SortOrder: nil,
				SortField: nil,
			})
		if err != nil {
			return false, fmt.Errorf("failed to get artifact files: %w", err)
		}
		if response.StatusCode() != http2.StatusOK {
			return false, fmt.Errorf("failed to get artifact files: %s", response.Status())
		}
		data := response.JSON200
		for _, v := range data.Files {
			if v.Name == fileURI {
				return true, nil
			}
		}
		if len(data.Files) < int(size) || (nil != data.PageCount && nil != data.PageIndex && (*data.PageIndex+1 >= *data.PageCount)) {
			break
		}
		page++
	}
	return false, nil
}

func (c *client) getRegistry(
	ctx context.Context,
	registry string,
) (types.RegistryInfo, error) {
	page := int64(0)
	size := int64(100)
	for {
		descendants := ar.GetAllRegistriesParamsScopeDescendants
		response, err := c.apiClient.GetAllRegistriesWithResponse(ctx, config.Global.AccountID,
			&ar.GetAllRegistriesParams{
				Page:       &page,
				Size:       &size,
				SearchTerm: &registry,
				Scope:      &descendants,
			})
		if err != nil || response.StatusCode() != http2.StatusOK {
			return types.RegistryInfo{}, fmt.Errorf("failed to get registry: %w", err)
		}
		data := response.JSON200
		if data == nil {
			return types.RegistryInfo{}, fmt.Errorf("failed to get data for registry: %s", response.Status())
		}
		registries := data.Data.Registries

		for _, v := range registries {
			if v.Identifier != registry {
				continue
			}
			return types.RegistryInfo{
				Type: string(v.Type),
				URL:  v.Url,
				Path: *v.Path,
			}, nil
		}
		if len(registries) < int(size) || (nil != data.Data.PageCount && nil != data.Data.PageIndex && (*data.Data.PageIndex+1 >= *data.Data.PageCount)) {
			break
		}
		page++
	}
	return types.RegistryInfo{}, fmt.Errorf("failed to find registry '%s'", registry)
}

func (c *client) artifactVersionExists(
	ctx context.Context,
	registryRef, pkg, version string,
	artifactType types.ArtifactType,
) (bool, error) {
	page := int64(0)
	size := int64(100)

	for {
		response, err := c.apiClient.GetAllArtifactVersionsWithResponse(ctx, registryRef, pkg,
			&ar.GetAllArtifactVersionsParams{
				Page:       &page,
				Size:       &size,
				SortOrder:  nil,
				SortField:  nil,
				SearchTerm: &version,
			})
		if err != nil {
			return false, fmt.Errorf("failed to get artifact versions: %w", err)
		}
		if response.StatusCode() != http2.StatusOK {
			return false, fmt.Errorf("failed to get artifact versions: %s", response.Status())
		}
		var data ar.ListArtifactVersion

		if response.JSON200 == nil {
			return false, fmt.Errorf("failed to get artifact 200 response: %s", response.Status())
		}
		data = response.JSON200.Data
		if data.ArtifactVersions == nil {
			return false, nil
		}

		for _, v := range *data.ArtifactVersions {
			if v.Name == version {
				return true, nil
			}
		}
		if len(*data.ArtifactVersions) < int(size) || (nil != data.PageCount && nil != data.PageIndex && (*data.PageIndex+1 >= *data.PageCount)) {
			break
		}
		page++
	}
	return false, nil
}

func (c *client) createGoVersion(
	registry string,
	artifactName string,
	version string,
	files []*types.PackageFiles,
) error {
	// Create a pipe to write the file contents
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	// Process multipart form asynchronously
	go func() {
		defer pw.Close()
		for _, file := range files {
			// Add the file
			extension := filepath.Ext(file.File.Name)
			formFieldName := strings.TrimPrefix(extension, ".")
			part, err := writer.CreateFormFile(formFieldName, file.File.Name)
			if err != nil {
				pw.CloseWithError(err)
				return
			}

			// Copy the file content
			if _, err := io.Copy(part, file.DownloadFile); err != nil {
				pw.CloseWithError(err)
				return
			}
		}
		// Close the writer
		if err := writer.Close(); err != nil {
			pw.CloseWithError(err)
			return
		}
	}()

	url := fmt.Sprintf("%s/pkg/%s/%s/go/upload", c.url, config.Global.AccountID, registry)
	// Create request
	req, err := http2.NewRequest(http2.MethodPut, url, pr)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Execute request with our client (which handles authentication)
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload file '%s/%s': %w", artifactName, version, err)
	}
	defer resp.Body.Close()

	// Check for successful response
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to upload file '%s/%s', status code: %d, response: %s",
			artifactName, version, resp.StatusCode, string(body))
	}

	return nil
}
