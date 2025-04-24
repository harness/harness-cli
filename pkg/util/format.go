package util

import (
	"encoding/json"
	"fmt"
)

// FormatOutput formats data based on the specified format
func FormatOutput(data interface{}, format string) error {
	switch format {
	case "json":
		return OutputJSON(data)
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

// OutputJSON outputs data as JSON
func OutputJSON(data interface{}) error {
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(jsonBytes))
	return nil
}
