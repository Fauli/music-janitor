//go:build !linux && !darwin
// +build !linux,!darwin

package util

import "syscall"

// detectPlatformNetwork is a stub for unsupported platforms
func detectPlatformNetwork(path string, stat *syscall.Statfs_t) (*NetworkInfo, error) {
	// Unsupported platform - assume local filesystem
	return &NetworkInfo{
		IsNetwork: false,
		Protocol:  "",
		MountPath: "",
	}, nil
}
