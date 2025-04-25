package main

import (
	"github.com/harness/harness-cli/cmd/ar"
	"github.com/harness/harness-cli/cmd/ci"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "hns",
	Short: "Harness multi-service CLI",
}

func init() {
	// register service-level commands (generated or handwritten)
	rootCmd.AddCommand(ar.NewCommand())
	rootCmd.AddCommand(ci.NewCommand())
}

// Execute runs the CLI root.
func main() { _ = rootCmd.Execute() }
