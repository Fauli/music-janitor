package util

import (
	"os"
	"runtime"
	"testing"
)

func TestDetectNetworkFilesystem(t *testing.T) {
	// Test with current directory (should be local)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	info, err := DetectNetworkFilesystem(cwd)
	if err != nil {
		t.Fatalf("DetectNetworkFilesystem failed: %v", err)
	}

	t.Logf("Current directory: %s", cwd)
	t.Logf("Is network: %v", info.IsNetwork)
	t.Logf("Protocol: %s", info.Protocol)
	t.Logf("Mount path: %s", info.MountPath)

	// Current directory should typically be local (unless running on NAS)
	// We can't assert this as tests might run on network storage
	if info.IsNetwork {
		t.Logf("WARNING: Tests are running on network storage (%s)", info.Protocol)
	}
}

func TestDetectNetworkFilesystem_TempDir(t *testing.T) {
	// Test with temp directory (should be local)
	tmpDir := os.TempDir()

	info, err := DetectNetworkFilesystem(tmpDir)
	if err != nil {
		t.Fatalf("DetectNetworkFilesystem failed for temp dir: %v", err)
	}

	t.Logf("Temp directory: %s", tmpDir)
	t.Logf("Is network: %v", info.IsNetwork)
	t.Logf("Protocol: %s", info.Protocol)
	t.Logf("Mount path: %s", info.MountPath)

	// Temp dir should almost always be local
	if info.IsNetwork {
		t.Logf("WARNING: Temp directory is on network storage (%s)", info.Protocol)
	}
}

func TestDetectNetworkFilesystem_Root(t *testing.T) {
	// Test with root directory
	var rootPath string
	if runtime.GOOS == "windows" {
		rootPath = "C:\\"
	} else {
		rootPath = "/"
	}

	info, err := DetectNetworkFilesystem(rootPath)
	if err != nil {
		t.Fatalf("DetectNetworkFilesystem failed for root: %v", err)
	}

	t.Logf("Root directory: %s", rootPath)
	t.Logf("Is network: %v", info.IsNetwork)
	t.Logf("Protocol: %s", info.Protocol)
	t.Logf("Mount path: %s", info.MountPath)
}

func TestIsNetworkPath(t *testing.T) {
	// Test convenience function with current directory
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	isNetwork := IsNetworkPath(cwd)
	t.Logf("IsNetworkPath(%s) = %v", cwd, isNetwork)

	// Just log the result, don't assert
	// (we can't know if tests are running on network storage)
}

func TestParseProcMounts_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Skipping Linux-specific test")
	}

	mounts, err := parseProcMounts()
	if err != nil {
		t.Fatalf("Failed to parse /proc/mounts: %v", err)
	}

	if len(mounts) == 0 {
		t.Error("Expected at least one mount point")
	}

	t.Logf("Found %d mount points", len(mounts))

	// Check for root filesystem
	if fsType, ok := mounts["/"]; ok {
		t.Logf("Root filesystem: %s", fsType)
	} else {
		t.Error("Root filesystem not found in mounts")
	}

	// Log any network filesystems found
	for mount, fsType := range mounts {
		fsTypeLower := fsType
		if len(fsTypeLower) > 0 {
			if fsTypeLower[0] == 'n' || fsTypeLower[0] == 'c' || fsTypeLower[0] == 's' {
				if fsTypeLower == "nfs" || fsTypeLower == "cifs" ||
				   fsTypeLower == "smb" || fsTypeLower == "smbfs" {
					t.Logf("Network mount found: %s (%s)", mount, fsType)
				}
			}
		}
	}
}

func TestDetectNetworkFilesystem_NonExistent(t *testing.T) {
	// Test with non-existent path
	nonExistent := "/this/path/does/not/exist/hopefully"

	_, err := DetectNetworkFilesystem(nonExistent)
	if err == nil {
		t.Error("Expected error for non-existent path")
	}
	t.Logf("Got expected error for non-existent path: %v", err)
}
