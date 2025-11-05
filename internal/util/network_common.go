package util

import (
	"fmt"
	"path/filepath"
	"syscall"
)

// NetworkInfo contains information about a filesystem's network characteristics
type NetworkInfo struct {
	IsNetwork bool   // Whether the filesystem is network-mounted
	Protocol  string // Protocol (smb, nfs, cifs, etc.) or empty if local
	MountPath string // Mount point of the filesystem
}

// DetectNetworkFilesystem checks if a path is on a network-mounted filesystem
// Supports SMB/CIFS, NFS on both Linux and macOS
func DetectNetworkFilesystem(path string) (*NetworkInfo, error) {
	// Get absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Get filesystem info
	var stat syscall.Statfs_t
	if err := syscall.Statfs(absPath, &stat); err != nil {
		return nil, fmt.Errorf("failed to stat filesystem: %w", err)
	}

	// Call platform-specific detection
	return detectPlatformNetwork(absPath, &stat)
}

// IsNetworkPath checks if a path is on a network filesystem (convenience function)
func IsNetworkPath(path string) bool {
	info, err := DetectNetworkFilesystem(path)
	if err != nil {
		return false
	}
	return info.IsNetwork
}
