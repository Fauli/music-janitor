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


func TestDetectNetworkFilesystem_NonExistent(t *testing.T) {
	// Test with non-existent path
	nonExistent := "/this/path/does/not/exist/hopefully"

	_, err := DetectNetworkFilesystem(nonExistent)
	if err == nil {
		t.Error("Expected error for non-existent path")
	}
	t.Logf("Got expected error for non-existent path: %v", err)
}
