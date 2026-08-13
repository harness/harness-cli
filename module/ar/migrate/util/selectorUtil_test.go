package util

import (
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/types"

	"github.com/stretchr/testify/assert"
)

func TestFilterPackagesBySelectors(t *testing.T) {
	tests := []struct {
		name     string
		pkgs     []types.Package
		filters  []types.PackageSelector
		wantLen  int
		wantPkgs []string
	}{
		{
			name: "empty filters returns all packages unchanged",
			pkgs: []types.Package{
				{Name: "express"},
				{Name: "lodash"},
				{Name: "react"},
			},
			filters:  []types.PackageSelector{},
			wantLen:  3,
			wantPkgs: []string{"express", "lodash", "react"},
		},
		{
			name: "filters naming a subset returns only matching packages",
			pkgs: []types.Package{
				{Name: "express"},
				{Name: "lodash"},
				{Name: "react"},
			},
			filters: []types.PackageSelector{
				{Package: "express"},
				{Package: "react"},
			},
			wantLen:  2,
			wantPkgs: []string{"express", "react"},
		},
		{
			name: "case-insensitive: selector Foo matches both foo and Foo",
			pkgs: []types.Package{
				{Name: "foo"},
				{Name: "Foo"},
				{Name: "bar"},
			},
			filters: []types.PackageSelector{
				{Package: "Foo"},
			},
			wantLen:  2,
			wantPkgs: []string{"foo", "Foo"},
		},
		{
			name: "no matching packages returns empty slice",
			pkgs: []types.Package{
				{Name: "express"},
				{Name: "lodash"},
			},
			filters: []types.PackageSelector{
				{Package: "nonexistent"},
			},
			wantLen:  0,
			wantPkgs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterPackagesBySelectors(tt.pkgs, tt.filters)
			assert.Len(t, result, tt.wantLen)
			for i, pkgName := range tt.wantPkgs {
				assert.Equal(t, pkgName, result[i].Name)
			}
		})
	}
}

func TestSelectorForPackage(t *testing.T) {
	tests := []struct {
		name       string
		mapping    *types.RegistryMapping
		pkgName    string
		wantSel    types.PackageSelector
		hasFilters bool
		matched    bool
	}{
		{
			name:       "nil mapping returns hasFilters=false, matched=false",
			mapping:    nil,
			pkgName:    "express",
			wantSel:    types.PackageSelector{},
			hasFilters: false,
			matched:    false,
		},
		{
			name: "mapping with empty PackageFilters returns hasFilters=false",
			mapping: &types.RegistryMapping{
				PackageFilters: []types.PackageSelector{},
			},
			pkgName:    "express",
			wantSel:    types.PackageSelector{},
			hasFilters: false,
			matched:    false,
		},
		{
			name: "mapping with filters but no name match returns hasFilters=true, matched=false",
			mapping: &types.RegistryMapping{
				PackageFilters: []types.PackageSelector{
					{Package: "lodash"},
					{Package: "react"},
				},
			},
			pkgName:    "express",
			wantSel:    types.PackageSelector{},
			hasFilters: true,
			matched:    false,
		},
		{
			name: "mapping with matching selector returns hasFilters=true, matched=true, and the right selector",
			mapping: &types.RegistryMapping{
				PackageFilters: []types.PackageSelector{
					{Package: "lodash", Versions: []string{"4.17.0"}},
					{Package: "express", Versions: []string{"4.18.0", "4.19.0"}, Files: []string{"express.tgz"}},
					{Package: "react"},
				},
			},
			pkgName: "express",
			wantSel: types.PackageSelector{
				Package:  "express",
				Versions: []string{"4.18.0", "4.19.0"},
				Files:    []string{"express.tgz"},
			},
			hasFilters: true,
			matched:    true,
		},
		{
			name: "case-insensitive matching: Express matches express",
			mapping: &types.RegistryMapping{
				PackageFilters: []types.PackageSelector{
					{Package: "express"},
				},
			},
			pkgName:    "Express",
			wantSel:    types.PackageSelector{Package: "express"},
			hasFilters: true,
			matched:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sel, hasFilters, matched := SelectorForPackage(tt.mapping, tt.pkgName)
			assert.Equal(t, tt.hasFilters, hasFilters)
			assert.Equal(t, tt.matched, matched)
			assert.Equal(t, tt.wantSel, sel)
		})
	}
}

func TestVersionSelectedBySelector(t *testing.T) {
	tests := []struct {
		name        string
		selector    types.PackageSelector
		versionName string
		want        bool
	}{
		{
			name: "empty Versions returns true for any version",
			selector: types.PackageSelector{
				Package:  "express",
				Versions: []string{},
			},
			versionName: "4.18.0",
			want:        true,
		},
		{
			name: "non-empty Versions returns true for listed version",
			selector: types.PackageSelector{
				Package:  "express",
				Versions: []string{"4.18.0", "4.19.0", "5.0.0"},
			},
			versionName: "4.19.0",
			want:        true,
		},
		{
			name: "non-empty Versions returns false for unlisted version",
			selector: types.PackageSelector{
				Package:  "express",
				Versions: []string{"4.18.0", "4.19.0"},
			},
			versionName: "5.0.0",
			want:        false,
		},
		{
			name: "non-empty Versions returns false for unlisted version",
			selector: types.PackageSelector{
				Package:  "express",
				Versions: []string{"4.18.0"},
			},
			versionName: "4.18.0-SNAPSHOT",
			want:        false,
		},
		{
			name: "version matching is case-insensitive",
			selector: types.PackageSelector{
				Package:  "mypkg",
				Versions: []string{"1.0.0-RC1"},
			},
			versionName: "1.0.0-rc1",
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VersionSelectedBySelector(tt.selector, tt.versionName)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFileSelectedBySelector(t *testing.T) {
	tests := []struct {
		name     string
		selector types.PackageSelector
		fileName string
		want     bool
	}{
		{
			name: "empty Files returns true for any file",
			selector: types.PackageSelector{
				Package: "express",
				Files:   []string{},
			},
			fileName: "express-4.18.0.tgz",
			want:     true,
		},
		{
			name: "non-empty Files returns true for listed file",
			selector: types.PackageSelector{
				Package: "express",
				Files:   []string{"express.tgz", "express.min.js"},
			},
			fileName: "express.tgz",
			want:     true,
		},
		{
			name: "case-insensitive match: selector File.TXT matches file.txt",
			selector: types.PackageSelector{
				Package: "mypackage",
				Files:   []string{"File.TXT", "Other.ZIP"},
			},
			fileName: "file.txt",
			want:     true,
		},
		{
			name: "case-insensitive match: selector file.txt matches FILE.TXT",
			selector: types.PackageSelector{
				Package: "mypackage",
				Files:   []string{"file.txt"},
			},
			fileName: "FILE.TXT",
			want:     true,
		},
		{
			name: "non-empty Files returns false for unlisted file",
			selector: types.PackageSelector{
				Package: "express",
				Files:   []string{"express.tgz"},
			},
			fileName: "express.min.js",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FileSelectedBySelector(tt.selector, tt.fileName)
			assert.Equal(t, tt.want, got)
		})
	}
}
