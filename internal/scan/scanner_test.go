package scan

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/franz/music-janitor/internal/store"
)

func TestIsAudioFile(t *testing.T) {
	scanner := &Scanner{
		extensions: map[string]bool{
			".mp3":  true,
			".flac": true,
			".m4a":  true,
		},
	}

	tests := []struct {
		path     string
		expected bool
	}{
		{"test.mp3", true},
		{"test.MP3", true}, // Case insensitive
		{"test.flac", true},
		{"test.m4a", true},
		{"test.txt", false},
		{"test.jpg", false},
		{"test", false},
		{".mp3", true},
	}

	for _, tt := range tests {
		result := scanner.isAudioFile(tt.path)
		if result != tt.expected {
			t.Errorf("isAudioFile(%s) = %v, expected %v", tt.path, result, tt.expected)
		}
	}
}

func TestScannerWithRealFiles(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Create test directory structure
	artistDir := filepath.Join(tmpDir, "Artist")
	albumDir := filepath.Join(artistDir, "Album")
	os.MkdirAll(albumDir, 0755)

	// Create some test files
	testFiles := []string{
		filepath.Join(albumDir, "01 - Track One.mp3"),
		filepath.Join(albumDir, "02 - Track Two.flac"),
		filepath.Join(artistDir, "single.m4a"),
		filepath.Join(tmpDir, "README.txt"), // Should be ignored
	}

	for _, path := range testFiles {
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		f.Close()
	}

	// Create temporary database
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create scanner
	scanner := New(&Config{
		Store:       db,
		Concurrency: 2,
	})

	// Run scan
	ctx := context.Background()
	result, err := scanner.Scan(ctx, tmpDir)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Verify results
	if result.FilesDiscovered != 3 {
		t.Errorf("Expected 3 audio files discovered, got %d", result.FilesDiscovered)
	}

	// Verify files are in database
	files, err := db.GetFilesByStatus("discovered")
	if err != nil {
		t.Fatalf("Failed to get files from database: %v", err)
	}

	if len(files) != 3 {
		t.Errorf("Expected 3 files in database, got %d", len(files))
	}

	// Verify file keys are unique
	keys := make(map[string]bool)
	for _, file := range files {
		if keys[file.FileKey] {
			t.Errorf("Duplicate file key: %s", file.FileKey)
		}
		keys[file.FileKey] = true
	}
}

func TestScannerIdempotency(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.mp3")
	f, err := os.Create(testFile)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	f.Close()

	// Create temporary database
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create scanner
	scanner := New(&Config{
		Store:       db,
		Concurrency: 1,
	})

	ctx := context.Background()

	// Run scan twice
	result1, err := scanner.Scan(ctx, tmpDir)
	if err != nil {
		t.Fatalf("First scan failed: %v", err)
	}

	result2, err := scanner.Scan(ctx, tmpDir)
	if err != nil {
		t.Fatalf("Second scan failed: %v", err)
	}

	// First scan should discover the file
	if result1.FilesDiscovered != 1 {
		t.Errorf("First scan: expected 1 file discovered, got %d", result1.FilesDiscovered)
	}

	// Second scan should skip the file (already in database)
	if result2.FilesDiscovered != 0 {
		t.Errorf("Second scan: expected 0 files discovered, got %d", result2.FilesDiscovered)
	}

	// Database should only have one file
	files, err := db.GetAllFiles()
	if err != nil {
		t.Fatalf("Failed to get files: %v", err)
	}

	if len(files) != 1 {
		t.Errorf("Expected 1 file in database after two scans, got %d", len(files))
	}
}
