package command

import (
	"context"

	"github.com/harness/harness-cli/util/common/upload"
)

// UploadStats summarizes the set of files that will be uploaded.
type UploadStats struct {
	FileCount  int
	TotalBytes int64
}

// Pusher defines the two-phase contract for artifact uploads:
//  1. GetFiles  – scan the source and build the list of upload jobs.
//  2. PushFiles – execute those jobs via the shared upload engine.

type Pusher interface {
	// GetFiles to collects all files that match the configured source pattern
	GetFiles() ([]upload.FileUploadJob, UploadStats, error)

	// PushFiles executes the upload for each job in jobs.
	PushFiles(ctx context.Context, jobs []upload.FileUploadJob) error
}
