package jfrog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/rs/zerolog/log"
	adp "harness/module/ar/migrate/adapter"
	"harness/module/ar/migrate/types"
)

const (
	mavenMetadataFile = "maven-metadata.xml"
	extensionMD5      = ".md5"
	extensionSHA1     = ".sha1"
	extensionSHA256   = ".sha256"
	extensionSHA512   = ".sha512"
	extensionPom      = ".pom"
	extensionJar      = ".jar"
	contentTypeJar    = "application/java-archive"
	contentTypeXML    = "text/xml"
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

func (a *adapter) GetPackages(registry string, artifactType types.ArtifactType, root *types.TreeNode) (
	[]types.Package,
	error,
) {
	var packages []types.Package
	if artifactType == types.GENERIC {
		packages = append(packages, types.Package{
			Registry: registry,
			Path:     "/",
			Name:     "default",
			Size:     -1,
		})
	} else if artifactType == types.MAVEN {
		packages = append(packages, types.Package{
			Registry: registry,
			Path:     "/",
			Name:     "",
			Size:     -1,
		})
	} else {
		return []types.Package{}, errors.New("unknown artifact type")
	}

	return packages, nil
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

	if artifactType == types.MAVEN {
		return []types.Version{
			{
				Registry: registry,
				Pkg:      pkg,
				Path:     "/",
				Name:     "",
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
	artifactType types.ArtifactType,
) error {
	return fmt.Errorf("not yet implemented")
}

func isMavenMetadataFile(filename string) bool {
	return filename == mavenMetadataFile ||
		filename == mavenMetadataFile+extensionMD5 ||
		filename == mavenMetadataFile+extensionSHA1 ||
		filename == mavenMetadataFile+extensionSHA256 ||
		filename == mavenMetadataFile+extensionSHA512
}
