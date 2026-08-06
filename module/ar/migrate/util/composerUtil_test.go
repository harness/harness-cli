package util

import "testing"

func TestComposerLogicalNameFromZipURI(t *testing.T) {
	tests := []struct {
		uri  string
		want string
		ok   bool
	}{
		{"/acme/billing-sdk/acme-billing-sdk-1.0.0.zip", "acme/billing-sdk", true},
		{"/harness/migtest/harness-migtest-2.0.0.zip", "harness/migtest", true},
		{"/harness-migtest/harness-migtest-1.0.0.zip", "harness/migtest", true},
		{"/acme-demo/acme-demo-1.0.0.zip", "acme/demo", true},
		{"/acme-demo-1.0.0.zip", "acme/demo", true},
		{"/harness-migtest-1.0.0.zip", "harness/migtest", true},
		{"/abc-xyz-qwe/file.zip", "abc/xyz-qwe", true},
		{"/abc-xyz/qwe/file.zip", "abc-xyz/qwe", true},
		{"/vendoronly/file.zip", "vendoronly", true},
		{"bad.zip", "", false},
		{"/single-segment.zip", "", false},
		{"/not-a-version.zip", "", false},
		{"/.composer/packages.json", "", false},
		{"/readme.txt", "", false},
	}
	for _, tt := range tests {
		got, ok := ComposerLogicalNameFromZipURI(tt.uri)
		if ok != tt.ok {
			t.Errorf("ComposerLogicalNameFromZipURI(%q) ok = %v, want %v", tt.uri, ok, tt.ok)
			continue
		}
		if got != tt.want {
			t.Errorf("ComposerLogicalNameFromZipURI(%q) = %q, want %q", tt.uri, got, tt.want)
		}
	}
}

func TestComposerZipBasename(t *testing.T) {
	tests := []struct {
		uri  string
		want string
	}{
		{"/acme/payments-api/payments-api-2.0.0.zip", "payments-api-2.0.0.zip"},
		{"/harness-migtest/harness-migtest-1.0.0.zip", "harness-migtest-1.0.0.zip"},
	}
	for _, tt := range tests {
		if got := ComposerZipBasename(tt.uri); got != tt.want {
			t.Errorf("ComposerZipBasename(%q) = %q, want %q", tt.uri, got, tt.want)
		}
	}
}

func TestComposerVersionFromZipFilename(t *testing.T) {
	tests := []struct {
		file string
		want string
	}{
		{"harness-migtest-1.0.0.zip", "1.0.0"},
		{"harness-migtest-2.0.0.zip", "2.0.0"},
		{"acme-billing-sdk-3.0.0.0.zip", "3.0.0.0"},
		{"3.0.0.0.zip", "3.0.0.0"},
		{"not-a-version.zip", "not-a-version"},
	}
	for _, tt := range tests {
		if got := ComposerVersionFromZipFilename(tt.file); got != tt.want {
			t.Errorf("ComposerVersionFromZipFilename(%q) = %q, want %q", tt.file, got, tt.want)
		}
	}
}
