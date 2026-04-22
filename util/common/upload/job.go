package upload

import (
	"context"
	"io"
)

// FileUploadJob represents a single file upload operation
type FileUploadJob interface {
	// GetID returns a unique identifier for this upload job
	GetID() string

	// GetFilePath returns the file path (for display purposes)
	GetFilePath() string

	// GetFileSize returns the file size in bytes
	GetFileSize() int64

	// Upload performs the actual upload operation
	Upload(ctx context.Context) error
}

// BaseFileUploadJob provides common functionality for file upload jobs
type BaseFileUploadJob struct {
	ID       string
	FilePath string
	FileSize int64
}

func (b *BaseFileUploadJob) GetID() string {
	return b.ID
}

func (b *BaseFileUploadJob) GetFilePath() string {
	return b.FilePath
}

func (b *BaseFileUploadJob) GetFileSize() int64 {
	return b.FileSize
}

// FileUploadResult represents the result of a file upload
type FileUploadResult struct {
	JobID    string
	FilePath string
	FileSize int64
	Error    error
	Success  bool
}

// FileContent represents file content that can be either from disk or in-memory
type FileContent struct {
	Reader     io.Reader
	Size       int64
	IsInMemory bool
	Data       []byte
}

// creates FileContent from a file path
func NewFileContentFromDisk(reader io.Reader, size int64) *FileContent {
	return &FileContent{
		Reader:     reader,
		Size:       size,
		IsInMemory: false,
	}
}

// creates FileContent from in-memory data
func NewFileContentFromMemory(data []byte) *FileContent {
	return &FileContent{
		Reader:     nil,
		Size:       int64(len(data)),
		IsInMemory: true,
		Data:       data,
	}
}
