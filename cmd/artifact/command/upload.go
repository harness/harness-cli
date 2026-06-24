package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/harness/harness-cli/cmd/cmdutils"

	"github.com/inhies/go-bytesize"
	"github.com/spf13/cobra"
)

// NewUploadArtifactCmd uploads files to a Harness Artifact Registry using
// SRC_PATTERN supports:
//
//   - match any characters within one path segment
//     **         match any characters across path segments (recursive)
//     (*)        capture one path segment → referenced as {1}, {2}, … in DEST_PATH
//     (**)       capture the entire remaining path → referenced as {1}, {2}, …
//     ?          match exactly one character
//
// Examples:
//
//	hc artifact upload "*.jar"               my-repo/libs/
//	hc artifact upload "**/*.jar"            my-repo/libs/
//	hc artifact upload "dist/(*)/*.zip"      my-repo/releases/{1}/
//	hc artifact upload "build/(*)/(*).jar"   my-repo/libs/{1}/{2}.jar
//	hc artifact upload "target/(**)"         my-repo/releases/{1}
func NewUploadArtifactCmd(c *cmdutils.Factory) *cobra.Command {
	const expectedArgumentCount = 2
	var packageVersion string

	cmd := &cobra.Command{
		Use:   "upload <SRC_PATH_PATTERN> <REGISTRY/DEST_PATH>",
		Short: "Upload artifact files to a registry using wildcard patterns",
		Long: "Upload one or more artifact files to a Harness Artifact Registry.\n" +
			"SRC_PATH_PATTERN supports wildcards (* ** ? (*) (**)).\n" +
			"Capture groups (*)/(** ) in the source can be referenced as {1}, {2}, … in DEST_PATH.",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != expectedArgumentCount {
				return fmt.Errorf(
					"accepts %d arg(s), received %d\nUsage:\n  %s",
					expectedArgumentCount, len(args), cmd.UseLine(),
				)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			srcPattern := args[0]
			target := args[1]

			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			var uploader Pusher = &GenericUploader{
				SrcPattern: srcPattern,
				Version:    packageVersion,
				PkgClient:  c.PkgHttpClient(),
			}

			registryName, err := uploader.GetRegistryAndPath(target)
			if err != nil {
				return err
			}

			fmt.Printf("Scanning pattern %q ...\n", srcPattern)
			jobs, stats, err := uploader.GetFiles()
			if err != nil {
				return err
			}
			if len(jobs) == 0 {
				return errors.New("no files matched the given pattern")
			}

			fmt.Printf("Found %d file(s) (%s) to upload to registry %q\n",
				stats.FileCount, bytesize.New(float64(stats.TotalBytes)), registryName)

			return uploader.PushFiles(ctx, jobs)
		},
	}

	cmd.Flags().StringVar(&packageVersion, "version", "1.0.0", "version for the artifact")

	return cmd
}
