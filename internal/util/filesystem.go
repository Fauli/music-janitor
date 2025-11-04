package util

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

// IsSameFilesystem checks if two paths are on the same filesystem
// by comparing their device IDs (st_dev).
// Returns (true, nil) if on same filesystem
// Returns (false, nil) if on different filesystems
// Returns (false, err) if paths cannot be stat'd
func IsSameFilesystem(path1, path2 string) (bool, error) {
	stat1, err := os.Stat(path1)
	if err != nil {
		return false, err
	}

	stat2, err := os.Stat(path2)
	if err != nil {
		return false, err
	}

	// Cast to syscall.Stat_t to access device ID
	sysStat1, ok1 := stat1.Sys().(*syscall.Stat_t)
	sysStat2, ok2 := stat2.Sys().(*syscall.Stat_t)

	if !ok1 || !ok2 {
		// If we can't get syscall.Stat_t, assume different filesystems
		// (better to warn when unsure)
		return false, nil
	}

	return sysStat1.Dev == sysStat2.Dev, nil
}

// DetectFilesystemCaseSensitivity detects if a filesystem is case-sensitive
// by attempting to create test files with different casing
func DetectFilesystemCaseSensitivity(path string) (bool, error) {
	// Quick checks for known platforms
	switch runtime.GOOS {
	case "windows":
		// Windows filesystems are always case-insensitive (NTFS, FAT32)
		return false, nil
	case "darwin":
		// macOS: depends on filesystem (APFS can be either, HFS+ usually case-insensitive)
		// Need to test by creating temp files
		break
	case "linux":
		// Linux: depends on filesystem, but ext4/xfs/btrfs are typically case-sensitive
		// However, SMB/CIFS mounts may be case-insensitive
		break
	}

	// Test by creating temp files with different casing
	// Create a temp directory for testing
	testDir := filepath.Join(path, ".mlc-case-test")

	// Clean up any existing test directory
	os.RemoveAll(testDir)

	// Create test directory
	if err := os.MkdirAll(testDir, 0755); err != nil {
		// Can't create test directory, fall back to OS defaults
		return runtime.GOOS == "linux", nil
	}
	defer os.RemoveAll(testDir)

	// Try to create two files with same name but different casing
	testFile1 := filepath.Join(testDir, "TestFile.tmp")
	testFile2 := filepath.Join(testDir, "testfile.tmp")

	// Create first file
	f1, err := os.Create(testFile1)
	if err != nil {
		// Can't create file, fall back to OS defaults
		return runtime.GOOS == "linux", nil
	}
	f1.Close()

	// Check if second file exists (case-insensitive filesystem)
	if _, err := os.Stat(testFile2); err == nil {
		// File exists with different casing -> case-insensitive
		return false, nil
	}

	// Try to create second file
	f2, err := os.Create(testFile2)
	if err != nil {
		// Can't create second file -> case-insensitive (first file blocks it)
		return false, nil
	}
	f2.Close()

	// Both files exist -> case-sensitive
	return true, nil
}

// NormalizePath normalizes a path for comparison on case-insensitive filesystems
// Returns lowercase path for case-insensitive systems, original for case-sensitive
func NormalizePath(path string, caseSensitive bool) string {
	if caseSensitive {
		return filepath.Clean(path)
	}
	return strings.ToLower(filepath.Clean(path))
}

// PathsEqual compares two paths, respecting filesystem case sensitivity
func PathsEqual(path1, path2 string, caseSensitive bool) bool {
	return NormalizePath(path1, caseSensitive) == NormalizePath(path2, caseSensitive)
}
