package util

import (
	"strings"

	"github.com/harness/harness-cli/module/ar/migrate/types"
)

// FilterPackagesBySelectors returns a slice containing only those packages whose
// Name appears in the filters. If filters is empty, all packages pass through
// unchanged (behavior-preserving passthrough). Package names are matched
// case-insensitively.
func FilterPackagesBySelectors(pkgs []types.Package, filters []types.PackageSelector) []types.Package {
	if len(filters) == 0 {
		return pkgs
	}

	// Build a set of lowercased package names from the selectors
	allowed := make(map[string]struct{}, len(filters))
	for _, filter := range filters {
		allowed[strings.ToLower(filter.Package)] = struct{}{}
	}

	// Filter packages to only those in the allowed set
	var filtered []types.Package
	for _, pkg := range pkgs {
		if _, ok := allowed[strings.ToLower(pkg.Name)]; ok {
			filtered = append(filtered, pkg)
		}
	}

	return filtered
}

// SelectorForPackage looks up a PackageSelector for the given package name in the
// mapping's PackageFilters. Returns (selector, hasFilters, matched) where:
//   - selector: the matching PackageSelector (zero value if not matched)
//   - hasFilters: true if the mapping has any PackageFilters configured
//   - matched: true if a selector for pkgName was found
//
// If the mapping has no filters, returns (zero, false, false) — a passthrough signal.
// If the mapping has filters but pkgName is not in the allow-list, returns
// (zero, true, false) — a block signal.
func SelectorForPackage(mapping *types.RegistryMapping, pkgName string) (types.PackageSelector, bool, bool) {
	if mapping == nil || len(mapping.PackageFilters) == 0 {
		return types.PackageSelector{}, false, false
	}

	// hasFilters = true: the mapping defines an allow-list. Package names are
	// matched case-insensitively.
	for _, sel := range mapping.PackageFilters {
		if strings.EqualFold(sel.Package, pkgName) {
			return sel, true, true
		}
	}

	// Package not in the allow-list
	return types.PackageSelector{}, true, false
}

// VersionSelectedBySelector reports whether the given version name is allowed by
// the selector. If the selector's Versions list is empty, all versions pass (returns
// true). Otherwise returns true iff versionName matches one of the entries
// (case-insensitive).
func VersionSelectedBySelector(sel types.PackageSelector, versionName string) bool {
	if len(sel.Versions) == 0 {
		return true
	}

	for _, v := range sel.Versions {
		if strings.EqualFold(v, versionName) {
			return true
		}
	}

	return false
}

// FileSelectedBySelector reports whether the given file name is allowed by the
// selector. If the selector's Files list is empty, all files pass (returns true).
// Otherwise returns true iff fileName matches one of the entries (case-insensitive).
// File names are lowercased before comparison to honor the migration engine's
// "file names are lowercased for matching" convention.
func FileSelectedBySelector(sel types.PackageSelector, fileName string) bool {
	if len(sel.Files) == 0 {
		return true
	}

	lowerFileName := strings.ToLower(fileName)
	for _, f := range sel.Files {
		if strings.ToLower(f) == lowerFileName {
			return true
		}
	}

	return false
}
