package util

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

	info := &NetworkInfo{
		IsNetwork: false,
		Protocol:  "",
		MountPath: "",
	}

	// Check filesystem type using platform-specific methods
	switch runtime.GOOS {
	case "linux":
		return detectLinuxNetwork(absPath, &stat)
	case "darwin":
		return detectDarwinNetwork(absPath, &stat)
	default:
		// Unsupported platform - assume local filesystem
		return info, nil
	}
}

// detectLinuxNetwork detects network filesystems on Linux by parsing /proc/mounts
func detectLinuxNetwork(path string, stat *syscall.Statfs_t) (*NetworkInfo, error) {
	info := &NetworkInfo{
		IsNetwork: false,
	}

	// Check common network filesystem types by magic number
	// These are Linux kernel VFS type constants
	networkTypes := map[uint32]string{
		0x6969:       "nfs",       // NFS_SUPER_MAGIC
		0xff534d42:   "cifs",      // CIFS_MAGIC_NUMBER
		0x517b:       "smb",       // SMB_SUPER_MAGIC
		0x01021994:   "smbfs",     // SMBFS_MAGIC (old)
		0x564c:       "ncp",       // NCP_SUPER_MAGIC (Netware)
		0x5346544e:   "ntfs",      // NTFS (might be network)
		0xfe534d42:   "smb2",      // SMB2_MAGIC_NUMBER
	}

	if proto, found := networkTypes[stat.Type]; found {
		info.IsNetwork = true
		info.Protocol = proto
	}

	// Also parse /proc/mounts to get mount point and confirm protocol
	mounts, err := parseProcMounts()
	if err != nil {
		// If we can't read /proc/mounts, rely on magic number check
		return info, nil
	}

	// Find mount point for this path
	bestMatch := ""
	for mountPoint, fsType := range mounts {
		if strings.HasPrefix(path, mountPoint) && len(mountPoint) > len(bestMatch) {
			bestMatch = mountPoint

			// Check if this is a known network filesystem
			fsTypeLower := strings.ToLower(fsType)
			if strings.Contains(fsTypeLower, "nfs") ||
			   strings.Contains(fsTypeLower, "cifs") ||
			   strings.Contains(fsTypeLower, "smb") ||
			   strings.Contains(fsTypeLower, "smbfs") ||
			   strings.Contains(fsTypeLower, "ncpfs") ||
			   strings.Contains(fsTypeLower, "fuse.sshfs") ||
			   strings.Contains(fsTypeLower, "fuse.rclone") {
				info.IsNetwork = true
				info.Protocol = fsTypeLower
				info.MountPath = mountPoint
			}
		}
	}

	return info, nil
}

// detectDarwinNetwork detects network filesystems on macOS
func detectDarwinNetwork(path string, stat *syscall.Statfs_t) (*NetworkInfo, error) {
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

// parseProcMounts parses /proc/mounts to get filesystem types and mount points
func parseProcMounts() (map[string]string, error) {
	file, err := os.Open("/proc/mounts")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	mounts := make(map[string]string)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}

		// device mountpoint fstype options dump pass
		// fields[0] = device
		// fields[1] = mount point
		// fields[2] = filesystem type
		mountPoint := fields[1]
		fsType := fields[2]

		mounts[mountPoint] = fsType
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return mounts, nil
}

// IsNetworkPath checks if a path is on a network filesystem (convenience function)
func IsNetworkPath(path string) bool {
	info, err := DetectNetworkFilesystem(path)
	if err != nil {
		return false
	}
	return info.IsNetwork
}
