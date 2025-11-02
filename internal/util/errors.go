package util

import "errors"

// Sentinel errors for common failure modes
var (
	// ErrUnsupported indicates a file format or operation is not supported
	ErrUnsupported = errors.New("unsupported")

	// ErrCorrupt indicates a file is corrupt or unreadable
	ErrCorrupt = errors.New("corrupt file")

	// ErrConflict indicates a destination file conflict
	ErrConflict = errors.New("destination conflict")

	// ErrNotFound indicates a required resource was not found
	ErrNotFound = errors.New("not found")

	// ErrInvalidConfig indicates invalid configuration
	ErrInvalidConfig = errors.New("invalid configuration")

	// ErrPermission indicates a permission error
	ErrPermission = errors.New("permission denied")

	// ErrDiskFull indicates insufficient disk space
	ErrDiskFull = errors.New("disk full")
)
