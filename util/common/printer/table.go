package printer

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/pterm/pterm"
	"github.com/rs/zerolog/log"
)

// ColumnMapping defines a mapping between original field names and display names
type ColumnMapping [][]string

// jsonToTableWithMapping converts JSON string to a table with custom column mapping
func jsonToTableWithMapping(jsonStr string, mapping ColumnMapping) error {
	var rows []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &rows); err != nil {
		return fmt.Errorf("parse json: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}

	// Prepare header and field mapping
	var header []string
	fieldToDisplay := make(map[string]string)

	if len(mapping) > 0 {
		// Use provided mapping for ordering and display names
		for _, m := range mapping {
			if len(m) >= 2 {
				original, display := m[0], m[1]
				header = append(header, display)
				fieldToDisplay[original] = display
			}
		}
	} else {
		// Fall back to alphabetical ordering
		for k := range rows[0] {
			header = append(header, k)
		}
		sort.Strings(header)
	}

	table := pterm.TableData{header} // first row = header

	for _, r := range rows {
		row := make([]string, len(header))

		if len(mapping) > 0 {
			// Use mapping order
			for i, m := range mapping {
				if len(m) >= 2 {
					originalField := m[0]
					val, ok := r[originalField]
					if !ok {
						row[i] = "-" // missing key
						continue
					}
					row[i] = fmt.Sprint(val) // stringify numbers, bools, etc.
				}
			}
		} else {
			// Fall back to alphabetical order
			for i, col := range header {
				val, ok := r[col]
				if !ok {
					row[i] = "-" // missing key
					continue
				}
				row[i] = fmt.Sprint(val) // stringify numbers, bools, etc.
			}
		}

		table = append(table, row)
	}

	return pterm.DefaultTable.
		WithHasHeader().
		WithBoxed(true).
		WithData(table).
		Render()
}

// TableOptions provides configuration for table output
type TableOptions struct {
	// ColumnMapping defines custom column ordering and display names
	// Format: [["originalField", "Display Name"], ...]
	ColumnMapping ColumnMapping

	// PageIndex is the current page number (zero-indexed)
	PageIndex int64

	// PageCount is the total number of pages
	PageCount int64

	// ItemCount is the total number of items
	ItemCount int64

	// ShowPagination determines whether to show pagination info
	ShowPagination bool
}

// DefaultTableOptions returns default configuration for table printing
func DefaultTableOptions() TableOptions {
	return TableOptions{
		ShowPagination: true,
	}
}

// PrintTableWithOptions prints the data in a table format using the provided options
func PrintTableWithOptions(res any, options TableOptions) error {
	mrListJSON, err := json.Marshal(res)
	if err != nil {
		return fmt.Errorf("failed to marshal data to JSON: %w", err)
	}

	err = jsonToTableWithMapping(string(mrListJSON), options.ColumnMapping)
	if err != nil {
		log.Error().Msgf("failed to render table: %v", err)
		return err
	}

	if options.ShowPagination {
		fmt.Printf("Page %d of %d (Total: %d)\n",
			options.PageIndex, options.PageCount, options.ItemCount)
	}

	return nil
}
