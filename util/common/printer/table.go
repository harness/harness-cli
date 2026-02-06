package printer

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/charmbracelet/lipgloss"
	lgtable "github.com/charmbracelet/lipgloss/table"
	"github.com/harness/harness-cli/internal/style"
	"github.com/pterm/pterm"
	"github.com/rs/zerolog/log"
)

// ColumnMapping defines a mapping between original field names and display names
type ColumnMapping [][]string

// parseTableData converts a JSON string + column mapping into headers and string rows.
func parseTableData(jsonStr string, mapping ColumnMapping) ([]string, [][]string, error) {
	var rows []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &rows); err != nil {
		return nil, nil, fmt.Errorf("parse json: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil, nil
	}

	// Prepare header and field mapping
	var header []string

	if len(mapping) > 0 {
		for _, m := range mapping {
			if len(m) >= 2 {
				header = append(header, m[1])
			}
		}
	} else {
		for k := range rows[0] {
			header = append(header, k)
		}
		sort.Strings(header)
	}

	var tableRows [][]string
	for _, r := range rows {
		row := make([]string, len(header))
		if len(mapping) > 0 {
			for i, m := range mapping {
				if len(m) >= 2 {
					val, ok := r[m[0]]
					if !ok {
						row[i] = "-"
						continue
					}
					row[i] = fmt.Sprint(val)
				}
			}
		} else {
			for i, col := range header {
				val, ok := r[col]
				if !ok {
					row[i] = "-"
					continue
				}
				row[i] = fmt.Sprint(val)
			}
		}
		tableRows = append(tableRows, row)
	}

	return header, tableRows, nil
}

// renderStyledTable renders a table using lipgloss/table with the project's colour theme.
func renderStyledTable(headers []string, rows [][]string) {
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(style.Cyan).
		Padding(0, 1)

	cellStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FAFAFA")).
		Padding(0, 1)

	dimCellStyle := lipgloss.NewStyle().
		Foreground(style.Dim).
		Padding(0, 1)

	t := lgtable.New().
		Headers(headers...).
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(style.Subtle)).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == lgtable.HeaderRow {
				return headerStyle
			}
			if row%2 == 0 {
				return cellStyle
			}
			return dimCellStyle
		})

	for _, r := range rows {
		t = t.Row(r...)
	}

	fmt.Println(t.Render())
}

// renderPtermTable renders a table using the legacy pterm renderer (for non-TTY / no-color).
func renderPtermTable(headers []string, rows [][]string) error {
	data := pterm.TableData{headers}
	for _, r := range rows {
		data = append(data, r)
	}
	return pterm.DefaultTable.
		WithHasHeader().
		WithBoxed(true).
		WithData(data).
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

// PrintTableWithOptions prints the data in a table format using the provided options.
// When colour is enabled (TTY), it renders using lipgloss/table with the project theme.
// Otherwise it falls back to the pterm boxed table for plain-text environments.
func PrintTableWithOptions(res any, options TableOptions) error {
	mrListJSON, err := json.Marshal(res)
	if err != nil {
		return fmt.Errorf("failed to marshal data to JSON: %w", err)
	}

	headers, rows, err := parseTableData(string(mrListJSON), options.ColumnMapping)
	if err != nil {
		log.Error().Msgf("failed to parse table data: %v", err)
		return err
	}

	if headers == nil {
		// No data to display
		return nil
	}

	if style.Enabled {
		renderStyledTable(headers, rows)
	} else {
		if err := renderPtermTable(headers, rows); err != nil {
			log.Error().Msgf("failed to render table: %v", err)
			return err
		}
	}

	if options.ShowPagination {
		if style.Enabled {
			fmt.Println(lipgloss.NewStyle().Foreground(style.Dim).Render(
				fmt.Sprintf("Page %d of %d (Total: %d)",
					options.PageIndex, options.PageCount, options.ItemCount)))
		} else {
			fmt.Printf("Page %d of %d (Total: %d)\n",
				options.PageIndex, options.PageCount, options.ItemCount)
		}
	}

	return nil
}
