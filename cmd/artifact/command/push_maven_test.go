package command

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/util/common/auth"
)

// withMavenServer spins up a stub server and points the global config at it
// for the duration of the test, restoring originals on cleanup.
func withMavenServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	origPkg := config.Global.Registry.PkgURL
	origAPIBase := config.Global.APIBaseURL
	origAccountID := config.Global.AccountID
	origAuthToken := config.Global.AuthToken

	config.Global.Registry.PkgURL = srv.URL
	config.Global.APIBaseURL = srv.URL
	config.Global.AccountID = "test-account"
	config.Global.AuthToken = "pat.test-account.aaa.bbb"

	t.Cleanup(func() {
		config.Global.Registry.PkgURL = origPkg
		config.Global.APIBaseURL = origAPIBase
		config.Global.AccountID = origAccountID
		config.Global.AuthToken = origAuthToken
	})

	return srv
}

// createTestJarFile creates a minimal valid JAR file with Maven metadata
func createTestJarFile(t *testing.T, groupID, artifactID, version string) string {
	t.Helper()
	dir := t.TempDir()
	jarPath := filepath.Join(dir, artifactID+"-"+version+".jar")

	zipFile, err := os.Create(jarPath)
	if err != nil {
		t.Fatalf("failed to create jar file: %v", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Add pom.properties
	pomPropsPath := "META-INF/maven/" + groupID + "/" + artifactID + "/pom.properties"
	pomPropsContent := "groupId=" + groupID + "\n" +
		"artifactId=" + artifactID + "\n" +
		"version=" + version + "\n"

	w, err := zipWriter.Create(pomPropsPath)
	if err != nil {
		t.Fatalf("failed to create pom.properties in jar: %v", err)
	}
	if _, err := w.Write([]byte(pomPropsContent)); err != nil {
		t.Fatalf("failed to write pom.properties: %v", err)
	}

	// Add a dummy class file
	classFile, err := zipWriter.Create("com/example/Main.class")
	if err != nil {
		t.Fatalf("failed to create class file: %v", err)
	}
	if _, err := classFile.Write([]byte("dummy class content")); err != nil {
		t.Fatalf("failed to write class file: %v", err)
	}

	return jarPath
}

// createTestPomFile creates a minimal valid POM file
func createTestPomFile(t *testing.T, groupID, artifactID, version, name string) string {
	t.Helper()
	dir := t.TempDir()
	pomPath := filepath.Join(dir, "pom.xml")

	pomContent := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
	<modelVersion>4.0.0</modelVersion>
	<groupId>` + groupID + `</groupId>
	<artifactId>` + artifactID + `</artifactId>
	<version>` + version + `</version>
	<name>` + name + `</name>
</project>`

	if err := os.WriteFile(pomPath, []byte(pomContent), 0644); err != nil {
		t.Fatalf("failed to write pom file: %v", err)
	}

	return pomPath
}

// runMavenCmd runs the maven push command directly with the given args
// and returns the resulting error.
func runMavenCmd(t *testing.T, args ...string) error {
	t.Helper()
	factory := &cmdutils.Factory{
		PkgHttpClient: func() *pkgclient.ClientWithResponses {
			client, err := pkgclient.NewClientWithResponses(config.Global.Registry.PkgURL,
				auth.GetAuthOptionARPKG())
			if err != nil {
				t.Fatalf("failed to create pkg client: %v", err)
			}
			return client
		},
	}
	cmd := NewPushMavenCmd(factory)
	cmd.SetArgs(args)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	return cmd.Execute()
}

func TestNewPushMavenCmd_Success(t *testing.T) {
	uploadCount := 0
	srv := withMavenServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Handle metadata download (404 for first time)
		if strings.Contains(r.URL.Path, "maven-metadata.xml") && r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Handle all uploads
		uploadCount++
		w.WriteHeader(http.StatusCreated)
	})
	defer srv.Close()

	jarFile := createTestJarFile(t, "com.example", "test-app", "1.0.0")
	pomFile := createTestPomFile(t, "com.example", "test-app", "1.0.0", "Test App")

	err := runMavenCmd(t, "test-registry", jarFile, "--pom-file", pomFile)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	// Should upload: jar, pom, jar.md5, jar.sha1, pom.md5, pom.sha1, maven-metadata.xml
	if uploadCount < 5 {
		t.Errorf("expected at least 5 uploads, got %d", uploadCount)
	}
}

func TestNewPushMavenCmd_PkgClientCreation(t *testing.T) {
	// This test specifically covers line 146: pkgClient := c.PkgHttpClient()
	clientCreated := false
	srv := withMavenServer(t, func(w http.ResponseWriter, r *http.Request) {
		clientCreated = true
		if strings.Contains(r.URL.Path, "maven-metadata.xml") && r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})
	defer srv.Close()

	jarFile := createTestJarFile(t, "com.example", "test-app", "1.0.0")
	pomFile := createTestPomFile(t, "com.example", "test-app", "1.0.0", "Test App")

	factory := &cmdutils.Factory{
		PkgHttpClient: func() *pkgclient.ClientWithResponses {
			client, _ := pkgclient.NewClientWithResponses(config.Global.Registry.PkgURL,
				auth.GetAuthOptionARPKG())
			return client
		},
	}

	cmd := NewPushMavenCmd(factory)
	cmd.SetArgs([]string{"test-registry", jarFile, "--pom-file", pomFile})
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	if !clientCreated {
		t.Error("PkgHttpClient was not called - line 146 not covered")
	}
}

func TestNewPushMavenCmd_FileNotFound(t *testing.T) {
	srv := withMavenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	pomFile := createTestPomFile(t, "com.example", "test-app", "1.0.0", "Test App")

	err := runMavenCmd(t, "test-registry", "/nonexistent/file.jar", "--pom-file", pomFile)
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
	if !strings.Contains(err.Error(), "failed to access package file") {
		t.Errorf("expected 'failed to access package file' error, got: %v", err)
	}
}

func TestNewPushMavenCmd_PomFileNotFound(t *testing.T) {
	srv := withMavenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	jarFile := createTestJarFile(t, "com.example", "test-app", "1.0.0")

	err := runMavenCmd(t, "test-registry", jarFile, "--pom-file", "/nonexistent/pom.xml")
	if err == nil {
		t.Fatal("expected error for non-existent pom file, got nil")
	}
	if !strings.Contains(err.Error(), "failed to access POM file") {
		t.Errorf("expected 'failed to access POM file' error, got: %v", err)
	}
}

func TestNewPushMavenCmd_InvalidJarExtension(t *testing.T) {
	srv := withMavenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	dir := t.TempDir()
	invalidFile := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(invalidFile, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	pomFile := createTestPomFile(t, "com.example", "test-app", "1.0.0", "Test App")

	err := runMavenCmd(t, "test-registry", invalidFile, "--pom-file", pomFile)
	if err == nil {
		t.Fatal("expected error for invalid file extension, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported extension") {
		t.Errorf("expected 'unsupported extension' error, got: %v", err)
	}
}

func TestNewPushMavenCmd_InvalidPomExtension(t *testing.T) {
	srv := withMavenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	jarFile := createTestJarFile(t, "com.example", "test-app", "1.0.0")

	dir := t.TempDir()
	invalidPom := filepath.Join(dir, "pom.txt")
	if err := os.WriteFile(invalidPom, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	err := runMavenCmd(t, "test-registry", jarFile, "--pom-file", invalidPom)
	if err == nil {
		t.Fatal("expected error for invalid pom extension, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported extension") {
		t.Errorf("expected 'unsupported extension' error, got: %v", err)
	}
}

func TestNewPushMavenCmd_CoordinatesMismatch(t *testing.T) {
	srv := withMavenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	// Create JAR with one set of coordinates
	jarFile := createTestJarFile(t, "com.example", "app1", "1.0.0")
	// Create POM with different coordinates
	pomFile := createTestPomFile(t, "com.example", "app2", "1.0.0", "Test App")

	err := runMavenCmd(t, "test-registry", jarFile, "--pom-file", pomFile)
	if err == nil {
		t.Fatal("expected error for coordinates mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "mismatch") {
		t.Errorf("expected 'mismatch' error, got: %v", err)
	}
}

func TestNewPushMavenCmd_SnapshotVersion(t *testing.T) {
	srv := withMavenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	jarFile := createTestJarFile(t, "com.example", "test-app", "1.0.0-SNAPSHOT")
	pomFile := createTestPomFile(t, "com.example", "test-app", "1.0.0-SNAPSHOT", "Test App")

	err := runMavenCmd(t, "test-registry", jarFile, "--pom-file", pomFile)
	if err == nil {
		t.Fatal("expected error for SNAPSHOT version, got nil")
	}
	if !strings.Contains(err.Error(), "SNAPSHOT") {
		t.Errorf("expected 'SNAPSHOT' error, got: %v", err)
	}
}

func TestNewPushMavenCmd_DirectoryPath(t *testing.T) {
	srv := withMavenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	dir := t.TempDir()
	pomFile := createTestPomFile(t, "com.example", "test-app", "1.0.0", "Test App")

	err := runMavenCmd(t, "test-registry", dir, "--pom-file", pomFile)
	if err == nil {
		t.Fatal("expected error for directory path, got nil")
	}
	if !strings.Contains(err.Error(), "must be a file, not a directory") {
		t.Errorf("expected 'must be a file' error, got: %v", err)
	}
}

func TestNewPushMavenCmd_WrongArgCount(t *testing.T) {
	err := runMavenCmd(t, "test-registry")
	if err == nil {
		t.Fatal("expected error for wrong argument count, got nil")
	}
	if !strings.Contains(err.Error(), "Invalid number of argument") {
		t.Errorf("expected 'Invalid number of argument' error, got: %v", err)
	}
}

func TestNewPushMavenCmd_MissingPomFlag(t *testing.T) {
	srv := withMavenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	jarFile := createTestJarFile(t, "com.example", "test-app", "1.0.0")

	err := runMavenCmd(t, "test-registry", jarFile)
	if err == nil {
		t.Fatal("expected error for missing --pom-file flag, got nil")
	}
}

func TestNewPushMavenCmd_ServerError(t *testing.T) {
	srv := withMavenServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Return 404 for metadata download
		if strings.Contains(r.URL.Path, "maven-metadata.xml") && r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Return error for uploads
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer srv.Close()

	jarFile := createTestJarFile(t, "com.example", "test-app", "1.0.0")
	pomFile := createTestPomFile(t, "com.example", "test-app", "1.0.0", "Test App")

	err := runMavenCmd(t, "test-registry", jarFile, "--pom-file", pomFile)
	if err == nil {
		t.Fatal("expected error for server error, got nil")
	}
}

func TestNewPushMavenCmd_CustomPkgUrl(t *testing.T) {
	srv := withMavenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata.xml") && r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})
	defer srv.Close()

	jarFile := createTestJarFile(t, "com.example", "test-app", "1.0.0")
	pomFile := createTestPomFile(t, "com.example", "test-app", "1.0.0", "Test App")

	err := runMavenCmd(t, "test-registry", jarFile, "--pom-file", pomFile, "--pkg-url", srv.URL)
	if err != nil {
		t.Fatalf("expected success with custom pkg-url, got error: %v", err)
	}
}

func TestNewPushMavenCmd_WarFile(t *testing.T) {
	srv := withMavenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata.xml") && r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})
	defer srv.Close()

	// Create WAR file instead of JAR
	dir := t.TempDir()
	warPath := filepath.Join(dir, "test-app-1.0.0.war")

	func() {
		zipFile, err := os.Create(warPath)
		if err != nil {
			t.Fatalf("failed to create war file: %v", err)
		}
		defer zipFile.Close()

		zipWriter := zip.NewWriter(zipFile)
		defer zipWriter.Close()

		// Add pom.properties
		pomPropsPath := "META-INF/maven/com.example/test-app/pom.properties"
		pomPropsContent := "groupId=com.example\nartifactId=test-app\nversion=1.0.0\n"

		w, err := zipWriter.Create(pomPropsPath)
		if err != nil {
			t.Fatalf("failed to create pom.properties in war: %v", err)
		}
		if _, err := w.Write([]byte(pomPropsContent)); err != nil {
			t.Fatalf("failed to write pom.properties: %v", err)
		}
	}()

	pomFile := createTestPomFile(t, "com.example", "test-app", "1.0.0", "Test App")

	err := runMavenCmd(t, "test-registry", warPath, "--pom-file", pomFile)
	if err != nil {
		t.Fatalf("expected success with WAR file, got error: %v", err)
	}
}

func TestIsValidMavenPackageFile(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		wantOk   bool
	}{
		{"valid jar", "app-1.0.0.jar", true},
		{"valid war", "app-1.0.0.war", true},
		{"invalid txt", "app.txt", false},
		{"empty name", "", false},
		{"no extension", "app", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, _ := isValidMavenPackageFile(tt.fileName)
			if ok != tt.wantOk {
				t.Errorf("isValidMavenPackageFile(%q) = %v, want %v", tt.fileName, ok, tt.wantOk)
			}
		})
	}
}

func TestIsValidPomFile(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		wantOk   bool
	}{
		{"valid xml", "pom.xml", true},
		{"valid pom", "app-1.0.0.pom", true},
		{"invalid txt", "pom.txt", false},
		{"empty name", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, _ := isValidPomFile(tt.fileName)
			if ok != tt.wantOk {
				t.Errorf("isValidPomFile(%q) = %v, want %v", tt.fileName, ok, tt.wantOk)
			}
		})
	}
}

func TestValidateSnapshotVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantErr bool
	}{
		{"release version", "1.0.0", false},
		{"snapshot uppercase", "1.0.0-SNAPSHOT", true},
		{"snapshot lowercase", "1.0.0-snapshot", true},
		{"snapshot mixed", "1.0.0-SnApShOt", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSnapshotVersion(tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSnapshotVersion(%q) error = %v, wantErr %v", tt.version, err, tt.wantErr)
			}
		})
	}
}

func TestNormalizePomFilename(t *testing.T) {
	coords := &mavenPackageMetadata{
		GroupID:    "com.example",
		ArtifactID: "test-app",
		Version:    "1.0.0",
	}

	result := normalizePomFilename(coords)
	expected := "test-app-1.0.0"

	if result != expected {
		t.Errorf("normalizePomFilename() = %q, want %q", result, expected)
	}
}

func TestCompareMavenCoordinates(t *testing.T) {
	coords1 := &mavenPackageMetadata{
		GroupID:    "com.example",
		ArtifactID: "test-app",
		Version:    "1.0.0",
	}

	coords2 := &mavenPackageMetadata{
		GroupID:    "com.example",
		ArtifactID: "test-app",
		Version:    "1.0.0",
	}

	// Should not error for matching coordinates
	if err := compareMavenCoordinates(coords1, coords2); err != nil {
		t.Errorf("compareMavenCoordinates() with matching coords returned error: %v", err)
	}

	// Test groupId mismatch
	coords3 := &mavenPackageMetadata{
		GroupID:    "com.other",
		ArtifactID: "test-app",
		Version:    "1.0.0",
	}
	if err := compareMavenCoordinates(coords1, coords3); err == nil {
		t.Error("compareMavenCoordinates() with groupId mismatch should return error")
	}

	// Test nil coordinates
	if err := compareMavenCoordinates(nil, coords2); err == nil {
		t.Error("compareMavenCoordinates() with nil coords should return error")
	}
}
