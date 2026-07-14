package harness

import (
	"os"
	"strings"
	"testing"
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
