package gopkg

import (
	"testing"
)

func TestValidateVersion(t *testing.T) {
	v := &defaultModuleValidator{}

	valid := []string{
		"v1.0.0",
		"v2.1.3",
		"v3.22.0",
		"v3.24.0-rc1",
		"v3.24.0-beta.1",
		"v3.24.0-alpha",
		"v1.0.0-rc.2",
		"v0.1.0-pre1",
	}

	invalid := []string{
		"1.0.0",
		"v1.0",
		"v1",
		"v1.0.0.0",
		"v1.0.0-",
		"latest",
		"",
	}

	for _, ver := range valid {
		if err := v.ValidateVersion(ver); err != nil {
			t.Errorf("expected %q to be valid, got error: %v", ver, err)
		}
	}

	for _, ver := range invalid {
		if err := v.ValidateVersion(ver); err == nil {
			t.Errorf("expected %q to be invalid, but got no error", ver)
		}
	}
}
