package util

import (
	"path"
	"regexp"
	"strings"
)

// JFrog Terraform module storage layouts (two variants observed in the wild):
//   Layout A (goreleaser): <repo>/<ns>/<name>/<provider>/<ver>/<name>-<ver>.tar.gz  (5+ parts)
//   Layout B (flat):       <repo>/<ns>/<name>/<provider>/<ver>.zip                  (4 parts, version = filename stem)
//
// JFrog Terraform provider storage layout:
//   <repo>/<namespace>/<type>/<version>/terraform-provider-<type>_<version>_<os>_<arch>.zip

// terraformProviderFilenameRegex matches the standard convention:
//   terraform-provider-{type}_{version}_{os}_{arch}.zip
var terraformProviderFilenameRegex = regexp.MustCompile(
	`^terraform-provider-([a-zA-Z0-9-]+)_(\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?)_([a-z0-9]+)_([a-z0-9]+)\.zip$`,
)

// ParseTerraformModulePath parses a JFrog module file URI and returns
// (namespace, name, provider, version, ok).
//
// Layout A (goreleaser): /<ns>/<name>/<provider>/<ver>/<name>-<ver>.tar.gz  (5+ segments)
// Layout B (flat zip):   /<ns>/<name>/<provider>/<ver>.zip                  (4 segments, version = filename stem)
func ParseTerraformModulePath(filePath string) (ns, name, provider, version string, ok bool) {
	filePath = strings.TrimPrefix(filePath, "/")
	parts := strings.Split(filePath, "/")

	// Layout A: ns / name / provider / ver / <filename>.tar.gz|.tgz
	if len(parts) >= 5 {
		filename := parts[len(parts)-1]
		lower := strings.ToLower(filename)
		if strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
			ns = parts[0]
			name = parts[1]
			provider = parts[2]
			version = parts[3]
			ok = ns != "" && name != "" && provider != "" && version != ""
			return
		}
	}

	// Layout B: ns / name / provider / <ver>.zip  (flat, version is the filename stem)
	// The stem must start with a digit (semver) to avoid matching provider filenames
	// like "terraform-provider-aws_2.0.0_linux_amd64.zip" which share the 4-part shape.
	if len(parts) == 4 {
		filename := parts[3]
		lower := strings.ToLower(filename)
		if strings.HasSuffix(lower, ".zip") {
			stem := filename[:len(filename)-4]
			if stem != "" && stem[0] >= '0' && stem[0] <= '9' {
				ns = parts[0]
				name = parts[1]
				provider = parts[2]
				version = stem
				ok = ns != "" && name != "" && provider != "" && version != ""
				return
			}
		}
	}

	return
}

// ParseTerraformProviderPath parses a JFrog provider file URI and returns
// (namespace, typeName, version, filename, os, arch, ok).
// Expected form: /<ns>/<type>/<ver>/terraform-provider-<type>_<ver>_<os>_<arch>.zip
func ParseTerraformProviderPath(filePath string) (ns, typeName, version, filename, osName, arch string, ok bool) {
	filePath = strings.TrimPrefix(filePath, "/")
	parts := strings.Split(filePath, "/")
	// Need at least: ns / type / ver / filename
	if len(parts) < 4 {
		return
	}
	filename = path.Base(filePath)
	if !strings.HasSuffix(strings.ToLower(filename), ".zip") {
		return
	}
	ns = parts[0]
	typeName = parts[1]
	version = parts[2]
	m := terraformProviderFilenameRegex.FindStringSubmatch(filename)
	if m == nil {
		return
	}
	// m[1]=type, m[2]=version, m[3]=os, m[4]=arch
	osName = m[3]
	arch = m[4]
	ok = ns != "" && typeName != "" && version != ""
	return
}

// IsTerraformModule reports whether a file path looks like a Terraform module.
func IsTerraformModule(filePath string) bool {
	_, _, _, _, ok := ParseTerraformModulePath(filePath)
	return ok
}

// IsTerraformProvider reports whether a file path looks like a Terraform provider.
func IsTerraformProvider(filePath string) bool {
	_, _, _, _, _, _, ok := ParseTerraformProviderPath(filePath)
	return ok
}
