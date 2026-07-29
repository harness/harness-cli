package util

import "testing"

func TestParseNugetFileNameWithPath(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantName    string
		wantVersion string
		wantOK      bool
	}{
		{
			name:        "prerelease with build metadata dots",
			input:       "/hello/hello.foo.bar.xxx.yyy/3.203.0-INTEGRATION/hello.foo.bar.xxx.yyy.3.203.0-pr-280.a52f7f9.1.nupkg",
			wantName:    "hello.foo.bar.xxx.yyy",
			wantVersion: "3.203.0-pr-280.a52f7f9.1",
			wantOK:      true,
		},
		{
			name:        "prerelease with build metadata dots1",
			input:       "/hello/hello.foo.bar.xxx.yyy/3.203.0-INTEGRATION/hello.foo.bar.xxx.yyy.3.203.0.nupkg",
			wantName:    "hello.foo.bar.xxx.yyy",
			wantVersion: "3.203.0",
			wantOK:      true,
		},
		{
			name:        "prerelease with build metadata dots2",
			input:       "/hello/hello.foo.bar.xxx.yyy/3.203.0-INTEGRATION/hello.foo.bar.xxx.yyy.3.203.0-beta.nupkg",
			wantName:    "hello.foo.bar.xxx.yyy",
			wantVersion: "3.203.0-beta",
			wantOK:      true,
		},
		{
			name:        "prerelease with build metadata dots3",
			input:       "/hello/hello.foo.bar.xxx.yyy/3.203.0-INTEGRATION/hello.foo.bar.xxx.yyy.3.203.0-pr-280+a52f7f9.1.nupkg",
			wantName:    "hello.foo.bar.xxx.yyy",
			wantVersion: "3.203.0-pr-280+a52f7f9.1",
			wantOK:      true,
		},
		{
			name:        "prerelease with build metadata dots3",
			input:       "/hello/hello.foo.bar.xxx.yyy/3.203.0-INTEGRATION/hello.foo.bar.xxx.yyy.3.203.0-pr-280.a52f7f9.1.nupkg",
			wantName:    "hello.foo.bar.xxx.yyy",
			wantVersion: "3.203.0-pr-280.a52f7f9.1",
			wantOK:      true,
		},
		{
			name:        "prerelease with build metadata dots3",
			input:       "/hello/hello.foo.bar.xxx.yyy/3.203.0-INTEGRATION/hello.foo.bar.xxx.yyy.3.203.0-pr-280.nupkg",
			wantName:    "hello.foo.bar.xxx.yyy",
			wantVersion: "3.203.0-pr-280",
			wantOK:      true,
		},
		{
			name:        "plain semver",
			input:       "/Newtonsoft.Json/13.0.3/Newtonsoft.Json.13.0.3.nupkg",
			wantName:    "newtonsoft.json",
			wantVersion: "13.0.3",
			wantOK:      true,
		},
		{
			name:        "bare file name, no path",
			input:       "Serilog.Sinks.Console.4.1.0.nupkg",
			wantName:    "serilog.sinks.console",
			wantVersion: "4.1.0",
			wantOK:      true,
		},
		{
			name:        "prerelease without build dots",
			input:       "My.Package.2.0.0-beta.nupkg",
			wantName:    "my.package",
			wantVersion: "2.0.0-beta",
			wantOK:      true,
		},
		{
			// A numeric-led (but not all-digit) name segment must not be
			// mistaken for the version's start.
			name:        "numeric-led package name segment",
			input:       "7zip.Portable.1.2.3.nupkg",
			wantName:    "7zip.portable",
			wantVersion: "1.2.3",
			wantOK:      true,
		},
		{
			name:        "snupkg extension",
			input:       "My.Symbols.1.0.0.snupkg",
			wantName:    "my.symbols",
			wantVersion: "1.0.0",
			wantOK:      true,
		},
		{
			name:        "wrong extension",
			input:       "My.Package.1.0.0.zip",
			wantName:    "",
			wantVersion: "",
			wantOK:      false,
		},
		{
			name:        "no numeric version segment",
			input:       "My.Package.nupkg",
			wantName:    "",
			wantVersion: "",
			wantOK:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotVersion, ok := ParseNugetFileNameWithPath(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if gotName != tt.wantName || gotVersion != tt.wantVersion {
				t.Fatalf("got (%q, %q), want (%q, %q)", gotName, gotVersion, tt.wantName, tt.wantVersion)
			}
		})
	}
}
