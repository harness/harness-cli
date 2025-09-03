package mock_jfrog

import (
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

// MockDataConfig allows easy configuration of mock data
type MockDataConfig struct {
	Registries  map[string]JFrogRepository
	Files       map[string][]types.File
	Catalogs    map[string][]string
	FileContent map[string]string
}

// UpdateMockData allows updating the mock data at runtime
func (c *client) UpdateMockData(config MockDataConfig) {
	if config.Registries != nil {
		for k, v := range config.Registries {
			c.mockData.registries[k] = v
		}
	}
	if config.Files != nil {
		for k, v := range config.Files {
			c.mockData.files[k] = v
		}
	}
	if config.Catalogs != nil {
		for k, v := range config.Catalogs {
			c.mockData.catalogs[k] = v
		}
	}
	if config.FileContent != nil {
		for k, v := range config.FileContent {
			c.mockData.fileContent[k] = v
		}
	}
}

// GetMockData returns the current mock data configuration
func (c *client) GetMockData() MockDataConfig {
	return MockDataConfig{
		Registries:  c.mockData.registries,
		Files:       c.mockData.files,
		Catalogs:    c.mockData.catalogs,
		FileContent: c.mockData.fileContent,
	}
}

// AddMockRegistry adds a new mock registry
func (c *client) AddMockRegistry(key string, repo JFrogRepository) {
	c.mockData.registries[key] = repo
}

// AddMockFiles adds mock files for a registry
func (c *client) AddMockFiles(registry string, files []types.File) {
	c.mockData.files[registry] = files
}

// AddMockCatalog adds a mock catalog for a registry
func (c *client) AddMockCatalog(registry string, repos []string) {
	c.mockData.catalogs[registry] = repos
}

// AddMockFileContent adds mock file content
func (c *client) AddMockFileContent(fileKey, content string) {
	c.mockData.fileContent[fileKey] = content
}
