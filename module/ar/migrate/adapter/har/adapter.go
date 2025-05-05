package jfrog

import (
	"context"
	"fmt"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	adp "harness/module/ar/migrate/adapter"
	"harness/module/ar/migrate/types"
	"io"
	"net/http"
)

func init() {
	adapterType := types.HAR
	if err := adp.RegisterFactory(adapterType, new(factory)); err != nil {
		return
	}
}

// factory section
type factory struct {
}

type adapter struct {
	client *client
	reg    types.RegistryConfig
	logger zerolog.Logger
}

// Create an adapter section
func (f factory) Create(ctx context.Context, config types.RegistryConfig) (adp.Adapter, error) {
	return newAdapter(config)
}

func newAdapter(config types.RegistryConfig) (adp.Adapter, error) {
	c := newClient(&config)
	logger := log.With().
		Str("adapter", "HAR").
		Logger()
	return &adapter{
		client: c,
		reg:    config,
		logger: logger,
	}, nil
}

func (a *adapter) ValidateCredentials() (bool, error)               { return false, nil }
func (a *adapter) GetRegistry(registry string) (interface{}, error) { return nil, nil }
func (a *adapter) CreateRegistryIfDoesntExist(registryRef string) (bool, error) {
	return false, nil
}
func (a *adapter) GetPackages(registry string, artifactType types.ArtifactType) ([]types.Package, error) {
	return nil, nil
}
func (a *adapter) GetVersions(registry, pkg string, artifactType types.ArtifactType) ([]types.Version, error) {
	return nil, nil
}
func (a *adapter) GetFiles(registry string) ([]types.File, error) { return nil, nil }

func (a *adapter) DownloadFile(registry string, uri string) (io.ReadCloser, http.Header, error) {
	return nil, http.Header{}, nil
}

func (a *adapter) UploadFile(
	registry string,
	file io.ReadCloser,
	f *types.File,
	header http.Header,
	artifactName string,
	version string,
) error {
	a.logger.Debug().Msgf("Uploaded file %s to registry: %s", f.Uri, registry)
	err := a.client.uploadFile(registry, artifactName, version, f, file)
	if err != nil {
		a.logger.Error().Err(err).Msgf("Failed to upload file %s to registry: %s", f.Uri, registry)
		return fmt.Errorf("failed to upload file %s to registry: %s", f.Uri, registry)
	}
	return nil
}
