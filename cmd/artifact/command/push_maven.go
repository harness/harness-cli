package command

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/util"
	"github.com/harness/harness-cli/util/common/auth"
	"github.com/harness/harness-cli/util/common/errors"
	p "github.com/harness/harness-cli/util/common/progress"
	"github.com/spf13/cobra"
)

const (
	warFileExtension = ".war"
	jarFileExtension = ".jar"
	xmlFileExtension = ".xml"
	pomFileExtension = ".pom"
)

func NewPushMavenCmd(c *cmdutils.Factory) *cobra.Command {
	var pkgURL string
	var pomPath string
	const expectedNumberOfArgument = 2
	cmd := &cobra.Command{
		Use:   "maven <registry_name> <file_path>",
		Short: "Push Maven Artifacts",
		Long:  "Push Maven Artifacts to Harness Artifact Registry",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != expectedNumberOfArgument {
				return fmt.Errorf(
					"Error: Invalid number of argument,  accepts %d arg(s), received %d  \nUsage :\n %s",
					expectedNumberOfArgument, len(args), cmd.UseLine(),
				)
			}
			return nil
		},
		PreRun: func(cmd *cobra.Command, args []string) {
			if pkgURL != "" {
				config.Global.Registry.PkgURL = util.GetPkgUrl(pkgURL)
			} else {
				config.Global.Registry.PkgURL = util.GetPkgUrl(config.Global.APIBaseURL)
			}

		},
		RunE: func(cmd *cobra.Command, args []string) error {
			registryName := args[0]
			pkgFilePath := args[1]

			// Create progress reporter
			progress := p.NewConsoleReporter()
			var mavenFilesToUpload []string

			packageFileName := filepath.Base(pkgFilePath)

			// Validate file exists
			fileInfo, err := os.Stat(pkgFilePath)
			if err != nil {
				return errors.NewValidationError("file_path", fmt.Sprintf("failed to access package file: %v", err))
			}
			if fileInfo.IsDir() {
				return errors.NewValidationError("file_path", "package file path must be a file, not a directory")
			}

			// validate file name
			valid, err := isValidMavenPackageFile(packageFileName)
			if !valid {
				progress.Error("Invalid file name")
				return errors.NewValidationError("file_path", fmt.Sprintf("failed to validate package file name: %v", err))
			}

			pomFileName := filepath.Base(pomPath)

			fileInfo, err = os.Stat(pomPath)
			if err != nil {
				return errors.NewValidationError("file_path", fmt.Sprintf("failed to access POM file: %v", err))
			}
			if fileInfo.IsDir() {
				return errors.NewValidationError("file_path", "POM file path must be a file, not a directory")
			}

			// validate file name
			valid, err = isValidPomFile(pomFileName)
			if !valid {
				progress.Error("Invalid file name")
				return errors.NewValidationError("file_path", fmt.Sprintf("failed to validate POM file name: %v", err))
			}

			//reading  project details from pom file
			coordsFromPom, err := parseMavenProjectLevelPom(pomPath)

			if err != nil {
				return errors.NewValidationError("XML_ERROR", fmt.Sprintf("failed to parse POM file: %v", err))
			}

			//reading  project detail from package war/jar file
			coordsFromPackage, err := parseMavenArtifact(pkgFilePath)

			if err != nil {
				return errors.NewValidationError("PACKAGE_ERROR", fmt.Sprintf("failed to parse provided package file: %v", err))
			}

			//verify that package and pom is of same project and versionß
			if err := compareMavenCoordinates(coordsFromPom, coordsFromPackage); err != nil {
				return errors.NewValidationError("ERROR", fmt.Sprintf("failed to match package and POM parameters: %v", err))

			}
			progress.Success(fmt.Sprintf("Maven coordinates validated successfully:"))

			// Check for SNAPSHOT version , currently not supported
			if err := validateSnapshotVersion(coordsFromPom.Version); err != nil {
				return errors.NewValidationError("SNAPSHOT_ERROR", fmt.Sprintf("Failed in validating version : %v", err))
			}
			progress.Success("Input parameters validated")

			//Adding pom and package file
			mavenFilesToUpload = append(mavenFilesToUpload, pkgFilePath)
			mavenFilesToUpload = append(mavenFilesToUpload, pomPath)

			if len(mavenFilesToUpload) == 0 {
				return errors.NewValidationError("Empty Folder", fmt.Sprintf("No Maven files found to process :"))
			}

			//Generating all  checksum artifacts in memory
			checksumFiles, err := getAllChecksumFileToUpload(mavenFilesToUpload, coordsFromPom)
			if err != nil {
				return errors.NewValidationError("CHECKSUM_ERROR", fmt.Sprintf("Failed to generate CHECKSUM files: %v", err))
			}

			progress.Success("checksum file is generated")

			// Initialize the package client
			pkgClient, err := pkgclient.NewClientWithResponses(config.Global.Registry.PkgURL,
				auth.GetAuthOptionARPKG())
			if err != nil {
				return fmt.Errorf("failed to create package client: %w", err)
			}

			for _, fileNameWithPath := range mavenFilesToUpload {
				progress.Step(fmt.Sprintf("Uploading %s ", filepath.Base(fileNameWithPath)))
				err := uploadSingleMavenPackageFile(pkgClient, fileNameWithPath, registryName, progress, coordsFromPom)
				if err != nil {
					return errors.NewValidationError("UPLOAD_ERROR", fmt.Sprintf("Failed in uploading : %v", err))
				}
			}
			progress.Success(fmt.Sprintf("Successfully uploaded package and pom"))

			//Uploading  checksum artifacts
			for _, checksumFile := range checksumFiles {
				err := uploadInMemoryMavenFile(pkgClient, checksumFile, registryName, progress, coordsFromPom)
				if err != nil {
					return errors.NewValidationError("UPLOAD_ERROR", fmt.Sprintf("Failed in uploading : %v", err))
				}
			}

			progress.Success(fmt.Sprintf("Successfully uploaded checksum files"))

			progress.Step("Downloading maven-metadata.xml")
			//download maven-metadata.xml
			mavenMetadataXML, isCreated, err := getMavenMetadataXml(pkgClient, registryName, coordsFromPom)

			if err != nil {
				return errors.NewValidationError("DOWNLOAD_ERROR", fmt.Sprintf("Failed in downlaoding  : %v", err))
			}

			if isCreated {
				progress.Success(fmt.Sprintf("maven-metadata.xml not found, created new metadata"))
			} else {
				progress.Success(fmt.Sprintf("maven-metadata.xml fetched successfully"))
			}

			progress.Step("Updating maven-metadata.xml")
			mavenMetadataXML.addNewVersion(coordsFromPom.Version)
			//uploading maven-metadata.xml

			err = uploadMavenMetadataXML(pkgClient, registryName, progress, coordsFromPom, mavenMetadataXML)
			if err != nil {
				return errors.NewValidationError("UPLOAD_ERROR", fmt.Sprintf("Failed in uploading : %v", err))
			}

			progress.Success("maven-metadata.xml uploaded successfully")
			progress.Success(fmt.Sprintf("Successfully uploaded package"))

			return nil

		},
	}

	cmd.Flags().StringVar(&pomPath, "pom-file", "", "pom file path")
	cmd.Flags().StringVar(&pkgURL, "pkg-url", "", "Base URL for the Packages")

	cmd.MarkFlagRequired("pom-file")
	cmd.MarkFlagRequired("pkg-url")

	return cmd
}

func uploadSingleMavenPackageFile(pkgClient *pkgclient.ClientWithResponses, fileNameWithPath string, registryName string, progress *p.ConsoleReporter, coords *mavenPackageMetadata) error {

	file, err := os.Open(fileNameWithPath)
	if err != nil {
		progress.Error("Failed to open package file")
		return err
	}
	defer file.Close()

	fileInfo, err := os.Stat(fileNameWithPath)
	if err != nil {
		return errors.NewValidationError("file_path", fmt.Sprintf("failed to access package file: %v", err))
	}
	if fileInfo.IsDir() {
		return errors.NewValidationError("file_path", "package file path must be a file, not a directory")
	}

	currentFileName := filepath.Base(fileNameWithPath)
	currentFileExt := strings.ToLower(filepath.Ext(fileNameWithPath))

	if strings.HasSuffix(fileNameWithPath, "pom.xml") {
		currentFileName = normalizePomFilename(coords) + ".pom"
	}

	// Initialize progress reader
	bufferSize := int64(fileInfo.Size())
	reader, closer := p.Reader(bufferSize, file, currentFileExt)
	defer closer()

	resp, err := pkgClient.UploadMavenPackageWithBodyWithResponse(
		context.Background(),
		config.Global.AccountID,
		registryName,
		coords.GroupID,
		coords.ArtifactID,
		coords.Version,
		currentFileName,
		"application/octet-stream",
		reader,
	)

	if err != nil {
		progress.Error("Failed to upload package")
		return err
	}
	// Check response
	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusCreated {
		progress.Error("Upload failed")
		return fmt.Errorf("failed to push package: %s \n response: %s", resp.Status(), resp.Body)
	}

	return nil
}

func isValidMavenPackageFile(fileName string) (bool, error) {
	if fileName == "" {
		return false, fmt.Errorf("empty filename")
	}

	name := fileName
	if strings.HasSuffix(name, warFileExtension) || strings.HasSuffix(name, jarFileExtension) {
		return true, nil
	}
	//in case of file is having other  extension than provided extension
	return false, fmt.Errorf("unsupported extension: %s", filepath.Ext(name))
}

func isValidPomFile(fileName string) (bool, error) {
	if fileName == "" {
		return false, fmt.Errorf("empty filename")
	}

	name := fileName
	if strings.HasSuffix(name, xmlFileExtension) || strings.HasSuffix(name, pomFileExtension) {
		return true, nil
	}
	//in case of file is having other  extension than provided extension
	return false, fmt.Errorf("unsupported extension: %s", filepath.Ext(name))
}

// ParseMavenPom parses a .pom or .xml file and returns  groupId, artifactId, version, name
func parseMavenProjectLevelPom(filePath string) (*mavenPackageMetadata, error) {
	if strings.TrimSpace(filePath) == "" {
		return nil, fmt.Errorf("file path cannot be empty")
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	if ext != pomFileExtension && ext != xmlFileExtension {
		return nil, fmt.Errorf(
			"unsupported file type %q, only %s or %s allowed",
			ext, pomFileExtension, xmlFileExtension,
		)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return parsePomXMLData(data)
}

func parseMavenArtifact(filePath string) (*mavenPackageMetadata, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext != jarFileExtension && ext != warFileExtension {
		return nil, fmt.Errorf("unsupported file type %q, only .jar or .war allowed", ext)
	}

	r, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open archive: %w", err)
	}
	defer r.Close()

	// reading  pom.properties
	if coords, err := readPomPropertiesFromPackageFile(r.File); err == nil {
		return coords, nil
	}

	// if above file is not available then read  pom.xml
	if coords, err := readPomXMLFromPackageFile(r.File); err == nil {
		return coords, nil
	}

	return nil, fmt.Errorf("maven metadata not found in provided  %q package ", ext)
}

func readPomPropertiesFromPackageFile(files []*zip.File) (*mavenPackageMetadata, error) {
	for _, f := range files {
		if strings.HasSuffix(f.Name, "pom.properties") &&
			strings.Contains(f.Name, "META-INF/maven/") {

			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()

			propertiesMap := make(map[string]string)
			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, err
			}

			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				kv := strings.SplitN(line, "=", 2)
				if len(kv) == 2 {
					propertiesMap[kv[0]] = kv[1]
				}
			}

			groupID := propertiesMap["groupId"]
			artifactID := propertiesMap["artifactId"]
			version := propertiesMap["version"]

			if groupID == "" || artifactID == "" || version == "" {
				return nil, fmt.Errorf("invalid pom.properties content present in provided Package ")
			}

			return &mavenPackageMetadata{
				GroupID:    groupID,
				ArtifactID: artifactID,
				Version:    version,
				Name:       "", // not present in properties
			}, nil
		}
	}
	return nil, fmt.Errorf("pom.properties not found in provided Package ")

}

func readPomXMLFromPackageFile(files []*zip.File) (*mavenPackageMetadata, error) {
	for _, f := range files {
		if strings.HasSuffix(f.Name, "pom.xml") &&
			strings.Contains(f.Name, "META-INF/maven/") {

			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()

			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, err
			}

			return parsePomXMLData(data)
		}
	}

	return nil, fmt.Errorf("pom.xml not found in provided package")
}

func parsePomXMLData(data []byte) (*mavenPackageMetadata, error) {
	var pom pomXML
	if err := xml.Unmarshal(data, &pom); err != nil {
		return nil, fmt.Errorf("invalid XML or not a Maven POM: %w", err)
	}

	// Resolve groupId (may be inherited)
	groupID := strings.TrimSpace(pom.GroupID)
	if groupID == "" && pom.Parent != nil {
		groupID = strings.TrimSpace(pom.Parent.GroupID)
	}

	// Resolve version (may be inherited)
	version := strings.TrimSpace(pom.Version)
	if version == "" && pom.Parent != nil {
		version = strings.TrimSpace(pom.Parent.Version)
	}

	artifactID := strings.TrimSpace(pom.ArtifactID)
	name := strings.TrimSpace(pom.Name)

	// Validating required Maven fields
	if groupID == "" {
		return nil, fmt.Errorf("groupId not found in pom")
	}
	if artifactID == "" {
		return nil, fmt.Errorf("artifactId not found in pom")
	}
	if version == "" {
		return nil, fmt.Errorf("version not found in pom")
	}

	return &mavenPackageMetadata{
		GroupID:    groupID,
		ArtifactID: artifactID,
		Version:    version,
		Name:       name,
	}, nil
}

func compareMavenCoordinates(coordsFromPackage, coordsFromPom *mavenPackageMetadata) error {

	if coordsFromPackage == nil {
		return fmt.Errorf("package coordinates are nil")
	}
	if coordsFromPom == nil {
		return fmt.Errorf("pom coordinates are nil")
	}

	if coordsFromPackage.GroupID != coordsFromPom.GroupID {
		return fmt.Errorf(
			"groupId mismatch: package=%q, pom=%q",
			coordsFromPackage.GroupID,
			coordsFromPom.GroupID,
		)
	}

	if coordsFromPackage.ArtifactID != coordsFromPom.ArtifactID {
		return fmt.Errorf(
			"artifactId mismatch: package=%q, pom=%q",
			coordsFromPackage.ArtifactID,
			coordsFromPom.ArtifactID,
		)
	}

	if coordsFromPackage.Version != coordsFromPom.Version {
		return fmt.Errorf(
			"version mismatch: package=%q, pom=%q",
			coordsFromPackage.Version,
			coordsFromPom.Version,
		)
	}

	return nil
}

func addChecksumFiles(
	mavenFilesToUpload *[]string,
	outputDir string,
) error {

	if mavenFilesToUpload == nil {
		return fmt.Errorf("file list is nil")
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output dir: %w", err)
	}

	originalFiles := append([]string{}, (*mavenFilesToUpload)...)

	for _, filePath := range originalFiles {
		if strings.TrimSpace(filePath) == "" {
			continue
		}

		if err := createChecksumFiles(filePath, outputDir, mavenFilesToUpload); err != nil {
			return err
		}
	}

	return nil
}

func createChecksumFiles(
	filePath string,
	outputDir string,
	mavenFilesToUpload *[]string,
) error {

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", filePath, err)
	}
	defer file.Close()

	md5Hash := md5.New()
	sha1Hash := sha1.New()

	if _, err := io.Copy(io.MultiWriter(md5Hash, sha1Hash), file); err != nil {
		return fmt.Errorf("failed to hash %s: %w", filePath, err)
	}

	md5Sum := hex.EncodeToString(md5Hash.Sum(nil))
	sha1Sum := hex.EncodeToString(sha1Hash.Sum(nil))

	baseName := filepath.Base(filePath)

	md5Path := filepath.Join(outputDir, baseName+".md5")
	sha1Path := filepath.Join(outputDir, baseName+".sha1")

	if err := os.WriteFile(md5Path, []byte(md5Sum), 0644); err != nil {
		return fmt.Errorf("failed to write md5 file: %w", err)
	}

	if err := os.WriteFile(sha1Path, []byte(sha1Sum), 0644); err != nil {
		return fmt.Errorf("failed to write sha1 file: %w", err)
	}

	*mavenFilesToUpload = append(*mavenFilesToUpload, md5Path, sha1Path)

	return nil
}
func getAllChecksumFileToUpload(mavenFiles []string, coordsFromPom *mavenPackageMetadata) ([]InMemoryUploadFile, error) {
	var checksumFiles []InMemoryUploadFile

	for _, filePath := range mavenFiles {
		ext := strings.ToLower(filepath.Ext(filePath))
		if ext == ".md5" || ext == ".sha1" {
			continue
		}

		artifacts, err := generateChecksumArtifacts(filePath, coordsFromPom)
		if err != nil {
			return nil, err
		}

		checksumFiles = append(checksumFiles, artifacts...)
	}

	return checksumFiles, nil
}
func generateChecksumArtifacts(filePath string, coordsFromPom *mavenPackageMetadata) ([]InMemoryUploadFile, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", filePath, err)
	}
	defer file.Close()

	md5Hash := md5.New()
	sha1Hash := sha1.New()

	if _, err := io.Copy(io.MultiWriter(md5Hash, sha1Hash), file); err != nil {
		return nil, fmt.Errorf("failed to hash %s: %w", filePath, err)
	}

	md5Sum := hex.EncodeToString(md5Hash.Sum(nil))
	sha1Sum := hex.EncodeToString(sha1Hash.Sum(nil))

	baseName := filepath.Base(filePath)

	if baseName == "pom.xml" {
		baseName = normalizePomFilename(coordsFromPom) + ".pom"
	}

	return []InMemoryUploadFile{
		{
			FileName: baseName + ".md5",
			Content:  []byte(md5Sum),
		},
		{
			FileName: baseName + ".sha1",
			Content:  []byte(sha1Sum),
		},
	}, nil
}

func uploadInMemoryMavenFile(
	pkgClient *pkgclient.ClientWithResponses,
	checkSumfile InMemoryUploadFile,
	registryName string,
	progress *p.ConsoleReporter,
	coords *mavenPackageMetadata,
) error {

	progress.Step(fmt.Sprintf("Uploading %s ", checkSumfile.FileName))

	checksumContent := checkSumfile.Content

	currentFileExt := strings.ToLower(filepath.Ext(checkSumfile.FileName))
	// Initialize progress reader
	bufferSize := int64(len(checksumContent))

	reader, closer := p.Reader(bufferSize, bytes.NewReader(checksumContent), currentFileExt)
	defer closer()

	resp, err := pkgClient.UploadMavenPackageWithBodyWithResponse(
		context.Background(),
		config.Global.AccountID,
		registryName,
		coords.GroupID,
		coords.ArtifactID,
		coords.Version,
		checkSumfile.FileName,
		"application/octet-stream",
		reader,
	)
	if err != nil {
		progress.Error("Failed to upload checksum file")
		return err
	}

	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusCreated {
		return fmt.Errorf("checksum upload failed: %s", resp.Status())
	}

	return nil
}

func validateSnapshotVersion(version string) error {
	if strings.HasSuffix(strings.ToUpper(version), "-SNAPSHOT") {
		return fmt.Errorf("SNAPSHOT version upload is currently not supported in CLI")
	}
	return nil
}

// converting pom.xml to maven build .pom structure
func normalizePomFilename(coords *mavenPackageMetadata) string {
	pomFileNameWithOutExt := coords.ArtifactID + "-" + coords.Version
	return pomFileNameWithOutExt
}

func getMavenMetadataXml(
	pkgClient *pkgclient.ClientWithResponses,
	registryName string,
	coords *mavenPackageMetadata,
) (*MavenMetadataXMLStruct, bool, error) {

	const metadataFile = "maven-metadata.xml"
	groupID := coords.GroupID
	artifactID := coords.ArtifactID
	currentVersion := coords.Version

	resp, err := pkgClient.DownloadMavenMetadataXmlWithResponse(
		context.Background(),
		config.Global.AccountID,
		registryName,
		groupID,
		artifactID,
		metadataFile,
	)
	if err != nil {
		return nil, false, fmt.Errorf("failed to call metadata API: %w", err)
	}

	switch resp.StatusCode() {

	case http.StatusOK:
		var metadata MavenMetadataXMLStruct
		if err := xml.Unmarshal(resp.Body, &metadata); err != nil {
			return nil, false, fmt.Errorf("failed to parse maven-metadata.xml: %w", err)
		}
		return &metadata, false, nil

	case http.StatusNotFound:
		// Metadata does not exist yet, Creating fresh metadata
		newMetadata := newMavenMetadataXml(groupID, artifactID, currentVersion)
		return newMetadata, true, nil

	default:
		return nil, false, fmt.Errorf(
			"unexpected response while fetching metadata: %s",
			resp.Status(),
		)
	}
}

func (m *MavenMetadataXMLStruct) addNewVersion(version string) {
	for _, v := range m.Versioning.Versions {
		if v == version {
			m.Versioning.Release = version
			m.Versioning.LastUpdated = time.Now().UTC().Format("20060102150405")
			return // already exists , still above two is need to be updated
		}
	}
	m.Versioning.Versions = append(m.Versioning.Versions, version)
	m.Versioning.Release = version
	m.Versioning.LastUpdated = time.Now().UTC().Format("20060102150405")
}
func (m *MavenMetadataXMLStruct) toXML() ([]byte, error) {
	return xml.MarshalIndent(m, "", "  ")
}
func (m *MavenMetadataXMLStruct) toXMLWithHeader() ([]byte, error) {
	body, err := xml.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}

	header := []byte(xml.Header) // "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"
	return append(header, body...), nil
}

// used when no maven-metadata.xml is present
func newMavenMetadataXml(groupID string, artifactID string, version string) *MavenMetadataXMLStruct {

	now := time.Now().UTC().Format("20060102150405")

	return &MavenMetadataXMLStruct{
		GroupID:    groupID,
		ArtifactID: artifactID,
		Versioning: MavenVersioning{
			Release:     version,
			Versions:    []string{version},
			LastUpdated: now,
		},
	}
}

func uploadMavenMetadataXML(
	pkgClient *pkgclient.ClientWithResponses,
	registryName string,
	progress *p.ConsoleReporter,
	coords *mavenPackageMetadata,
	metadata *MavenMetadataXMLStruct,
) error {

	// Convert metadata to XML with header
	xmlBytes, err := metadata.toXMLWithHeader()
	if err != nil {
		progress.Error("Failed to marshal maven-metadata.xml")
		return fmt.Errorf("failed to generate metadata XML: %w", err)
	}

	fileName := "maven-metadata.xml"

	progress.Step(fmt.Sprintf("Uploading %s ", fileName))

	currentFileExt := strings.ToLower(filepath.Ext(fileName))
	// Initialize progress reader

	bufferSize := int64(len(xmlBytes))
	reader, closer := p.Reader(
		bufferSize,
		bytes.NewReader(xmlBytes),
		currentFileExt,
	)
	defer closer()

	resp, err := pkgClient.UploadMavenMetadataXmlWithBodyWithResponse(
		context.Background(),
		config.Global.AccountID,
		registryName,
		coords.GroupID,
		coords.ArtifactID,
		fileName,
		"application/octet-stream",
		reader,
	)
	if err != nil {
		progress.Error("Failed to upload maven-metadata.xml")
		return err
	}

	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusCreated {
		progress.Error("Upload failed")
		return fmt.Errorf(
			"failed to upload maven-metadata.xml: %s\nresponse: %s",
			resp.Status(),
			resp.Body,
		)
	}
	return nil
}

// internal representation of a Maven POM
type pomXML struct {
	XMLName    xml.Name `xml:"project"`
	GroupID    string   `xml:"groupId"`
	ArtifactID string   `xml:"artifactId"`
	Version    string   `xml:"version"`
	Name       string   `xml:"name"`

	Parent *struct {
		GroupID string `xml:"groupId"`
		Version string `xml:"version"`
	} `xml:"parent"`
}

type mavenPackageMetadata struct {
	GroupID    string
	ArtifactID string
	Version    string
	Name       string
}

// InMemoryUploadFile represents a file being uploaded, stored entirely in memory with its name and content.
// using it for storing checksum file
type InMemoryUploadFile struct {
	FileName string
	Content  []byte
}

// MavenMetadataXMLStruct represents the structure of a maven-metadata XML file for describing package versions.
type MavenMetadataXMLStruct struct {
	XMLName    xml.Name        `xml:"metadata"`
	GroupID    string          `xml:"groupId"`
	ArtifactID string          `xml:"artifactId"`
	Versioning MavenVersioning `xml:"versioning"`
}

// MavenVersioning represents versioning details in Maven metadata, including release, and available versions.
type MavenVersioning struct {
	Release     string   `xml:"release,omitempty"`
	Versions    []string `xml:"versions>version"`
	LastUpdated string   `xml:"lastUpdated,omitempty"`
}
