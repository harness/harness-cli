package metadata

import (
	"github.com/harness/harness-cli/internal/api/ar_v2"
	"github.com/harness/harness-cli/util/common/printer"
)

var metadataColumns = [][]string{
	{"key", "Key"},
	{"value", "Value"},
}

func PrintMetadataOutput(items []ar_v2.MetadataItemOutput) error {
	return printer.Print(items, 0, 1, int64(len(items)), false, metadataColumns)
}

func PrintMetadataInput(items []ar_v2.MetadataItemInput) error {
	return printer.Print(items, 0, 1, int64(len(items)), false, metadataColumns)
}
