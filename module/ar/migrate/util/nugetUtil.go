package util

import (
	"path"
	"strings"
)

/*
This will parse Nuget file with filePath  like /package.version.nupkg or file name package.version.nupkg
and return package , version and success , traversing from right side fo file name
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

	// Traverse from right to find 3rd dot
	dotCount := 0
	splitIdx := -1

	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '.' {
			dotCount++
			if dotCount == 3 {
				splitIdx = i
				break
			}
		}
	}

	// less than 3 dots implies invalid format,
	if splitIdx == -1 {
		return "", "", false
	}

	packageName := name[:splitIdx]
	version := name[splitIdx+1:]

	// Normalize package name to lowercase
	packageName = strings.ToLower(packageName)

	if packageName == "" || version == "" {
		return "", "", false
	}

	return packageName, version, true
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
