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
	migratehttp "github.com/harness/harness-cli/module/ar/migrate/http"
	"github.com/harness/harness-cli/module/ar/migrate/types"

	"github.com/rs/zerolog"
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

// TestUploadTerraformFile covers module (.tar.gz) and provider (.zip) upload paths
// including conflict (409 → ErrArtifactAlreadyExists) and non-2xx error handling.
func TestUploadTerraformFile(t *testing.T) {
	config.Global.AccountID = "acct1"

	tests := []struct {
		name         string
		fileName     string
		pkg          string
		version      string
		status       int
		wantConflict bool
		wantErr      bool
		wantPathHas  string
	}{
		{
			name:     "module upload success",
			fileName: "vpc-1.0.0.tar.gz", pkg: "hashicorp/vpc/aws", version: "1.0.0",
			status: http.StatusCreated, wantPathHas: "/hashicorp/vpc/aws/1.0.0",
		},
		{
			name:     "module upload tgz extension",
			fileName: "vpc-1.0.0.tgz", pkg: "hashicorp/vpc/aws", version: "1.0.0",
			status: http.StatusCreated, wantPathHas: "/hashicorp/vpc/aws/1.0.0",
		},
		{
			name:     "module upload conflict",
			fileName: "vpc-1.0.0.tar.gz", pkg: "hashicorp/vpc/aws", version: "1.0.0",
			status: http.StatusConflict, wantConflict: true,
		},
		{
			name:     "module upload error",
			fileName: "vpc-1.0.0.tar.gz", pkg: "hashicorp/vpc/aws", version: "1.0.0",
			status: http.StatusBadRequest, wantErr: true,
		},
		{
			name:     "provider upload success",
			fileName: "terraform-provider-aws_2.0.0_linux_amd64.zip", pkg: "hashicorp/aws", version: "2.0.0",
			status: http.StatusCreated, wantPathHas: "/hashicorp/aws/2.0.0",
		},
		{
			name:     "provider upload conflict",
			fileName: "terraform-provider-aws_2.0.0_linux_amd64.zip", pkg: "hashicorp/aws", version: "2.0.0",
			status: http.StatusConflict, wantConflict: true,
		},
		{
			name:     "provider upload error",
			fileName: "terraform-provider-aws_2.0.0_linux_amd64.zip", pkg: "hashicorp/aws", version: "2.0.0",
			status: http.StatusInternalServerError, wantErr: true,
		},
		{
			name:     "unsupported extension",
			fileName: "terraform-something.txt", pkg: "hashicorp/aws", version: "2.0.0",
			status: http.StatusCreated, wantErr: true,
		},
		{
			name:     "module bad pkg format",
			fileName: "vpc-1.0.0.tar.gz", pkg: "hashicorp", version: "1.0.0",
			status: http.StatusCreated, wantErr: true,
		},
		{
			name:     "provider bad pkg format",
			fileName: "terraform-provider-aws_2.0.0_linux_amd64.zip", pkg: "hashicorp", version: "2.0.0",
			status: http.StatusCreated, wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				if tt.status >= 400 {
					w.WriteHeader(tt.status)
					_, _ = w.Write([]byte("rejected"))
					return
				}
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			c := newTestClient(t, srv.URL)
			f := &types.File{Name: tt.fileName, Uri: "/" + tt.fileName}
			body := io.NopCloser(strings.NewReader("bytes"))

			err := c.uploadTerraformFile("reg1", f, tt.pkg, tt.version, body)

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
				if tt.wantPathHas != "" && !strings.Contains(gotPath, tt.wantPathHas) {
					t.Errorf("path = %q, want to contain %q", gotPath, tt.wantPathHas)
				}
			}
		})
	}
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


// TestUploadFileCRAN covers the CRAN adapter path, which routes through uploadRawFile
// (/files PUT). Paths are already remapped to flat contrib by the migrator before upload.
func TestUploadFileCRAN(t *testing.T) {
	config.Global.AccountID = "acct1"

	tests := []struct {
		name         string
		status       int
		fileUri      string
		wantErr      bool
		wantConflict bool
		wantPathHas  string
	}{
		{"source contrib", http.StatusCreated, "src/contrib/jsonlite_1.8.0.tar.gz", false, false,
			"/files/src/contrib/jsonlite_1.8.0.tar.gz"},
		{"remapped archive source", http.StatusCreated, "src/contrib/jsonlite_1.7.0.tar.gz", false, false,
			"/files/src/contrib/jsonlite_1.7.0.tar.gz"},
		{"windows binary", http.StatusCreated, "bin/windows/contrib/4.4/jsonlite_1.8.0.zip", false, false,
			"/files/bin/windows/contrib/4.4/jsonlite_1.8.0.zip"},
		{"leading slash trimmed", http.StatusCreated, "/src/contrib/jsonlite_1.8.0.tar.gz", false, false,
			"/files/src/contrib/jsonlite_1.8.0.tar.gz"},
		{"conflict", http.StatusConflict, "src/contrib/jsonlite_1.8.0.tar.gz", false, true,
			"/files/src/contrib/jsonlite_1.8.0.tar.gz"},
		{"bad request", http.StatusBadRequest, "src/contrib/jsonlite_1.8.0.tar.gz", true, false,
			"/files/src/contrib/jsonlite_1.8.0.tar.gz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotMethod, gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.Path
				w.WriteHeader(tt.status)
				if tt.status >= 400 {
					_, _ = w.Write([]byte("upload rejected"))
				}
			}))
			defer srv.Close()

			a := &adapter{
				client: newTestClient(t, srv.URL),
				logger: zerolog.Nop(),
			}
			body := io.NopCloser(strings.NewReader("cran-bytes"))
			err := a.UploadFile(
				"reg1",
				body,
				&types.File{Name: "jsonlite_1.8.0.tar.gz", Uri: tt.fileUri},
				nil,
				"jsonlite",
				"1.8.0",
				types.CRAN,
				nil,
			)

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
		})
	}
}


// TestUploadComposerFile verifies composer uploads and maps HTTP 409 to
// ErrArtifactAlreadyExists so re-runs are recorded as skips, not failures.
func TestUploadComposerFile(t *testing.T) {
	config.Global.AccountID = "acct1"

	tests := []struct {
		name         string
		status       int
		wantErr      bool
		wantConflict bool
	}{
		{"success 201", http.StatusCreated, false, false},
		{"success 200", http.StatusOK, false, false},
		{"conflict surfaces ErrArtifactAlreadyExists", http.StatusConflict, false, true},
		{"bad request surfaces error", http.StatusBadRequest, true, false},
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
					_, _ = w.Write([]byte(`{"message":"version already exists"}`))
					return
				}
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			c := &client{
				url:    srv.URL,
				client: migratehttp.NewClient(srv.Client()),
			}
			body := io.NopCloser(strings.NewReader("zip-bytes"))
			err := c.uploadComposerFile("composer_mig", "harness-migtest-1.0.0.zip", body)

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
			if gotMethod != http.MethodPost {
				t.Errorf("method = %q, want POST", gotMethod)
			}
			wantPath := "/pkg/acct1/composer_mig/composer/upload"
			if gotPath != wantPath {
				t.Errorf("path = %q, want %q", gotPath, wantPath)
			}
			if !tt.wantErr && gotCT != "application/octet-stream" {
				t.Errorf("content-type = %q, want application/octet-stream", gotCT)
			}
		})
	}
}

// TestUploadRubyFile verifies Ruby gem uploads and maps HTTP 409 to
// ErrArtifactAlreadyExists so re-runs are recorded as skips, not failures.
func TestUploadRubyFile(t *testing.T) {
	config.Global.AccountID = "acct1"

	tests := []struct {
		name         string
		status       int
		wantErr      bool
		wantConflict bool
	}{
		{"success 201", http.StatusCreated, false, false},
		{"success 200", http.StatusOK, false, false},
		{"conflict surfaces ErrArtifactAlreadyExists", http.StatusConflict, false, true},
		{"bad request surfaces error", http.StatusBadRequest, true, false},
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
					_, _ = w.Write([]byte(`{"message":"gem already exists"}`))
					return
				}
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			c := newTestClient(t, srv.URL)
			body := io.NopCloser(strings.NewReader("gem-bytes"))
			err := c.uploadRubyFile("ruby_mig", &types.File{Name: "rails-8.0.2.gem"}, body)

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
			if gotMethod != http.MethodPost {
				t.Errorf("method = %q, want POST", gotMethod)
			}
			wantPath := "/pkg/acct1/ruby_mig/ruby/api/v1/gems"
			if gotPath != wantPath {
				t.Errorf("path = %q, want %q", gotPath, wantPath)
			}
			if !tt.wantErr && gotCT != "application/octet-stream" {
				t.Errorf("content-type = %q, want application/octet-stream", gotCT)
			}
		})
	}
}

// TestFileExistsCRAN ensures CRAN existence checks use HEAD on /files (same as RAW).
func TestFileExistsCRAN(t *testing.T) {
	config.Global.AccountID = "acct1"

	tests := []struct {
		name       string
		status     int
		wantExists bool
		wantErr    bool
		fileUri    string
		wantPath   string
	}{
		{"exists", http.StatusOK, true, false, "src/contrib/jsonlite_1.8.0.tar.gz",
			"/pkg/acct1/reg1/files/src/contrib/jsonlite_1.8.0.tar.gz"},
		{"missing", http.StatusNotFound, false, false, "src/contrib/jsonlite_1.8.0.tar.gz",
			"/pkg/acct1/reg1/files/src/contrib/jsonlite_1.8.0.tar.gz"},
		{"leading slash trimmed", http.StatusOK, true, false, "/src/contrib/jsonlite_1.8.0.tar.gz",
			"/pkg/acct1/reg1/files/src/contrib/jsonlite_1.8.0.tar.gz"},
		{"server error", http.StatusInternalServerError, false, true, "src/contrib/jsonlite_1.8.0.tar.gz",
			"/pkg/acct1/reg1/files/src/contrib/jsonlite_1.8.0.tar.gz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotMethod, gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.Path
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			a := &adapter{
				client: newTestClient(t, srv.URL),
				logger: zerolog.Nop(),
			}
			exists, err := a.FileExists(
				nil,
				"reg1",
				"jsonlite",
				"1.8.0",
				&types.File{Name: "jsonlite_1.8.0.tar.gz", Uri: tt.fileUri},
				types.CRAN,
			)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if exists != tt.wantExists {
				t.Errorf("exists = %v, want %v", exists, tt.wantExists)
			}
			if gotMethod != http.MethodHead {
				t.Errorf("method = %q, want HEAD", gotMethod)
			}
			if gotPath != tt.wantPath {
				t.Errorf("path = %q, want %q", gotPath, tt.wantPath)
			}
		})
	}
}
