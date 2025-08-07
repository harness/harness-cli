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

	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar"
	"github.com/harness/harness-cli/module/ar/migrate/http"
	"github.com/harness/harness-cli/module/ar/migrate/http/auth/xApiKey"
	"github.com/harness/harness-cli/module/ar/migrate/types"
	"github.com/harness/harness-cli/util/common/auth"
)

// newClient constructs a jfrog client
func newClient(reg *types.RegistryConfig) *client {
	username, token := "", ""

	username = reg.Credentials.Username
	token = reg.Credentials.Password

	arClient, _ := ar.NewClientWithResponses(config.Global.APIBaseURL+"/gateway/har/api/v1", auth.GetXApiKeyOptionAR())
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
		//config.Global.AuthToken, config.Global.AccountID,
		//	config.Global.OrgID, config.Global.ProjectID),
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
	fileUri := strings.TrimPrefix(f.Uri, "/")
	url := fmt.Sprintf("%s/pkg/%s/%s/npm/%s", c.url, config.Global.AccountID, registry, fileUri)

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

func (c *client) AddNPMTag(version string, uri string) error {
	versionJSON, err := json.Marshal(version)
	if err != nil {
		return fmt.Errorf("failed to marshal version to json: %w", err)
	}

	req, err := http2.NewRequest(http2.MethodPut, uri, bytes.NewReader(versionJSON))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload file '%s': %w", uri, err)
	}
	defer resp.Body.Close()

	// Check for successful response
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to upload file '%s', status code: %d, response: %s",
			uri, resp.StatusCode, string(body))
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
		data := response.JSON200.Data
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
