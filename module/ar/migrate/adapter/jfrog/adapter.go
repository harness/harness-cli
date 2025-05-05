package jfrog

import (
	"context"
	"errors"
	"fmt"
	"github.com/rs/zerolog/log"
	adp "harness/module/ar/migrate/adapter"
	"harness/module/ar/migrate/types"
	"io"
	"net/http"
)

func init() {
	adapterType := types.JFROG
	if err := adp.RegisterFactory(adapterType, new(factory)); err != nil {
		return
	}
}

// factory section
type factory struct {
}

// Create an adapter section
func (f factory) Create(ctx context.Context, config types.RegistryConfig) (adp.Adapter, error) {
	return newAdapter(config)
}

type adapter struct {
	client *client
	reg    types.RegistryConfig
}

func newAdapter(config types.RegistryConfig) (adp.Adapter, error) {
	return &adapter{
		client: newClient(&config),
		reg:    config,
	}, nil
}

func (a *adapter) ValidateCredentials() (bool, error)                        { return false, nil }
func (a *adapter) GetRegistry(registry string) (interface{}, error)          { return nil, nil }
func (a *adapter) CreateRegistryIfDoesntExist(registry string) (bool, error) { return false, nil }

func (a *adapter) GetPackages(registry string, artifactType types.ArtifactType) ([]types.Package, error) {
	if artifactType == types.GENERIC {
		return []types.Package{
			{
				Registry: registry,
				Path:     "/",
				Name:     "default",
				Size:     -1,
			},
		}, nil
	}
	return []types.Package{}, errors.New("unknown artifact type")
}

func (a *adapter) GetVersions(registry, pkg string, artifactType types.ArtifactType) ([]types.Version, error) {
	if artifactType == types.GENERIC {
		return []types.Version{
			{
				Registry: registry,
				Pkg:      pkg,
				Path:     "/",
				Name:     "default",
				Size:     -1,
			},
		}, nil
	}
	return []types.Version{}, errors.New("unknown artifact type")

}
func (a *adapter) GetFiles(registry string) ([]types.File, error) {
	files, err := a.client.getFiles(registry)
	if err != nil {
		log.Error().Msgf("Failed to get files from registry: %v", err)
		return nil, fmt.Errorf("failed to get files from registry: %w", err)
	}
	log.Info().Msgf("Get files from registry: %v", files)
	return files, nil
}

func (a *adapter) DownloadFile(registry string, uri string) (io.ReadCloser, http.Header, error) {
	return a.client.getFile(registry, uri)
}

func (a *adapter) UploadFile(
	registry string,
	file io.ReadCloser,
	f *types.File,
	header http.Header,
	artifactName string,
	version string,
) error {
	return fmt.Errorf("not yet implemented")
}
