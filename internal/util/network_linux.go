//go:build linux
// +build linux

package util

import (
	"bufio"
	"os"
	"strings"
	"syscall"
)

// detectPlatformNetwork detects network filesystems on Linux
func detectPlatformNetwork(path string, stat *syscall.Statfs_t) (*NetworkInfo, error) {
	info := &NetworkInfo{
		IsNetwork: false,
	}

	// Check common network filesystem types by magic number
	// These are Linux kernel VFS type constants
	// Note: stat.Type is int64 on Linux
	fsType := uint32(stat.Type)

	networkTypes := map[uint32]string{
		0x6969:       "nfs",       // NFS_SUPER_MAGIC
		0xff534d42:   "cifs",      // CIFS_MAGIC_NUMBER
		0x517b:       "smb",       // SMB_SUPER_MAGIC
		0x01021994:   "smbfs",     // SMBFS_MAGIC (old)
		0x564c:       "ncp",       // NCP_SUPER_MAGIC (Netware)
		0x5346544e:   "ntfs",      // NTFS (might be network)
		0xfe534d42:   "smb2",      // SMB2_MAGIC_NUMBER
	}

	if proto, found := networkTypes[fsType]; found {
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
	for mountPoint, fsTypeName := range mounts {
		if strings.HasPrefix(path, mountPoint) && len(mountPoint) > len(bestMatch) {
			bestMatch = mountPoint

			// Check if this is a known network filesystem
			fsTypeLower := strings.ToLower(fsTypeName)
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
