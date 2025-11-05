//go:build darwin
// +build darwin

package util

import (
	"strings"
	"syscall"
)

// detectPlatformNetwork detects network filesystems on macOS
func detectPlatformNetwork(path string, stat *syscall.Statfs_t) (*NetworkInfo, error) {
	info := &NetworkInfo{
		IsNetwork: false,
	}

	// On macOS, we can check the filesystem type name from statfs
	// Convert int8 array to string (null-terminated)
	fsTypeName := int8ArrayToString(stat.Fstypename[:])
	fsTypeName = strings.ToLower(fsTypeName)

	// Check for network filesystem types
	networkTypes := []string{"nfs", "smbfs", "afpfs", "cifs", "webdav", "osxfuse"}
	for _, netType := range networkTypes {
		if strings.Contains(fsTypeName, netType) {
			info.IsNetwork = true
			info.Protocol = fsTypeName

			// Get mount point
			mntFromName := int8ArrayToString(stat.Mntonname[:])
			info.MountPath = mntFromName
			break
		}
	}

	return info, nil
}

// int8ArrayToString converts a null-terminated int8 array to a Go string
func int8ArrayToString(arr []int8) string {
	// Find the null terminator
	n := 0
	for n < len(arr) && arr[n] != 0 {
		n++
	}

	// Convert to bytes then string
	bytes := make([]byte, n)
	for i := 0; i < n; i++ {
		bytes[i] = byte(arr[i])
	}
	return string(bytes)
}
