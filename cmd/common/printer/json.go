// Package printer provides output formatting utilities for the CLI
package printer

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// JsonOptions provides configuration for the JSON output
type JsonOptions struct {
	// Writer is the output destination (defaults to os.Stdout if nil)
	Writer io.Writer
	// Indent specifies if pretty-printing should be used
	Indent bool
	// IndentPrefix is the prefix used at the beginning of each line in the indented output
	IndentPrefix string
	// IndentSize is the number of spaces used for each indentation level
	IndentSize int
	// PageIndex is the current page number (zero-indexed)
	PageIndex int64
	// PageCount is the total number of pages
	PageCount int64
	// ItemCount is the total number of items
	ItemCount int64
	// ShowPagination determines whether to show pagination info
	ShowPagination bool
}

// DefaultJsonOptions returns standard options for JSON printing
func DefaultJsonOptions() JsonOptions {
	return JsonOptions{
		Writer:         os.Stdout,
		Indent:         true,
		IndentPrefix:   "",
		IndentSize:     2,
		ShowPagination: true,
	}
}

// PrintJsonWithOptions prints the provided data as JSON with the specified options
//
// Example:
//
//	options := printer.DefaultJsonOptions()
//	options.Indent = true
//	options.ShowPagination = true
//	printer.PrintJsonWithOptions(results, options)
func PrintJsonWithOptions(res any, options JsonOptions) error {
	// Use default writer if none provided
	writer := options.Writer
	if writer == nil {
		writer = os.Stdout
	}

	// Create encoder
	encoder := json.NewEncoder(writer)

	// Set indentation if requested
	if options.Indent {
		indent := ""
		for i := 0; i < options.IndentSize; i++ {
			indent += " "
		}
		encoder.SetIndent(options.IndentPrefix, indent)
	}

	// Encode the data
	if err := encoder.Encode(res); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	// Print pagination info if requested
	if options.ShowPagination {
		if _, err := fmt.Fprintf(writer, "Page %d of %d (Total: %d)\n",
			options.PageIndex, options.PageCount, options.ItemCount); err != nil {
			return fmt.Errorf("failed to print pagination info: %w", err)
		}
	}

	return nil
}
