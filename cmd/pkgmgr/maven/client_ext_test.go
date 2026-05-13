package maven

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMavenClientNameAndType(t *testing.T) {
	c := NewClient()
	assert.Equal(t, "mvn", c.Name())
	assert.Equal(t, "maven", c.PackageType())
}

func TestHarURLPatternMaven(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		matches bool
	}{
		{"subdomain format", "https://pkg.harness.io/acct123/my-registry/maven", true},
		{"path format with pkg", "https://app.harness.io/pkg/acct123/my-registry/maven", true},
		{"trailing slash", "https://pkg.harness.io/acct123/my-registry/maven/", true},
		{"http", "http://pkg.harness.io/acct123/my-registry/maven", true},
		{"not maven", "https://pkg.harness.io/acct123/my-registry/npm", false},
		{"too few segments", "https://pkg.harness.io/maven", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.matches, harURLPattern.MatchString(tt.url))
		})
	}
}

func TestParseSettingsXMLForHAR(t *testing.T) {
	t.Run("valid settings with HAR mirror", func(t *testing.T) {
		tmpDir := t.TempDir()
		settingsPath := filepath.Join(tmpDir, "settings.xml")
		content := `<?xml version="1.0" encoding="UTF-8"?>
<settings>
  <mirrors>
    <mirror>
      <id>harness-maven</id>
      <url>https://pkg.harness.io/acct123/my-maven-reg/maven</url>
      <mirrorOf>*</mirrorOf>
    </mirror>
  </mirrors>
  <servers>
    <server>
      <id>harness-maven</id>
      <username>user</username>
      <password>pat.token123</password>
    </server>
  </servers>
</settings>`
		require.NoError(t, os.WriteFile(settingsPath, []byte(content), 0644))

		info, err := parseSettingsXMLForHAR(settingsPath, "")
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "my-maven-reg", info.RegistryIdentifier)
		assert.Equal(t, "acct123", info.AccountID)
		assert.Equal(t, "pat.token123", info.AuthToken)
	})

	t.Run("explicit registry match", func(t *testing.T) {
		tmpDir := t.TempDir()
		settingsPath := filepath.Join(tmpDir, "settings.xml")
		content := `<?xml version="1.0" encoding="UTF-8"?>
<settings>
  <mirrors>
    <mirror>
      <id>har</id>
      <url>https://pkg.harness.io/acct/target-reg/maven</url>
    </mirror>
  </mirrors>
  <servers>
    <server><id>har</id><password>tok</password></server>
  </servers>
</settings>`
		require.NoError(t, os.WriteFile(settingsPath, []byte(content), 0644))

		info, err := parseSettingsXMLForHAR(settingsPath, "target-reg")
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "target-reg", info.RegistryIdentifier)
	})

	t.Run("explicit registry mismatch", func(t *testing.T) {
		tmpDir := t.TempDir()
		settingsPath := filepath.Join(tmpDir, "settings.xml")
		content := `<?xml version="1.0" encoding="UTF-8"?>
<settings>
  <mirrors>
    <mirror>
      <id>har</id>
      <url>https://pkg.harness.io/acct/other-reg/maven</url>
    </mirror>
  </mirrors>
</settings>`
		require.NoError(t, os.WriteFile(settingsPath, []byte(content), 0644))

		info, err := parseSettingsXMLForHAR(settingsPath, "wanted-reg")
		assert.Error(t, err)
		assert.Nil(t, info)
	})

	t.Run("no HAR URL in settings", func(t *testing.T) {
		tmpDir := t.TempDir()
		settingsPath := filepath.Join(tmpDir, "settings.xml")
		content := `<?xml version="1.0" encoding="UTF-8"?>
<settings>
  <mirrors>
    <mirror>
      <id>central</id>
      <url>https://repo.maven.apache.org/maven2</url>
    </mirror>
  </mirrors>
</settings>`
		require.NoError(t, os.WriteFile(settingsPath, []byte(content), 0644))

		info, err := parseSettingsXMLForHAR(settingsPath, "")
		assert.Error(t, err)
		assert.Nil(t, info)
	})

	t.Run("missing file", func(t *testing.T) {
		info, err := parseSettingsXMLForHAR("/nonexistent/settings.xml", "")
		assert.Error(t, err)
		assert.Nil(t, info)
	})

	t.Run("invalid XML", func(t *testing.T) {
		tmpDir := t.TempDir()
		settingsPath := filepath.Join(tmpDir, "settings.xml")
		require.NoError(t, os.WriteFile(settingsPath, []byte(`not xml`), 0644))

		info, err := parseSettingsXMLForHAR(settingsPath, "")
		assert.Error(t, err)
		assert.Nil(t, info)
	})
}

func TestParsePomForDeps(t *testing.T) {
	t.Run("valid pom with dependencies", func(t *testing.T) {
		tmpDir := t.TempDir()
		pomPath := filepath.Join(tmpDir, "pom.xml")
		content := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <dependencies>
    <dependency>
      <groupId>com.google.guava</groupId>
      <artifactId>guava</artifactId>
      <version>31.1-jre</version>
    </dependency>
    <dependency>
      <groupId>org.apache.commons</groupId>
      <artifactId>commons-lang3</artifactId>
      <version>3.12.0</version>
    </dependency>
  </dependencies>
</project>`
		require.NoError(t, os.WriteFile(pomPath, []byte(content), 0644))

		deps, err := parsePomForDeps(pomPath)
		require.NoError(t, err)
		assert.Len(t, deps, 2)
		assert.Equal(t, "com.google.guava:guava", deps[0].Name)
		assert.Equal(t, "31.1-jre", deps[0].Version)
		assert.Equal(t, "pom.xml", deps[0].Source)
	})

	t.Run("pom with dependency management", func(t *testing.T) {
		tmpDir := t.TempDir()
		pomPath := filepath.Join(tmpDir, "pom.xml")
		content := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <dependencyManagement>
    <dependencies>
      <dependency>
        <groupId>org.springframework</groupId>
        <artifactId>spring-core</artifactId>
        <version>5.3.0</version>
      </dependency>
    </dependencies>
  </dependencyManagement>
</project>`
		require.NoError(t, os.WriteFile(pomPath, []byte(content), 0644))

		deps, err := parsePomForDeps(pomPath)
		require.NoError(t, err)
		assert.Len(t, deps, 1)
		assert.Equal(t, "org.springframework:spring-core", deps[0].Name)
	})

	t.Run("dep without version gets latest", func(t *testing.T) {
		tmpDir := t.TempDir()
		pomPath := filepath.Join(tmpDir, "pom.xml")
		content := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <dependencies>
    <dependency>
      <groupId>com.example</groupId>
      <artifactId>no-version</artifactId>
    </dependency>
  </dependencies>
</project>`
		require.NoError(t, os.WriteFile(pomPath, []byte(content), 0644))

		deps, err := parsePomForDeps(pomPath)
		require.NoError(t, err)
		assert.Len(t, deps, 1)
		assert.Equal(t, "latest", deps[0].Version)
	})

	t.Run("deduplicates dependencies", func(t *testing.T) {
		tmpDir := t.TempDir()
		pomPath := filepath.Join(tmpDir, "pom.xml")
		content := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <dependencies>
    <dependency>
      <groupId>com.example</groupId>
      <artifactId>lib</artifactId>
      <version>1.0</version>
    </dependency>
  </dependencies>
  <dependencyManagement>
    <dependencies>
      <dependency>
        <groupId>com.example</groupId>
        <artifactId>lib</artifactId>
        <version>1.0</version>
      </dependency>
    </dependencies>
  </dependencyManagement>
</project>`
		require.NoError(t, os.WriteFile(pomPath, []byte(content), 0644))

		deps, err := parsePomForDeps(pomPath)
		require.NoError(t, err)
		assert.Len(t, deps, 1)
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := parsePomForDeps("/nonexistent/pom.xml")
		assert.Error(t, err)
	})

	t.Run("invalid XML", func(t *testing.T) {
		tmpDir := t.TempDir()
		pomPath := filepath.Join(tmpDir, "pom.xml")
		require.NoError(t, os.WriteFile(pomPath, []byte(`not xml at all`), 0644))

		_, err := parsePomForDeps(pomPath)
		assert.Error(t, err)
	})
}

func TestCalculateDepth(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		depth int
	}{
		{"direct dep", "[INFO] +- com.google.guava:guava:jar:31.1:compile", 1},
		{"transitive level 2", "[INFO] |  +- com.google:lib:jar:1.0:compile", 2},
		{"transitive level 2 last", "[INFO] |  \\- com.google:lib:jar:1.0:compile", 2},
		{"empty after prefix", "[INFO] ", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.depth, calculateDepth(tt.line))
		})
	}
}

func TestParseDependencyTreeEmpty(t *testing.T) {
	output := `[INFO] Scanning for projects...
[INFO] BUILD SUCCESS`

	deps := parseDependencyTree(output)
	assert.Empty(t, deps)
}

func TestParseDependencyTreeDeduplication(t *testing.T) {
	output := `[INFO] +- com.google.guava:guava:jar:31.1-jre:compile
[INFO] +- com.google.guava:guava:jar:31.1-jre:compile`

	deps := parseDependencyTree(output)
	assert.Len(t, deps, 1)
}
