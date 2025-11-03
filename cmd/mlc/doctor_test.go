package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/franz/music-janitor/internal/store"
)

func TestCheckFFprobe(t *testing.T) {
	result := checkFFprobe()

	// FFprobe should be available (required for the project)
	if result.error {
		t.Errorf("ffprobe check failed: %s", result.message)
	}

	if result.message == "" {
		t.Error("expected version information in message")
	}
}

func TestCheckFpcalc(t *testing.T) {
	result := checkFpcalc()

	// fpcalc is optional, so we just verify the result is valid
	// It can be either success or warning, but not error
	if result.error {
		t.Errorf("fpcalc check should not error (it's optional), got error: %s", result.message)
	}
}

func TestCheckSQLite(t *testing.T) {
	result := checkSQLite()

	if result.error {
		t.Errorf("SQLite check failed: %s", result.message)
	}

	if result.message == "" {
		t.Error("expected version information in message")
	}
}

func TestCheckDatabase_NonExistent(t *testing.T) {
	// Check a database that doesn't exist
	dbPath := filepath.Join(t.TempDir(), "nonexistent.db")

	result := checkDatabase(dbPath)

	// Should not error - database will be created on first run
	if result.error {
		t.Errorf("non-existent database check should not error: %s", result.message)
	}

	if result.message == "" {
		t.Error("expected message about database creation")
	}
}

func TestCheckDatabase_Existing(t *testing.T) {
	// Create a real database
	dbPath := filepath.Join(t.TempDir(), "test.db")

	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Add a test file
	file := &store.File{
		FileKey:   "test-key",
		SrcPath:   "/test/path.mp3",
		SizeBytes: 1024,
		Status:    "meta_ok",
	}
	if err := db.InsertFile(file); err != nil {
		t.Fatalf("failed to insert test file: %v", err)
	}
	db.Close()

	// Now check the database
	result := checkDatabase(dbPath)

	if result.error {
		t.Errorf("database check failed: %s", result.message)
	}

	if result.message == "" {
		t.Error("expected message with database info")
	}
}

func TestCheckDatabase_Empty(t *testing.T) {
	// Test with empty database path
	result := checkDatabase("")

	if !result.warning {
		t.Error("expected warning for empty database path")
	}
}

func TestCheckSourceDirectory_Valid(t *testing.T) {
	// Use current directory
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	result := checkSourceDirectory(dir)

	if result.error {
		t.Errorf("source directory check failed: %s", result.message)
	}
}

func TestCheckSourceDirectory_NonExistent(t *testing.T) {
	result := checkSourceDirectory("/nonexistent/path/that/does/not/exist")

	if !result.error {
		t.Error("expected error for non-existent directory")
	}
}

func TestCheckSourceDirectory_File(t *testing.T) {
	// Create a file instead of directory
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "file.txt")
	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	result := checkSourceDirectory(filePath)

	if !result.error {
		t.Error("expected error when path is a file, not a directory")
	}
}

func TestCheckDestinationDirectory_Valid(t *testing.T) {
	dir := t.TempDir()

	result := checkDestinationDirectory(dir)

	if result.error {
		t.Errorf("destination directory check failed: %s", result.message)
	}
}

func TestCheckDestinationDirectory_Create(t *testing.T) {
	tmpDir := t.TempDir()
	newDir := filepath.Join(tmpDir, "newdir")

	result := checkDestinationDirectory(newDir)

	if result.error {
		t.Errorf("destination directory check failed: %s", result.message)
	}

	// Verify directory was created
	if _, err := os.Stat(newDir); os.IsNotExist(err) {
		t.Error("expected directory to be created")
	}
}

func TestCheckDestinationDirectory_File(t *testing.T) {
	// Create a file instead of directory
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "file.txt")
	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	result := checkDestinationDirectory(filePath)

	if !result.error {
		t.Error("expected error when path is a file, not a directory")
	}
}

func TestCheckDiskSpace(t *testing.T) {
	// Use temp directory which should have disk space info
	dir := t.TempDir()

	result := checkDiskSpace(dir, "test")

	// Should not error
	if result.error {
		t.Errorf("disk space check failed: %s", result.message)
	}

	if result.message == "" {
		t.Error("expected message with disk space info")
	}
}

func TestCheckDiskSpace_NonExistent(t *testing.T) {
	result := checkDiskSpace("/nonexistent/path", "test")

	// Should produce a warning (not error)
	if !result.warning {
		t.Error("expected warning for non-existent path")
	}
}
