package util

import "testing"

func TestParseChartFileName(t *testing.T) {
	tests := []struct {
		name        string
		filename    string
		wantName    string
		wantVersion string
		wantOK      bool
	}{
		{
			name:        "simple chart",
			filename:    "nginx-1.0.0.tgz",
			wantName:    "nginx",
			wantVersion: "1.0.0",
			wantOK:      true,
		},
		{
			name:        "hyphenated chart name",
			filename:    "prometheus-mysql-exporter-1.2.3.tgz",
			wantName:    "prometheus-mysql-exporter",
			wantVersion: "1.2.3",
			wantOK:      true,
		},
		{
			name:        "prerelease version with hyphen",
			filename:    "nginx-1.0.0-alpha.1.tgz",
			wantName:    "nginx",
			wantVersion: "1.0.0-alpha.1",
			wantOK:      true,
		},
		{
			name:        "hyphenated name and prerelease version",
			filename:    "my-app-2.0.0-rc.1.tgz",
			wantName:    "my-app",
			wantVersion: "2.0.0-rc.1",
			wantOK:      true,
		},
		{
			name:        "build metadata",
			filename:    "nginx-1.0.0+build.5.tgz",
			wantName:    "nginx",
			wantVersion: "1.0.0+build.5",
			wantOK:      true,
		},
		{
			name:        "v-prefixed version",
			filename:    "cert-manager-v1.20.0.tgz",
			wantName:    "cert-manager",
			wantVersion: "v1.20.0",
			wantOK:      true,
		},
		{
			name:        "provenance file",
			filename:    "nginx-1.0.0.tgz.prov",
			wantName:    "nginx",
			wantVersion: "1.0.0",
			wantOK:      true,
		},
		{
			name:        "nested directory prefix is stripped to leaf",
			filename:    "ChartA/ChartB/abc-1.0.1.tgz",
			wantName:    "abc",
			wantVersion: "1.0.1",
			wantOK:      true,
		},
		{
			name:        "nested prefix on provenance",
			filename:    "ChartA/ChartB/abc-1.0.1.tgz.prov",
			wantName:    "abc",
			wantVersion: "1.0.1",
			wantOK:      true,
		},
		{
			name:     "no version",
			filename: "nginx.tgz",
			wantOK:   false,
		},
		{
			name:     "no hyphen",
			filename: "nginx1.0.0.tgz",
			wantOK:   false,
		},
		{
			name:     "non-semver right side",
			filename: "some-thing.tgz",
			wantOK:   false,
		},
		{
			name:     "empty",
			filename: "",
			wantOK:   false,
		},
		{
			name:     "trailing hyphen only",
			filename: "nginx-.tgz",
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotVersion, gotOK := ParseChartFileName(tt.filename)
			if gotOK != tt.wantOK {
				t.Fatalf("ParseChartFileName(%q) ok = %v, want %v", tt.filename, gotOK, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if gotName != tt.wantName || gotVersion != tt.wantVersion {
				t.Errorf("ParseChartFileName(%q) = (%q, %q), want (%q, %q)",
					tt.filename, gotName, gotVersion, tt.wantName, tt.wantVersion)
			}
		})
	}
}

func TestGetChartFileName(t *testing.T) {
	tests := []struct {
		name    string
		chart   string
		version string
		want    string
	}{
		{"flat", "nginx", "1.0.0", "nginx-1.0.0.tgz"},
		{"nested prefix preserved", "ChartA/ChartB/abc", "1.0.1", "ChartA/ChartB/abc-1.0.1.tgz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetChartFileName(tt.chart, tt.version); got != tt.want {
				t.Errorf("GetChartFileName(%q, %q) = %q, want %q", tt.chart, tt.version, got, tt.want)
			}
		})
	}
}

func TestGetChartProvFileName(t *testing.T) {
	if got := GetChartProvFileName("nginx", "1.0.0"); got != "nginx-1.0.0.tgz.prov" {
		t.Errorf("GetChartProvFileName = %q, want nginx-1.0.0.tgz.prov", got)
	}
}

func TestIsHelmChartArchive(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"chart", "nginx-1.0.0.tgz", true},
		{"nested chart", "ChartA/ChartB/abc-1.0.1.tgz", true},
		{"prov is not a chart", "nginx-1.0.0.tgz.prov", false},
		{"index is not a chart", "index.yaml", false},
		{"checksum is not a chart", "nginx-1.0.0.tgz.sha256", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsHelmChartArchive(tt.in); got != tt.want {
				t.Errorf("IsHelmChartArchive(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
