package util

import "testing"

func TestParsePuppetFileNameWithPath(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantName    string
		wantVersion string
		wantOK      bool
	}{
		{"forge layout", "/puppetlabs/stdlib/puppetlabs-stdlib-9.4.1.tar.gz", "puppetlabs-stdlib", "9.4.1", true},
		{"author-module-version", "puppetlabs-apache-12.3.0.tar.gz", "puppetlabs-apache", "12.3.0", true},
		{"semver pre-release", "acme-foo-1.0.0-rc.1.tar.gz", "acme-foo", "1.0.0-rc.1", true},
		{"semver build metadata", "acme-foo-1.0.0+build.1.tar.gz", "acme-foo", "1.0.0+build.1", true},
		{"underscore in module", "myorg-my_module-2.5.0.tar.gz", "myorg-my_module", "2.5.0", true},
		{"missing extension", "puppetlabs-stdlib-9.4.1.zip", "", "", false},
		{"missing version", "puppetlabs-stdlib.tar.gz", "", "", false},
		{"invalid module name", "1bad-module-1.0.0.tar.gz", "", "", false},
		{"invalid version", "puppetlabs-stdlib-not.a.semver.tar.gz", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotVersion, ok := ParsePuppetFileNameWithPath(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if gotName != tt.wantName || gotVersion != tt.wantVersion {
				t.Fatalf("got (%q, %q), want (%q, %q)", gotName, gotVersion, tt.wantName, tt.wantVersion)
			}
		})
	}
}
