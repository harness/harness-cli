package harness

import (
	"os"
	"strings"
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/types"
)

// RequireEnv reads the live QA credentials/endpoints from the environment. If
// any required variable is missing the test is skipped, so the suite is inert
// unless explicitly configured (e.g. via `make e2e-migration`).
func RequireEnv(t *testing.T) Creds {
	t.Helper()

	creds := Creds{
		APIKey:    os.Getenv("HARNESS_API_KEY"),
		AccountID: os.Getenv("HARNESS_ACCOUNT_ID"),
		OrgID:     os.Getenv("HARNESS_ORG_ID"),
		ProjectID: os.Getenv("HARNESS_PROJECT_ID"),
		APIURL:    strings.TrimRight(os.Getenv("HARNESS_API_URL"), "/"),
		PkgURL:    strings.TrimRight(os.Getenv("HARNESS_PKG_URL"), "/"),
	}

	var missing []string
	if creds.APIKey == "" {
		missing = append(missing, "HARNESS_API_KEY")
	}
	if creds.AccountID == "" {
		missing = append(missing, "HARNESS_ACCOUNT_ID")
	}
	if creds.APIURL == "" {
		missing = append(missing, "HARNESS_API_URL")
	}
	if creds.PkgURL == "" {
		missing = append(missing, "HARNESS_PKG_URL")
	}

	if len(missing) > 0 {
		t.Skipf("skipping live E2E migration test; missing env: %s", strings.Join(missing, ", "))
	}

	if creds.OrgID == "" {
		creds.OrgID = defaultE2EOrg
	}
	if creds.ProjectID == "" {
		if v := os.Getenv("HARNESS_E2E_PROJECT_ID"); v != "" {
			creds.ProjectID = v
		} else {
			creds.ProjectID = defaultE2EProject
		}
	}

	return creds
}

// OCISource holds credentials and coordinates for a live OCI source registry
// (typically JFrog Artifactory). Live DOCKER/HELM e2e tests read these from the
// environment and skip when unset so the offline mock suite stays the default.
type OCISource struct {
	Endpoint            string // e.g. https://mycompany.jfrog.io/artifactory
	Registry            string // repo key, e.g. docker-local
	Username            string
	Password            string
	Image               string // catalog image name, e.g. nginx
	Tag                 string // tag to migrate and reconcile, e.g. 1.25
	PackageHostname     string // optional OCI hostname override (SourcePackageHostname)
	Insecure            bool   // set E2E_OCI_SOURCE_INSECURE=1 for plain HTTP sources
}

// RequireOCISource reads live OCI source settings from the environment. When any
// required variable is missing the test is skipped. Required:
//   - E2E_OCI_SOURCE_ENDPOINT
//   - E2E_OCI_SOURCE_REGISTRY
//   - E2E_OCI_SOURCE_USERNAME
//   - E2E_OCI_SOURCE_PASSWORD
//   - E2E_OCI_SOURCE_IMAGE
//   - E2E_OCI_SOURCE_TAG
//
// Optional: E2E_OCI_SOURCE_PACKAGE_HOSTNAME, E2E_OCI_SOURCE_INSECURE=1
func RequireOCISource(t *testing.T) OCISource {
	t.Helper()

	src := OCISource{
		Endpoint:        strings.TrimRight(os.Getenv("E2E_OCI_SOURCE_ENDPOINT"), "/"),
		Registry:        os.Getenv("E2E_OCI_SOURCE_REGISTRY"),
		Username:        os.Getenv("E2E_OCI_SOURCE_USERNAME"),
		Password:        os.Getenv("E2E_OCI_SOURCE_PASSWORD"),
		Image:           os.Getenv("E2E_OCI_SOURCE_IMAGE"),
		Tag:             os.Getenv("E2E_OCI_SOURCE_TAG"),
		PackageHostname: os.Getenv("E2E_OCI_SOURCE_PACKAGE_HOSTNAME"),
		Insecure:        os.Getenv("E2E_OCI_SOURCE_INSECURE") == "1",
	}

	var missing []string
	if src.Endpoint == "" {
		missing = append(missing, "E2E_OCI_SOURCE_ENDPOINT")
	}
	if src.Registry == "" {
		missing = append(missing, "E2E_OCI_SOURCE_REGISTRY")
	}
	if src.Username == "" {
		missing = append(missing, "E2E_OCI_SOURCE_USERNAME")
	}
	if src.Password == "" {
		missing = append(missing, "E2E_OCI_SOURCE_PASSWORD")
	}
	if src.Image == "" {
		missing = append(missing, "E2E_OCI_SOURCE_IMAGE")
	}
	if src.Tag == "" {
		missing = append(missing, "E2E_OCI_SOURCE_TAG")
	}

	if len(missing) > 0 {
		t.Skipf("skipping live OCI migration test; missing env: %s", strings.Join(missing, ", "))
	}

	return src
}

// SpecFromOCISource builds a migration Spec for DOCKER or HELM from a live OCI
// source. artifactType must be "DOCKER" or "HELM"; packageType is the HAR
// destination registry type (usually the same).
func SpecFromOCISource(t *testing.T, src OCISource, artifactType, packageType, destPrefix string) Spec {
	t.Helper()
	return Spec{
		ArtifactType:          artifactType,
		PackageType:           packageType,
		SourceRegistry:        src.Registry,
		SourceEndpoint:        src.Endpoint,
		SourceType:            types.JFROG,
		SourceUsername:        src.Username,
		SourcePassword:        src.Password,
		SourcePackageHostname: src.PackageHostname,
		Insecure:              src.Insecure,
		DestRegistry:          UniqueRegistry(t, destPrefix),
		ExpectedTags: []ExpectedTag{
			{Image: src.Image, Tag: src.Tag},
		},
	}
}
