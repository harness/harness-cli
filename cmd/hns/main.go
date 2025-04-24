package main

import (
	"fmt"
	"os"

	"github.com/harness/harness-cli/pkg/services/ar"
	"github.com/spf13/cobra"
)

var (
	outputFormat string
	rootCmd      = &cobra.Command{
		Use:   "hns",
		Short: "Harness CLI",
		Long:  `A CLI tool for interacting with Harness services`,
	}
)

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&outputFormat, "format", "table", "Output format (table, json)")
	
	// Add services
	rootCmd.AddCommand(ar.NewARCommand())
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}
