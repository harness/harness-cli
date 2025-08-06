package util

import (
	"fmt"
	"github.com/pterm/pterm"
	"os"
	"strings"
)

func GenOCIImagePath(host string, pathParams ...string) string {
	params := strings.Join(pathParams, "/")
	return fmt.Sprintf("%s/%s", host, params)
}

func GetRegistryRef(account string, ref string, registry string) string {
	result := []string{account}
	ref = strings.TrimSuffix(ref, "/")
	ref = strings.TrimPrefix(ref, "/")
	registry = strings.TrimPrefix(registry, "/")
	registry = strings.TrimSuffix(registry, "/")
	if ref != "" {
		result = append(result, ref)
	}
	result = append(result, registry)
	return strings.Join(result, "/")
}

func GetSkipPrinter() *pterm.PrefixPrinter {
	return &pterm.PrefixPrinter{
		MessageStyle: &pterm.ThemeDefault.WarningMessageStyle,
		Prefix: pterm.Prefix{
			Style: &pterm.ThemeDefault.WarningPrefixStyle,
			Text:  "SKIPPED",
		},
		Writer: os.Stdout,
	}
}
