package pkgmgr

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseWrappedArgs(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantRegistry string
		wantNative   []string
	}{
		{
			name:         "no args",
			args:         []string{},
			wantRegistry: "",
			wantNative:   nil,
		},
		{
			name:         "registry flag with space",
			args:         []string{"install", "--registry", "my-reg", "--save"},
			wantRegistry: "my-reg",
			wantNative:   []string{"install", "--save"},
		},
		{
			name:         "registry flag with equals",
			args:         []string{"install", "--registry=my-reg", "--save"},
			wantRegistry: "my-reg",
			wantNative:   []string{"install", "--save"},
		},
		{
			name:         "verbose short flag",
			args:         []string{"-v", "install"},
			wantRegistry: "",
			wantNative:   []string{"install"},
		},
		{
			name:         "verbose long flag",
			args:         []string{"--verbose", "install"},
			wantRegistry: "",
			wantNative:   []string{"install"},
		},
		{
			name:         "all flags combined",
			args:         []string{"install", "--registry", "reg1", "-v", "lodash"},
			wantRegistry: "reg1",
			wantNative:   []string{"install", "lodash"},
		},
		{
			name:         "registry flag at end without value",
			args:         []string{"install", "--registry"},
			wantRegistry: "",
			wantNative:   []string{"install", "--registry"},
		},
		{
			name:         "only native args",
			args:         []string{"install", "express", "--save-dev"},
			wantRegistry: "",
			wantNative:   []string{"install", "express", "--save-dev"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseWrappedArgs(tt.args)
			assert.Equal(t, tt.wantRegistry, result.RegistryName)
			assert.Equal(t, tt.wantNative, result.NativeArgs)
		})
	}
}

func TestConstants(t *testing.T) {
	assert.Equal(t, "npm", PackageTypeNpm)
	assert.Equal(t, "maven", PackageTypeMaven)
	assert.Equal(t, "pypi", PackageTypePyPI)
	assert.Equal(t, "nuget", PackageTypeNuGet)

	assert.Equal(t, "npm", CommandNpm)
	assert.Equal(t, "mvn", CommandMvn)
	assert.Equal(t, "pip", CommandPip)
	assert.Equal(t, "dotnet", CommandDotnet)
}
