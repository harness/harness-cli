package errors

import (
	"errors"
	"fmt"
)

// Common errors that can be used across packages
var (
	ErrNotFound         = errors.New("resource not found")
	ErrInvalidArgument  = errors.New("invalid argument")
	ErrInvalidOperation = errors.New("invalid operation")
	ErrUnauthorized     = errors.New("unauthorized")
	ErrInternal         = errors.New("internal error")
)

// ValidationError represents an error that occurs during validation
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed for %s: %s", e.Field, e.Message)
}

// NewValidationError creates a new ValidationError
func NewValidationError(field, message string) error {
	return &ValidationError{
		Field:   field,
		Message: message,
	}
}

// FileError represents an error that occurs during file operations
type FileError struct {
	Path    string
	Op      string
	Wrapped error
}

func (e *FileError) Error() string {
	if e.Wrapped != nil {
		return fmt.Sprintf("%s operation failed on %s: %v", e.Op, e.Path, e.Wrapped)
	}
	return fmt.Sprintf("%s operation failed on %s", e.Op, e.Path)
}

func (e *FileError) Unwrap() error {
	return e.Wrapped
}

// NewFileError creates a new FileError
func NewFileError(path, op string, wrapped error) error {
	return &FileError{
		Path:    path,
		Op:      op,
		Wrapped: wrapped,
	}
}

// VCSError represents an error that occurs during version control operations
type VCSError struct {
	Op      string
	Path    string
	Wrapped error
}

func (e *VCSError) Error() string {
	if e.Wrapped != nil {
		return fmt.Sprintf("VCS %s operation failed for %s: %v", e.Op, e.Path, e.Wrapped)
	}
	return fmt.Sprintf("VCS %s operation failed for %s", e.Op, e.Path)
}

func (e *VCSError) Unwrap() error {
	return e.Wrapped
}

// NewVCSError creates a new VCSError
func NewVCSError(op, path string, wrapped error) error {
	return &VCSError{
		Op:      op,
		Path:    path,
		Wrapped: wrapped,
	}
}

// PackageError represents an error that occurs during package operations
type PackageError struct {
	Op      string
	Package string
	Version string
	Wrapped error
}

func (e *PackageError) Error() string {
	if e.Version != "" {
		if e.Wrapped != nil {
			return fmt.Sprintf("package %s operation failed for %s@%s: %v", e.Op, e.Package, e.Version, e.Wrapped)
		}
		return fmt.Sprintf("package %s operation failed for %s@%s", e.Op, e.Package, e.Version)
	}
	if e.Wrapped != nil {
		return fmt.Sprintf("package %s operation failed for %s: %v", e.Op, e.Package, e.Wrapped)
	}
	return fmt.Sprintf("package %s operation failed for %s", e.Op, e.Package)
}

func (e *PackageError) Unwrap() error {
	return e.Wrapped
}

// NewPackageError creates a new PackageError
func NewPackageError(op, pkg, version string, wrapped error) error {
	return &PackageError{
		Op:      op,
		Package: pkg,
		Version: version,
		Wrapped: wrapped,
	}
}

// Is reports whether target matches err.
// It enables errors.Is() to work with our custom error types.
func Is(err, target error) bool {
	return errors.Is(err, target)
}

// As finds the first error in err's chain that matches target.
// It enables errors.As() to work with our custom error types.
func As(err error, target interface{}) bool {
	return errors.As(err, target)
}

// Wrap wraps an error with additional context
func Wrap(err error, message string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}
