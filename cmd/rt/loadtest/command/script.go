package command

import (
	"encoding/base64"
	"fmt"
	"os"

	"github.com/harness/harness-cli/cmd/rt/loadtest/api"

	"github.com/spf13/cobra"
)

// NewScriptCmd groups the operations on a load test's script and its stored
// revisions.
func NewScriptCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "script",
		Short: "Manage load test scripts and their revisions",
		Long: `Download, replace and inspect the script a load test runs.

Every upload is kept as a numbered revision, so a previous version can always
be recovered. These commands apply to tests in script mode; a test that runs
from a container image is changed with "hc rt loadtest update --image".`,
	}

	cmd.AddCommand(newScriptGetCmd())
	cmd.AddCommand(newScriptUpdateCmd())
	cmd.AddCommand(newScriptListRevisionsCmd())
	cmd.AddCommand(newScriptGetRevisionCmd())

	return cmd
}

func newScriptUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <identity>",
		Short: "Replace the script of a load test",
		Long: `Upload a new script for a load test, creating a new revision.

The previous script is kept as a revision, so a bad upload can be inspected
with "hc rt loadtest script list-revisions" and recovered from. Runs already in
flight keep the script they started with.

This only applies to tests in script mode. For a test that runs from a
container image, publish a new image and point at it with
"hc rt loadtest update --image".`,
		Example: `  hc rt loadtest script update checkout-load --script ./checkout-v2.jmx \
    --description "Add the payment step"`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, _ := cmd.Flags().GetString("script")
			if path == "" {
				return fmt.Errorf("--script is required: pass the path to the new script file")
			}

			encoded, err := encodeScript(path)
			if err != nil {
				return err
			}

			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			description, _ := cmd.Flags().GetString("description")
			revision, err := sess.client.UpdateScript(cmd.Context(), args[0], api.UpdateScriptRequest{
				ScriptContent: encoded,
				Description:   description,
			})
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.ErrOrStderr(), "Uploaded %s as revision %d.\n", path, revision.RevisionNumber)
			return sess.printer.Print(revision, revisionTable([]*api.ScriptRevision{revision}))
		},
	}

	cmd.Flags().String("script", "", "Path to the local script file to upload (required)")
	cmd.Flags().String("description", "", "What changed in this revision")

	return cmd
}

func newScriptGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <identity>",
		Short: "Download the script of a load test",
		Long: `Print the current script of a load test.

The script is decoded and written to stdout, so it can be piped or redirected
to a file. Pass --output-file to write it directly. When the script is a .zip
workspace the archive is binary, so --output-file is required.`,
		Example: `  hc rt loadtest script get checkout-load
  hc rt loadtest script get checkout-load --output-file ./checkout.jmx`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			revision, err := sess.client.GetScript(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			return writeScript(cmd, revision)
		},
	}

	cmd.Flags().String("output-file", "", "Write the script to this path instead of stdout")

	return cmd
}

func newScriptListRevisionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list-revisions <identity>",
		Short: "List the script revisions of a load test",
		Long: `List every stored version of a load test's script, newest first.

Fetch the body of one with "hc rt loadtest script get-revision".`,
		Example:      `  hc rt loadtest script list-revisions checkout-load`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			revisions, err := sess.client.ListScriptRevisions(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			return sess.printer.Print(revisions, revisionTable(revisions))
		},
	}
}

func newScriptGetRevisionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get-revision <identity> <revision>",
		Short: "Download one script revision",
		Long: `Print a specific revision of a load test's script.

The revision argument is the revision number reported by
"hc rt loadtest script list-revisions" — 1 for the version the load test was
created with, counting up from there. The revision identity from the same
listing is accepted too, for scripting against a fixed version.

A revision stored as a zip workspace cannot be printed; pass --output-file to
save it.`,
		Example: `  hc rt loadtest script get-revision checkout-load 3
  hc rt loadtest script get-revision checkout-load 3 --output-file ./old.jmx`,
		Args:         cobra.ExactArgs(2),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			revision, err := sess.client.GetScriptRevision(cmd.Context(), args[0], args[1])
			if err != nil {
				return err
			}

			return writeScript(cmd, revision)
		},
	}

	cmd.Flags().String("output-file", "", "Write the script to this path instead of stdout")

	return cmd
}

// writeScript decodes a stored script and sends it to a file or to stdout.
func writeScript(cmd *cobra.Command, revision *api.ScriptRevision) error {
	decoded, err := base64.StdEncoding.DecodeString(revision.ScriptContent)
	if err != nil {
		return fmt.Errorf("decoding script for revision %s: %w", revision.Identity, err)
	}

	destination, _ := cmd.Flags().GetString("output-file")
	if destination == "" {
		if revision.IsBundle {
			return fmt.Errorf("revision %s is a %s workspace and cannot be printed: pass --output-file to save it",
				revision.Identity, bundleKind(revision))
		}
		_, err := cmd.OutOrStdout().Write(decoded)
		return err
	}

	if err := os.WriteFile(destination, decoded, 0o600); err != nil {
		return fmt.Errorf("writing %q: %w", destination, err)
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "Wrote %d bytes to %s.\n", len(decoded), destination)
	return nil
}

func bundleKind(revision *api.ScriptRevision) string {
	if revision.BundleMainFile != "" {
		return fmt.Sprintf("zip (main plan %s)", revision.BundleMainFile)
	}
	return "zip"
}
