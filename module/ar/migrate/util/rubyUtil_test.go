package util

import "testing"

func TestParseRubyGemFileNameWithPath(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantName     string
		wantVersion  string
		wantPlatform string
		wantOK       bool
	}{
		{"default platform gem", "rails-8.0.2.gem", "rails", "8.0.2", "", true},
		{"nested path", "/gems/rails-8.0.2.gem", "rails", "8.0.2", "", true},
		{"hyphenated gem name", "faraday-net_http-3.1.1.gem", "faraday-net_http", "3.1.1", "", true},
		{
			"platform gem mingw",
			"platform-gem-2.0.0-x86-mingw32-20.gem",
			"platform-gem",
			"2.0.0",
			"x86-mingw32-20",
			true,
		},
		{
			"platform gem linux",
			"nokogiri-1.15.0-x86_64-linux.gem",
			"nokogiri",
			"1.15.0",
			"x86_64-linux",
			true,
		},
		{"ruby prerelease dot", "mygem-1.0.0.beta1.gem", "mygem", "1.0.0.beta1", "", true},
		{"ruby short prerelease", "mygem-1.0.pre.gem", "mygem", "1.0.pre", "", true},
		{"missing extension", "rails-8.0.2.zip", "", "", "", false},
		{"missing version", "rails.gem", "", "", "", false},
		{"no valid version", "foo-bar.gem", "", "", "", false},
		{"empty name", "-1.0.0.gem", "", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, ok := ParseRubyGemFileNameWithPath(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if meta.Name != tt.wantName || meta.Version != tt.wantVersion || meta.Platform != tt.wantPlatform {
				t.Fatalf("got name=%q version=%q platform=%q, want name=%q version=%q platform=%q",
					meta.Name, meta.Version, meta.Platform, tt.wantName, tt.wantVersion, tt.wantPlatform)
			}
		})
	}
}

func TestParseGemFilename_platform(t *testing.T) {
	meta, err := parseGemFilename("nokogiri-1.15.0-x86_64-linux.gem")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Name != "nokogiri" || meta.Version != "1.15.0" || meta.Platform != "x86_64-linux" {
		t.Fatalf("got name=%q version=%q platform=%q", meta.Name, meta.Version, meta.Platform)
	}
}
