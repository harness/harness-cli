package npm

import "time"

// nolint:tagliatelle
type PackageMetadata struct {
	ID             string                             `json:"_id"`
	Rev            string                             `json:"_rev,omitempty"`
	Name           string                             `json:"name"`
	Description    interface{}                        `json:"description"`
	DistTags       map[string]string                  `json:"dist-tags,omitempty"`
	Versions       map[string]*PackageMetadataVersion `json:"versions"`
	Readme         string                             `json:"readme,omitempty"`
	Maintainers    []User                             `json:"maintainers,omitempty"`
	Contributors   interface{}                        `json:"contributors,omitempty"`
	Time           map[string]time.Time               `json:"time,omitempty"`
	Homepage       interface{}                        `json:"homepage,omitempty"`
	Keywords       []string                           `json:"keywords,omitempty"`
	Repository     interface{}                        `json:"repository,omitempty"`
	Author         interface{}                        `json:"author"`
	Bugs           interface{}                        `json:"bugs,omitempty"`
	ReadmeFilename string                             `json:"readmeFilename,omitempty"`
	Users          map[string]bool                    `json:"users,omitempty"`
	License        interface{}                        `json:"license,omitempty"`
}

// PackageMetadataVersion documentation:
// https://github.com/npm/registry/blob/master/docs/REGISTRY-API.md#version
// PackageMetadataVersion response:
// https://github.com/npm/registry/blob/master/docs/responses/package-metadata.md#abbreviated-version-object
// nolint:tagliatelle
type PackageMetadataVersion struct {
	ID                   string              `json:"_id"`
	Name                 string              `json:"name"`
	Version              string              `json:"version"`
	Description          interface{}         `json:"description"`
	Author               interface{}         `json:"author"`
	Homepage             interface{}         `json:"homepage,omitempty"`
	License              interface{}         `json:"license,omitempty"`
	Repository           interface{}         `json:"repository,omitempty"`
	Keywords             interface{}         `json:"keywords,omitempty"`
	Dependencies         interface{}         `json:"dependencies,omitempty"`
	BundleDependencies   interface{}         `json:"bundleDependencies,omitempty"`
	DevDependencies      interface{}         `json:"devDependencies,omitempty"`
	PeerDependencies     interface{}         `json:"peerDependencies,omitempty"`
	PeerDependenciesMeta interface{}         `json:"peerDependenciesMeta,omitempty"`
	Bin                  interface{}         `json:"bin,omitempty"`
	OptionalDependencies interface{}         `json:"optionalDependencies,omitempty"`
	AcceptDependencies   interface{}         `json:"acceptDependencies,omitempty"`
	Readme               string              `json:"readme,omitempty"`
	Dist                 PackageDistribution `json:"dist"`
	Maintainers          interface{}         `json:"maintainers,omitempty"`
	Contributors         interface{}         `json:"contributors,omitempty"`
	Bugs                 interface{}         `json:"bugs,omitempty"`
	Engines              interface{}         `json:"engines,omitempty"`
	Deprecated           interface{}         `json:"deprecated,omitempty"`
	Directories          interface{}         `json:"directories,omitempty"`
	Funding              interface{}         `json:"funding,omitempty"`
	CPU                  interface{}         `json:"cpu,omitempty"`
	OS                   interface{}         `json:"os,omitempty"`
	Main                 interface{}         `json:"main,omitempty"`
	Module               interface{}         `json:"module,omitempty"`
	Types                interface{}         `json:"types,omitempty"`
	Typings              interface{}         `json:"typings,omitempty"`
	Exports              interface{}         `json:"exports,omitempty"`
	Imports              interface{}         `json:"imports,omitempty"`
	Files                interface{}         `json:"files,omitempty"`
	Workspaces           interface{}         `json:"workspaces,omitempty"`
	Scripts              interface{}         `json:"scripts,omitempty"`
	Config               interface{}         `json:"config,omitempty"`
	PublishConfig        interface{}         `json:"publishConfig,omitempty"`
	SideEffects          interface{}         `json:"sideEffects,omitempty"`
	HasShrinkwrap        interface{}         `json:"_hasShrinkwrap,omitempty"`
	HasInstallScript     interface{}         `json:"hasInstallScript,omitempty"`
	NodeVersion          interface{}         `json:"_nodeVersion,omitempty"`
	NpmUser              interface{}         `json:"_npmUser,omitempty"`
	NpmVersion           interface{}         `json:"_npmVersion,omitempty"`
}

// Repository https://github.com/npm/registry/blob/master/docs/REGISTRY-API.md#version
// nolint:tagliatelle
type Repository struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// PackageDistribution https://github.com/npm/registry/blob/master/docs/REGISTRY-API.md#version
// nolint:tagliatelle
type PackageDistribution struct {
	Integrity    string `json:"integrity"`
	Shasum       string `json:"shasum"`
	Tarball      string `json:"tarball"`
	FileCount    int    `json:"fileCount,omitempty"`
	UnpackedSize int    `json:"unpackedSize,omitempty"`
	NpmSignature string `json:"npm-signature,omitempty"`
}

type User struct {
	Username string `json:"username,omitempty"`
	Name     string `json:"name"`
	Email    string `json:"email,omitempty"`
	URL      string `json:"url,omitempty"`
}

type PackageAttachment struct {
	ContentType string `json:"content_type"`
	Data        string `json:"data"`
	Length      int    `json:"length"`
}

// nolint:tagliatelle
type PackageUpload struct {
	PackageMetadata
	Attachments map[string]*PackageAttachment `json:"_attachments"`
}
