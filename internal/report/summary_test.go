package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/franz/music-janitor/internal/store"
)

func TestGenerateSummaryReport(t *testing.T) {
	// Create temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Insert test data
	setupTestData(t, db)

	// Generate report
	report, err := GenerateSummaryReport(db, "test-events.jsonl")
	if err != nil {
		t.Fatalf("GenerateSummaryReport failed: %v", err)
	}

	// Verify statistics
	if report.FilesScanned <= 0 {
		t.Error("Expected files scanned > 0")
	}
	if report.ClustersCreated <= 0 {
		t.Error("Expected clusters created > 0")
	}
	if report.EventLogPath != "test-events.jsonl" {
		t.Errorf("Expected event log path 'test-events.jsonl', got '%s'", report.EventLogPath)
	}
	if report.GeneratedAt.IsZero() {
		t.Error("Expected GeneratedAt to be set")
	}
}

func TestWriteMarkdownReport(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "reports", "summary.md")

	// Create a test report
	report := &SummaryReport{
		GeneratedAt:       time.Now(),
		FilesScanned:      100,
		FilesValid:        95,
		FilesWithErrors:   5,
		ClustersCreated:   80,
		SingletonClusters: 60,
		DuplicateClusters: 20,
		WinnersPlanned:    80,
		DuplicatesSkipped: 15,
		FilesExecuted:     75,
		FilesFailed:       5,
		BytesWritten:      1024 * 1024 * 500, // 500 MB
		DatabasePath:      "/test/database.db",
		EventLogPath:      "/test/events.jsonl",
		DuplicateSets: []DuplicateSet{
			{
				ClusterKey: "test-cluster-1",
				Hint:       "Artist - Song Title",
				Winner: DuplicateFile{
					Path:       "/music/artist/song.flac",
					Score:      85.5,
					Codec:      "flac",
					Bitrate:    0,
					SampleRate: 44100,
					Lossless:   true,
					SizeBytes:  50 * 1024 * 1024, // 50 MB
				},
				Losers: []DuplicateFile{
					{
						Path:       "/music/duplicates/song.mp3",
						Score:      68.0,
						Codec:      "mp3",
						Bitrate:    320,
						SampleRate: 44100,
						Lossless:   false,
						SizeBytes:  10 * 1024 * 1024, // 10 MB
					},
				},
			},
		},
		TopErrors: []ErrorSummary{
			{Error: "failed to read tags", Count: 3},
			{Error: "file not found", Count: 2},
		},
	}

	// Write report
	err := WriteMarkdownReport(report, outputPath)
	if err != nil {
		t.Fatalf("WriteMarkdownReport failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatalf("Report file was not created at %s", outputPath)
	}

	// Read and verify content
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read report file: %v", err)
	}

	contentStr := string(content)

	// Verify headers
	if !strings.Contains(contentStr, "# Music Library Cleaner - Summary Report") {
		t.Error("Report missing main header")
	}
	if !strings.Contains(contentStr, "## 📊 Overview") {
		t.Error("Report missing Overview section")
	}
	if !strings.Contains(contentStr, "## 🔗 Clustering") {
		t.Error("Report missing Clustering section")
	}
	if !strings.Contains(contentStr, "## 📋 Planning") {
		t.Error("Report missing Planning section")
	}
	if !strings.Contains(contentStr, "## ⚡ Execution") {
		t.Error("Report missing Execution section")
	}

	// Verify statistics are present
	if !strings.Contains(contentStr, "100") { // Files scanned
		t.Error("Report missing files scanned count")
	}
	if !strings.Contains(contentStr, "500.0 MB") { // Bytes written
		t.Error("Report missing bytes written")
	}

	// Verify duplicate set information
	if !strings.Contains(contentStr, "Artist - Song Title") {
		t.Error("Report missing duplicate set hint")
	}
	if !strings.Contains(contentStr, "✅ Winner") {
		t.Error("Report missing winner indicator")
	}
	if !strings.Contains(contentStr, "❌ Duplicates") {
		t.Error("Report missing duplicates indicator")
	}
	if !strings.Contains(contentStr, "flac") {
		t.Error("Report missing codec information")
	}

	// Verify errors section
	if !strings.Contains(contentStr, "## ⚠️ Top Errors") {
		t.Error("Report missing Top Errors section")
	}
	if !strings.Contains(contentStr, "failed to read tags") {
		t.Error("Report missing error message")
	}

	// Verify database path
	if !strings.Contains(contentStr, "/test/database.db") {
		t.Error("Report missing database path")
	}
}

func TestGatherDuplicateSets(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create test data with duplicates
	// File 1 - Winner
	file1 := &store.File{
		FileKey:   "file1",
		SrcPath:   "/music/song1.flac",
		SizeBytes: 50000000,
		MtimeUnix: time.Now().Unix(),
		Status:    "meta_ok",
	}
	db.InsertFile(file1)

	metadata1 := &store.Metadata{
		FileID:     1,
		Codec:      "flac",
		Lossless:   true,
		SampleRate: 44100,
	}
	db.InsertMetadata(metadata1)

	// File 2 - Loser
	file2 := &store.File{
		FileKey:   "file2",
		SrcPath:   "/music/song1.mp3",
		SizeBytes: 10000000,
		MtimeUnix: time.Now().Unix(),
		Status:    "meta_ok",
	}
	db.InsertFile(file2)

	metadata2 := &store.Metadata{
		FileID:     2,
		Codec:      "mp3",
		Lossless:   false,
		BitrateKbps: 320,
		SampleRate: 44100,
	}
	db.InsertMetadata(metadata2)

	// Create cluster
	cluster := &store.Cluster{
		ClusterKey: "cluster1",
		Hint:       "Test Song",
	}
	db.InsertCluster(cluster)

	// Add members
	member1 := &store.ClusterMember{
		ClusterKey:   "cluster1",
		FileID:       1,
		QualityScore: 85.5,
		Preferred:    true,
	}
	db.InsertClusterMember(member1)

	member2 := &store.ClusterMember{
		ClusterKey:   "cluster1",
		FileID:       2,
		QualityScore: 68.0,
		Preferred:    false,
	}
	db.InsertClusterMember(member2)

	// Gather duplicate sets
	sets := gatherDuplicateSets(db, 10)

	// Verify results
	if len(sets) != 1 {
		t.Fatalf("Expected 1 duplicate set, got %d", len(sets))
	}

	set := sets[0]
	if set.ClusterKey != "cluster1" {
		t.Errorf("Expected cluster key 'cluster1', got '%s'", set.ClusterKey)
	}
	if set.Hint != "Test Song" {
		t.Errorf("Expected hint 'Test Song', got '%s'", set.Hint)
	}
	if set.Winner.Score != 85.5 {
		t.Errorf("Expected winner score 85.5, got %.1f", set.Winner.Score)
	}
	if set.Winner.Codec != "flac" {
		t.Errorf("Expected winner codec 'flac', got '%s'", set.Winner.Codec)
	}
	if len(set.Losers) != 1 {
		t.Errorf("Expected 1 loser, got %d", len(set.Losers))
	}
	if set.Losers[0].Score != 68.0 {
		t.Errorf("Expected loser score 68.0, got %.1f", set.Losers[0].Score)
	}
}

func TestGatherTopErrors(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Test with empty database first
	topErrors := gatherTopErrors(db, 10)
	if len(topErrors) != 0 {
		t.Errorf("Expected 0 errors from empty DB, got %d", len(topErrors))
	}

	// Insert files with errors - using actual duplicate error messages
	// to test the counting functionality
	testErrors := []struct {
		msg   string
		count int
	}{
		{"failed to read tags", 3},
		{"file not found", 2},
		{"permission denied", 1},
	}

	for _, te := range testErrors {
		for i := 0; i < te.count; i++ {
			file := &store.File{
				FileKey:   "error-" + te.msg + "-" + string(rune('a'+i)),
				SrcPath:   "/music/error.mp3",
				SizeBytes: 1000,
				MtimeUnix: time.Now().Unix(),
				Status:    "discovered",
			}
			if err := db.InsertFile(file); err != nil {
				t.Fatalf("Failed to insert error file: %v", err)
			}
			// Update status to error with error message
			if err := db.UpdateFileStatus(file.ID, "error", te.msg); err != nil {
				t.Fatalf("Failed to update file status: %v", err)
			}
		}
	}

	// Gather top errors
	topErrors = gatherTopErrors(db, 10)

	// Verify we got errors back
	if len(topErrors) == 0 {
		// Debug: check what's in the database
		errorFiles, _ := db.GetFilesByStatus("error")
		t.Logf("Files in DB with status 'error': %d", len(errorFiles))
		for i, f := range errorFiles {
			t.Logf("  File %d: Error='%s'", i, f.Error)
		}
		t.Fatal("Expected some errors, got 0")
	}

	// Verify count
	if len(topErrors) != 3 {
		t.Errorf("Expected 3 unique errors, got %d", len(topErrors))
	}

	// Check they're sorted by count (most common first)
	if len(topErrors) >= 1 && topErrors[0].Count == 3 {
		// Good - most common error has count 3
	} else if len(topErrors) >= 1 {
		t.Errorf("Expected first error count 3, got %d", topErrors[0].Count)
	}

	// Verify all counts are correct
	for _, topErr := range topErrors {
		found := false
		for _, te := range testErrors {
			if te.msg == topErr.Error && te.count == topErr.Count {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Unexpected error or count: '%s' with count %d", topErr.Error, topErr.Count)
		}
	}
}

func TestTruncatePath(t *testing.T) {
	testCases := []struct {
		name   string
		path   string
		maxLen int
	}{
		{
			name:   "Short path - no truncation",
			path:   "/music/song.mp3",
			maxLen: 50,
		},
		{
			name:   "Long path - truncate middle",
			path:   "/very/long/path/to/some/music/collection/artist/album/song.mp3",
			maxLen: 30,
		},
		{
			name:   "Exactly at limit",
			path:   "/music/test.mp3",
			maxLen: 16,
		},
		{
			name:   "Very long path",
			path:   "/extremely/long/path/that/needs/significant/truncation/to/fit/within/limits/file.mp3",
			maxLen: 40,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := truncatePath(tc.path, tc.maxLen)

			// Verify length constraint
			if len(result) > tc.maxLen {
				t.Errorf("Result length %d exceeds maxLen %d", len(result), tc.maxLen)
			}

			// Verify result contains "..." if truncated
			if len(tc.path) > tc.maxLen && !strings.Contains(result, "...") {
				t.Error("Expected truncated path to contain '...'")
			}

			// Verify no truncation for short paths
			if len(tc.path) <= tc.maxLen && result != tc.path {
				t.Errorf("Short path should not be truncated: expected '%s', got '%s'", tc.path, result)
			}
		})
	}
}

func TestMarkdownReportStructure(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "summary.md")

	// Minimal report
	report := &SummaryReport{
		GeneratedAt:  time.Now(),
		FilesScanned: 10,
		FilesValid:   10,
	}

	err := WriteMarkdownReport(report, outputPath)
	if err != nil {
		t.Fatalf("WriteMarkdownReport failed: %v", err)
	}

	content, _ := os.ReadFile(outputPath)
	contentStr := string(content)

	// Verify Markdown structure
	lines := strings.Split(contentStr, "\n")

	// Check for headers (should start with #)
	headerCount := 0
	tableCount := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "#") {
			headerCount++
		}
		if strings.Contains(line, "|") {
			tableCount++
		}
	}

	if headerCount < 2 {
		t.Errorf("Expected at least 2 headers, got %d", headerCount)
	}
	if tableCount < 3 {
		t.Errorf("Expected at least 3 table rows, got %d", tableCount)
	}

	// Verify footer
	if !strings.Contains(contentStr, "Generated by") {
		t.Error("Report missing footer")
	}
}

func TestReportWithEmptyData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Generate report from empty database
	report, err := GenerateSummaryReport(db, "")
	if err != nil {
		t.Fatalf("GenerateSummaryReport failed: %v", err)
	}

	// Should not crash with empty data
	if report.FilesScanned != 0 {
		t.Errorf("Expected 0 files scanned for empty DB, got %d", report.FilesScanned)
	}

	// Write report should work even with empty data
	outputPath := filepath.Join(tmpDir, "empty-summary.md")
	err = WriteMarkdownReport(report, outputPath)
	if err != nil {
		t.Fatalf("WriteMarkdownReport failed on empty data: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Error("Report file was not created for empty data")
	}
}

// setupTestData creates a complete test dataset
func setupTestData(t *testing.T, db *store.Store) {
	// Insert files
	for i := 1; i <= 5; i++ {
		file := &store.File{
			FileKey:   "file-" + string(rune(i)),
			SrcPath:   "/music/song" + string(rune(i)) + ".mp3",
			SizeBytes: 10000000,
			MtimeUnix: time.Now().Unix(),
			Status:    "meta_ok",
		}
		if err := db.InsertFile(file); err != nil {
			t.Fatalf("Failed to insert file: %v", err)
		}

		metadata := &store.Metadata{
			FileID:      int64(i),
			Codec:       "mp3",
			Lossless:    false,
			BitrateKbps: 320,
			SampleRate:  44100,
			TagArtist:   "Test Artist",
			TagTitle:    "Song " + string(rune(i)),
		}
		if err := db.InsertMetadata(metadata); err != nil {
			t.Fatalf("Failed to insert metadata: %v", err)
		}
	}

	// Create clusters
	for i := 1; i <= 3; i++ {
		cluster := &store.Cluster{
			ClusterKey: "cluster-" + string(rune(i)),
			Hint:       "Test Artist - Song " + string(rune(i)),
		}
		if err := db.InsertCluster(cluster); err != nil {
			t.Fatalf("Failed to insert cluster: %v", err)
		}

		member := &store.ClusterMember{
			ClusterKey:   "cluster-" + string(rune(i)),
			FileID:       int64(i),
			QualityScore: 75.0,
			Preferred:    true,
		}
		if err := db.InsertClusterMember(member); err != nil {
			t.Fatalf("Failed to insert cluster member: %v", err)
		}
	}

	// Create plans
	for i := 1; i <= 3; i++ {
		plan := &store.Plan{
			FileID:   int64(i),
			Action:   "copy",
			DestPath: "/dest/song" + string(rune(i)) + ".mp3",
			Reason:   "winner",
		}
		if err := db.InsertPlan(plan); err != nil {
			t.Fatalf("Failed to insert plan: %v", err)
		}
	}
}
