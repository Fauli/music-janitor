package plan

import (
	"strings"
	"testing"

	"github.com/franz/music-janitor/internal/store"
)

func TestGenerateDestPath(t *testing.T) {
	testCases := []struct {
		name     string
		destRoot string
		metadata *store.Metadata
		srcPath  string
		expected string
	}{
		{
			name:     "standard album track",
			destRoot: "/dest",
			metadata: &store.Metadata{
				TagArtist:      "The Beatles",
				TagAlbumArtist: "The Beatles",
				TagAlbum:       "Abbey Road",
				TagTitle:       "Come Together",
				TagDate:        "1969",
				TagTrack:       1,
				TagTrackTotal:  17,
			},
			srcPath:  "/src/music/song.mp3",
			expected: "/dest/The Beatles/1969 - Abbey Road/01 - Come Together.mp3",
		},
		{
			name:     "multi-disc album",
			destRoot: "/dest",
			metadata: &store.Metadata{
				TagArtist:      "Artist",
				TagAlbumArtist: "Artist",
				TagAlbum:       "Album",
				TagTitle:       "Song",
				TagTrack:       5,
				TagDisc:        2,
				TagDiscTotal:   3,
			},
			srcPath:  "/src/song.flac",
			expected: "/dest/Artist/Album/Disc 02/05 - Song.flac",
		},
		{
			name:     "artist fallback when no album artist",
			destRoot: "/dest",
			metadata: &store.Metadata{
				TagArtist: "Solo Artist",
				TagAlbum:  "Album",
				TagTitle:  "Title",
			},
			srcPath:  "/src/file.m4a",
			expected: "/dest/Solo Artist/Album/Title.m4a",
		},
		{
			name:     "no album - uses _Singles",
			destRoot: "/dest",
			metadata: &store.Metadata{
				TagArtist: "Artist",
				TagTitle:  "Single Track",
			},
			srcPath:  "/src/single.mp3",
			expected: "/dest/Artist/_Singles/Single Track.mp3",
		},
		{
			name:     "no tags - uses Unknown Artist",
			destRoot: "/dest",
			metadata: &store.Metadata{},
			srcPath:  "/src/unknown.mp3",
			expected: "/dest/Unknown Artist/_Singles/unknown.mp3",
		},
		{
			name:     "track number padding - 100+ tracks",
			destRoot: "/dest",
			metadata: &store.Metadata{
				TagArtist:     "Artist",
				TagAlbum:      "Album",
				TagTitle:      "Title",
				TagTrack:      105,
				TagTrackTotal: 120,
			},
			srcPath:  "/src/track.mp3",
			expected: "/dest/Artist/Album/105 - Title.mp3",
		},
		{
			name:     "sanitize illegal characters",
			destRoot: "/dest",
			metadata: &store.Metadata{
				TagArtist: "Artist/Name",
				TagAlbum:  "Album:Title",
				TagTitle:  "Song?Name",
			},
			srcPath:  "/src/file.mp3",
			expected: "/dest/Artist_Name/Album_Title/Song_Name.mp3",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GenerateDestPath(tc.destRoot, tc.metadata, tc.srcPath, false)

			if result != tc.expected {
				t.Errorf("Expected: %s\nGot:      %s", tc.expected, result)
			}
		})
	}
}

func TestSanitizePathComponent(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"Normal Text", "Normal Text"},
		{"Text/With/Slashes", "Text_With_Slashes"},
		{"Text\\Backslash", "Text_Backslash"},
		{"Text:Colon", "Text_Colon"},
		{"Text*Star", "Text_Star"},
		{"Text?Question", "Text_Question"},
		{"Text\"Quote", "Text_Quote"},
		{"Text<Angle>", "Text_Angle_"},
		{"Text|Pipe", "Text_Pipe"},
		{"  Leading and trailing  ", "Leading and trailing"},
		{"...Dots...", "Dots"},
		{"Multiple___Underscores", "Multiple_Underscores"},
		{"All/\\:*?\"<>|Illegal", "All_Illegal"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := SanitizePathComponent(tc.input)

			if result != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestSanitizePathComponentLength(t *testing.T) {
	// Test that very long strings are truncated
	longString := strings.Repeat("a", 250)
	result := SanitizePathComponent(longString)

	if len(result) > 200 {
		t.Errorf("Expected length <= 200, got %d", len(result))
	}
}

func TestExtractYear(t *testing.T) {
	testCases := []struct {
		date     string
		expected string
	}{
		{"2023", "2023"},
		{"2023-01-15", "2023"},
		{"15/01/2023", "2023"},
		{"1969", "1969"},
		{"1999-12-31", "1999"},
		{"", ""},
		{"Not a year", ""},
		{"123", ""}, // Too short
		{"9999", ""},             // Out of range
		{"1800", ""},             // Out of range
		{"2099", "2099"},         // Edge of range
		{"1900", "1900"},         // Edge of range
		{"abc2022def", "2022"},   // Year in middle
		{"Released 2020", "2020"}, // Year in text
	}

	for _, tc := range testCases {
		t.Run(tc.date, func(t *testing.T) {
			result := extractYear(tc.date)

			if result != tc.expected {
				t.Errorf("Date %q: expected %q, got %q", tc.date, tc.expected, result)
			}
		})
	}
}

func TestPathCollisionResolution(t *testing.T) {
	// Create temporary database
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create test files with same destination path but different quality
	// Scenario: Two different recordings of "Song" by "Artist" from "Album" track 1
	// One is FLAC (high quality), one is MP3 (lower quality)

	// File 1: FLAC version (higher quality)
	file1 := &store.File{
		FileKey:   "file1-key",
		SrcPath:   "/src/song-flac.flac",
		SizeBytes: 30000000,
		Status:    "meta_ok",
	}
	if err := db.InsertFile(file1); err != nil {
		t.Fatalf("Failed to insert file1: %v", err)
	}

	metadata1 := &store.Metadata{
		FileID:         file1.ID,
		Codec:          "flac",
		Lossless:       true,
		TagArtist:      "Artist",
		TagAlbumArtist: "Artist",
		TagAlbum:       "Album",
		TagTitle:       "Song",
		TagTrack:       1,
		DurationMs:     180000,
	}
	if err := db.InsertMetadata(metadata1); err != nil {
		t.Fatalf("Failed to insert metadata1: %v", err)
	}

	// File 2: MP3 version (lower quality)
	file2 := &store.File{
		FileKey:   "file2-key",
		SrcPath:   "/src/song-mp3.mp3",
		SizeBytes: 5000000,
		Status:    "meta_ok",
	}
	if err := db.InsertFile(file2); err != nil {
		t.Fatalf("Failed to insert file2: %v", err)
	}

	metadata2 := &store.Metadata{
		FileID:         file2.ID,
		Codec:          "mp3",
		Lossless:       false,
		TagArtist:      "Artist",
		TagAlbumArtist: "Artist",
		TagAlbum:       "Album",
		TagTitle:       "Song",
		TagTrack:       1,
		DurationMs:     181000, // Slightly different duration - would be different clusters
	}
	if err := db.InsertMetadata(metadata2); err != nil {
		t.Fatalf("Failed to insert metadata2: %v", err)
	}

	// Create two clusters (different durations, so clustered separately)
	cluster1 := &store.Cluster{
		ClusterKey: "cluster1",
		Hint:       "Artist - Song (180s)",
	}
	if err := db.InsertCluster(cluster1); err != nil {
		t.Fatalf("Failed to insert cluster1: %v", err)
	}

	cluster2 := &store.Cluster{
		ClusterKey: "cluster2",
		Hint:       "Artist - Song (181s)",
	}
	if err := db.InsertCluster(cluster2); err != nil {
		t.Fatalf("Failed to insert cluster2: %v", err)
	}

	// Add members with quality scores
	member1 := &store.ClusterMember{
		ClusterKey:   "cluster1",
		FileID:       file1.ID,
		QualityScore: 85.0, // FLAC has higher score
		Preferred:    true,
	}
	if err := db.InsertClusterMember(member1); err != nil {
		t.Fatalf("Failed to insert member1: %v", err)
	}

	member2 := &store.ClusterMember{
		ClusterKey:   "cluster2",
		FileID:       file2.ID,
		QualityScore: 50.0, // MP3 has lower score
		Preferred:    true,
	}
	if err := db.InsertClusterMember(member2); err != nil {
		t.Fatalf("Failed to insert member2: %v", err)
	}

	// Create plans for both files (both would go to same dest_path)
	destPath := "/dest/Artist/Album/01 - Song.flac" // Note: extension from first file

	plan1 := &store.Plan{
		FileID:   file1.ID,
		Action:   "copy",
		DestPath: destPath,
		Reason:   "winner (score: 85.0)",
	}
	if err := db.InsertPlan(plan1); err != nil {
		t.Fatalf("Failed to insert plan1: %v", err)
	}

	plan2 := &store.Plan{
		FileID:   file2.ID,
		Action:   "copy",
		DestPath: destPath, // Same dest_path - collision!
		Reason:   "winner (score: 50.0)",
	}
	if err := db.InsertPlan(plan2); err != nil {
		t.Fatalf("Failed to insert plan2: %v", err)
	}

	// Create planner and resolve collisions
	planner := &Planner{
		store: db,
	}

	collisionsResolved, err := planner.resolvePathCollisions()
	if err != nil {
		t.Fatalf("Failed to resolve collisions: %v", err)
	}

	if collisionsResolved != 1 {
		t.Errorf("Expected 1 collision resolved, got %d", collisionsResolved)
	}

	// Verify that file1 (FLAC, higher quality) is still copying
	plan1After, err := db.GetPlan(file1.ID)
	if err != nil {
		t.Fatalf("Failed to get plan1 after collision resolution: %v", err)
	}

	if plan1After.Action != "copy" {
		t.Errorf("Expected file1 (FLAC) to still be copying, got action: %s", plan1After.Action)
	}

	// Verify that file2 (MP3, lower quality) is now skipped
	plan2After, err := db.GetPlan(file2.ID)
	if err != nil {
		t.Fatalf("Failed to get plan2 after collision resolution: %v", err)
	}

	if plan2After.Action != "skip" {
		t.Errorf("Expected file2 (MP3) to be skipped, got action: %s", plan2After.Action)
	}

	if !strings.Contains(plan2After.Reason, "path collision") {
		t.Errorf("Expected reason to mention 'path collision', got: %s", plan2After.Reason)
	}
}

func TestVariousArtistsCompilation(t *testing.T) {
	testCases := []struct {
		name           string
		destRoot       string
		metadata       *store.Metadata
		srcPath        string
		isCompilation  bool
		expectedFolder string // Expected artist folder
		expectedFile   string // Expected filename
	}{
		{
			name:     "compilation album - Various Artists folder with artist in filename",
			destRoot: "/dest",
			metadata: &store.Metadata{
				TagArtist:      "The Beatles",
				TagAlbumArtist: "Various Artists", // Often set on compilations
				TagAlbum:       "Greatest Hits 2000",
				TagTitle:       "Hey Jude",
				TagDate:        "2000",
				TagTrack:       5,
			},
			srcPath:        "/src/compilation.mp3",
			isCompilation:  true,
			expectedFolder: "Various Artists",
			expectedFile:   "05 - The Beatles - Hey Jude.mp3",
		},
		{
			name:     "non-compilation album - normal artist folder",
			destRoot: "/dest",
			metadata: &store.Metadata{
				TagArtist:      "The Beatles",
				TagAlbumArtist: "The Beatles",
				TagAlbum:       "Abbey Road",
				TagTitle:       "Come Together",
				TagDate:        "1969",
				TagTrack:       1,
			},
			srcPath:        "/src/normal.mp3",
			isCompilation:  false,
			expectedFolder: "The Beatles",
			expectedFile:   "01 - Come Together.mp3",
		},
		{
			name:     "compilation without album artist - uses Various Artists",
			destRoot: "/dest",
			metadata: &store.Metadata{
				TagArtist: "Madonna",
				TagAlbum:  "Now That's What I Call Music 50",
				TagTitle:  "Hung Up",
				TagTrack:  12,
			},
			srcPath:        "/src/comp2.mp3",
			isCompilation:  true,
			expectedFolder: "Various Artists",
			expectedFile:   "12 - Madonna - Hung Up.mp3",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GenerateDestPath(tc.destRoot, tc.metadata, tc.srcPath, tc.isCompilation)

			// Check that folder contains expected artist
			if !strings.Contains(result, tc.expectedFolder) {
				t.Errorf("Expected path to contain folder '%s', got: %s", tc.expectedFolder, result)
			}

			// Check that filename matches expected
			if !strings.Contains(result, tc.expectedFile) {
				t.Errorf("Expected path to contain file '%s', got: %s", tc.expectedFile, result)
			}
		})
	}
}

func TestIsRealCompilation(t *testing.T) {
	// Create temporary database
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create a compilation album with 4 different artists
	albumName := "Greatest Hits 2000"
	artists := []string{"Artist A", "Artist B", "Artist C", "Artist D"}

	for i, artist := range artists {
		file := &store.File{
			FileKey:   string(rune('a' + i)),
			SrcPath:   "/src/track" + string(rune('1'+i)) + ".mp3",
			SizeBytes: 5000000,
			Status:    "meta_ok",
		}
		if err := db.InsertFile(file); err != nil {
			t.Fatalf("Failed to insert file: %v", err)
		}

		metadata := &store.Metadata{
			FileID:         file.ID,
			TagArtist:      artist,
			TagAlbumArtist: "Various Artists",
			TagAlbum:       albumName,
			TagTitle:       "Track " + string(rune('1'+i)),
			TagTrack:       i + 1,
			TagCompilation: true,
		}
		if err := db.InsertMetadata(metadata); err != nil {
			t.Fatalf("Failed to insert metadata: %v", err)
		}
	}

	// Create planner
	planner := &Planner{
		store: db,
	}

	// Test: should return true (4 different artists)
	isComp := planner.isRealCompilation(1, albumName)
	if !isComp {
		t.Errorf("Expected isRealCompilation to return true for album with 4 different artists")
	}

	// Create a false-positive compilation (compilation flag set but same artist)
	albumName2 := "Solo Album"
	for i := 0; i < 5; i++ {
		file := &store.File{
			FileKey:   "solo" + string(rune('a'+i)),
			SrcPath:   "/src/solo" + string(rune('1'+i)) + ".mp3",
			SizeBytes: 5000000,
			Status:    "meta_ok",
		}
		if err := db.InsertFile(file); err != nil {
			t.Fatalf("Failed to insert file: %v", err)
		}

		metadata := &store.Metadata{
			FileID:         file.ID,
			TagArtist:      "Single Artist", // Same artist for all tracks
			TagAlbumArtist: "Single Artist",
			TagAlbum:       albumName2,
			TagTitle:       "Track " + string(rune('1'+i)),
			TagTrack:       i + 1,
			TagCompilation: true, // Flag set incorrectly
		}
		if err := db.InsertMetadata(metadata); err != nil {
			t.Fatalf("Failed to insert metadata: %v", err)
		}
	}

	// Test: should return false (same artist for all tracks, despite compilation flag)
	isComp2 := planner.isRealCompilation(5, albumName2)
	if isComp2 {
		t.Errorf("Expected isRealCompilation to return false for album with only 1 artist")
	}
}
