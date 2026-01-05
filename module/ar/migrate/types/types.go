package types

import (
	"errors"
	"io"
	"net/http"
	"time"
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
	URL      string
	Version  string
}

type Version struct {
	Registry string
	Pkg      string
	Name     string
	Path     string
	Size     int
}

type MetadataItem struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Artifact struct {
	Name       string
	Version    string
	Type       string
	Registry   string
	Size       int64
	Properties map[string]string
}

type Status string

const (
	StatusSuccess Status = "Success"
	StatusSkip    Status = "Skipped"
	StatusFail    Status = "Failed"
)

type FileStat struct {
	Name     string
	Registry string
	Uri      string
	Status   Status
	Size     int64
	Error    string
}

type TransferStats struct {
	FileStats []FileStat
}

type RegistryInfo struct {
	Type string
	URL  string
	Path string
}

const (
	ChartLayerMediaType = "application/vnd.cncf.helm.chart.layer.v1.tar+gzip"
	ConfigMediaType     = "application/vnd.cncf.helm.config.v1+json"
)

type HelmOCIConfig struct {
	APIVersion  string            `json:"apiVersion"`
	Created     time.Time         `json:"created"`
	Annotations map[string]string `json:"annotations"`
}

type PackageFiles struct {
	File         *File
	DownloadFile io.ReadCloser
	Header       *http.Header
}
