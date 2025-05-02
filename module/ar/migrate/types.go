package migrate

import (
	"errors"
)

// Common errors
var (
	ErrUnsupportedRegistryType = errors.New("unsupported ar type")
	ErrArtifactNotFound        = errors.New("artifact not found")
	ErrRegistryNotFound        = errors.New("ar not found")
	ErrInvalidCredentials      = errors.New("invalid credentials")
)

// Artifact represents a single artifact in ar
type Package struct {
	Registry string
	Path     string
	Name     string
	Size     int
}

type Artifact struct {
	Name       string
	Version    string
	Type       string
	Registry   string
	Size       int64
	Properties map[string]string
}
