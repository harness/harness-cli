package api

import (
	"net/http"
	"strings"
	"testing"
)

// A duplicate run identity is rejected at the database index and the driver
// error comes through unwrapped, so it arrives as a 500 quoting MongoDB.
func TestIsConflictRecognisesTheDuplicateKeyRejection(t *testing.T) {
	duplicate := newAPIError("POST", "/load-tests/checkout-load/runs", http.StatusInternalServerError,
		[]byte(`{"error":"failed to create load test run: write exception: write errors: `+
			`[E11000 duplicate key error collection: harness-chaos.loadTestRuns index: `+
			`account_org_project_identity_unique_partial_idx dup key: { identity: \"checkout-load\" }]"}`))

	if !IsConflict(duplicate) {
		t.Fatal("a duplicate key rejection was not recognised as a conflict")
	}
}

func TestIsConflictRecognisesA409(t *testing.T) {
	if !IsConflict(newAPIError("POST", "/load-tests", http.StatusConflict, []byte(`{"error":"already exists"}`))) {
		t.Fatal("a 409 was not recognised as a conflict")
	}
}

// A genuine server error must not be retried as if it were a name clash.
func TestIsConflictLeavesOtherFailuresAlone(t *testing.T) {
	cases := map[string]*APIError{
		"internal error": newAPIError("POST", "/x", http.StatusInternalServerError, []byte(`{"error":"boom"}`)),
		"not found":      newAPIError("GET", "/x", http.StatusNotFound, []byte(`{"error":"not found"}`)),
		"bad request":    newAPIError("POST", "/x", http.StatusBadRequest, []byte(`{"error":"invalid"}`)),
	}
	for name, err := range cases {
		if IsConflict(err) {
			t.Errorf("%s was misread as a conflict", name)
		}
	}
	if IsConflict(nil) {
		t.Error("a nil error was read as a conflict")
	}
}

// loadTestManager puts the reason in "error" and sends no "message", so reading
// only "message" leaves the raw JSON document to print.
func TestErrorMessageFallsBackToTheErrorField(t *testing.T) {
	apiErr := newAPIError("POST", "/runs/checkout-load-9d1/stop", http.StatusBadRequest,
		[]byte(`{"error":"Can only stop running or pending tests"}`))

	got := apiErr.Error()
	if !strings.Contains(got, "Can only stop running or pending tests") {
		t.Fatalf("error does not state the reason: %s", got)
	}
	if strings.Contains(got, `{"error"`) {
		t.Fatalf("error shows the raw response document: %s", got)
	}
}

// A response that is not JSON at all still has to say something. A gateway
// returning HTML for a 502 is the usual case.
func TestErrorFallsBackToTheBodyThenTheStatus(t *testing.T) {
	html := newAPIError("GET", "/load-tests", http.StatusBadGateway, []byte("<html>bad gateway</html>"))
	if !strings.Contains(html.Error(), "bad gateway") {
		t.Fatalf("non-JSON body was dropped: %s", html.Error())
	}

	empty := newAPIError("GET", "/load-tests", http.StatusServiceUnavailable, nil)
	if !strings.Contains(empty.Error(), http.StatusText(http.StatusServiceUnavailable)) {
		t.Fatalf("empty body left no description: %s", empty.Error())
	}
}
