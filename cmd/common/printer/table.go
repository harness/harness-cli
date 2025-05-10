package printer

import (
	"encoding/json"
	"fmt"
	"github.com/pterm/pterm"
	"github.com/rs/zerolog/log"
	"sort"
)

func jsonToTable(jsonStr string) error {
	var rows []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &rows); err != nil {
		return fmt.Errorf("parse json: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}

	// Get a stable header order (lexicographic)
	header := make([]string, 0, len(rows[0]))
	for k := range rows[0] {
		header = append(header, k)
	}
	sort.Strings(header)

	table := pterm.TableData{header} // first row = header

	for _, r := range rows {
		row := make([]string, len(header))
		for i, col := range header {
			val, ok := r[col]
			if !ok {
				row[i] = "-" // missing key
				continue
			}
			row[i] = fmt.Sprint(val) // stringify numbers, bools, etc.
		}
		table = append(table, row)
	}

	pterm.SetForcedTerminalSize(300, 40)    // width, height :contentReference[oaicite:0]{index=0}
	defer pterm.SetForcedTerminalSize(0, 0) // restore auto-detection later

	return pterm.DefaultTable.
		WithHasHeader().
		WithBoxed(true).
		WithData(table).
		Render()
}

func PrintTable(res any, pageIndex, pageCount, itemCount int64) error {
	mrListJSON, _ := json.Marshal(res)
	err := jsonToTable(string(mrListJSON))
	if err != nil {
		log.Error().Msgf("failed to render table: %v", err)
		return err
	}

	fmt.Printf("Page %d of %d (Total: %d)\n",
		pageIndex, pageCount, itemCount)
	
	return nil
}
