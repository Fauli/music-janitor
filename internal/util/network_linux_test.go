//go:build linux
// +build linux

package util

import (
	"testing"
)

func TestParseProcMounts(t *testing.T) {
	mounts, err := parseProcMounts()
	if err != nil {
		t.Fatalf("Failed to parse /proc/mounts: %v", err)
	}

	// Should have at least root filesystem
	if len(mounts) == 0 {
		t.Error("Expected at least one mount point")
	}

	// Root should be mounted
	if _, found := mounts["/"]; !found {
		t.Error("Expected root filesystem to be mounted")
	}
}
