package command

import (
	"encoding/base64"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func NewExportYAMLCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export-yaml <identity>",
		Short: "Export a load test as YAML",
		Long: `Print a load test's declarative YAML form.

This is the representation the Harness UI shows in its YAML view: the
definition rather than the stored record, so it carries no account identifiers,
timestamps or run history and can be committed to a repository. Use
--output-file to save it.

A JMeter test built from a .zip workspace masks the archive here, so the
exported YAML alone is not enough to recreate that test.`,
		Example: `  hc rt loadtest export-yaml checkout-load
  hc rt loadtest export-yaml checkout-load --output-file ./checkout-load.yaml`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			test, err := sess.client.Get(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if test.YAML == "" {
				return fmt.Errorf("load test %q was returned without a YAML form", args[0])
			}

			// The field is base64 today. Treating a decode failure as "already
			// plain" means this keeps working if that ever stops being true.
			document := []byte(test.YAML)
			if decoded, err := base64.StdEncoding.DecodeString(test.YAML); err == nil {
				document = decoded
			}

			// YAML already, so it bypasses the printer and --format;
			// re-encoding would defeat the point of an export.
			destination, _ := cmd.Flags().GetString("output-file")
			if destination == "" {
				_, err := cmd.OutOrStdout().Write(document)
				return err
			}

			if err := os.WriteFile(destination, document, 0o600); err != nil {
				return fmt.Errorf("writing %q: %w", destination, err)
			}

			fmt.Fprintf(cmd.ErrOrStderr(), "Wrote %d bytes to %s.\n", len(document), destination)
			return nil
		},
	}

	cmd.Flags().String("output-file", "", "Write the YAML to this path instead of stdout")

	return cmd
}
