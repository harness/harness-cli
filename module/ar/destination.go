package ar

import (
	"bytes"
	"encoding/json"
	"fmt"
	"harness/config"
	"harness/module/ar/types"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"strings"
	"time"
)

// HARDestinationRegistry implements the DestinationRegistry interface for Harness Artifact Registry
type HARDestinationRegistry struct {
	config     types.DestinationConfig
	httpClient *http.Client
}

// NewHARDestinationRegistry creates a new HAR destination ar client
func NewHARDestinationRegistry(cfg types.DestinationConfig) (*HARDestinationRegistry, error) {
	if cfg.Type != "HAR" {
		return nil, fmt.Errorf("%w: expected HAR, got %s", ErrUnsupportedRegistryType, cfg.Type)
	}

	return &HARDestinationRegistry{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // Longer timeout for uploads
		},
	}, nil
}

// UploadArtifact uploads an artifact to HAR
func (h *HARDestinationRegistry) UploadArtifact(artifact Artifact, data []byte) error {
	// Get base URL for ar endpoint
	baseURL, err := url.Parse(h.config.Endpoint)
	if err != nil {
		return fmt.Errorf("invalid ar endpoint URL: %w", err)
	}

	// Parse destination ar path
	// Format could be: "ar", "org/ar", or "org/project/ar"
	registryParts := strings.Split(artifact.Registry, "/")
	if len(registryParts) == 0 {
		return fmt.Errorf("invalid destination ar format: %s", artifact.Registry)
	}

	// Construct the artifact upload URL
	uploadURL := *baseURL

	// Path structure depends on artifact type
	var apiPath string
	switch strings.ToLower(artifact.Type) {
	case "maven", "jar", "java":
		// Example: /maven/{org}/{project}/{name}/{version}/artifact.jar
		artifactName := fmt.Sprintf("%s-%s.jar", artifact.Name, artifact.Version)
		apiPath = path.Join("maven", artifact.Registry, artifact.Name, artifact.Version, artifactName)
	case "python", "py":
		// Example: /python/{org}/{project}/{name}/{version}/artifact.whl
		artifactName := fmt.Sprintf("%s-%s.whl", artifact.Name, artifact.Version)
		apiPath = path.Join("python", artifact.Registry, artifact.Name, artifact.Version, artifactName)
	default:
		// Default to generic path
		// Example: /generic/{org}/{project}/{name}/{version}/artifact
		apiPath = path.Join("generic", artifact.Registry, artifact.Name, artifact.Version, artifact.Name)
	}

	uploadURL.Path = path.Join(uploadURL.Path, apiPath)

	// Create multipart form for upload
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add artifact metadata
	// Create metadata part
	metadataHeader := make(textproto.MIMEHeader)
	metadataHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="metadata"`))
	metadataPart, err := writer.CreatePart(metadataHeader)
	if err != nil {
		return fmt.Errorf("error creating metadata part: %w", err)
	}

	metadata := map[string]interface{}{
		"name":    artifact.Name,
		"version": artifact.Version,
		"type":    artifact.Type,
	}

	if err := json.NewEncoder(metadataPart).Encode(metadata); err != nil {
		return fmt.Errorf("error encoding metadata: %w", err)
	}

	// Add artifact file
	fileHeader := make(textproto.MIMEHeader)
	fileHeader.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="file"; filename="%s"`, path.Base(uploadURL.Path)))
	filePart, err := writer.CreatePart(fileHeader)
	if err != nil {
		return fmt.Errorf("error creating file part: %w", err)
	}

	if _, err := io.Copy(filePart, bytes.NewReader(data)); err != nil {
		return fmt.Errorf("error writing file data: %w", err)
	}

	// Close the multipart writer
	if err := writer.Close(); err != nil {
		return fmt.Errorf("error closing multipart writer: %w", err)
	}

	// Create request
	req, err := http.NewRequest("PUT", uploadURL.String(), body)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	// Set content type for multipart form
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Set authentication headers
	req.Header.Set("Authorization", "Bearer "+h.config.Credentials.Token)

	// Execute request
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return ErrInvalidCredentials
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HAR API error (%d): %s", resp.StatusCode, body)
	}

	return nil
}

// CreateRegistry creates a new ar in HAR
func (h *HARDestinationRegistry) CreateRegistry(registry string) error {
	// Get base URL for ar endpoint
	baseURL, err := url.Parse(h.config.Endpoint)
	if err != nil {
		return fmt.Errorf("invalid ar endpoint URL: %w", err)
	}

	// Parse ar path to extract org and project IDs if present
	// Format could be: "ar", "org/ar", or "org/project/ar"
	parts := strings.Split(registry, "/")

	createRequest := map[string]interface{}{
		"name":              parts[len(parts)-1], // Last part is always the ar name
		"type":              "generic",           // Default type, can be overridden based on artifact types
		"accountIdentifier": config.Global.AccountID,
	}

	// Add organization ID if present (if parts length > 1)
	if len(parts) > 1 {
		createRequest["organizationIdentifier"] = parts[0]
	}

	// Add project ID if present (if parts length > 2)
	if len(parts) > 2 {
		createRequest["projectIdentifier"] = parts[1]
	}

	// Convert request to JSON
	requestBody, err := json.Marshal(createRequest)
	if err != nil {
		return fmt.Errorf("error marshaling create ar request: %w", err)
	}

	// Construct the create ar URL
	createURL := *baseURL
	createURL.Path = path.Join(createURL.Path, "clients/registries")

	// Create request
	req, err := http.NewRequest("POST", createURL.String(), bytes.NewBuffer(requestBody))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	// Set content type and authentication headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.config.Credentials.Token)

	// Execute request
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return ErrInvalidCredentials
	}

	// 409 Conflict typically means the ar already exists
	if resp.StatusCode == http.StatusConflict {
		return nil // Registry already exists, not treating as an error
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HAR API error (%d): %s", resp.StatusCode, body)
	}

	return nil
}
