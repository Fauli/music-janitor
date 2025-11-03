package store

import (
	"os"
	"testing"
)

func TestStoreOpenAndMigrate(t *testing.T) {
	// Create a temporary database file
	tmpFile := "test-store.db"
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + "-shm")
	defer os.Remove(tmpFile + "-wal")

	// Open the store
	store, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// Verify schema version
	version, err := store.getSchemaVersion()
	if err != nil {
		t.Fatalf("failed to get schema version: %v", err)
	}

	if version != currentSchemaVersion {
		t.Errorf("expected schema version %d, got %d", currentSchemaVersion, version)
	}

	// Verify tables exist
	tables := []string{"files", "metadata", "clusters", "cluster_members", "plans", "executions", "schema_version"}
	for _, table := range tables {
		var count int
		err := store.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count)
		if err != nil {
			t.Fatalf("failed to query table %s: %v", table, err)
		}
		if count != 1 {
			t.Errorf("expected table %s to exist", table)
		}
	}

	// Verify v2 performance indexes exist
	v2Indexes := []string{
		"idx_cluster_members_quality",
		"idx_metadata_duration",
		"idx_files_status_id",
	}
	for _, index := range v2Indexes {
		var count int
		err := store.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?", index).Scan(&count)
		if err != nil {
			t.Fatalf("failed to query index %s: %v", index, err)
		}
		if count != 1 {
			t.Errorf("expected index %s to exist (schema v2)", index)
		}
	}
}

func TestFileInsertAndRetrieve(t *testing.T) {
	// Create a temporary database file
	tmpFile := "test-files.db"
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + "-shm")
	defer os.Remove(tmpFile + "-wal")

	store, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// Insert a file
	file := &File{
		FileKey:   "test-key-123",
		SrcPath:   "/path/to/test.mp3",
		SizeBytes: 1024,
		MtimeUnix: 1234567890,
		Status:    "discovered",
	}

	err = store.InsertFile(file)
	if err != nil {
		t.Fatalf("failed to insert file: %v", err)
	}

	if file.ID == 0 {
		t.Error("expected file ID to be set after insert")
	}

	// Retrieve the file
	retrieved, err := store.GetFileByKey("test-key-123")
	if err != nil {
		t.Fatalf("failed to retrieve file: %v", err)
	}

	if retrieved == nil {
		t.Fatal("expected to retrieve file, got nil")
	}

	if retrieved.FileKey != file.FileKey {
		t.Errorf("expected FileKey %s, got %s", file.FileKey, retrieved.FileKey)
	}

	if retrieved.SrcPath != file.SrcPath {
		t.Errorf("expected SrcPath %s, got %s", file.SrcPath, retrieved.SrcPath)
	}

	// Update status
	err = store.UpdateFileStatus(file.ID, "meta_ok", "")
	if err != nil {
		t.Fatalf("failed to update file status: %v", err)
	}

	// Retrieve again and verify status
	retrieved, err = store.GetFileByKey("test-key-123")
	if err != nil {
		t.Fatalf("failed to retrieve file after update: %v", err)
	}

	if retrieved.Status != "meta_ok" {
		t.Errorf("expected status 'meta_ok', got '%s'", retrieved.Status)
	}
}

func TestMetadataInsertAndRetrieve(t *testing.T) {
	// Create a temporary database file
	tmpFile := "test-metadata.db"
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + "-shm")
	defer os.Remove(tmpFile + "-wal")

	store, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// Insert a file first
	file := &File{
		FileKey:   "test-key-456",
		SrcPath:   "/path/to/test.flac",
		SizeBytes: 20480,
		MtimeUnix: 1234567890,
		Status:    "discovered",
	}

	err = store.InsertFile(file)
	if err != nil {
		t.Fatalf("failed to insert file: %v", err)
	}

	// Insert metadata
	metadata := &Metadata{
		FileID:         file.ID,
		Format:         "FLAC",
		Codec:          "flac",
		Container:      "flac",
		DurationMs:     240000,
		SampleRate:     44100,
		BitDepth:       16,
		Channels:       2,
		BitrateKbps:    0,
		Lossless:       true,
		TagArtist:      "Test Artist",
		TagAlbum:       "Test Album",
		TagTitle:       "Test Title",
		TagAlbumArtist: "Test Artist",
		TagDate:        "2023",
		TagDisc:        1,
		TagDiscTotal:   1,
		TagTrack:       1,
		TagTrackTotal:  10,
		TagCompilation: false,
		RawTagsJSON:    "{}",
	}

	err = store.InsertMetadata(metadata)
	if err != nil {
		t.Fatalf("failed to insert metadata: %v", err)
	}

	// Retrieve metadata
	retrieved, err := store.GetMetadata(file.ID)
	if err != nil {
		t.Fatalf("failed to retrieve metadata: %v", err)
	}

	if retrieved == nil {
		t.Fatal("expected to retrieve metadata, got nil")
	}

	if retrieved.TagArtist != metadata.TagArtist {
		t.Errorf("expected TagArtist %s, got %s", metadata.TagArtist, retrieved.TagArtist)
	}

	if retrieved.DurationMs != metadata.DurationMs {
		t.Errorf("expected DurationMs %d, got %d", metadata.DurationMs, retrieved.DurationMs)
	}

	if !retrieved.Lossless {
		t.Error("expected Lossless to be true")
	}
}
