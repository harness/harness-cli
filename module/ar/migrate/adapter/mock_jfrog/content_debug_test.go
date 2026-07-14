package mock_jfrog

import (
	"io/fs"
	"strings"
	"testing"
)

// TestMockContentKeys verifies partial mock-init embeds still receive required
// programmatic defaults (python-local/.pypi, helm-http-local/index.yaml, etc.).
func TestMockContentKeys(t *testing.T) {
	embedded := 0
	_ = fs.WalkDir(testdataFS, "testdata/binary/content", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		embedded++
		t.Logf("embedded: %s", path)
		return nil
	})
	t.Logf("embedded content count: %d", embedded)

	c := NewMockClient().(*mockClient)
	var pythonKeys []string
	for k := range c.fileContent {
		if strings.HasPrefix(k, "python-local/") {
			pythonKeys = append(pythonKeys, k)
		}
	}
	t.Logf("python fileContent keys: %v (total fileContent: %d)", pythonKeys, len(c.fileContent))
	if len(pythonKeys) == 0 {
		t.Fatal("expected python-local file content keys")
	}
}

func TestMockRPMBytes(t *testing.T) {
	c := NewMockClient().(*mockClient)
	b := c.binaryContent["rpm-local/mockpkg-1.0.0-1.x86_64.rpm"]
	if len(b) == 0 {
		t.Fatal("expected non-empty rpm fixture")
	}
	t.Logf("rpm bytes: %d", len(b))
}
