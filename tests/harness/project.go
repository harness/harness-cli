package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

const defaultE2EOrg = "har_auto_org_migration"
const defaultE2EProject = "har_auto_project_migration"

// EnsureProject guarantees the configured org/project exist before any registry
// is created. Registries are always provisioned at project scope (parentRef =
// account/org/project); a dedicated, low-churn project keeps HAR lookups fast in
// shared QA accounts.
func EnsureProject(t *testing.T, creds Creds) {
	t.Helper()

	if creds.OrgID == "" || creds.ProjectID == "" {
		t.Fatalf("EnsureProject: org and project must be set (org=%q project=%q)", creds.OrgID, creds.ProjectID)
	}

	EnsureOrg(t, creds)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	switch err := getProject(ctx, creds); {
	case err == nil:
		t.Logf("using existing project %s/%s", creds.OrgID, creds.ProjectID)
		return
	case isForbidden(err):
		// PATs scoped to HAR often cannot call the NG project API. Registry
		// provisioning still works at project scope as long as the project exists.
		t.Logf("warning: cannot verify project %s/%s via NG API (403); proceeding — create it in the Harness UI if registry create fails", creds.OrgID, creds.ProjectID)
		return
	case !isNotFound(err):
		t.Fatalf("failed to look up project %s/%s: %v", creds.OrgID, creds.ProjectID, err)
	}

	t.Logf("creating project %s/%s for e2e migrations", creds.OrgID, creds.ProjectID)
	if err := createProject(ctx, creds); err != nil {
		switch {
		case isAlreadyExists(err):
			t.Logf("project %s/%s already exists (race), reusing", creds.OrgID, creds.ProjectID)
		case isForbidden(err):
			t.Fatalf("project %s/%s does not exist and this token cannot create projects (403). Create the project in Harness UI (org=%s, id=%s) and re-run", creds.OrgID, creds.ProjectID, creds.OrgID, creds.ProjectID)
		default:
			t.Fatalf("failed to create project %s/%s: %v", creds.OrgID, creds.ProjectID, err)
		}
		return
	}
	t.Logf("created project %s/%s", creds.OrgID, creds.ProjectID)
}

// EnsureOrg guarantees the configured org exists before project/registry setup.
func EnsureOrg(t *testing.T, creds Creds) {
	t.Helper()

	if creds.OrgID == "" {
		t.Fatalf("EnsureOrg: org must be set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	switch err := getOrg(ctx, creds); {
	case err == nil:
		t.Logf("using existing org %s", creds.OrgID)
		return
	case isForbidden(err):
		t.Logf("warning: cannot verify org %s via NG API (403); proceeding — create it in the Harness UI if project create fails", creds.OrgID)
		return
	case !isNotFound(err):
		t.Fatalf("failed to look up org %s: %v", creds.OrgID, err)
	}

	t.Logf("creating org %s for e2e migrations", creds.OrgID)
	if err := createOrg(ctx, creds); err != nil {
		switch {
		case isAlreadyExists(err):
			t.Logf("org %s already exists (race), reusing", creds.OrgID)
		case isForbidden(err):
			t.Fatalf("org %s does not exist and this token cannot create orgs (403). Create the org in Harness UI (id=%s) and re-run", creds.OrgID, creds.OrgID)
		default:
			t.Fatalf("failed to create org %s: %v", creds.OrgID, err)
		}
		return
	}
	t.Logf("created org %s", creds.OrgID)
}

func getOrg(ctx context.Context, creds Creds) error {
	u, err := url.Parse(creds.APIURL + "/ng/api/organizations/" + url.PathEscape(creds.OrgID))
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("accountIdentifier", creds.AccountID)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", creds.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusNotFound:
		return errNotFound{msg: strings.TrimSpace(string(body))}
	case http.StatusForbidden:
		return errForbidden{msg: strings.TrimSpace(string(body))}
	default:
		return fmt.Errorf("GET organization: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
}

func createOrg(ctx context.Context, creds Creds) error {
	u, err := url.Parse(creds.APIURL + "/ng/api/organizations")
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("accountIdentifier", creds.AccountID)
	u.RawQuery = q.Encode()

	payload := map[string]any{
		"organization": map[string]any{
			"identifier":  creds.OrgID,
			"name":        "Harness CLI E2E Migrations",
			"description": "Dedicated org for harness-cli MOCK_JFROG -> HAR migration e2e tests",
			"tags":        map[string]string{},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", creds.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		return nil
	case http.StatusForbidden:
		return errForbidden{msg: strings.TrimSpace(string(body))}
	default:
		return classifyAlreadyExists(resp.StatusCode, body)
	}
}

func getProject(ctx context.Context, creds Creds) error {
	u, err := url.Parse(creds.APIURL + "/ng/api/projects/" + url.PathEscape(creds.ProjectID))
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("accountIdentifier", creds.AccountID)
	q.Set("orgIdentifier", creds.OrgID)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", creds.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusNotFound:
		return errNotFound{msg: strings.TrimSpace(string(body))}
	case http.StatusForbidden:
		return errForbidden{msg: strings.TrimSpace(string(body))}
	default:
		return fmt.Errorf("GET project: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
}

func createProject(ctx context.Context, creds Creds) error {
	u, err := url.Parse(creds.APIURL + "/ng/api/projects")
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("accountIdentifier", creds.AccountID)
	q.Set("orgIdentifier", creds.OrgID)
	u.RawQuery = q.Encode()

	payload := map[string]any{
		"project": map[string]any{
			"orgIdentifier": creds.OrgID,
			"identifier":    creds.ProjectID,
			"name":          "Harness CLI E2E Migrations",
			"description":   "Dedicated project for harness-cli MOCK_JFROG -> HAR migration e2e tests",
			"color":         "#0063F7",
			"modules":       []string{},
			"tags":          map[string]string{},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", creds.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		return nil
	case http.StatusForbidden:
		return errForbidden{msg: strings.TrimSpace(string(body))}
	default:
		return classifyAlreadyExists(resp.StatusCode, body)
	}
}

func classifyAlreadyExists(status int, body []byte) error {
	msg := strings.ToLower(string(body))
	if status == http.StatusConflict ||
		strings.Contains(msg, "already exists") ||
		strings.Contains(msg, "already present") ||
		strings.Contains(msg, "duplicate_field") {
		return errAlreadyExists{msg: strings.TrimSpace(string(body))}
	}
	return fmt.Errorf("request failed: status %d: %s", status, strings.TrimSpace(string(body)))
}

type errNotFound struct{ msg string }

func (e errNotFound) Error() string { return "project not found: " + e.msg }

type errAlreadyExists struct{ msg string }

func (e errAlreadyExists) Error() string { return "project already exists: " + e.msg }

type errForbidden struct{ msg string }

func (e errForbidden) Error() string { return "forbidden: " + e.msg }

func isNotFound(err error) bool {
	_, ok := err.(errNotFound)
	return ok
}

func isAlreadyExists(err error) bool {
	_, ok := err.(errAlreadyExists)
	return ok
}

func isForbidden(err error) bool {
	_, ok := err.(errForbidden)
	return ok
}
