package upload

import (
	"context"

	"github.com/harness/harness-cli/util/common/upload"
)

// UploadStats summarizes the set of files that will be uploaded.
type UploadStats struct {
	FileCount  int
	TotalBytes int64
}

type Pusher interface {

	// GetRegistryAndPath :- Each implementation  defines its own parsing rules (generic splits on first "/"
	GetRegistryAndPath(target string) (registryName string, err error)

	// GetFiles collects all files that match the configured source pattern
	GetFiles() ([]upload.FileUploadJob, UploadStats, error)

	// PreUpload runs before PushFiles. can be used for dry-run or applying any other filter or validation
	PreUpload(jobs []upload.FileUploadJob) (skip bool, err error)

	// PushFiles executes the upload for each job in jobs.
	PushFiles(ctx context.Context, jobs []upload.FileUploadJob) error
}
