package fileutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/harness/harness-cli/util/common/errors"
)

// validatePath checks if a path is valid and accessible.
// Returns an error if the path is empty, contains invalid characters,
// or if the parent directory is not accessible.
func validatePath(path string) error {
	if path == "" {
		return errors.NewValidationError("path", "path cannot be empty")
	}

	// Check for invalid characters in path
	if strings.ContainsAny(path, "<>:|?*\\") {
		return errors.NewValidationError("path", "path contains invalid characters")
	}

	// Check if parent directory exists and is accessible
	parent := filepath.Dir(path)
	if parent != "." {
		if _, err := os.Stat(parent); err != nil {
			return errors.NewFileError(parent, "access", err)
		}
	}

	return nil
}

// validateWritePermissions checks if a directory is writable.
// Returns an error if the directory is not writable or if testing
// write permissions fails.
func validateWritePermissions(dir string) error {
	// Create a temporary file to test write permissions
	testFile := filepath.Join(dir, ".write_test")
	f, err := os.OpenFile(testFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		return errors.NewFileError(dir, "write_permission", err)
	}
	f.Close()
	os.Remove(testFile)
	return nil
}

// ResetDir removes a directory if it exists and creates a fresh empty one.
// It validates the path and checks write permissions before proceeding.
func ResetDir(path string) error {
	// Validate path
	if err := validatePath(path); err != nil {
		return err
	}

	// Remove existing directory if it exists
	if err := os.RemoveAll(path); err != nil {
		return errors.NewFileError(path, "remove", err)
	}

	// Create new directory
	if err := os.MkdirAll(path, 0755); err != nil {
		return errors.NewFileError(path, "create", err)
	}

	// Verify write permissions
	if err := validateWritePermissions(path); err != nil {
		return err
	}

	return nil
}

// ReadFile reads the entire file and returns its contents.
// It validates the path and checks if the file exists and is readable.
func ReadFile(path string) ([]byte, error) {
	// Validate path
	if err := validatePath(path); err != nil {
		return nil, err
	}

	// Check if file exists and is readable
	info, err := os.Stat(path)
	if err != nil {
		return nil, errors.NewFileError(path, "stat", err)
	}
	if info.IsDir() {
		return nil, errors.NewValidationError("path", "path is a directory, expected a file")
	}

	// Read file contents
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.NewFileError(path, "read", err)
	}
	return data, nil
}

// WriteFile writes data to a file, creating it if necessary.
// It validates the path, creates parent directories if needed,
// and verifies write permissions before writing.
func WriteFile(path string, data []byte) error {
	// Validate path
	if err := validatePath(path); err != nil {
		return err
	}

	// Create parent directories if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.NewFileError(path, "create_dir", err)
	}

	// Verify write permissions
	if err := validateWritePermissions(dir); err != nil {
		return err
	}

	// Write file contents
	if err := os.WriteFile(path, data, 0644); err != nil {
		return errors.NewFileError(path, "write", err)
	}
	return nil
}

// CopyFile copies a file from src to dst.
// It validates both paths, ensures the source exists and is readable,
// creates parent directories if needed, and verifies write permissions.
func CopyFile(src, dst string) error {
	// Validate source path
	if err := validatePath(src); err != nil {
		return err
	}

	// Validate destination path
	if err := validatePath(dst); err != nil {
		return err
	}

	// Check if source exists and is readable
	srcInfo, err := os.Stat(src)
	if err != nil {
		return errors.NewFileError(src, "stat", err)
	}
	if srcInfo.IsDir() {
		return errors.NewValidationError("src", "source path is a directory, expected a file")
	}

	// Create parent directories if needed
	dstDir := filepath.Dir(dst)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return errors.NewFileError(dst, "create_dir", err)
	}

	// Verify write permissions
	if err := validateWritePermissions(dstDir); err != nil {
		return err
	}

	// Open source file
	srcFile, err := os.Open(src)
	if err != nil {
		return errors.NewFileError(src, "open", err)
	}
	defer srcFile.Close()

	// Create destination file
	dstFile, err := os.Create(dst)
	if err != nil {
		return errors.NewFileError(dst, "create", err)
	}
	defer dstFile.Close()

	// Copy file contents
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return errors.NewFileError(dst, "copy", err)
	}
	return nil
}

// Exists checks if a file or directory exists
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// IsDir checks if the path is a directory
func IsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// IsFile checks if the path is a regular file
func IsFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

/*
 * Check for Empty filename
 * Check for correctness of file name for provided extension
 */
func IsFilenameAcceptable(fileName, extension string) (bool, error) {
	if fileName == "" {
		return false, fmt.Errorf("empty filename")
	}

	name := fileName
	if strings.HasSuffix(name, extension) {
		return true, nil
	}
	//in case of file is having other  extension than provided extension
	return false, fmt.Errorf("unsupported extension: %s", filepath.Ext(name))
}
