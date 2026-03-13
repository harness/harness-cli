package util

import (
	"errors"
	"regexp"
	"strings"
)

var (
	extensions = []string{
		".tar.gz",
		".tar.bz2",
		".tar.xz",
		".zip",
		".whl",
		".egg",

		".exe",
		".app",
		".dmg",
	}

	exeRegex = regexp.MustCompile(`(\d+(?:\.\d+)+)`)
)

func GetPyPIVersion(filename string) string {
	base, ext, err := stripRecognizedExtension(filename)
	if err != nil {
		return ""
	}

	splits := strings.Split(base, "-")
	if len(splits) < 2 {
		return ""
	}

	switch ext {
	case ".whl", ".egg":
		return splits[1]
	case ".tar.gz", ".tar.bz2", ".tar.xz", ".zip", ".dmg", ".app":
		// Source distribution format: {name}-{version}.ext
		// The version starts at the first segment that begins with a digit (or 'v'/'V' + digit).
		// We join all remaining segments to handle versions containing hyphens (e.g., "4.0-b3").
		if idx := findVersionStart(splits); idx > 0 {
			return strings.Join(splits[idx:], "-")
		}
		return splits[len(splits)-1]
	case ".exe":
		match := exeRegex.FindStringSubmatch(filename)
		if len(match) > 1 {
			return match[1]
		}
		if idx := findVersionStart(splits); idx > 0 {
			return strings.Join(splits[idx:], "-")
		}
		return splits[len(splits)-1]
	default:
		return ""
	}
}

func findVersionStart(segments []string) int {
	for i := 1; i < len(segments); i++ {
		s := segments[i]
		if len(s) > 0 && s[0] >= '0' && s[0] <= '9' {
			return i
		}
		if len(s) > 1 && (s[0] == 'v' || s[0] == 'V') && s[1] >= '0' && s[1] <= '9' {
			return i
		}
	}
	return -1
}

func stripRecognizedExtension(filename string) (string, string, error) {
	for _, x := range extensions {
		if strings.HasSuffix(strings.ToLower(filename), x) {
			base := filename[:len(filename)-len(x)]
			return base, x, nil
		}
	}

	return "", "", errors.New("unrecognized file extension")
}
