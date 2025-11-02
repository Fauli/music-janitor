package meta

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/franz/music-janitor/internal/store"
)

// TestMetadataExtractionIntegration tests metadata extraction with real audio files
// Run: go test -v ./internal/meta -run TestMetadataExtractionIntegration
func TestMetadataExtractionIntegration(t *testing.T) {
	// Check if test fixtures exist
	fixturesDir := filepath.Join("testdata")
	if _, err := os.Stat(fixturesDir); os.IsNotExist(err) {
		t.Skip("Test fixtures not generated. Run: cd internal/meta/testdata && ./generate-fixtures.sh")
	}

	// Create temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create extractor
	extractor := New(&Config{
		Store:       db,
		Concurrency: 1,
	})

	// Test cases for different file formats
	testCases := []struct {
		filename         string
		expectedCodec    string
		expectedLossless bool
		shouldSucceed    bool
		description      string
	}{
		{
			filename:         "test-mp3-320.mp3",
			expectedCodec:    "mp3",
			expectedLossless: false,
			shouldSucceed:    true,
			description:      "MP3 CBR 320kbps",
		},
		{
			filename:         "test-flac-16-44.flac",
			expectedCodec:    "flac",
			expectedLossless: true,
			shouldSucceed:    true,
			description:      "FLAC 16-bit 44.1kHz",
		},
		{
			filename:         "test-flac-24-96.flac",
			expectedCodec:    "flac",
			expectedLossless: true,
			shouldSucceed:    true,
			description:      "FLAC 24-bit 96kHz",
		},
		{
			filename:         "test-aac-256.m4a",
			expectedCodec:    "aac",
			expectedLossless: false,
			shouldSucceed:    true,
			description:      "M4A AAC 256kbps",
		},
		{
			filename:         "test-vorbis-q6.ogg",
			expectedCodec:    "vorbis",
			expectedLossless: false,
			shouldSucceed:    true,
			description:      "OGG Vorbis quality 6",
		},
		{
			filename:         "test-opus-128.opus",
			expectedCodec:    "opus",
			expectedLossless: false,
			shouldSucceed:    true,
			description:      "Opus 128kbps",
		},
		{
			filename:         "test-wav-16.wav",
			expectedCodec:    "pcm_s16le",
			expectedLossless: true,
			shouldSucceed:    true,
			description:      "WAV PCM 16-bit",
		},
		{
			filename:         "test-aiff-16.aiff",
			expectedCodec:    "pcm_s16be",
			expectedLossless: true,
			shouldSucceed:    true,
			description:      "AIFF PCM 16-bit",
		},
		{
			filename:      "test-corrupt.mp3",
			shouldSucceed: false,
			description:   "Corrupted file (empty)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			testFile := filepath.Join(fixturesDir, tc.filename)

			// Check if file exists
			if _, err := os.Stat(testFile); os.IsNotExist(err) {
				t.Skipf("Test file not found: %s", testFile)
			}

			// Insert file into database
			file := &store.File{
				FileKey:   tc.filename,
				SrcPath:   testFile,
				SizeBytes: 0,
				MtimeUnix: 0,
				Status:    "discovered",
			}

			if err := db.InsertFile(file); err != nil {
				t.Fatalf("Failed to insert file: %v", err)
			}

			// Extract metadata
			err := extractor.extractFile(file)

			if tc.shouldSucceed {
				if err != nil {
					t.Errorf("Expected success but got error: %v", err)
					return
				}

				// Retrieve metadata
				metadata, err := db.GetMetadata(file.ID)
				if err != nil {
					t.Errorf("Failed to retrieve metadata: %v", err)
					return
				}

				if metadata == nil {
					t.Error("Metadata is nil")
					return
				}

				// Verify codec (from ffprobe)
				if tc.expectedCodec != "" && metadata.Codec != tc.expectedCodec {
					t.Errorf("Expected codec %s, got %s", tc.expectedCodec, metadata.Codec)
				}

				// Verify lossless detection
				if metadata.Lossless != tc.expectedLossless {
					t.Errorf("Expected lossless=%v, got %v (codec: %s)", tc.expectedLossless, metadata.Lossless, metadata.Codec)
				}

				// Basic sanity checks (ffprobe provides these)
				if metadata.DurationMs <= 0 {
					t.Errorf("Duration should be > 0, got %d", metadata.DurationMs)
				}

				if metadata.SampleRate <= 0 {
					t.Errorf("Sample rate should be > 0, got %d", metadata.SampleRate)
				}

				if metadata.Channels <= 0 {
					t.Errorf("Channels should be > 0, got %d", metadata.Channels)
				}

				t.Logf("Successfully extracted: codec=%s, lossless=%v, duration=%dms, rate=%dHz, channels=%d",
					metadata.Codec, metadata.Lossless, metadata.DurationMs, metadata.SampleRate, metadata.Channels)

			} else {
				if err == nil {
					t.Error("Expected error but got success")
				}
			}
		})
	}
}

// TestMetadataTagExtraction tests tag extraction with known values
func TestMetadataTagExtraction(t *testing.T) {
	fixturesDir := filepath.Join("testdata")
	if _, err := os.Stat(fixturesDir); os.IsNotExist(err) {
		t.Skip("Test fixtures not generated. Run: cd internal/meta/testdata && ./generate-fixtures.sh")
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	extractor := New(&Config{
		Store:       db,
		Concurrency: 1,
	})

	testCases := []struct {
		filename      string
		expectedTitle string
		expectedArtist string
		expectedAlbum string
		expectedDate  string
	}{
		{
			filename:      "test-mp3-tagged.mp3",
			expectedTitle: "Test Song",
			expectedArtist: "Test Artist",
			expectedAlbum: "Test Album",
			expectedDate:  "2023",
		},
		{
			filename:      "test-flac-tagged.flac",
			expectedTitle: "Test Song",
			expectedArtist: "Test Artist",
			expectedAlbum: "Test Album",
			expectedDate:  "2023",
		},
		{
			filename:      "test-aac-tagged.m4a",
			expectedTitle: "Test Song",
			expectedArtist: "Test Artist",
			expectedAlbum: "Test Album",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.filename, func(t *testing.T) {
			testFile := filepath.Join(fixturesDir, tc.filename)

			if _, err := os.Stat(testFile); os.IsNotExist(err) {
				t.Skipf("Test file not found: %s", testFile)
			}

			file := &store.File{
				FileKey:   tc.filename,
				SrcPath:   testFile,
				Status:    "discovered",
			}

			if err := db.InsertFile(file); err != nil {
				t.Fatalf("Failed to insert file: %v", err)
			}

			if err := extractor.extractFile(file); err != nil {
				t.Fatalf("Extraction failed: %v", err)
			}

			metadata, err := db.GetMetadata(file.ID)
			if err != nil || metadata == nil {
				t.Fatalf("Failed to retrieve metadata: %v", err)
			}

			if metadata.TagTitle != tc.expectedTitle {
				t.Errorf("Expected title '%s', got '%s'", tc.expectedTitle, metadata.TagTitle)
			}

			if metadata.TagArtist != tc.expectedArtist {
				t.Errorf("Expected artist '%s', got '%s'", tc.expectedArtist, metadata.TagArtist)
			}

			if metadata.TagAlbum != tc.expectedAlbum {
				t.Errorf("Expected album '%s', got '%s'", tc.expectedAlbum, metadata.TagAlbum)
			}

			if tc.expectedDate != "" && metadata.TagDate != tc.expectedDate {
				t.Errorf("Expected date '%s', got '%s'", tc.expectedDate, metadata.TagDate)
			}
		})
	}
}

// TestMetadataUnicodeHandling tests unicode tag handling
func TestMetadataUnicodeHandling(t *testing.T) {
	fixturesDir := filepath.Join("testdata")
	testFile := filepath.Join(fixturesDir, "test-unicode.mp3")

	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("Unicode test file not found")
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	extractor := New(&Config{Store: db, Concurrency: 1})

	file := &store.File{
		FileKey: "unicode-test",
		SrcPath: testFile,
		Status:  "discovered",
	}

	if err := db.InsertFile(file); err != nil {
		t.Fatalf("Failed to insert file: %v", err)
	}

	if err := extractor.extractFile(file); err != nil {
		t.Fatalf("Extraction failed: %v", err)
	}

	metadata, err := db.GetMetadata(file.ID)
	if err != nil || metadata == nil {
		t.Fatalf("Failed to retrieve metadata: %v", err)
	}

	// Verify unicode is preserved
	if metadata.TagTitle == "" {
		t.Error("Title with unicode should not be empty")
	}

	if metadata.TagArtist == "" {
		t.Error("Artist with unicode should not be empty")
	}

	t.Logf("Unicode title: %s", metadata.TagTitle)
	t.Logf("Unicode artist: %s", metadata.TagArtist)
}

// TestMetadataExtractionResume tests that extraction can resume
func TestMetadataExtractionResume(t *testing.T) {
	fixturesDir := filepath.Join("testdata")
	if _, err := os.Stat(fixturesDir); os.IsNotExist(err) {
		t.Skip("Test fixtures not generated")
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Insert multiple files
	files := []string{"test-mp3-320.mp3", "test-flac-16-44.flac", "test-aac-256.m4a"}
	for _, filename := range files {
		testFile := filepath.Join(fixturesDir, filename)
		if _, err := os.Stat(testFile); os.IsNotExist(err) {
			continue
		}

		file := &store.File{
			FileKey: filename,
			SrcPath: testFile,
			Status:  "discovered",
		}
		db.InsertFile(file)
	}

	// First extraction
	extractor := New(&Config{Store: db, Concurrency: 1})
	ctx := context.Background()
	result1, err := extractor.Extract(ctx)
	if err != nil {
		t.Fatalf("First extraction failed: %v", err)
	}

	// Second extraction should find nothing to process
	result2, err := extractor.Extract(ctx)
	if err != nil {
		t.Fatalf("Second extraction failed: %v", err)
	}

	if result2.Processed != 0 {
		t.Errorf("Second extraction should process 0 files, processed %d", result2.Processed)
	}

	if result1.Success == 0 {
		t.Error("First extraction should have succeeded for at least one file")
	}
}
