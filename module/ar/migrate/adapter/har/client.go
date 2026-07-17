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
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/internal/api/ar_v3"
	"github.com/harness/harness-cli/module/ar/migrate/http"
	"github.com/harness/harness-cli/module/ar/migrate/http/auth/xApiKey"
	"github.com/harness/harness-cli/module/ar/migrate/http/modifier/useragent"
	"github.com/harness/harness-cli/module/ar/migrate/types"
	"github.com/harness/harness-cli/util/common/auth"

	"github.com/google/uuid"
	retryablehttp "github.com/hashicorp/go-retryablehttp"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
)

func retryingPkgHTTPClient() *http2.Client {
	rc := retryablehttp.NewClient()
	rc.RetryMax = 5
	rc.RetryWaitMin = 200 * time.Millisecond
	rc.RetryWaitMax = 1 * time.Minute
	rc.Backoff = retryablehttp.RateLimitLinearJitterBackoff
	rc.Logger = nil

	std := rc.StandardClient() // returns *http.Client using a retrying RoundTripper
	std.Timeout = 30 * time.Minute
	return std
}

func retryingArHTTPClient() *http2.Client {
	rc := retryablehttp.NewClient()
	rc.RetryMax = 5
	rc.RetryWaitMin = 200 * time.Millisecond
	rc.RetryWaitMax = 1 * time.Minute
	rc.Backoff = retryablehttp.RateLimitLinearJitterBackoff
	rc.Logger = nil

	std := rc.StandardClient() // returns *http.Client using a retrying RoundTripper
	std.Timeout = 2 * time.Minute
	return std
}

// rawPkgHTTPClient returns a retry-enabled *http.Client that injects the same
// auth headers as auth.GetAuthOptionARPKG (x-api-key + Authorization for JWT
// tokens). Used for raw/generic file uploads that bypass the generated client.
func rawPkgHTTPClient() *http2.Client {
	c := retryingPkgHTTPClient()
	c.Transport = &pkgAuthTransport{wrapped: c.Transport}
	return c
}

// pkgAuthTransport mirrors the auth injected by auth.GetAuthOptionARPKG.
type pkgAuthTransport struct {
	wrapped http2.RoundTripper
}

func (t *pkgAuthTransport) RoundTrip(req *http2.Request) (*http2.Response, error) {
	r := req.Clone(req.Context())
	r.Header.Set("x-api-key", config.Global.AuthToken)
	if strings.HasPrefix(config.Global.AuthToken, auth.JWTTokenPrefix) {
		r.Header.Set("Authorization", config.Global.AuthToken)
	}
	r.Header.Set("User-Agent", config.UserAgent())
	return t.wrapped.RoundTrip(r)
}

// newClient constructs a jfrog client
func newClient(reg *types.RegistryConfig) *client {
	username, token := "", ""

	username = reg.Credentials.Username
	token = reg.Credentials.Password

	arClient, _ := ar.NewClientWithResponses(config.Global.APIBaseURL+"/gateway/har/api/v1",
		ar.WithHTTPClient(retryingArHTTPClient()),
		auth.GetXApiKeyOptionAR())

	arV3Client, _ := ar_v3.NewClientWithResponses(config.Global.APIBaseURL+"/gateway/har/api/v3",
		ar_v3.WithHTTPClient(retryingArHTTPClient()),
		auth.GetXApiKeyOptionARV3())

	pkgClient, _ := pkgclient.NewClientWithResponses(reg.Endpoint,
		pkgclient.WithHTTPClient(retryingPkgHTTPClient()),
		auth.GetAuthOptionARPKG())

	return &client{
		client: http.NewClient(
			&http2.Client{
				Transport: http.GetHTTPTransport(http.WithInsecure(true)),
			},
			xApiKey.NewAuthorizer(token),
			useragent.NewModifier(),
		),
		pkgClient:        pkgClient,
		rawPkgHTTPClient: rawPkgHTTPClient(),
		url:              reg.Endpoint,
		insecure:         true,
		username:         username,
		password:         token,
		apiClient:        arClient,
		arV3Client:       arV3Client,
	}
}

type client struct {
	apiClient        *ar.ClientWithResponses
	arV3Client       *ar_v3.ClientWithResponses
	client           *http.Client
	rawPkgHTTPClient *http2.Client
	url              string
	insecure         bool
	username         string
	password         string
	pkgClient        *pkgclient.ClientWithResponses
}

func (c *client) uploadGenericFile(registry, artifactName, version string, f *types.File, file io.ReadCloser) error {
	fileUri := strings.TrimPrefix(f.Uri, "/")
	fullPath := fmt.Sprintf("%s/%s/%s", artifactName, version, fileUri)
	defer file.Close()

	base := strings.TrimRight(c.url, "/")
	url := fmt.Sprintf("%s/pkg/%s/%s/files/%s", base, config.Global.AccountID, registry, fullPath)
	req, err := http2.NewRequest(http2.MethodPut, url, file)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.rawPkgHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload file '%s/%s': %w", artifactName, version, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to upload file '%s/%s', status code: %d, response: %s",
			artifactName, version, resp.StatusCode, string(body))
	}
	return nil
}

func (c *client) headRawFile(registryRef string, fileUri string) (bool, error) {
	fileUri = strings.TrimPrefix(fileUri, "/")
	parts := strings.Split(registryRef, "/")
	registry := parts[len(parts)-1]

	base := strings.TrimRight(c.url, "/")
	url := fmt.Sprintf("%s/pkg/%s/%s/files/%s", base, config.Global.AccountID, registry, fileUri)
	req, err := http2.NewRequest(http2.MethodHead, url, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.rawPkgHTTPClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to HEAD raw file '%s': %w", fileUri, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http2.StatusOK:
		return true, nil
	case http2.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("unexpected status code %d for HEAD on raw file '%s'", resp.StatusCode, fileUri)
	}
}

func (c *client) uploadRawFile(registry string, f *types.File, file io.ReadCloser) error {
	fileUri := strings.TrimPrefix(f.Uri, "/")
	defer file.Close()

	base := strings.TrimRight(c.url, "/")
	url := fmt.Sprintf("%s/pkg/%s/%s/files/%s", base, config.Global.AccountID, registry, fileUri)
	req, err := http2.NewRequest(http2.MethodPut, url, file)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.rawPkgHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload raw file '%s': %w", fileUri, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http2.StatusConflict:
		return types.ErrArtifactAlreadyExists
	case resp.StatusCode >= 200 && resp.StatusCode <= 299:
		return nil
	default:
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to upload raw file '%s', status code: %d, response: %s",
			fileUri, resp.StatusCode, string(body))
	}
}

func (c *client) uploadMavenFile(
	registry string,
	name string,
	version string,
	f *types.File,
	file io.ReadCloser,
) error {
	fileUri := strings.TrimPrefix(f.Uri, "/")
	url := fmt.Sprintf("%s/pkg/%s/%s/maven/%s", c.url, config.Global.AccountID, registry, fileUri)
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

	subDir := nugetSubDir(fileUri)

	url := fmt.Sprintf("%s/pkg/%s/%s/nuget/%s", c.url, config.Global.AccountID, registry, subDir)
	if strings.HasSuffix(f.Name, ".snupkg") {
		url = fmt.Sprintf("%s/pkg/%s/%s/nuget/symbolpackage/%s", c.url, config.Global.AccountID, registry, subDir)
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

// nugetSubDir extracts the directory prefix from a NuGet file URI.
// Returns everything except the filename (last segment), with a trailing slash,
// or empty string if the file is at the root.
//
// Examples:
//
//	"a/b/c/d/proto-bindings.0.8.662.nupkg" → "a/b/c/d/"
//	"foo/company.grpc.pkg.1.0.0.nupkg"     → "foo/"
//	"company.grpc.pkg.1.0.0.nupkg"         → ""
func nugetSubDir(fileUri string) string {
	parts := strings.Split(strings.TrimPrefix(fileUri, "/"), "/")
	if len(parts) <= 1 {
		return ""
	}
	return strings.Join(parts[:len(parts)-1], "/") + "/"
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

func (c *client) uploadDebianFile(
	registry string,
	filename string,
	file io.ReadCloser,
	metadata map[string]interface{},
) error {
	fileUri := strings.TrimPrefix(filename, "/")

	// Extract distribution, component, file type, package, and version from metadata
	distribution := ""
	component := ""
	fileType := "deb" // default to binary package
	packageName := ""
	version := ""

	if metadata != nil {
		if dist, ok := metadata["distribution"].(string); ok {
			distribution = dist
		}
		if comp, ok := metadata["component"].(string); ok {
			component = comp
		}
		if ft, ok := metadata["fileType"].(string); ok {
			fileType = ft
		}
		if pkg, ok := metadata["package"].(string); ok {
			packageName = pkg
		}
		if ver, ok := metadata["version"].(string); ok {
			version = ver
		}
	}

	// Validate required parameters
	if distribution == "" {
		return fmt.Errorf("distribution is required in metadata")
	}
	if component == "" {
		return fmt.Errorf("component is required in metadata")
	}

	// For source files, package and version are required
	if fileType == "src" {
		if packageName == "" {
			return fmt.Errorf("package is required in metadata for source files")
		}
		if version == "" {
			return fmt.Errorf("version is required in metadata for source files")
		}
	}

	// Determine endpoint based on file type
	// /deb for .deb files, /dsc for .dsc files, /src for other source files
	var endpoint string
	switch fileType {
	case "dsc":
		endpoint = "dsc"
	case "src":
		endpoint = "src"
	default:
		endpoint = "deb"
	}

	// Build URL: /pkg/{account}/{registry}/debian/{endpoint} with query parameters
	baseURL := fmt.Sprintf("%s/pkg/%s/%s/debian/%s", c.url, config.Global.AccountID, registry, endpoint)

	// Build query parameters
	params := []string{
		fmt.Sprintf("distribution=%s", distribution),
		fmt.Sprintf("component=%s", component),
	}

	// Add package and version for source files
	if fileType == "src" {
		params = append(params, fmt.Sprintf("package=%s", packageName))
		params = append(params, fmt.Sprintf("version=%s", version))
	}

	url := fmt.Sprintf("%s?%s", baseURL, strings.Join(params, "&"))

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

func (c *client) uploadCondaFile(
	registry string,
	filename string,
	file io.ReadCloser,
	metadata map[string]interface{},
) error {
	fileUri := strings.TrimPrefix(filename, "/")
	url := fmt.Sprintf("%s/pkg/%s/%s/conda/upload", c.url, config.Global.AccountID, registry)
	// Create request
	req, err := http2.NewRequest(http2.MethodPut, url, file)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	// Add headers from metadata
	for key, val := range metadata {
		switch v := val.(type) {
		case string:
			if v != "" {
				req.Header.Set(key, v)
			}
		}
	}

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

// uploadConanFile PUTs a single Conan file to its recipe (RREV) or package
// (PKGID/PREV) revision path via the Conan v2 endpoints. Coordinates and the
// source SHA1 are carried in metadata; the SHA1 rides as X-Checksum-Sha1 so the
// server can verify the upload. A 409 (immutable-revision conflict) maps to
// ErrArtifactAlreadyExists so the caller can skip it.
func (c *client) uploadConanFile(
	registry string,
	file io.ReadCloser,
	metadata map[string]interface{},
) error {
	defer file.Close()

	get := func(key string) string {
		if metadata == nil {
			return ""
		}
		if v, ok := metadata[key].(string); ok {
			return v
		}
		return ""
	}

	name := get("name")
	version := get("version")
	rrev := get("rrev")
	filename := get("filename")
	if name == "" || version == "" || rrev == "" || filename == "" {
		return fmt.Errorf("conan upload: missing required coordinates (name/version/rrev/filename)")
	}

	user := get("user")
	if user == "" {
		user = "_"
	}
	channel := get("channel")
	if channel == "" {
		channel = "_"
	}

	sha1 := get("sha1")
	reqEditor := func(_ context.Context, req *http2.Request) error {
		if sha1 != "" {
			req.Header.Set("X-Checksum-Sha1", sha1)
		}
		return nil
	}

	var statusCode int
	var respBody []byte
	if get("layer") == "package" {
		pkgid := get("pkgid")
		prev := get("prev")
		if pkgid == "" || prev == "" {
			return fmt.Errorf("conan upload: missing pkgid/prev for package-layer file %q", filename)
		}
		resp, err := c.pkgClient.UploadConanPackageFileWithBodyWithResponse(
			context.Background(), config.Global.AccountID, registry,
			name, version, user, channel, rrev, pkgid, prev, filename,
			"application/octet-stream", file, reqEditor)
		if err != nil {
			return fmt.Errorf("failed to upload conan package file %q: %w", filename, err)
		}
		statusCode = resp.StatusCode()
		respBody = resp.Body
	} else {
		resp, err := c.pkgClient.UploadConanRecipeFileWithBodyWithResponse(
			context.Background(), config.Global.AccountID, registry,
			name, version, user, channel, rrev, filename,
			"application/octet-stream", file, reqEditor)
		if err != nil {
			return fmt.Errorf("failed to upload conan recipe file %q: %w", filename, err)
		}
		statusCode = resp.StatusCode()
		respBody = resp.Body
	}

	switch {
	case statusCode == http2.StatusConflict:
		return types.ErrArtifactAlreadyExists
	case statusCode >= 200 && statusCode <= 299:
		return nil
	default:
		return fmt.Errorf("failed to upload conan file %q, status code: %d, response: %s",
			filename, statusCode, string(respBody))
	}
}

func (c *client) uploadComposerFile(
	registry string,
	filename string,
	file io.ReadCloser,
) error {
	url := fmt.Sprintf("%s/pkg/%s/%s/composer/upload", c.url, config.Global.AccountID, registry)

	// Create request
	req, err := http2.NewRequest(http2.MethodPost, url, file)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload file '%s': %w", filename, err)
	}
	defer resp.Body.Close()

	// Check for successful response
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to upload file '%s', status code: %d, response: %s",
			filename, resp.StatusCode, string(body))
	}
	return nil
}

func (c *client) uploadSwiftFile(
	registry string,
	filename string,
	file io.ReadCloser,
	packageName string,
	version string,
) error {

	// Parse package name to extract scope, name
	// packageName is in scope.name format (e.g., "myscope.harness")
	parts := strings.SplitN(packageName, ".", 2)
	if len(parts) < 2 {
		return fmt.Errorf("invalid Swift package name format: %s, expected format: scope.name", packageName)
	}

	scope := parts[0]
	name := parts[1]

	url := fmt.Sprintf("%s/pkg/%s/%s/swift/%s/%s/%s",
		c.url, config.Global.AccountID, registry, scope, name, version)

	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	go func() {
		defer pw.Close()
		defer writer.Close()

		// Create the form field "source-archive" to match Swift upload API
		part, err := writer.CreateFormFile("source-archive", filename)
		if err != nil {
			pw.CloseWithError(err)
			return
		}

		// Copy the file into the multipart field
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
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload Swift file '%s': %w", filename, err)
	}
	defer resp.Body.Close()

	// Check for successful response
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to upload Swift file '%s', status code: %d, response: %s",
			filename, resp.StatusCode, string(body))
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

func (c *client) uploadDartFile(
	registry string,
	name string,
	version string,
	f *types.File,
	file io.ReadCloser,
) error {
	// POST {endpoint}/pkg/{account_id}/{registry}/pub/api/packages/versions/new/upload/{upload_id}
	// with multipart form: -F "file=@file.tar.gz"
	uploadID := uuid.New().String()

	base := strings.TrimRight(c.url, "/")
	url := fmt.Sprintf("%s/pkg/%s/%s/pub/api/packages/versions/new/upload/%s", base, config.Global.AccountID, registry,
		uploadID)

	// Create a pipe for streaming multipart form data
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	// Write multipart form in a goroutine
	go func() {
		defer pw.Close()
		defer writer.Close()

		// Create form file field "file"
		part, err := writer.CreateFormFile("file", f.Name)
		if err != nil {
			pw.CloseWithError(fmt.Errorf("failed to create form file: %w", err))
			return
		}

		// Copy file content to the form field
		if _, err := io.Copy(part, file); err != nil {
			pw.CloseWithError(fmt.Errorf("failed to write file to form: %w", err))
			return
		}
	}()

	req, err := http2.NewRequest(http2.MethodPost, url, pr)
	if err != nil {
		return fmt.Errorf("failed to create Dart upload request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload Dart package to '%s': %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to upload Dart package '%s', status code: %d, response: %s",
			url, resp.StatusCode, string(body))
	}

	return nil
}

// uploadPuppetFile streams a Puppet module .tar.gz tarball to HAR via the
// puppet upload endpoint. The server re-parses the tarball's metadata.json,
// so the client only needs to forward the bytes as a multipart "file" field.
func (c *client) uploadPuppetFile(
	registry string,
	f *types.File,
	file io.ReadCloser,
) error {
	base := strings.TrimRight(c.url, "/")
	url := fmt.Sprintf("%s/pkg/%s/%s/puppet/upload", base, config.Global.AccountID, registry)

	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	go func() {
		defer pw.Close()
		defer writer.Close()

		part, err := writer.CreateFormFile("file", f.Name)
		if err != nil {
			pw.CloseWithError(fmt.Errorf("failed to create form file: %w", err))
			return
		}
		if _, err := io.Copy(part, file); err != nil {
			pw.CloseWithError(fmt.Errorf("failed to write file to form: %w", err))
			return
		}
	}()

	req, err := http2.NewRequest(http2.MethodPut, url, pr)
	if err != nil {
		return fmt.Errorf("failed to create Puppet upload request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload Puppet module '%s': %w", f.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to upload Puppet module '%s', status code: %d, response: %s",
			f.Name, resp.StatusCode, string(body))
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
	registryRef, pkg, version string,
	file *types.File,
	artifactType types.ArtifactType,
) (bool, error) {
	page := int64(0)
	size := int64(100)
	fileURI := file.Name
	if artifactType == types.GENERIC || artifactType == types.RAW {
		fileURI = strings.TrimPrefix(file.Uri, "/")
	} else {
		fileURI = strings.TrimPrefix(fileURI, "/")
	}

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
		for _, v := range data.Data.Files {
			if v.Name == fileURI {
				return true, nil
			}
		}
		if len(data.Data.Files) < int(size) || (nil != data.Data.PageCount && nil != data.Data.PageIndex && (*data.Data.PageIndex+1 >= *data.Data.PageCount)) {
			break
		}
		page++
	}
	return false, nil
}

func (c *client) buildExistingIndex(
	ctx context.Context, registryName string, concurrency int,
) (*types.ExistingIndex, error) {
	// Step 1: Resolve registry name -> registry object
	reg, err := c.resolveRegistry(ctx, registryName)
	if err != nil {
		return nil, fmt.Errorf("registry resolution failed: %w", err)
	}
	regID := reg.Id

	// HACK: the v3 batch endpoints require org/project identifiers, but the
	// migration config does not carry them. Derive them from the resolved
	// registry Path (`<accountID>/<?org>/<?project>/<registry>`) instead.
	orgID, projectID := orgProjectFromPath(reg.Path)

	// Step 2: Enumerate all versions for this registry
	versions, err := c.listAllVersionsV3(ctx, regID, orgID, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list versions: %w", err)
	}

	idx := types.NewExistingIndex()

	// Keep only versions that may carry files; the index tracks files only, so
	// zero-file versions contribute nothing. Fetch when FileCount > 0 or nil
	// (unknown).
	var versionsNeedingFiles []ar_v3.Version
	for _, v := range versions {
		if v.FileCount == nil || *v.FileCount > 0 {
			versionsNeedingFiles = append(versionsNeedingFiles, v)
		}
	}

	// Step 3: Fetch files per non-empty version, bounded concurrency
	g, gctx := errgroup.WithContext(ctx)
	if concurrency > 0 {
		g.SetLimit(concurrency)
	}
	for _, v := range versionsNeedingFiles {
		v := v
		g.Go(func() error {
			names, err := c.listFilesV3ForVersion(gctx, regID, v.Id, orgID, projectID)
			if err != nil {
				// Best-effort: log & continue; a miss only causes an idempotent re-upload
				log.Warn().Err(err).
					Str("package", v.PackageName).
					Str("version", v.Name).
					Msg("Failed to fetch files for version during index build")
				return nil
			}
			for _, name := range names {
				// Store the HAR path verbatim; ExistingIndex.HasFile owns the
				// reverse conversion to source-relative form at lookup time, so
				// all per-type path logic lives in one place (existing_index.go).
				idx.AddFile(v.PackageName, v.Name, name)
			}
			return nil
		})
	}
	_ = g.Wait()

	return idx, nil
}

// orgProjectFromPath derives the org and project identifiers from a registry
// Path of the form `<accountID>/<?org>/<?project>/<registry>`, where org and
// project are optional. Returns (nil, nil) when the scope is account-level.
//
// Layouts:
//
//	accountID/registry                 → (nil, nil)
//	accountID/org/registry             → (org, nil)
//	accountID/org/project/registry     → (org, project)
func orgProjectFromPath(path *string) (orgID, projectID *string) {
	if path == nil {
		return nil, nil
	}
	parts := strings.Split(strings.Trim(*path, "/"), "/")
	switch len(parts) {
	case 3:
		org := parts[1]
		return &org, nil
	case 4:
		org := parts[1]
		project := parts[2]
		return &org, &project
	default:
		// 2 (account/registry) or any unexpected shape → account scope.
		return nil, nil
	}
}

// resolveRegistry looks up a registry by name via the ar_v3 API and returns
// the full registry object (Id, Type, Url, Path, …) so callers needing any of
// those fields make a single call instead of separate v1/v3 lookups.
//
// HACK: hardcoded to always return a fixed test registry, bypassing the real
// lookup below. Remove once done testing.
func (c *client) resolveRegistry(ctx context.Context, registryName string) (ar_v3.Registry, error) {

	page := int64(0)
	size := int64(100)

	accountID := config.Global.AccountID
	var orgID, projectID *string
	if config.Global.OrgID != "" {
		orgID = &config.Global.OrgID
	}
	if config.Global.ProjectID != "" {
		projectID = &config.Global.ProjectID
	}

	d := ar_v3.ListRegistriesV3ParamsScopeDescendants

	for {
		params := &ar_v3.ListRegistriesV3Params{
			AccountIdentifier: accountID,
			OrgIdentifier:     orgID,
			ProjectIdentifier: projectID,
			SearchTerm:        &registryName,
			Page:              &page,
			Size:              &size,
			Scope:             &d,
		}

		resp, err := c.arV3Client.ListRegistriesV3WithResponse(ctx, params)
		if err != nil {
			return ar_v3.Registry{}, fmt.Errorf("ListRegistriesV3 failed: %w", err)
		}
		if resp.StatusCode() != http2.StatusOK {
			return ar_v3.Registry{}, fmt.Errorf("ListRegistriesV3 returned %s", resp.Status())
		}

		body := resp.JSON200
		for _, reg := range body.Items {
			if reg.Name == registryName {
				return reg, nil
			}
		}

		if !body.HasMore || len(body.Items) == 0 {
			break
		}
		page++
	}

	return ar_v3.Registry{}, fmt.Errorf("registry %q not found", registryName)
}

func (c *client) listAllVersionsV3(ctx context.Context, regID openapi_types.UUID, orgID, projectID *string) ([]ar_v3.Version, error) {
	page := int64(0)
	size := int64(100)

	accountID := config.Global.AccountID

	registryIDs := &[]openapi_types.UUID{regID}

	var allVersions []ar_v3.Version
	for {
		params := &ar_v3.ListVersionsV3Params{
			AccountIdentifier: accountID,
			OrgIdentifier:     orgID,
			ProjectIdentifier: projectID,
			RegistryIds:       registryIDs,
			Page:              &page,
			Size:              &size,
		}

		resp, err := c.arV3Client.ListVersionsV3WithResponse(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("ListVersionsV3 failed: %w", err)
		}
		if resp.StatusCode() != http2.StatusOK {
			return nil, fmt.Errorf("ListVersionsV3 returned %s", resp.Status())
		}

		body := resp.JSON200
		allVersions = append(allVersions, body.Items...)

		if !body.HasMore || len(body.Items) == 0 {
			break
		}
		page++
	}

	return allVersions, nil
}

func (c *client) listFilesV3ForVersion(ctx context.Context, regID openapi_types.UUID, versionID openapi_types.UUID, orgID, projectID *string) ([]string, error) {
	page := int64(0)
	size := int64(100)

	accountID := config.Global.AccountID

	regIDStr := regID.String()
	versionIDStr := versionID.String()

	var allFileNames []string
	for {
		params := &ar_v3.ListFilesV3Params{
			AccountIdentifier: accountID,
			OrgIdentifier:     orgID,
			ProjectIdentifier: projectID,
			RegistryId:        &regIDStr,
			VersionId:         &versionIDStr,
			Page:              &page,
			Size:              &size,
		}

		resp, err := c.arV3Client.ListFilesV3WithResponse(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("ListFilesV3 failed: %w", err)
		}
		if resp.StatusCode() != http2.StatusOK {
			return nil, fmt.Errorf("ListFilesV3 returned %s", resp.Status())
		}

		body := resp.JSON200
		for _, f := range body.Items {
			allFileNames = append(allFileNames, f.Path)
		}

		if !body.HasMore || len(body.Items) == 0 {
			break
		}
		page++
	}

	return allFileNames, nil
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
		if response.StatusCode() == http2.StatusNotFound {
			return false, nil
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
