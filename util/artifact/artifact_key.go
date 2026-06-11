package artifact

import (
	"fmt"
	"strings"
)

// ArtifactKey represents a generic set of key-value pairs that uniquely identify an artifact
// This is package-type agnostic and can be used for any artifact registry type
type ArtifactKey map[string]string

// ParseArtifactKeyString parses a comma-separated key=value string into an ArtifactKey
// Example: "architecture=amd64,distribution=focal,component=main"
// Any key names are accepted - no validation is performed on key names
func ParseArtifactKeyString(keyStr string) (ArtifactKey, error) {
	if keyStr == "" {
		return nil, nil
	}

	key := make(ArtifactKey)
	pairs := strings.Split(keyStr, ",")

	for _, pair := range pairs {
		kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid artifact key format: expected key=value, got '%s'", pair)
		}

		fieldName := strings.TrimSpace(kv[0])
		fieldValue := strings.TrimSpace(kv[1])

		if fieldName == "" {
			return nil, fmt.Errorf("empty key name in pair '%s'", pair)
		}

		if fieldValue == "" {
			return nil, fmt.Errorf("empty value for key '%s'", fieldName)
		}

		key[fieldName] = fieldValue
	}

	return key, nil
}

// String returns a formatted string representation
func (ak ArtifactKey) String() string {
	if len(ak) == 0 {
		return ""
	}

	var parts []string
	for key, value := range ak {
		parts = append(parts, fmt.Sprintf("%s=%s", key, value))
	}
	return strings.Join(parts, ",")
}

// IsEmpty returns true if the artifact key has no entries
func (ak ArtifactKey) IsEmpty() bool {
	return len(ak) == 0
}
