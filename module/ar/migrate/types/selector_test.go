package types

import "testing"

func TestSupportedSelectorGranularity(t *testing.T) {
	tests := []struct {
		artifactType ArtifactType
		expected     SelectorGranularity
	}{
		// GranularityVersion
		{GO, GranularityVersion},
		// GranularityFile
		{GENERIC, GranularityFile},
		{RAW, GranularityFile},
		{MAVEN, GranularityFile},
		{PYTHON, GranularityFile},
		{NUGET, GranularityFile},
		{NPM, GranularityFile},
		{DART, GranularityFile},
		{PUPPET, GranularityFile},
		{RUBY, GranularityFile},
		// GranularityPackage
		{DOCKER, GranularityPackage},
		{HELM, GranularityPackage},
		{HELM_LEGACY, GranularityPackage},
		{HELM_HTTP, GranularityPackage},
		{RPM, GranularityPackage},
		{DEBIAN, GranularityPackage},
		{CONDA, GranularityPackage},
		{COMPOSER, GranularityPackage},
		{SWIFT, GranularityPackage},
		{CONAN, GranularityPackage},
	}

	for _, tt := range tests {
		t.Run(string(tt.artifactType), func(t *testing.T) {
			got := SupportedSelectorGranularity(tt.artifactType)
			if got != tt.expected {
				t.Errorf("SupportedSelectorGranularity(%s) = %v, want %v", tt.artifactType, got, tt.expected)
			}
		})
	}
}

func TestValidatePackageFilters(t *testing.T) {
	tests := []struct {
		name         string
		filters      []PackageSelector
		artifactType ArtifactType
		wantErr      bool
	}{
		{
			name:         "empty filters returns no error",
			filters:      nil,
			artifactType: GENERIC,
			wantErr:      false,
		},
		{
			name: "file-level type (GENERIC) with Files set returns no error",
			filters: []PackageSelector{
				{Package: "foo", Files: []string{"a.txt"}},
			},
			artifactType: GENERIC,
			wantErr:      false,
		},
		{
			name: "file-level type (GENERIC) with Versions set returns no error",
			filters: []PackageSelector{
				{Package: "foo", Versions: []string{"1.0.0"}},
			},
			artifactType: GENERIC,
			wantErr:      false,
		},
		{
			name: "GO with Versions set returns no error",
			filters: []PackageSelector{
				{Package: "github.com/foo/bar", Versions: []string{"v1.0.0"}},
			},
			artifactType: GO,
			wantErr:      false,
		},
		{
			name: "GO with Files set returns error",
			filters: []PackageSelector{
				{Package: "github.com/foo/bar", Files: []string{"foo.go"}},
			},
			artifactType: GO,
			wantErr:      true,
		},
		{
			name: "package-level type (RPM) with Versions set returns error",
			filters: []PackageSelector{
				{Package: "foo", Versions: []string{"1.0.0"}},
			},
			artifactType: RPM,
			wantErr:      true,
		},
		{
			name: "package-level type (DOCKER) with Files set returns error",
			filters: []PackageSelector{
				{Package: "myimage", Files: []string{"layer.tar"}},
			},
			artifactType: DOCKER,
			wantErr:      true,
		},
		{
			name: "empty Package name returns error",
			filters: []PackageSelector{
				{Package: "", Versions: []string{"1.0.0"}},
			},
			artifactType: GENERIC,
			wantErr:      true,
		},
		{
			name: "multiple filters with one invalid returns error",
			filters: []PackageSelector{
				{Package: "valid", Files: []string{"a.txt"}},
				{Package: "", Files: []string{"b.txt"}},
			},
			artifactType: GENERIC,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePackageFilters(tt.filters, tt.artifactType)
			if tt.wantErr && err == nil {
				t.Errorf("ValidatePackageFilters() expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidatePackageFilters() unexpected error: %v", err)
			}
		})
	}
}

func TestValidateConfig_PackageFilters(t *testing.T) {
	t.Run("GENERIC with Files filter passes validation", func(t *testing.T) {
		config := baseValidConfig()
		config.Mappings[0].ArtifactType = GENERIC
		config.Mappings[0].PackageFilters = []PackageSelector{
			{Package: "foo", Files: []string{"a.txt"}},
		}

		err := validateConfig(config)
		if err != nil {
			t.Errorf("validateConfig() expected nil error for GENERIC with Files, got: %v", err)
		}
	})

	t.Run("RPM with Files filter fails validation", func(t *testing.T) {
		config := baseValidConfig()
		config.Mappings[0].ArtifactType = RPM
		config.Mappings[0].PackageFilters = []PackageSelector{
			{Package: "foo", Files: []string{"a.txt"}},
		}

		err := validateConfig(config)
		if err == nil {
			t.Error("validateConfig() expected error for RPM with Files, got nil")
		}
	})

	t.Run("empty Package name fails validation", func(t *testing.T) {
		config := baseValidConfig()
		config.Mappings[0].ArtifactType = GENERIC
		config.Mappings[0].PackageFilters = []PackageSelector{
			{Package: "", Files: []string{"a.txt"}},
		}

		err := validateConfig(config)
		if err == nil {
			t.Error("validateConfig() expected error for empty Package name, got nil")
		}
	})

	t.Run("PYTHON with Versions filter passes validation", func(t *testing.T) {
		config := baseValidConfig()
		config.Mappings[0].ArtifactType = PYTHON
		config.Mappings[0].PackageFilters = []PackageSelector{
			{Package: "django", Versions: []string{"4.0.0"}},
		}

		err := validateConfig(config)
		if err != nil {
			t.Errorf("validateConfig() expected nil error for PYTHON with Versions, got: %v", err)
		}
	})

	t.Run("GO with Files filter fails validation", func(t *testing.T) {
		config := baseValidConfig()
		config.Mappings[0].ArtifactType = GO
		config.Mappings[0].PackageFilters = []PackageSelector{
			{Package: "github.com/foo/bar", Files: []string{"main.go"}},
		}

		err := validateConfig(config)
		if err == nil {
			t.Error("validateConfig() expected error for GO with Files, got nil")
		}
	})
}
