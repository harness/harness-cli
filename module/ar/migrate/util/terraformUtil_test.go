package util

import (
	"testing"
)

func TestParseTerraformModulePath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantNS   string
		wantName string
		wantProv string
		wantVer  string
		wantOK   bool
	}{
		{
			name:   "happy path with leading slash",
			path:   "/hashicorp/vpc/aws/1.0.0/vpc-1.0.0.tar.gz",
			wantNS: "hashicorp", wantName: "vpc", wantProv: "aws", wantVer: "1.0.0", wantOK: true,
		},
		{
			name:   "happy path without leading slash",
			path:   "hashicorp/vpc/aws/1.0.0/vpc-1.0.0.tar.gz",
			wantNS: "hashicorp", wantName: "vpc", wantProv: "aws", wantVer: "1.0.0", wantOK: true,
		},
		{
			name:   "tgz extension",
			path:   "/hashicorp/vpc/aws/2.0.0/vpc-2.0.0.tgz",
			wantNS: "hashicorp", wantName: "vpc", wantProv: "aws", wantVer: "2.0.0", wantOK: true,
		},
		{
			name:   "wrong extension",
			path:   "/hashicorp/vpc/aws/1.0.0/vpc-1.0.0.zip",
			wantOK: false,
		},
		{
			name:   "too few segments (depth 4)",
			path:   "/hashicorp/vpc/aws/vpc-1.0.0.tar.gz",
			wantOK: false,
		},
		{
			name:   "too few segments (depth 3)",
			path:   "/hashicorp/vpc/vpc-1.0.0.tar.gz",
			wantOK: false,
		},
		{
			name:   "empty path",
			path:   "",
			wantOK: false,
		},
		{
			name:   "only slash",
			path:   "/",
			wantOK: false,
		},
		{
			name:   "depth 6 (extra subdirectory)",
			path:   "/hashicorp/vpc/aws/1.0.0/subdir/vpc-1.0.0.tar.gz",
			wantNS: "hashicorp", wantName: "vpc", wantProv: "aws", wantVer: "1.0.0", wantOK: true,
		},
		// Layout B: flat zip, version = filename stem
		{
			name:   "layout B flat zip",
			path:   "/myorg/s3module/aws/1.0.0.zip",
			wantNS: "myorg", wantName: "s3module", wantProv: "aws", wantVer: "1.0.0", wantOK: true,
		},
		{
			name:   "layout B flat zip without leading slash",
			path:   "myorg/s3module/google/2.3.4.zip",
			wantNS: "myorg", wantName: "s3module", wantProv: "google", wantVer: "2.3.4", wantOK: true,
		},
		{
			name:   "layout B rejected: provider filename in 4-segment path",
			path:   "/hashicorp/aws/2.0.0/terraform-provider-aws_linux_amd64.zip",
			wantOK: false,
		},
		{
			name:   "layout B rejected: module.json sidecar",
			path:   "/myorg/s3module/aws/module.json",
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns, name, provider, version, ok := ParseTerraformModulePath(tt.path)
			if ok != tt.wantOK {
				t.Fatalf("ok=%v want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if ns != tt.wantNS {
				t.Errorf("ns=%q want %q", ns, tt.wantNS)
			}
			if name != tt.wantName {
				t.Errorf("name=%q want %q", name, tt.wantName)
			}
			if provider != tt.wantProv {
				t.Errorf("provider=%q want %q", provider, tt.wantProv)
			}
			if version != tt.wantVer {
				t.Errorf("version=%q want %q", version, tt.wantVer)
			}
		})
	}
}

func TestParseTerraformProviderPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantNS   string
		wantType string
		wantVer  string
		wantFile string
		wantOS   string
		wantArch string
		wantOK   bool
	}{
		{
			name:   "happy path linux amd64",
			path:   "/hashicorp/aws/2.0.0/terraform-provider-aws_2.0.0_linux_amd64.zip",
			wantNS: "hashicorp", wantType: "aws", wantVer: "2.0.0",
			wantFile: "terraform-provider-aws_2.0.0_linux_amd64.zip",
			wantOS:   "linux", wantArch: "amd64", wantOK: true,
		},
		{
			name:   "filename missing version segment",
			path:   "/hashicorp/aws/2.0.0/terraform-provider-aws_linux_amd64.zip",
			wantOK: false,
		},
		{
			name:   "darwin arm64",
			path:   "/hashicorp/aws/2.0.0/terraform-provider-aws_2.0.0_darwin_arm64.zip",
			wantNS: "hashicorp", wantType: "aws", wantVer: "2.0.0",
			wantFile: "terraform-provider-aws_2.0.0_darwin_arm64.zip",
			wantOS:   "darwin", wantArch: "arm64", wantOK: true,
		},
		{
			name:   "with leading slash",
			path:   "/hashicorp/google/3.1.0/terraform-provider-google_3.1.0_windows_386.zip",
			wantNS: "hashicorp", wantType: "google", wantVer: "3.1.0",
			wantFile: "terraform-provider-google_3.1.0_windows_386.zip",
			wantOS:   "windows", wantArch: "386", wantOK: true,
		},
		{
			name:   "non-provider zip (filename doesn't match regex)",
			path:   "/hashicorp/aws/2.0.0/aws-sdk-2.0.0_linux_amd64.zip",
			wantOK: false,
		},
		{
			name:   "tar.gz not a provider",
			path:   "/hashicorp/aws/2.0.0/terraform-provider-aws_linux_amd64.tar.gz",
			wantOK: false,
		},
		{
			name:   "too few path segments (3)",
			path:   "/hashicorp/aws/terraform-provider-aws_linux_amd64.zip",
			wantOK: false,
		},
		{
			name:   "empty path",
			path:   "",
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns, typeName, version, filename, osName, arch, ok := ParseTerraformProviderPath(tt.path)
			if ok != tt.wantOK {
				t.Fatalf("ok=%v want %v for path %q", ok, tt.wantOK, tt.path)
			}
			if !ok {
				return
			}
			if ns != tt.wantNS {
				t.Errorf("ns=%q want %q", ns, tt.wantNS)
			}
			if typeName != tt.wantType {
				t.Errorf("typeName=%q want %q", typeName, tt.wantType)
			}
			if version != tt.wantVer {
				t.Errorf("version=%q want %q", version, tt.wantVer)
			}
			if filename != tt.wantFile {
				t.Errorf("filename=%q want %q", filename, tt.wantFile)
			}
			if osName != tt.wantOS {
				t.Errorf("os=%q want %q", osName, tt.wantOS)
			}
			if arch != tt.wantArch {
				t.Errorf("arch=%q want %q", arch, tt.wantArch)
			}
		})
	}
}

func TestIsTerraformModule(t *testing.T) {
	if !IsTerraformModule("/hashicorp/vpc/aws/1.0.0/vpc-1.0.0.tar.gz") {
		t.Error("expected true for Layout A module path")
	}
	if !IsTerraformModule("/myorg/s3module/aws/1.0.0.zip") {
		t.Error("expected true for Layout B flat zip module path")
	}
	if IsTerraformModule("/hashicorp/aws/2.0.0/terraform-provider-aws_linux_amd64.zip") {
		t.Error("expected false for provider filename in 4-segment path")
	}
	if IsTerraformModule("/myorg/s3module/aws/module.json") {
		t.Error("expected false for module.json sidecar")
	}
}

func TestIsTerraformProvider(t *testing.T) {
	if !IsTerraformProvider("/hashicorp/aws/2.0.0/terraform-provider-aws_2.0.0_linux_amd64.zip") {
		t.Error("expected true for valid provider path")
	}
	if IsTerraformProvider("/hashicorp/vpc/aws/1.0.0/vpc-1.0.0.tar.gz") {
		t.Error("expected false for module path")
	}
}
