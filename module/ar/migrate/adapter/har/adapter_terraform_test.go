package har

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/module/ar/migrate/types"
	"github.com/rs/zerolog"
)

// TestUploadFileTerraformRouting verifies that adapter.UploadFile dispatches
// TERRAFORM to uploadTerraformFile (module and provider paths).
func TestUploadFileTerraformRouting(t *testing.T) {
	config.Global.AccountID = "acct1"

	tests := []struct {
		name        string
		fileName    string
		pkg         string
		version     string
		wantPathHas string
	}{
		{
			name:        "module routed correctly",
			fileName:    "vpc-1.0.0.tar.gz",
			pkg:         "hashicorp/vpc/aws",
			version:     "1.0.0",
			wantPathHas: "/hashicorp/vpc/aws/1.0.0",
		},
		{
			name:        "provider routed correctly",
			fileName:    "terraform-provider-aws_2.0.0_linux_amd64.zip",
			pkg:         "hashicorp/aws",
			version:     "2.0.0",
			wantPathHas: "/hashicorp/aws/2.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				w.WriteHeader(http.StatusCreated)
			}))
			defer srv.Close()

			c := newTestClient(t, srv.URL)
			a := &adapter{client: c, logger: zerolog.Nop()}

			f := &types.File{Name: tt.fileName, Uri: "/" + tt.fileName}
			body := io.NopCloser(strings.NewReader("bytes"))

			err := a.UploadFile("reg1", body, f, http.Header{}, tt.pkg, tt.version, types.TERRAFORM, nil)
			if err != nil {
				t.Fatalf("UploadFile error: %v", err)
			}
			if !strings.Contains(gotPath, tt.wantPathHas) {
				t.Errorf("path = %q, want to contain %q", gotPath, tt.wantPathHas)
			}
		})
	}
}
