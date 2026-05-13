package pkgmgr

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Package type identifiers used in registry URLs and client detection.
const (
	PackageTypeNpm   = "npm"
	PackageTypeMaven = "maven"
	PackageTypePyPI  = "pypi"
	PackageTypeNuGet = "nuget"
)

// Command names for each package manager CLI tool.
const (
	CommandNpm    = "npm"
	CommandMvn    = "mvn"
	CommandPip    = "pip"
	CommandDotnet = "dotnet"
)

type ParsedArgs struct {
	RegistryName string
	NativeArgs   []string
}

func ParseWrappedArgs(args []string) ParsedArgs {
	var result ParsedArgs

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--registry" && i+1 < len(args):
			result.RegistryName = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--registry="):
			result.RegistryName = strings.TrimPrefix(args[i], "--registry=")
		case args[i] == "-v" || args[i] == "--verbose":
			logWriter := zerolog.ConsoleWriter{
				Out:        os.Stderr,
				TimeFormat: time.RFC3339,
				NoColor:    false,
			}
			log.Logger = log.Output(logWriter)
		default:
			result.NativeArgs = append(result.NativeArgs, args[i])
		}
	}

	return result
}

func RunNativeCommand(binary string, args []string) error {
	binPath, err := exec.LookPath(binary)
	if err != nil {
		return fmt.Errorf("%s not found in PATH: %w", binary, err)
	}

	cmd := exec.Command(binPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}
	return nil
}
