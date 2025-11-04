package util

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"syscall"
	"time"
)

// RetryConfig holds retry configuration
type RetryConfig struct {
	MaxAttempts int           // Maximum number of retry attempts
	InitialWait time.Duration // Initial wait duration (will be doubled each retry)
	MaxWait     time.Duration // Maximum wait duration between retries
}

// DefaultRetryConfig returns the default retry configuration
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts: 3,
		InitialWait: 100 * time.Millisecond,
		MaxWait:     5 * time.Second,
	}
}

// NASRetryConfig returns retry config optimized for NAS operations
func NASRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts: 3,
		InitialWait: 200 * time.Millisecond,
		MaxWait:     10 * time.Second,
	}
}

// IsRetryableError checks if an error is worth retrying
// Returns true for transient network/filesystem errors
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for specific retryable error types
	var pathError *os.PathError
	var linkError *os.LinkError
	var syscallError syscall.Errno

	// Unwrap PathError and LinkError
	if errors.As(err, &pathError) {
		err = pathError.Err
	}
	if errors.As(err, &linkError) {
		err = linkError.Err
	}

	// Check for retryable syscall errors
	if errors.As(err, &syscallError) {
		switch syscallError {
		case syscall.EAGAIN,      // Resource temporarily unavailable
			syscall.ETIMEDOUT,    // Connection timed out
			syscall.ECONNRESET,   // Connection reset
			syscall.ECONNABORTED, // Connection aborted
			syscall.ECONNREFUSED, // Connection refused
			syscall.ENETDOWN,     // Network is down
			syscall.ENETUNREACH,  // Network unreachable
			syscall.EHOSTDOWN,    // Host is down
			syscall.EHOSTUNREACH, // Host unreachable
			syscall.EIO:          // I/O error (can be transient on network)
			return true
		}
	}

	// Check error messages for common transient patterns
	errMsg := strings.ToLower(err.Error())
	transientPatterns := []string{
		"timeout",
		"timed out",
		"connection reset",
		"connection refused",
		"connection aborted",
		"broken pipe",
		"no route to host",
		"network is unreachable",
		"network is down",
		"host is down",
		"temporary failure",
		"resource temporarily unavailable",
		"i/o error",
		"too many open files", // Can be transient if files are being closed
	}

	for _, pattern := range transientPatterns {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}

	return false
}

// RetryWithBackoff executes a function with exponential backoff retry logic
// Returns the result of the function or the final error after all retries exhausted
func RetryWithBackoff[T any](cfg *RetryConfig, operation func() (T, error), operationName string) (T, error) {
	var result T
	var err error

	if cfg == nil {
		cfg = DefaultRetryConfig()
	}

	waitDuration := cfg.InitialWait

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		// Execute the operation
		result, err = operation()

		// Success - return immediately
		if err == nil {
			if attempt > 1 {
				DebugLog("Retry: %s succeeded on attempt %d/%d",
					operationName, attempt, cfg.MaxAttempts)
			}
			return result, nil
		}

		// Check if error is retryable
		if !IsRetryableError(err) {
			// Non-retryable error - fail immediately
			DebugLog("Retry: %s failed with non-retryable error: %v", operationName, err)
			return result, err
		}

		// Last attempt - return the error
		if attempt == cfg.MaxAttempts {
			WarnLog("Retry: %s failed after %d attempts: %v",
				operationName, cfg.MaxAttempts, err)
			return result, fmt.Errorf("max retries exceeded (%d attempts): %w",
				cfg.MaxAttempts, err)
		}

		// Log retry attempt
		DebugLog("Retry: %s failed (attempt %d/%d), retrying in %v: %v",
			operationName, attempt, cfg.MaxAttempts, waitDuration, err)

		// Wait before retry (exponential backoff)
		time.Sleep(waitDuration)

		// Double the wait time for next retry (exponential backoff)
		waitDuration *= 2
		if waitDuration > cfg.MaxWait {
			waitDuration = cfg.MaxWait
		}
	}

	// Should never reach here, but return error just in case
	return result, fmt.Errorf("unexpected retry loop exit: %w", err)
}

// Retry executes a function with retry logic (no return value)
// Convenience wrapper for operations that don't return a value
func Retry(cfg *RetryConfig, operation func() error, operationName string) error {
	_, err := RetryWithBackoff(cfg, func() (struct{}, error) {
		return struct{}{}, operation()
	}, operationName)
	return err
}

// RetryableOpen opens a file with retry logic
func RetryableOpen(path string, cfg *RetryConfig) (*os.File, error) {
	return RetryWithBackoff(cfg, func() (*os.File, error) {
		return os.Open(path)
	}, fmt.Sprintf("open(%s)", path))
}

// RetryableCreate creates a file with retry logic
func RetryableCreate(path string, cfg *RetryConfig) (*os.File, error) {
	return RetryWithBackoff(cfg, func() (*os.File, error) {
		return os.Create(path)
	}, fmt.Sprintf("create(%s)", path))
}

// RetryableStat stats a file with retry logic
func RetryableStat(path string, cfg *RetryConfig) (fs.FileInfo, error) {
	return RetryWithBackoff(cfg, func() (fs.FileInfo, error) {
		return os.Stat(path)
	}, fmt.Sprintf("stat(%s)", path))
}

// RetryableRemove removes a file with retry logic
func RetryableRemove(path string, cfg *RetryConfig) error {
	return Retry(cfg, func() error {
		return os.Remove(path)
	}, fmt.Sprintf("remove(%s)", path))
}

// RetryableRename renames a file with retry logic
func RetryableRename(oldpath, newpath string, cfg *RetryConfig) error {
	return Retry(cfg, func() error {
		return os.Rename(oldpath, newpath)
	}, fmt.Sprintf("rename(%s -> %s)", oldpath, newpath))
}

// RetryableMkdirAll creates a directory with retry logic
func RetryableMkdirAll(path string, perm os.FileMode, cfg *RetryConfig) error {
	return Retry(cfg, func() error {
		return os.MkdirAll(path, perm)
	}, fmt.Sprintf("mkdir(%s)", path))
}
