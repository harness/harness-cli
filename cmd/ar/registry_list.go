package ar

import (
	"github.com/spf13/cobra"
)

func newRegistryListCmd() *cobra.Command {
	var host string
	c := &cobra.Command{
		Use:   "registry list",
		Short: "Custom implementation of GET /registries",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	c.Flags().StringVar(&host, "host", "http://localhost:8080", "service base URL")
	return c
}

func init() { ServiceCmd.AddCommand(newRegistryListCmd()) }
