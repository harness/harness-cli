package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/harness/harness-cli/config"

	"github.com/spf13/cobra"
)

func GetRootCmd() *cobra.Command {
	var method string
	var data string
	var headers []string

	rootCmd := &cobra.Command{
		Use:   "api [path]",
		Short: "Make raw REST API calls to Harness",
		Long: `Make raw REST API calls to Harness. This is a power user feature that allows
you to interact directly with the Harness API.

Examples:
  # GET request
  hc api /har/api/v1/registries

  # POST request with data
  hc api /har/api/v1/registries --method POST --data '{"identifier":"my-registry"}'

  # With custom headers
  hc api /har/api/v1/registries --header "Content-Type: application/json"
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]

			// Ensure path starts with /
			if !strings.HasPrefix(path, "/") {
				path = "/" + path
			}

			// Build the full URL
			url := config.Global.APIBaseURL + "/gateway" + path

			// Default method
			if method == "" {
				if data != "" {
					method = "POST"
				} else {
					method = "GET"
				}
			}

			// Create request
			var body io.Reader
			if data != "" {
				body = bytes.NewBufferString(data)
			}

			req, err := http.NewRequest(method, url, body)
			if err != nil {
				return fmt.Errorf("failed to create request: %w", err)
			}

			// Add auth header
			if config.Global.AuthToken != "" {
				req.Header.Set("x-api-key", config.Global.AuthToken)
			}

			// Add account header
			if config.Global.AccountID != "" {
				req.Header.Set("Harness-Account", config.Global.AccountID)
			}

			// Add custom headers
			for _, h := range headers {
				parts := strings.SplitN(h, ":", 2)
				if len(parts) == 2 {
					req.Header.Set(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
				}
			}

			// Set default content type for POST/PUT/PATCH
			if (method == "POST" || method == "PUT" || method == "PATCH") && req.Header.Get("Content-Type") == "" {
				req.Header.Set("Content-Type", "application/json")
			}

			// Make request
			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("failed to make request: %w", err)
			}
			defer resp.Body.Close()

			// Read response
			respBody, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("failed to read response: %w", err)
			}

			// Print status
			fmt.Printf("HTTP %d %s\n", resp.StatusCode, resp.Status)

			// Pretty print JSON response
			var jsonData interface{}
			if err := json.Unmarshal(respBody, &jsonData); err == nil {
				prettyJSON, err := json.MarshalIndent(jsonData, "", "  ")
				if err == nil {
					fmt.Println(string(prettyJSON))
					return nil
				}
			}

			// If not JSON, print raw
			fmt.Println(string(respBody))

			return nil
		},
	}

	rootCmd.Flags().StringVarP(&method, "method", "X", "", "HTTP method (GET, POST, PUT, DELETE, etc.)")
	rootCmd.Flags().StringVarP(&data, "data", "d", "", "Request body data")
	rootCmd.Flags().StringArrayVarP(&headers, "header", "H", []string{}, "Custom headers (can be specified multiple times)")

	return rootCmd
}
