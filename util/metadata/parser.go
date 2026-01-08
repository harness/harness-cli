package metadata

import (
	"fmt"
	"strings"

	"github.com/harness/harness-cli/internal/api/ar_v2"
)

func ParseMetadataString(metadataStr string) ([]ar_v2.MetadataItemInput, error) {
	if metadataStr == "" {
		return nil, fmt.Errorf("metadata string cannot be empty")
	}

	var items []ar_v2.MetadataItemInput
	pairs := strings.Split(metadataStr, ",")

	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		parts := strings.SplitN(pair, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("metadata must be in key:value format, got: %s", pair)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if key == "" {
			return nil, fmt.Errorf("metadata key cannot be empty in pair: %s", pair)
		}

		if value == "" {
			return nil, fmt.Errorf("metadata value cannot be empty in pair: %s", pair)
		}

		if strings.Contains(key, ":") {
			return nil, fmt.Errorf("metadata key cannot contain ':' character, got key: %s", key)
		}

		items = append(items, ar_v2.MetadataItemInput{
			Key:   key,
			Value: value,
		})
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no valid metadata key-value pairs found")
	}

	return items, nil
}

func FormatMetadataOutput(items []ar_v2.MetadataItemOutput) string {
	if len(items) == 0 {
		return "No metadata found"
	}

	var sb strings.Builder
	for i, item := range items {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("%s: %s", item.Key, item.Value))
	}
	return sb.String()
}
