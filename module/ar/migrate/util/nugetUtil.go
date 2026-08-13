package util

import (
	"path"
	"strings"
)

/*
This will parse a Nuget file with a filePath like /package.version.nupkg or a
file name package.version.nupkg and return package, version and success.

A nupkg file name is "<id>.<version>". Both the id and the version may contain
dots — the version via prerelease/build metadata (e.g. 3.203.0-pr-280.a52f7f9.1)
— so a fixed dot count cannot separate them. A NuGet version always begins with
a purely-numeric segment (the SemVer major), so we split the id off at the first
all-digit segment. Requiring the segment to be *all* digits (not merely
digit-led) avoids mistaking a numeric-led name segment such as "7zip" for the
start of the version.
*/
func ParseNugetFileNameWithPath(filePath string) (string, string, bool) {
	fileName := path.Base(filePath)

	// Validate extension
	if !strings.HasSuffix(fileName, ".nupkg") &&
		!strings.HasSuffix(fileName, ".snupkg") &&
		!strings.HasSuffix(fileName, ".nuspec") {
		return "", "", false
	}

	// Remove extensions
	name := strings.TrimSuffix(fileName, ".nupkg")
	name = strings.TrimSuffix(name, ".snupkg")
	name = strings.TrimSuffix(name, ".nuspec")

	// Find the first all-digit segment (the SemVer major); everything before it
	// is the package id, everything from it onward is the version. Start at
	// index 1 so the package id is never empty.
	parts := strings.Split(name, ".")
	versionStart := -1
	for i := 1; i < len(parts); i++ {
		if isAllDigits(parts[i]) {
			versionStart = i
			break
		}
	}

	if versionStart == -1 {
		return "", "", false
	}

	packageName := strings.Join(parts[:versionStart], ".")
	version := strings.Join(parts[versionStart:], ".")

	// Normalize package name to lowercase
	packageName = strings.ToLower(packageName)

	if packageName == "" || version == "" {
		return "", "", false
	}

	return packageName, version, true
}

// isAllDigits reports whether s is non-empty and consists solely of ASCII digits.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

/*
This will parse Nuget file with filePath  like /package.version.nupkg or file name package.version.nupkg
and return package , version and success . this isplit logic using dot
*/
func ParseNugetFileNameWithPath_old(filePath string) (string, string, bool) {
	fileName := path.Base(filePath)

	if !strings.HasSuffix(fileName, ".nupkg") && !strings.HasSuffix(fileName, ".snupkg") {
		return "", "", false
	}

	name := strings.TrimSuffix(fileName, ".nupkg")
	name = strings.TrimSuffix(name, ".snupkg")

	parts := strings.Split(name, ".")
	if len(parts) < 2 {
		return "", "", false
	}

	versionStartIdx := -1
	for i, part := range parts {
		if len(part) > 0 && part[0] >= '0' && part[0] <= '9' {
			versionStartIdx = i
			break
		}
	}

	if versionStartIdx <= 0 {
		return "", "", false
	}

	packageName := strings.Join(parts[:versionStartIdx], ".")
	version := strings.Join(parts[versionStartIdx:], ".")

	return packageName, version, true
}
