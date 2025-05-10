// Package printer provides output formatting utilities for the CLI
package printer

import (
	"fmt"
	"harness/config"
	"io"
	"os"
)

// PrintOptions combines options for both JSON and table output
type PrintOptions struct {
	// Format specifies the output format ("json" or "table")
	Format string
	// Writer is the output destination (defaults to os.Stdout if nil)
	Writer io.Writer
	// PageIndex is the current page number (zero-indexed)
	PageIndex int64
	// PageCount is the total number of pages
	PageCount int64
	// ItemCount is the total number of items
	ItemCount int64
	// ShowPagination determines if pagination information should be displayed
	ShowPagination bool
	// JsonIndent specifies if JSON should be pretty-printed
	JsonIndent bool
	// ColumnMapping defines custom column ordering and display names for table format
	// Format: [["originalField", "Display Name"], ...]
	ColumnMapping ColumnMapping
}

// DefaultPrintOptions returns standard print options using the global config
func DefaultPrintOptions() PrintOptions {
	return PrintOptions{
		Format:         config.Global.Format,
		Writer:         os.Stdout,
		ShowPagination: true,
		JsonIndent:     true,
	}
}

// Print formats and outputs data based on the global format setting
// This function is backward compatible with existing code
func Print(res any, pageIndex, pageCount, itemCount int64, showPagination bool, mappings [][]string) error {
	options := DefaultPrintOptions()
	options.PageIndex = pageIndex
	options.PageCount = pageCount
	options.ItemCount = itemCount
	options.ShowPagination = showPagination
	options.ColumnMapping = mappings

	return PrintWithOptions(res, options)
}

// PrintWithOptions formats and outputs data using the provided options
func PrintWithOptions(res any, options PrintOptions) error {
	var err error

	if options.Format == "json" {
		// Convert to JsonOptions
		jsonOpts := DefaultJsonOptions()
		jsonOpts.Writer = options.Writer
		jsonOpts.PageIndex = options.PageIndex
		jsonOpts.PageCount = options.PageCount
		jsonOpts.ItemCount = options.ItemCount
		jsonOpts.Indent = options.JsonIndent

		err = PrintJsonWithOptions(res, jsonOpts)
	} else {
		// Convert to TableOptions
		tableOpts := DefaultTableOptions()
		tableOpts.PageIndex = options.PageIndex
		tableOpts.PageCount = options.PageCount
		tableOpts.ItemCount = options.ItemCount
		tableOpts.ShowPagination = options.ShowPagination
		tableOpts.ColumnMapping = options.ColumnMapping

		err = PrintTableWithOptions(res, tableOpts)
	}

	if err != nil {
		// Only print error to stdout if the writer is something else
		if options.Writer != os.Stdout {
			fmt.Println(err)
		}
		return err
	}

	return nil
}
