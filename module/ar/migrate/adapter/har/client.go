package har

import (
	"fmt"
	"io"
	"mime/multipart"
	http2 "net/http"
	"strings"

	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar"
	"github.com/harness/harness-cli/module/ar/migrate/http"
	"github.com/harness/harness-cli/module/ar/migrate/http/auth/xApiKey"
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

// newClient constructs a jfrog client
func newClient(reg *types.RegistryConfig) *client {
	username, token := "", ""

	username = reg.Credentials.Username
	token = reg.Credentials.Password

	arclient, _ := ar.NewClient(config.Global.APIBaseURL)
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
		apiClient: arclient,
		//config.Global.AuthToken, config.Global.AccountID,
		//	config.Global.OrgID, config.Global.ProjectID),
	}
}

type client struct {
	apiClient *ar.Client
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

func (c *client) uploadPythonFile(
	registry string,
	name string,
	version string,
	f *types.File,
	file io.ReadCloser,
) error {
	fileUri := strings.TrimPrefix(f.Uri, "/")
	url := fmt.Sprintf("%s/pkg/%s/%s/python/%s", c.url, config.Global.AccountID, registry, fileUri)
	// Create request
	req, err := http2.NewRequest(http2.MethodPost, url, file)
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
