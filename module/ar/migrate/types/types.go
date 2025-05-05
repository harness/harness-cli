package types

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

type File struct {
	Name         string
	Registry     string
	Uri          string
	Folder       bool
	Size         int
	LastModified string
	SHA1         string
	SHA2         string
}

type TreeNode struct {
	Name     string
	Key      string
	Children []TreeNode
	IsLeaf   bool
	File     *File
}

type Package struct {
	Registry string
	Path     string
	Name     string
	Size     int
}

type Version struct {
	Registry string
	Name     string
	Path     string
	Pkg      string
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
