package har

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/harness/harness-cli/config"
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

// newTestClient builds a har client whose generated pkg client points at the
// given test server URL.
func newTestClient(t *testing.T, serverURL string) *client {
	t.Helper()
	pc, err := pkgclient.NewClientWithResponses(serverURL)
	if err != nil {
		t.Fatalf("new pkg client: %v", err)
	}
	return &client{pkgClient: pc, url: serverURL, rawPkgHTTPClient: http.DefaultClient}
}

// TestUploadRawFile covers the generic raw upload path, which RAW artifacts use
// directly and Helm-over-HTTP charts/.prov sidecars now route through.
func TestUploadRawFile(t *testing.T) {
	config.Global.AccountID = "acct1"

	tests := []struct {
		name         string
		status       int
		fileUri      string
		wantErr      bool
		wantConflict bool
		wantPathHas  string
	}{
		{"success 201 flat", http.StatusCreated, "nginx-1.0.0.tgz", false, false, "/files/nginx-1.0.0.tgz"},
		{"success 200", http.StatusOK, "nginx-1.0.0.tgz", false, false, "/files/nginx-1.0.0.tgz"},
		{"nested path preserved", http.StatusCreated, "ChartA/ChartB/abc-1.0.1.tgz", false, false, "/files/ChartA/ChartB/abc-1.0.1.tgz"},
		{"prov upload", http.StatusCreated, "nginx-1.0.0.tgz.prov", false, false, "/files/nginx-1.0.0.tgz.prov"},
		{"leading slash trimmed", http.StatusCreated, "/nginx-1.0.0.tgz", false, false, "/files/nginx-1.0.0.tgz"},
		{"conflict surfaces ErrArtifactAlreadyExists", http.StatusConflict, "nginx-1.0.0.tgz", false, true, "/files/nginx-1.0.0.tgz"},
		{"bad request surfaces error", http.StatusBadRequest, "nginx-1.0.0.tgz", true, false, "/files/nginx-1.0.0.tgz"},
		{"server error surfaces error", http.StatusInternalServerError, "nginx-1.0.0.tgz", true, false, "/files/nginx-1.0.0.tgz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotMethod, gotPath, gotCT string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.Path
				gotCT = r.Header.Get("Content-Type")
				if tt.status >= 400 {
					w.WriteHeader(tt.status)
					_, _ = w.Write([]byte("upload rejected"))
					return
				}
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			c := newTestClient(t, srv.URL)
			body := io.NopCloser(strings.NewReader("file-bytes"))
			err := c.uploadRawFile("reg1", &types.File{Uri: tt.fileUri}, body)

			switch {
			case tt.wantConflict:
				if !errors.Is(err, types.ErrArtifactAlreadyExists) {
					t.Fatalf("expected ErrArtifactAlreadyExists, got %v", err)
				}
			case tt.wantErr:
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
			default:
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
			if gotMethod != http.MethodPut {
				t.Errorf("method = %q, want PUT", gotMethod)
			}
			if !strings.HasSuffix(gotPath, tt.wantPathHas) {
				t.Errorf("path = %q, want suffix %q", gotPath, tt.wantPathHas)
			}
			if !tt.wantErr && gotCT != "application/octet-stream" {
				t.Errorf("content-type = %q, want application/octet-stream", gotCT)
			}
		})
	}
}
