package util

import (
	"crypto/sha1"
	"fmt"
	"io"
	"os"
	"syscall"
)

// GenerateFileKey creates a stable key for a file based on its filesystem metadata
// Key is SHA1 of (dev, inode, size, mtime) for fast comparison
// This allows detecting file moves/renames without reading content
func GenerateFileKey(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("failed to stat file: %w", err)
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		// Fallback: use size and mtime only (less precise but portable)
		return GenerateSimpleFileKey(info.Size(), info.ModTime().Unix()), nil
	}

	// Combine filesystem metadata into a stable key
	h := sha1.New()
	fmt.Fprintf(h, "%d:%d:%d:%d", stat.Dev, stat.Ino, info.Size(), info.ModTime().Unix())
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// GenerateSimpleFileKey creates a key from size and mtime only (portable fallback)
func GenerateSimpleFileKey(size int64, mtimeUnix int64) string {
	h := sha1.New()
	fmt.Fprintf(h, "%d:%d", size, mtimeUnix)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// GenerateContentHash creates a SHA1 hash of file content
// Used for verification and winner selection
func GenerateContentHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("failed to hash file: %w", err)
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// GetFileMetadata extracts basic filesystem metadata
func GetFileMetadata(path string) (size int64, mtime int64, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to stat file: %w", err)
	}

	return info.Size(), info.ModTime().Unix(), nil
}
