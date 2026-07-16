package util

import "testing"

func TestParseComposerFileName(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantName    string
		wantVersion string
		wantOK      bool
	}{
		{"flat filename", "vendor-package-1.0.0.zip", "vendor-package", "1.0.0", true},
		{"with path", "/repo/vendor-package-2.3.1.zip", "vendor-package", "2.3.1", true},
		{"v prefix version", "acme-billing-v1.0.0.zip", "acme-billing", "v1.0.0", true},
		{"pre-release", "acme-sdk-1.0.0-alpha.1.zip", "acme-sdk", "1.0.0-alpha.1", true},
		{"build metadata", "acme-sdk-1.0.0+build.1.zip", "acme-sdk", "1.0.0+build.1", true},
		{"underscore in name", "my_vendor-my_pkg-3.0.0.zip", "my_vendor-my_pkg", "3.0.0", true},
		{"missing extension", "vendor-package-1.0.0.tar.gz", "", "", false},
		{"no version", "vendor-package.zip", "", "", false},
		{"invalid version", "vendor-package-notaversion.zip", "", "", false},
		{"single segment", "package-1.0.0.zip", "", "", false},
		{"multi hyphen name", "acme-corp-billing-sdk-1.0.0.zip", "acme-corp-billing-sdk", "1.0.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotVersion, ok := ParseComposerFileName(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if gotName != tt.wantName || gotVersion != tt.wantVersion {
				t.Fatalf("got (%q, %q), want (%q, %q)", gotName, gotVersion, tt.wantName, tt.wantVersion)
			}
		})
	}
}
