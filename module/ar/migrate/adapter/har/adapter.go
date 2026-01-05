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
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
	adp "github.com/harness/harness-cli/module/ar/migrate/adapter"
	"github.com/harness/harness-cli/module/ar/migrate/types"
	"github.com/harness/harness-cli/module/ar/migrate/util"
	"github.com/harness/harness-cli/util/common/auth"

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
	client    *client
	reg       types.RegistryConfig
	logger    zerolog.Logger
	pkgClient *pkgclient.ClientWithResponses
}

// Create an adapter section
func (f factory) Create(ctx context.Context, config types.RegistryConfig) (adp.Adapter, error) {
	return newAdapter(config)
}

func newAdapter(config2 types.RegistryConfig) (adp.Adapter, error) {
	c := newClient(&config2)
	pkgClient, err := pkgclient.NewClientWithResponses(config.Global.Registry.PkgURL,
		auth.GetAuthOptionARPKG())
	if err != nil {
		return nil, fmt.Errorf("failed to create pkg client: %v", err)
	}
	logger := log.With().
		Str("adapter", "HAR").
		Logger()
	return &adapter{
		client:    c,
		pkgClient: pkgClient,
		reg:       config2,
		logger:    logger,
	}, nil
}

func (a *adapter) GetKeyChain(_ string) (authn.Keychain, error) {
	parseUrl, err := url.Parse(a.reg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to parse [%s], err: %w", a.reg.Endpoint, err)
	}
	return NewHarKeychain(a.reg.Credentials.Username, a.reg.Credentials.Password, parseUrl.Host), nil
}

func (a *adapter) GetConfig() types.RegistryConfig {
	return a.reg
}
func (a *adapter) ValidateCredentials() (bool, error) { return false, nil }
func (a *adapter) GetRegistry(ctx context.Context, registry string) (types.RegistryInfo, error) {
	return a.client.getRegistry(ctx, registry)
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
	} else if artifactType == types.CONDA {
		err = a.client.uploadCondaFile(registry, f.Name, file, metadata)
	} else if artifactType == types.COMPOSER {
		err = a.client.uploadComposerFile(registry, f.Name, file)
	} else if artifactType == types.DART {
		err = a.client.uploadDartFile(registry, artifactName, version, f, file)
	}

	if err != nil {
		a.logger.Error().Err(err).Msgf("Failed to upload file %s to registry: %s", f.Uri, registry)
		return fmt.Errorf("failed to upload file %s to registry: %s, %v", f.Uri, registry, err)
	}
	return nil
}

func (a *adapter) GetOCIImagePath(registry string, _ string, image string) (string, error) {
	parse, _ := url.Parse(a.reg.Endpoint)
	return util.GenOCIImagePath(parse.Host, strings.ToLower(config.Global.AccountID), registry, image), nil
}

func (a *adapter) AddNPMTag(registry string, name string, version string, uri string) error {
	return a.client.AddNPMTag(registry, name, version, uri)
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
	registryRef, pkg, version string,
	file *types.File,
	artifactType types.ArtifactType,
) (bool, error) {
	return a.client.artifactFileExists(ctx, registryRef, pkg, version, file, artifactType)
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

func (a *adapter) GetRegistryMetadata(ctx context.Context, registry string) ([]types.MetadataItem, error) {
	return a.client.getMetadata(ctx, registry, nil, nil)
}

func (a *adapter) GetPackageMetadata(ctx context.Context, registry, pkg string) ([]types.MetadataItem, error) {
	return a.client.getMetadata(ctx, registry, &pkg, nil)
}

func (a *adapter) GetVersionMetadata(ctx context.Context, registry, pkg, version string) ([]types.MetadataItem, error) {
	return a.client.getMetadata(ctx, registry, &pkg, &version)
}

func (a *adapter) SetRegistryMetadata(ctx context.Context, registry string, metadata []types.MetadataItem) error {
	return a.client.setMetadata(ctx, registry, nil, nil, metadata)
}

func (a *adapter) SetPackageMetadata(ctx context.Context, registry, pkg string, metadata []types.MetadataItem) error {
	return a.client.setMetadata(ctx, registry, &pkg, nil, metadata)
}

func (a *adapter) SetVersionMetadata(ctx context.Context, registry, pkg, version string, metadata []types.MetadataItem) error {
	return a.client.setMetadata(ctx, registry, &pkg, &version, metadata)
}
