package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
)

var (
	service    = flag.String("service", "", "service name (folder under api/)")
	inFile     = flag.String("in", "", "path to openapi.yaml")
	cmdFile    = flag.String("cmd", "", "path to command.yaml")
	outDir     = flag.String("out", "", "directory for the generated Cobra commands")
	skipClient = flag.Bool("skip-client", false, "skip client generation")
	module     = "github.com/harness/harness-cli"
)

func main() {
	flag.Parse()
	if *service == "" || *inFile == "" || *outDir == "" {
		flag.Usage()
		os.Exit(1)
	}

	// Generate client unless skipped
	if !*skipClient {
		genClient(*service, *inFile)
	}

	if *cmdFile != "" {
		// Generate commands using command.yaml if provided
		genCommandsFromCommandFile(*service, *inFile, *cmdFile, *outDir)
	}
}

// -------------------------------------------------------------------------
// 1. Generate REST client with oapi-codegen
func genClient(pkg, spec string) {
	targetDir := filepath.Join("internal", "api", pkg)
	must(os.MkdirAll(targetDir, 0o755))

	args := []string{
		"--package", pkg,
		"--generate", "types,client",
		"-response-type-suffix", "Resp",
		"--o", filepath.Join(targetDir, "client_gen.go"),
		spec,
	}

	cmd := exec.Command("oapi-codegen", args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	must(cmd.Run())
}

// -------------------------------------------------------------------------
// 3. Generate Cobra commands based on command.yaml and OpenAPI spec
func genCommandsFromCommandFile(pkg, spec, cmdFile, dir string) {
	// Load the OpenAPI spec
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(spec)
	must(err)
	must(doc.Validate(loader.Context))

	// Load the command yaml file
	cmdData, err := ioutil.ReadFile(cmdFile)
	must(err)

	var commands CommandFile
	err = yaml.Unmarshal(cmdData, &commands)
	must(err)

	// Create gen directory
	genDir := filepath.Join(dir, "command")
	must(os.MkdirAll(genDir, 0o755))

	// Build a map of OpenAPI operations for lookup
	opMap := make(map[string]map[string]*openapi3.Operation)
	for path, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}

		opMap[path] = make(map[string]*openapi3.Operation)
		for method, op := range item.Operations() {
			opMap[path][strings.ToUpper(method)] = op
		}
	}

	// Generate commands based on command.yaml definitions
	for _, cmd := range commands.Commands {
		log.Printf("Processing command: %s %s -> %s\n", cmd.Method, cmd.URL, cmd.Command)

		// Normalize URL for matching with OpenAPI paths
		normalizedURL := normalizeURL(cmd.URL)

		// Find matching OpenAPI operation
		op, exists := findOperation(opMap, normalizedURL, strings.ToUpper(cmd.Method))
		if !exists {
			log.Printf("Warning: No OpenAPI operation found for %s %s\n", cmd.Method, cmd.URL)
			continue
		}

		// Parse command structure (e.g., "artifact delete <name>")
		parts := strings.Fields(cmd.Command)
		if len(parts) < 2 {
			log.Printf("Warning: Invalid command format: %s\n", cmd.Command)
			continue
		}

		// Extract resource and action from command
		res := parts[0] // e.g., "artifact"
		act := parts[1] // e.g., "delete"

		// Extract required arguments from command
		var requiredArgs []string
		for _, part := range parts[2:] {
			if strings.HasPrefix(part, "<") && strings.HasSuffix(part, ">") {
				// Extract argument name without the angle brackets
				arg := part[1 : len(part)-1]
				requiredArgs = append(requiredArgs, arg)
			}
		}

		// Define output file path
		file := filepath.Join(genDir, fmt.Sprintf("%s_%s.go", act, res))
		overrideFile := filepath.Join(dir, fmt.Sprintf("%s_%s.go", act, res))

		if fileExists(overrideFile) {
			// User-supplied implementation beats generated one
			log.Printf("Skipping %s_%s - override exists\n", act, res)
			continue
		}

		// Write command file
		writeCmdFileWithArgs(pkg, res, act, cmd, op.OperationID, file, requiredArgs)
		log.Printf("Generated %s\n", file)
	}
}

// normalizeURL converts a URL with path params to a compatible format for matching
// with OpenAPI paths. For example, /registry/{registry_ref} -> /registry/{registry_ref}
func normalizeURL(url string) string {
	// OpenAPI paths already have the format we need
	return url
}

// findOperation finds the OpenAPI operation that best matches the given URL and method
func findOperation(opMap map[string]map[string]*openapi3.Operation, url, method string) (*openapi3.Operation, bool) {
	// Try exact match first
	if methodMap, exists := opMap[url]; exists {
		if op, exists := methodMap[method]; exists {
			return op, true
		}
	}

	// If no exact match, try pattern matching
	for path, methodMap := range opMap {
		if pathMatch(path, url) {
			if op, exists := methodMap[method]; exists {
				return op, true
			}
		}
	}

	return nil, false
}

// pathMatch determines if path pattern matches the URL
// e.g., /registry/{registry_ref} should match /registry/{registry_ref}
func pathMatch(pattern, url string) bool {
	// Convert OpenAPI path pattern to regex pattern
	regexPattern := regexp.MustCompile(`\{([^\}]+)\}`).ReplaceAllString(pattern, "[^/]+")
	regexPattern = "^" + regexPattern + "$"

	// Match with regex
	matches, err := regexp.MatchString(regexPattern, url)
	return err == nil && matches
}

// -------------------------------------------------------------------------
// 3. Helpers
func deriveNames(method, path string) (resource, action string) {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	resource = singular(strings.Split(segments[0], "{")[0])

	switch strings.ToUpper(method) {
	case "GET":
		if strings.Contains(path, "}") {
			action = "get"
		} else {
			action = "list"
		}
	case "POST":
		action = "create"
	case "PUT", "PATCH":
		action = "update"
	case "DELETE":
		action = "delete"
	default:
		action = strings.ToLower(method)
	}
	return
}

func singular(plural string) string {
	if strings.HasSuffix(plural, "ies") {
		return plural[:len(plural)-3] + "y"
	}
	if strings.HasSuffix(plural, "s") {
		return plural[:len(plural)-1]
	}
	return plural
}

var tmpl = template.Must(
	template.New("cmd").
		Funcs(template.FuncMap{
			"title": func(s string) string {
				if len(s) == 0 {
					return s
				}
				return strings.ToUpper(s[:1]) + s[1:]
			},
			"hasArgs": func(args []string) bool {
				return len(args) > 0
			},
		}).
		Parse(`package command

import (
	"errors"

	"github.com/spf13/cobra"
	client "github.com/harness/harness-cli/internal/api/{{ .Pkg }}"
)

// new{{ .Act | title }}{{ .Res | title }}Cmd wires up:
//   hns {{ .Pkg }} {{ .Res }} {{ .Act }}{{ if hasArgs .RequiredArgs }} <args>{{ end }}
func New{{ .Act | title }}{{ .Res | title }}Cmd() *cobra.Command {
	var host string
	var format string
	cmd := &cobra.Command{
		Use:   "{{ .Res }} {{ .Act }}{{ if hasArgs .RequiredArgs }} {{ range .RequiredArgs }}{{ . }} {{ end }}{{ end }}",
		Short: "{{ .Short }}",
		Long: "{{ .Long }}",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Create client
			_, err := client.NewClient(host, nil)
			if err != nil {
				return err
			}

{{ if hasArgs .RequiredArgs }}
			// Validate required arguments
			if len(args) < {{ len .RequiredArgs }} {
				return errors.New("missing required arguments: {{ range .RequiredArgs }}{{ . }} {{ end }}")
			}

{{ end }}			// Call API
			//resp, err := cli.{{ .OpID }}(context.Background(){{ if hasArgs .RequiredArgs }}, args[0]{{ if gt (len .RequiredArgs) 1 }}, args[1:]{{ end }}{{ end }})
			//if err != nil {
			//	return err
			//}

			// Format output based on format flag
			//switch format {
			//case "json":
				// TODO: output JSON here
			//	fmt.Printf("%+v\n", resp)
			//case "table":
				// TODO: format as table
			//	fmt.Printf("%+v\n", resp)
			//default:
			//	fmt.Printf("%+v\n", resp)
			//}
			return nil
		},
	}

	// Common flags
	cmd.Flags().StringVar(&host, "host", "{{ .DefaultHost }}", "service base URL")
	cmd.Flags().StringVar(&format, "format", "table", "output format: table or json")

	// TODO: Add any command-specific flags here

	return cmd
}
`))

// Helper to upper-case first letter in template
func init() { tmpl = tmpl.Funcs(template.FuncMap{"title": strings.Title}) }

// Command represents a CLI command defined in the command.yaml file
type Command struct {
	URL     string `yaml:"url"`
	Method  string `yaml:"method"`
	Command string `yaml:"command"`          // The CLI command format (e.g., "artifact delete <name>")
	Long    string `yaml:"longDescription"`  // The CLI command format (e.g., "artifact delete <name>")
	Short   string `yaml:"shortDescription"` // The CLI command format (e.g., "artifact delete <name>")
}

// CommandFile represents the structure of the command.yaml file
type CommandFile struct {
	Commands []Command `yaml:"commands"`
}

type cmdData struct {
	Pkg, Module        string
	Res, Act           string
	Short, Long        string
	Method, Path, OpID string
	DefaultHost        string
	RequiredArgs       []string // Required arguments from command.yaml
}

func writeCmdFileWithArgs(pkg, res, act string, cmd Command, opID, filename string, requiredArgs []string) {
	var buf bytes.Buffer
	_ = tmpl.Execute(&buf, cmdData{
		Pkg:          pkg,
		Module:       module,
		Res:          res,
		Act:          act,
		Path:         cmd.URL,
		Method:       cmd.Method,
		Short:        cmd.Short,
		Long:         cmd.Long,
		OpID:         opID,
		DefaultHost:  "http://localhost:8080",
		RequiredArgs: requiredArgs,
	})

	src, err := format.Source(buf.Bytes())
	must(err)

	if _, err := os.Stat(filename); os.IsNotExist(err) {
		must(os.WriteFile(filename, src, 0o644))
	} else {
		fmt.Printf("Skipping %s: file already exists\n", filename)
	}
}

func fileExists(path string) bool { _, err := os.Stat(path); return err == nil }

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
