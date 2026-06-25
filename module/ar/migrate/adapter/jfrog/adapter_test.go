package jfrog

import "testing"

func TestChartRepoRelPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "relative path remains unchanged",
			in:   "team-a/abc-1.0.1.tgz",
			want: "team-a/abc-1.0.1.tgz",
		},
		{
			name: "leading slash is trimmed",
			in:   "/team-a/abc-1.0.1.tgz",
			want: "team-a/abc-1.0.1.tgz",
		},
		{
			name: "absolute artifactory URL drops repo prefix",
			in:   "https://jfrog.example/artifactory/helm-http-local/team-a/abc-1.0.1.tgz",
			want: "team-a/abc-1.0.1.tgz",
		},
		{
			name: "absolute artifactory path drops repo prefix",
			in:   "/artifactory/helm-http-local/team-a/abc-1.0.1.tgz",
			want: "team-a/abc-1.0.1.tgz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chartRepoRelPath(tt.in)
			if got != tt.want {
				t.Errorf("chartRepoRelPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestGetNestedName(t *testing.T) {
	tests := []struct {
		name        string
		packageName string
		urls        []string
		wantName    string
		wantErr     bool
	}{
		{
			name:        "relative URL keeps flat package name",
			packageName: "nginx",
			urls:        []string{"nginx-1.0.0.tgz"},
			wantName:    "nginx",
		},
		{
			name:        "absolute URL carries nested prefix",
			packageName: "nginx",
			urls:        []string{"https://jfrog.example/artifactory/helm-http-local/team-a/nginx-1.0.0.tgz"},
			wantName:    "team-a/nginx",
		},
		{
			name:        "empty URL list errors",
			packageName: "nginx",
			urls:        nil,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getNestedName(tt.packageName, tt.urls)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantName {
				t.Errorf("getNestedName(%q, %v) = %q, want %q", tt.packageName, tt.urls, got, tt.wantName)
			}
		})
	}
}
