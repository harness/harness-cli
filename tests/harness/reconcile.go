package harness

import (
	"context"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/harness/harness-cli/module/ar/migrate/adapter"
	"github.com/harness/harness-cli/module/ar/migrate/types"
	"github.com/harness/harness-cli/module/ar/migrate/util"

	"github.com/google/go-containerregistry/pkg/crane"

	// Register the adapter factories used for in-process reconciliation.
	_ "github.com/harness/harness-cli/module/ar/migrate/adapter/har"
	_ "github.com/harness/harness-cli/module/ar/migrate/adapter/jfrog"
	_ "github.com/harness/harness-cli/module/ar/migrate/adapter/mock_jfrog"
)

// Reconcile independently verifies that every file requested to be migrated is
// actually present in the destination HAR registry. It reuses the production
// source enumeration (GetFiles + FilterFilesByPatterns) to derive the requested
// set and the production destination existence primitives (FileExists /
// GetAllFilesForVersion / OCI tag listing) to confirm presence, so a migration
// that silently failed still fails the test.
func Reconcile(t *testing.T, creds Creds, spec Spec) {
	t.Helper()

	ApplyGlobalConfig(creds)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	srcAdapter, err := adapter.GetAdapter(ctx, types.RegistryConfig{
		Endpoint:    spec.sourceEndpoint(),
		Type:        types.MOCK_JFROG,
		Credentials: types.CredentialsConfig{Username: "dummy", Password: "dummy"},
		Insecure:    spec.Insecure,
	})
	if err != nil {
		t.Fatalf("reconcile: failed to build source adapter: %v", err)
	}

	destAdapter, err := adapter.GetAdapter(ctx, types.RegistryConfig{
		Endpoint:    creds.PkgURL,
		Type:        types.HAR,
		Credentials: types.CredentialsConfig{Username: "harness", Password: creds.APIKey},
	})
	if err != nil {
		t.Fatalf("reconcile: failed to build destination adapter: %v", err)
	}

	registryRef := creds.registryRef(spec.DestRegistry)
	artifactType := types.ArtifactType(spec.ArtifactType)

	// Log the source-derived requested set as a sanity signal.
	requested := requestedFiles(srcAdapter, spec)
	t.Logf("reconcile: %d file(s) requested to be migrated from %s", len(requested), spec.SourceRegistry)

	switch artifactType {
	case types.DOCKER, types.HELM, types.HELM_LEGACY:
		reconcileOCI(t, ctx, destAdapter, spec)
	case types.HELM_HTTP:
		if len(spec.ExpectedRawURIs) > 0 {
			reconcileRaw(t, ctx, destAdapter, registryRef, spec, requested)
		} else {
			reconcileOCI(t, ctx, destAdapter, spec)
		}
	case types.RAW:
		reconcileRaw(t, ctx, destAdapter, registryRef, spec, requested)
	case types.GENERIC:
		reconcileGeneric(t, ctx, destAdapter, registryRef, spec, requested)
	default:
		reconcileVersioned(t, ctx, destAdapter, registryRef, spec)
	}
}

// requestedFiles reproduces the migration's file-level enumeration: list source
// files and apply include/exclude patterns, dropping folders.
func requestedFiles(srcAdapter adapter.Adapter, spec Spec) []types.File {
	files, err := srcAdapter.GetFiles(spec.SourceRegistry)
	if err != nil {
		return nil
	}
	files = util.FilterFilesByPatterns(files, spec.IncludePatterns, spec.ExcludePatterns)
	out := make([]types.File, 0, len(files))
	for _, f := range files {
		if f.Folder {
			continue
		}
		out = append(out, f)
	}
	return out
}

// reconcileRaw checks each expected RAW file URI via a HEAD existence check.
func reconcileRaw(t *testing.T, ctx context.Context, destAdapter adapter.Adapter, registryRef string, spec Spec, requested []types.File) {
	t.Helper()

	uris := spec.ExpectedRawURIs
	if len(uris) == 0 {
		for _, f := range requested {
			uris = append(uris, normalizeRawURI(strings.TrimPrefix(f.Uri, "/")))
		}
	}
	if len(uris) == 0 {
		t.Fatalf("reconcile RAW: no expected file URIs and none derived from source")
	}

	var missing []string
	for _, uri := range uris {
		file := &types.File{Uri: uri, Name: path.Base(uri)}
		exists, err := destAdapter.FileExists(ctx, registryRef, "", "", file, types.RAW)
		if err != nil {
			t.Errorf("reconcile RAW: existence check failed for %q: %v", uri, err)
			continue
		}
		if !exists {
			missing = append(missing, uri)
		}
	}
	if len(missing) > 0 {
		t.Errorf("reconcile RAW: %d/%d requested file(s) missing in registry %q:\n  %s",
			len(missing), len(uris), spec.DestRegistry, strings.Join(missing, "\n  "))
	}
}

// reconcileGeneric checks each file landed under default/default/<uri>, matching
// uploadGenericFile's path layout. Generic artifacts are not indexed as a
// default@default version in the management API, so we HEAD the generic-file
// endpoint instead (same primitive RAW uses).
func reconcileGeneric(t *testing.T, ctx context.Context, destAdapter adapter.Adapter, registryRef string, spec Spec, requested []types.File) {
	t.Helper()

	uris := spec.ExpectedRawURIs
	if len(uris) == 0 {
		for _, f := range requested {
			uris = append(uris, "default/default/"+strings.TrimPrefix(f.Uri, "/"))
		}
	}
	if len(uris) == 0 {
		t.Fatalf("reconcile GENERIC: no expected file URIs and none derived from source")
	}

	var missing []string
	for _, uri := range uris {
		file := &types.File{Uri: uri, Name: path.Base(uri)}
		exists, err := destAdapter.FileExists(ctx, registryRef, "", "", file, types.RAW)
		if err != nil {
			t.Errorf("reconcile GENERIC: existence check failed for %q: %v", uri, err)
			continue
		}
		if !exists {
			missing = append(missing, uri)
		}
	}
	if len(missing) > 0 {
		t.Errorf("reconcile GENERIC: %d/%d requested file(s) missing in registry %q:\n  %s",
			len(missing), len(uris), spec.DestRegistry, strings.Join(missing, "\n  "))
	}
}

// reconcileVersioned verifies that each expected package version is present in
// the destination. Version presence (VersionExists) is the reliable gate; when
// an ExpectedFile also names a file, membership in the destination's per-version
// file listing is additionally required, exactly as the migration's skip logic
// checks it.
func reconcileVersioned(t *testing.T, ctx context.Context, destAdapter adapter.Adapter, registryRef string, spec Spec) {
	t.Helper()

	if len(spec.ExpectedFiles) == 0 {
		t.Fatalf("reconcile %s: ExpectedFiles must be specified for versioned artifact types", spec.ArtifactType)
	}

	artifactType := types.ArtifactType(spec.ArtifactType)

	type pv struct{ pkg, version string }
	versionChecked := map[pv]bool{} // whether VersionExists has been asserted
	fileCache := map[pv]map[string]bool{}

	var missing []string
	for _, ef := range spec.ExpectedFiles {
		key := pv{ef.Pkg, ef.Version}

		if !versionChecked[key] {
			versionChecked[key] = true
			exists, err := versionExistsWithRetry(ctx, destAdapter, registryRef, ef.Pkg, ef.Version, artifactType)
			if err != nil {
				t.Errorf("reconcile %s: version existence check failed for %s@%s: %v", spec.ArtifactType, ef.Pkg, ef.Version, err)
			} else if !exists {
				missing = append(missing, ef.Pkg+"@"+ef.Version+" (version not found)")
				continue
			}
		}

		if ef.FileName == "" {
			continue
		}

		set, ok := fileCache[key]
		if !ok {
			names, err := destAdapter.GetAllFilesForVersion(ctx, registryRef, ef.Pkg, ef.Version)
			if err != nil {
				t.Errorf("reconcile %s: failed listing files for %s@%s: %v", spec.ArtifactType, ef.Pkg, ef.Version, err)
				continue
			}
			set = map[string]bool{}
			for _, n := range names {
				set[strings.ToLower(strings.TrimPrefix(n, "/"))] = true
			}
			fileCache[key] = set
		}
		want := strings.ToLower(strings.TrimPrefix(ef.FileName, "/"))
		if !set[want] {
			missing = append(missing, ef.Pkg+"@"+ef.Version+"/"+ef.FileName)
		}
	}
	if len(missing) > 0 {
		t.Errorf("reconcile %s: %d expected artifact(s) missing in registry %q:\n  %s",
			spec.ArtifactType, len(missing), spec.DestRegistry, strings.Join(missing, "\n  "))
	}
}

// reconcileOCI checks that each expected image tag exists in the destination OCI
// registry by listing tags on the HAR OCI reference.
func reconcileOCI(t *testing.T, ctx context.Context, destAdapter adapter.Adapter, spec Spec) {
	t.Helper()

	if len(spec.ExpectedTags) == 0 {
		t.Fatalf("reconcile %s: ExpectedTags must be specified for OCI artifact types", spec.ArtifactType)
	}

	keychain, err := destAdapter.GetKeyChain("")
	if err != nil {
		t.Fatalf("reconcile %s: failed to get destination keychain: %v", spec.ArtifactType, err)
	}

	var missing []string
	for _, et := range spec.ExpectedTags {
		repoRef, err := destAdapter.GetOCIImagePath(spec.DestRegistry, "", et.Image)
		if err != nil {
			t.Errorf("reconcile %s: failed to build OCI path for %q: %v", spec.ArtifactType, et.Image, err)
			continue
		}
		tags, err := crane.ListTags(repoRef, crane.WithContext(ctx), crane.WithAuthFromKeychain(keychain))
		if err != nil {
			t.Errorf("reconcile %s: failed to list tags for %q: %v", spec.ArtifactType, repoRef, err)
			continue
		}
		if !contains(tags, et.Tag) {
			missing = append(missing, et.Image+":"+et.Tag)
		}
	}
	if len(missing) > 0 {
		t.Errorf("reconcile %s: %d/%d expected image tag(s) missing in registry %q:\n  %s",
			spec.ArtifactType, len(missing), len(spec.ExpectedTags), spec.DestRegistry, strings.Join(missing, "\n  "))
	}
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// normalizeRawURI matches uploadRawFile's path layout for GENERIC registries.
func normalizeRawURI(uri string) string {
	if strings.Count(uri, "/") >= 2 {
		return uri
	}
	return "default/default/" + uri
}

func versionExistsWithRetry(
	ctx context.Context,
	destAdapter adapter.Adapter,
	registryRef, pkg, version string,
	artifactType types.ArtifactType,
) (bool, error) {
	const attempts = 3
	for i := 0; i < attempts; i++ {
		exists, err := destAdapter.VersionExists(ctx, types.Package{}, registryRef, pkg, version, artifactType)
		if err != nil {
			return false, err
		}
		if exists {
			return true, nil
		}
		if i == attempts-1 {
			break
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return false, nil
}
