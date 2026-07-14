package har

import (
	"context"
	"errors"
	"fmt"
	//"github.com/harness/harness-cli/module/ar/migrate"
	//client2 "github.com/harness/harness-cli/util/client"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"

	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar"
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

	// ociPrefixMu guards ociPrefixCache, which maps registryRef → OCI path
	// prefix returned by GetClientSetupDetails (e.g. "oci"). Cached per-registry
	// to avoid repeated API calls during tag-by-tag migration.
	ociPrefixMu    sync.Mutex
	ociPrefixCache map[string]string
}

func (a *adapter) SearchFiles(registry string) ([]types.SearchedFile, error) {
	return nil, fmt.Errorf("search Not implemented for this Client")
}

// Create an adapter section
func (f factory) Create(ctx context.Context, config types.RegistryConfig) (adp.Adapter, error) {
	return newAdapter(config)
}

func newAdapter(config2 types.RegistryConfig) (adp.Adapter, error) {
	c := newClient(&config2)
	pkgClient, err := pkgclient.NewClientWithResponses(config.Global.Registry.PkgURL,
		pkgclient.WithHTTPClient(retryingPkgHTTPClient()),
		auth.GetAuthOptionARPKG())
	if err != nil {
		return nil, fmt.Errorf("failed to create pkg client: %v", err)
	}
	logger := log.With().
		Str("adapter", "HAR").
		Logger()
	return &adapter{
		client:         c,
		pkgClient:      pkgClient,
		reg:            config2,
		logger:         logger,
		ociPrefixCache: make(map[string]string),
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
	switch artifactType {
	case types.GENERIC:
		err = a.client.uploadGenericFile(registry, artifactName, version, f, file)
	case types.MAVEN:
		err = a.client.uploadMavenFile(registry, artifactName, version, f, file)
	case types.PYTHON:
		err = a.client.uploadPythonFile(registry, artifactName, version, f, file, metadata)
	case types.NUGET:
		err = a.client.uploadNugetFile(registry, artifactName, version, f, file)
	case types.NPM:
		err = a.client.uploadNPMFile(registry, artifactName, version, f, file)
	case types.RPM:
		err = a.client.uploadRPMFile(registry, f.Name, file)
	case types.DEBIAN:
		err = a.client.uploadDebianFile(registry, f.Name, file, metadata)
	case types.CONDA:
		err = a.client.uploadCondaFile(registry, f.Name, file, metadata)
	case types.COMPOSER:
		err = a.client.uploadComposerFile(registry, f.Name, file)
	case types.SWIFT:
		err = a.client.uploadSwiftFile(registry, f.Name, file, artifactName, version)
	case types.DART:
		err = a.client.uploadDartFile(registry, artifactName, version, f, file)
	case types.PUPPET:
		err = a.client.uploadPuppetFile(registry, f, file)
	case types.CONAN:
		err = a.client.uploadConanFile(registry, file, metadata)
	case types.RAW:
		err = a.client.uploadRawFile(registry, f, file)
	default:
		return fmt.Errorf("unsupported artifact type: %s", artifactType)
	}

	if err != nil {
		if errors.Is(err, types.ErrArtifactAlreadyExists) {
			return err
		}
		a.logger.Error().Err(err).Msgf("Failed to upload file %s to registry: %s", f.Uri, registry)
		return fmt.Errorf("failed to upload file %s to registry: %s, %v", f.Uri, registry, err)
	}
	return nil
}

func (a *adapter) GetOCIImagePath(registry string, _ string, image string) (string, error) {
	parse, _ := url.Parse(a.reg.Endpoint)
	host := parse.Host

	prefix, err := a.ociPrefix(registry)
	if err != nil {
		// Fall back to the account-ID-based path used before vanity URL support.
		a.logger.Warn().Err(err).Msgf(
			"Could not resolve OCI prefix from client-setup API for registry %s; falling back to account-ID prefix", registry)
		prefix = strings.ToLower(config.Global.AccountID)
	}
	return util.GenOCIImagePath(host, prefix, registry, image), nil
}

// ociPrefix returns the OCI path prefix for registry by calling the
// GetClientSetupDetails API and parsing the helm push/pull URL from the
// response. Results are cached per registry so each registry is only queried
// once per migration run.
func (a *adapter) ociPrefix(registryRef string) (string, error) {
	a.ociPrefixMu.Lock()
	defer a.ociPrefixMu.Unlock()

	if prefix, ok := a.ociPrefixCache[registryRef]; ok {
		return prefix, nil
	}

	ctx := context.Background()
	resp, err := a.client.apiClient.GetClientSetupDetailsWithResponse(ctx, registryRef, &ar.GetClientSetupDetailsParams{})
	if err != nil {
		return "", fmt.Errorf("GetClientSetupDetails request failed: %w", err)
	}
	if resp.JSON200 == nil {
		return "", fmt.Errorf("GetClientSetupDetails returned status %s", resp.Status())
	}

	prefix, err := extractOCIPrefix(resp.JSON200.Data, registryRef)
	if err != nil {
		return "", err
	}

	a.ociPrefixCache[registryRef] = prefix
	return prefix, nil
}

// extractOCIPrefix scans the ClientSetupDetails sections for a command that
// contains an "oci://" URL and derives the path prefix that sits between the
// hostname and the registry name.
//
// Example command value: "helm push <CHART_TGZ_FILE> oci://har-automation.harness.io/oci/my-registry"
// For registryRef "account123/my-registry" the returned prefix is "oci".
func extractOCIPrefix(details ar.ClientSetupDetails, registryRef string) (string, error) {
	// registryRef is "accountID/registryName" — we only need the leaf name.
	registryName := registryRef
	if idx := strings.LastIndex(registryRef, "/"); idx >= 0 {
		registryName = registryRef[idx+1:]
	}

	for _, section := range details.Sections {
		cfg, err := section.AsClientSetupStepConfig()
		if err != nil || cfg.Steps == nil {
			continue
		}
		for _, step := range *cfg.Steps {
			if step.Commands == nil {
				continue
			}
			for _, cmd := range *step.Commands {
				if cmd.Value == nil {
					continue
				}
				prefix, ok := ociPrefixFromCommand(*cmd.Value, registryName)
				if ok {
					return prefix, nil
				}
			}
		}
	}
	return "", fmt.Errorf("no OCI push/pull URL found in client-setup details for registry %s", registryRef)
}

// ociPrefixFromCommand extracts the path segment(s) between the hostname and
// the registry name from an "oci://" URL embedded in a setup command string.
// Returns ("", false) when the string contains no matching URL.
func ociPrefixFromCommand(cmdValue, registryName string) (string, bool) {
	const scheme = "oci://"
	idx := strings.Index(cmdValue, scheme)
	if idx < 0 {
		return "", false
	}
	rest := cmdValue[idx+len(scheme):]
	// rest is "host/prefix/registryName [optional-suffix]" — take only the URL token.
	if sp := strings.IndexAny(rest, " \t\n"); sp >= 0 {
		rest = rest[:sp]
	}
	// rest is now "host/prefix/registryName"
	parts := strings.SplitN(rest, "/", 3) // ["host", "prefix", "registryName"]
	if len(parts) < 3 {
		return "", false
	}
	// Verify the last part starts with the registry name we expect.
	if !strings.HasPrefix(parts[2], registryName) {
		return "", false
	}
	return parts[1], true
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
	if artifactType == types.HELM_HTTP {
		// A HELM_HTTP destination names the chart by its leaf (Chart.yaml) name,
		// while pkg may carry the nested source layout (e.g. "ChartA/ChartB/abc").
		// Query the leaf so the lookup matches the stored artifact and the path
		// param contains no '/' that would break the route.
		pkg = path.Base(pkg)
	}
	return a.client.artifactVersionExists(ctx, registryRef, pkg, version, artifactType)
}

func (a *adapter) FileExists(
	ctx context.Context,
	registryRef, pkg, version string,
	file *types.File,
	artifactType types.ArtifactType,
) (bool, error) {
	if artifactType == types.RAW {
		return a.client.headRawFile(registryRef, file.Uri)
	}
	return a.client.artifactFileExists(ctx, registryRef, pkg, version, file, artifactType)
}

func (a *adapter) GetAllFilesForVersion(
	ctx context.Context,
	registryRef, pkg, version string,
) ([]string, error) {
	return a.client.artifactGetFilesForVersion(ctx, registryRef, pkg, version)
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
