package harbor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	adp "github.com/harness/harness-cli/module/ar/migrate/adapter"
	"github.com/harness/harness-cli/module/ar/migrate/types"
	"github.com/harness/harness-cli/module/ar/migrate/util"

	"github.com/google/go-containerregistry/pkg/authn"
)

func init() {
	adapterType := types.HARBOR
	if err := adp.RegisterFactory(adapterType, new(factory)); err != nil {
		return
	}
}

type factory struct{}

func (f factory) Create(_ context.Context, config types.RegistryConfig) (adp.Adapter, error) {
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

// assertOCISupported returns a descriptive error for non-OCI artifact types
func assertOCISupported(artifactType types.ArtifactType) error {
	if artifactType == types.DOCKER || artifactType == types.HELM {
		return nil
	}
	return fmt.Errorf("HARBOR source supports only OCI artifact types (DOCKER, HELM); got %s", artifactType)
}

func (a *adapter) GetKeyChain(sourcePackageHostname string) (authn.Keychain, error) {
	var host string
	if sourcePackageHostname != "" {
		host = sourcePackageHostname
	} else {
		parsed, err := url.Parse(a.reg.Endpoint)
		if err != nil {
			return nil, fmt.Errorf("failed to parse [%s], err: %w", a.reg.Endpoint, err)
		}
		host = parsed.Host
	}
	return NewHarborKeychain(a.reg.Credentials.Username, a.reg.Credentials.Password, host), nil
}

func (a *adapter) GetConfig() types.RegistryConfig {
	return a.reg
}

func (a *adapter) ValidateCredentials() (bool, error) {
	if err := a.client.health(); err != nil {
		return false, fmt.Errorf("failed to validate credentials: %w", err)
	}
	return true, nil
}

func (a *adapter) GetRegistry(_ context.Context, registry string) (types.RegistryInfo, error) {
	project, err := a.client.getProject(registry)
	if err != nil {
		return types.RegistryInfo{}, fmt.Errorf("failed to get project %s: %w", registry, err)
	}
	return types.RegistryInfo{
		Type: "harbor",
		URL:  a.reg.Endpoint,
		Path: project.Name,
	}, nil
}

// CreateRegistryIfDoesntExist is a no-op for Harbor source adapter
func (a *adapter) CreateRegistryIfDoesntExist(_ string) (bool, error) {
	return false, nil
}

// GetFiles returns an empty slice — OCI migration does not use the file tree
func (a *adapter) GetFiles(_ string) ([]types.File, error) {
	return []types.File{}, nil
}

func (a *adapter) GetPackages(registry string, artifactType types.ArtifactType, _ *types.TreeNode) (
	[]types.Package,
	error,
) {
	if err := assertOCISupported(artifactType); err != nil {
		return nil, err
	}

	repos, err := a.client.listRepositories(registry)
	if err != nil {
		return nil, fmt.Errorf("failed to list repositories in project %s: %w", registry, err)
	}

	packages := make([]types.Package, 0, len(repos))
	for _, repo := range repos {
		// Harbor's full repo name is "<project>/<repo>"; strip the project prefix for
		// use as the image name so crane paths are constructed correctly
		name := repoShortName(registry, repo.Name)
		packages = append(packages, types.Package{
			Registry: registry,
			Path:     "/",
			Name:     name,
			Size:     -1,
		})
	}
	return packages, nil
}

// GetOCIImagePath builds the crane-compatible image reference for a Harbor repository.
// Harbor image path: <host>/<project>/<repo>
func (a *adapter) GetOCIImagePath(registry string, packageHostname string, image string) (string, error) {
	var host string
	if packageHostname != "" {
		host = packageHostname
	} else {
		parsed, err := url.Parse(a.reg.Endpoint)
		if err != nil {
			return "", fmt.Errorf("failed to parse endpoint: %w", err)
		}
		host = parsed.Host
	}
	// registry is the Harbor project, and image is the per-repo short name stored in
	// Package.Name (the project prefix is stripped in GetPackages). Rebuild the full
	// Harbor reference: <host>/<project>/<repo>.
	return util.GenOCIImagePath(host, registry, image), nil
}

// --- Stubs for non-OCI operations (Harbor is OCI-only) ---

func (a *adapter) GetVersions(
	_ types.Package,
	_ *types.TreeNode,
	_, _ string,
	artifactType types.ArtifactType,
) ([]types.Version, error) {
	if err := assertOCISupported(artifactType); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("GetVersions not implemented for HARBOR (OCI uses crane)")
}

func (a *adapter) DownloadFile(_ string, _ string) (io.ReadCloser, http.Header, error) {
	return nil, nil, fmt.Errorf("DownloadFile not implemented for HARBOR")
}

func (a *adapter) UploadFile(
	_ string,
	_ io.ReadCloser,
	_ *types.File,
	_ http.Header,
	_, _ string,
	_ types.ArtifactType,
	_ map[string]interface{},
) error {
	return fmt.Errorf("UploadFile not implemented for HARBOR")
}

func (a *adapter) AddNPMTag(_ string, _ string, _ string, _ string) error {
	return nil
}

func (a *adapter) VersionExists(
	_ context.Context,
	_ types.Package,
	_, _, _ string,
	_ types.ArtifactType,
) (bool, error) {
	return false, fmt.Errorf("VersionExists not implemented for HARBOR")
}

func (a *adapter) FileExists(
	_ context.Context,
	_, _, _ string,
	_ *types.File,
	_ types.ArtifactType,
) (bool, error) {
	return false, fmt.Errorf("FileExists not implemented for HARBOR")
}

func (a *adapter) GetAllFilesForVersion(
	_ context.Context,
	_, _, _ string,
) ([]string, error) {
	return nil, fmt.Errorf("GetAllFilesForVersion not implemented for HARBOR")
}

func (a *adapter) CreateVersion(
	_ string,
	_ string,
	_ string,
	_ types.ArtifactType,
	_ []*types.PackageFiles,
	_ map[string]interface{},
) error {
	return fmt.Errorf("CreateVersion not implemented for HARBOR")
}
