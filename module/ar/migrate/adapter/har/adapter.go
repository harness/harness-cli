package har

import (
	"context"
	"fmt"

	//"github.com/harness/harness-cli/module/ar/migrate"
	//client2 "github.com/harness/harness-cli/util/client"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/harness/harness-cli/config"
	adp "github.com/harness/harness-cli/module/ar/migrate/adapter"
	"github.com/harness/harness-cli/module/ar/migrate/types"
	"github.com/harness/harness-cli/module/ar/migrate/util"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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

func (a *adapter) GetKeyChain(reg string) authn.Keychain {
	parseUrl, _ := url.Parse(a.reg.Endpoint)
	return NewHarKeychain(a.reg.Credentials.Username, a.reg.Credentials.Password, parseUrl.Host)
}

func (a *adapter) GetConfig() types.RegistryConfig {
	return a.reg
}
func (a *adapter) ValidateCredentials() (bool, error) { return false, nil }
func (a *adapter) GetRegistry(registry string) (types.RegistryInfo, error) {
	return types.RegistryInfo{}, nil
}
func (a *adapter) CreateRegistryIfDoesntExist(registryRef string) (bool, error) {
	return false, nil
}
func (a *adapter) GetPackages(registry string, artifactType types.ArtifactType, root *types.TreeNode) (
	[]types.Package,
	error,
) {
	return nil, nil
}
func (a *adapter) GetVersions(
	p types.Package,
	node *types.TreeNode,
	registry, pkg string,
	artifactType types.ArtifactType,
) ([]types.Version, error) {
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
	artifactType types.ArtifactType,
	metadata map[string]interface{},
) error {
	a.logger.Debug().Msgf("Uploaded file %s to registry: %s", f.Uri, registry)
	var err error
	if artifactType == types.GENERIC {
		err = a.client.uploadGenericFile(registry, artifactName, version, f, file)
	} else if artifactType == types.MAVEN {
		err = a.client.uploadMavenFile(registry, artifactName, version, f, file)
	} else if artifactType == types.PYTHON {
		err = a.client.uploadPythonFile(registry, artifactName, version, f, file, metadata)
	} else if artifactType == types.NUGET {
		err = a.client.uploadNugetFile(registry, artifactName, version, f, file)
	} else if artifactType == types.NPM {
		err = a.client.uploadNPMFile(registry, artifactName, version, f, file)
	} else if artifactType == types.RPM {
		err = a.client.uploadRPMFile(registry, f.Name, file)
	}

	if err != nil {
		a.logger.Error().Err(err).Msgf("Failed to upload file %s to registry: %s", f.Uri, registry)
		return fmt.Errorf("failed to upload file %s to registry: %s, %v", f.Uri, registry, err)
	}
	return nil
}

func (a *adapter) GetOCIImagePath(registry string, image string) (string, error) {
	parse, _ := url.Parse(a.reg.Endpoint)
	return util.GenOCIImagePath(parse.Host, strings.ToLower(config.Global.AccountID), registry, image), nil
}

func (a *adapter) AddNPMTag(version string, uri string) error {
	return a.client.AddNPMTag(version, uri)
}

func (a *adapter) VersionExists(
	ctx context.Context,
	p types.Package,
	registryRef, pkg, version string,
	artifactType types.ArtifactType,
) (bool, error) {
	if artifactType == types.HELM_LEGACY {
		artifactType = types.HELM
	}
	return a.client.artifactVersionExists(ctx, registryRef, pkg, version, artifactType)
}

func (a *adapter) FileExists(
	ctx context.Context,
	registryRef, pkg, version, fileName string,
	artifactType types.ArtifactType,
) (bool, error) {
	return a.client.artifactFileExists(ctx, registryRef, pkg, version, fileName, artifactType)
}

func (a *adapter) CreateVersion(
	registry string,
	artifactName string,
	version string,
	artifactType types.ArtifactType,
	files []*types.PackageFiles,
	_ map[string]interface{},
) error {
	switch artifactType {
	case types.GO:
		return a.client.createGoVersion(registry, artifactName, version, files)
	default:
		return fmt.Errorf("not implemented")
	}
}
