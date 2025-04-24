package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

const (
	// Templates for generating config files for oapi-codegen
	clientConfigTemplate = `package: {{ .Package }}
generate:
  models: true
  client: true
output: {{ .OutputFile }}
`
	// Operation wrapper template for CLI operations
	operationsWrapperTemplate = `package {{ .Package }}

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"github.com/olekukonko/tablewriter"
	"context"
	"github.com/go-resty/resty/v2"
)

// Helper function to initialize API client
func NewAPIClient(baseURL string) *ClientWithResponses {
	client := resty.New()
	client.SetBaseURL(baseURL)

	return &ClientWithResponses{}
}

// Wrapper for handling CLI operations and output formatting
// This file is not auto-generated and will need to be manually updated
// to add new operations or modify existing ones
`
)

type TemplateData struct {
	Package    string
	OutputFile string
}

func main() {
	var specPath, serviceName, outputDir string
	flag.StringVar(&specPath, "spec", "", "Path to OpenAPI spec file")
	flag.StringVar(&serviceName, "service", "", "Service name")
	flag.StringVar(&outputDir, "output", "", "Output directory")
	flag.Parse()

	if specPath == "" || serviceName == "" || outputDir == "" {
		fmt.Println("Error: All parameters are required")
		flag.Usage()
		os.Exit(1)
	}

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Printf("Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	// Create config directory for generation configs
	configDir := filepath.Join(outputDir, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		fmt.Printf("Error creating config directory: %v\n", err)
		os.Exit(1)
	}

	// Generate client config file
	clientConfigFile := filepath.Join(configDir, "client.yaml")
	data := TemplateData{
		Package:    serviceName,
		OutputFile: filepath.Join(outputDir, "client.gen.go"),
	}
	
	if err := generateConfigFile(clientConfigFile, clientConfigTemplate, data); err != nil {
		fmt.Printf("Error generating client config: %v\n", err)
		os.Exit(1)
	}

	// Run oapi-codegen to generate client code
	fmt.Printf("Generating client code for %s service...\n", serviceName)
	// Use go run to execute oapi-codegen instead of looking for the binary
	cmd := exec.Command("go", "run", "github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest", "--config", clientConfigFile, specPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("Error running oapi-codegen: %v\n", err)
		os.Exit(1)
	}

	// Create operations wrapper file
	operationsFile := filepath.Join(outputDir, "operations.gen.go")
	if err := generateConfigFile(operationsFile, operationsWrapperTemplate, data); err != nil {
		fmt.Printf("Error generating operations wrapper: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully generated code for %s service\n", serviceName)
}

// generateConfigFile generates a configuration file using the provided template and data
func generateConfigFile(filename, templateStr string, data TemplateData) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	tmpl, err := template.New("config").Parse(templateStr)
	if err != nil {
		return err
	}

	if err := tmpl.Execute(file, data); err != nil {
		return err
	}

	return nil
}
