package cmdutils

import "testing"

func TestParseRemoteURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantRef   string
		wantBase  string
		wantAcct  string
		wantErr   bool
	}{
		{
			name:     "HTTPS project-level",
			url:      "https://git0.harness.io/l7B_kbSEQD2wjrM7PShm5w/default/CD/codepulse.git",
			wantRef:  "l7B_kbSEQD2wjrM7PShm5w/default/CD/codepulse",
			wantBase: "https://harness0.harness.io/gateway",
			wantAcct: "l7B_kbSEQD2wjrM7PShm5w",
		},
		{
			name:     "HTTPS org-level",
			url:      "https://git0.harness.io/acct123/myorg/myrepo.git",
			wantRef:  "acct123/myorg/myrepo",
			wantBase: "https://harness0.harness.io/gateway",
			wantAcct: "acct123",
		},
		{
			name:     "HTTPS no .git suffix",
			url:      "https://git0.harness.io/acct123/default/proj/repo",
			wantRef:  "acct123/default/proj/repo",
			wantBase: "https://harness0.harness.io/gateway",
			wantAcct: "acct123",
		},
		{
			name:     "SSH project-level",
			url:      "git@git0.harness.io:l7B_kbSEQD2wjrM7PShm5w/default/CD/codepulse.git",
			wantRef:  "l7B_kbSEQD2wjrM7PShm5w/default/CD/codepulse",
			wantBase: "https://harness0.harness.io/gateway",
			wantAcct: "l7B_kbSEQD2wjrM7PShm5w",
		},
		{
			name:    "non-harness URL",
			url:     "https://github.com/user/repo.git",
			wantErr: true,
		},
		{
			name:    "empty path",
			url:     "https://git0.harness.io/",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := ParseRemoteURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRemoteURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if ctx.RepoRef != tt.wantRef {
				t.Errorf("RepoRef = %q, want %q", ctx.RepoRef, tt.wantRef)
			}
			if ctx.BaseURL != tt.wantBase {
				t.Errorf("BaseURL = %q, want %q", ctx.BaseURL, tt.wantBase)
			}
			if ctx.AccountID != tt.wantAcct {
				t.Errorf("AccountID = %q, want %q", ctx.AccountID, tt.wantAcct)
			}
		})
	}
}
