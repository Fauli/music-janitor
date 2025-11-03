package util

import (
	"os"
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
