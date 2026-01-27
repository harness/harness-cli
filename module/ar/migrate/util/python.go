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
		return splits[len(splits)-1]
	case ".exe":
		match := exeRegex.FindStringSubmatch(filename)
		if len(match) > 1 {
			return match[1]
		}
		return splits[len(splits)-1]
	default:
		return ""
	}
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
