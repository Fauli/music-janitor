package cluster

import (
	"testing"

	"github.com/franz/music-janitor/internal/store"
)

func TestGenerateClusterKey(t *testing.T) {
	testCases := []struct {
		name     string
		metadata *store.Metadata
		srcPath  string
		expected string
	}{
		{
			name: "basic metadata",
			metadata: &store.Metadata{
				TagArtist:  "The Beatles",
				TagTitle:   "Yesterday",
				DurationMs: 125000, // 125 seconds -> bucket 126
			},
			srcPath:  "/music/song.mp3",
			expected: "the beatles|yesterday|studio|126|disc0",
		},
		{
			name: "duration bucketing - 124.8s",
			metadata: &store.Metadata{
				TagArtist:  "Artist",
				TagTitle:   "Title",
				DurationMs: 124800, // 124.8s -> bucket 126 (nearest 3s)
			},
			srcPath:  "/music/song.mp3",
			expected: "artist|title|studio|126|disc0",
		},
		{
			name: "duration bucketing - 126.2s",
			metadata: &store.Metadata{
				TagArtist:  "Artist",
				TagTitle:   "Title",
				DurationMs: 126200, // 126.2s -> bucket 126
			},
			srcPath:  "/music/song.mp3",
			expected: "artist|title|studio|126|disc0",
		},
		{
			name: "unicode normalization",
			metadata: &store.Metadata{
				TagArtist:  "Björk",
				TagTitle:   "Café",
				DurationMs: 180000,
			},
			srcPath:  "/music/song.mp3",
			expected: "björk|café|studio|180|disc0",
		},
		{
			name: "empty tags - uses filename",
			metadata: &store.Metadata{
				TagArtist:  "",
				TagTitle:   "",
				DurationMs: 100000,
			},
			srcPath:  "/music/05 Track 05.wav",
			expected: "unknown|05 track 05|studio|99|disc0",
		},
		{
			name: "empty tags - different filename",
			metadata: &store.Metadata{
				TagArtist:  "",
				TagTitle:   "",
				DurationMs: 100000,
			},
			srcPath:  "/music/19 Track 19.wav",
			expected: "unknown|19 track 19|studio|99|disc0",
		},
		{
			name: "remix version",
			metadata: &store.Metadata{
				TagArtist:  "Artist",
				TagTitle:   "Song (Remix)",
				DurationMs: 180000,
			},
			srcPath:  "/music/song.mp3",
			expected: "artist|song|remix|180|disc0",
		},
		{
			name: "live version",
			metadata: &store.Metadata{
				TagArtist:  "Artist",
				TagTitle:   "Song (Live)",
				DurationMs: 180000,
			},
			srcPath:  "/music/song.mp3",
			expected: "artist|song|live|180|disc0",
		},
		{
			name: "acoustic version",
			metadata: &store.Metadata{
				TagArtist:  "Artist",
				TagTitle:   "Song (Acoustic)",
				DurationMs: 180000,
			},
			srcPath:  "/music/song.mp3",
			expected: "artist|song|acoustic|180|disc0",
		},
		{
			name: "remaster version - still studio",
			metadata: &store.Metadata{
				TagArtist:  "Artist",
				TagTitle:   "Song (2011 Remaster)",
				DurationMs: 180000,
			},
			srcPath:  "/music/song.mp3",
			expected: "artist|song|studio|180|disc0",
		},
		{
			name: "multi-disc album - disc 1",
			metadata: &store.Metadata{
				TagArtist:  "Nat King Cole",
				TagTitle:   "Track No03",
				DurationMs: 180000,
				TagDisc:    1,
			},
			srcPath:  "/music/Nat King Cole/Mona Lisa/Disco 1/Track No03.mp3",
			expected: "nat king cole|track no03|studio|180|disc1",
		},
		{
			name: "multi-disc album - disc 2",
			metadata: &store.Metadata{
				TagArtist:  "Nat King Cole",
				TagTitle:   "Track No03",
				DurationMs: 180000,
				TagDisc:    2,
			},
			srcPath:  "/music/Nat King Cole/Mona Lisa/Disco 2/Track No03.mp3",
			expected: "nat king cole|track no03|studio|180|disc2",
		},
		{
			name: "multi-disc album - disc 3",
			metadata: &store.Metadata{
				TagArtist:  "Nat King Cole",
				TagTitle:   "Track No03",
				DurationMs: 180000,
				TagDisc:    3,
			},
			srcPath:  "/music/Nat King Cole/Mona Lisa/Disco 3/Track No03.mp3",
			expected: "nat king cole|track no03|studio|180|disc3",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GenerateClusterKey(tc.metadata, tc.srcPath)
			if result != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestBucketDuration(t *testing.T) {
	testCases := []struct {
		durationMs     int
		expectedBucket int
	}{
		{0, 0},
		{1000, 0},      // 1s -> 0
		{1500, 3},      // 1.5s -> 3 (rounds to 3)
		{2000, 3},      // 2s -> 3
		{3000, 3},      // 3s -> 3
		{4000, 3},      // 4s -> 3
		{4500, 6},      // 4.5s -> 6 (closer to 6 than 3)
		{125000, 126},  // 125s -> 126
		{126000, 126},  // 126s -> 126
		{127000, 126},  // 127s -> 126
		{128000, 129},  // 128s -> 129
		{180000, 180},  // 180s -> 180
		{222000, 222},  // 222s -> 222
		{223000, 222},  // 223s -> 222
	}

	for _, tc := range testCases {
		t.Run(string(rune(tc.durationMs)), func(t *testing.T) {
			result := bucketDuration(tc.durationMs)
			if result != tc.expectedBucket {
				t.Errorf("Duration %d: expected bucket %d, got %d", tc.durationMs, tc.expectedBucket, result)
			}
		})
	}
}

func TestGetDurationDelta(t *testing.T) {
	testCases := []struct {
		dur1     int
		dur2     int
		expected int
	}{
		{100, 100, 0},
		{100, 110, 10},
		{110, 100, 10},
		{1000, 1500, 500},
		{1500, 1000, 500},
	}

	for _, tc := range testCases {
		result := GetDurationDelta(tc.dur1, tc.dur2)
		if result != tc.expected {
			t.Errorf("GetDurationDelta(%d, %d): expected %d, got %d", tc.dur1, tc.dur2, tc.expected, result)
		}
	}
}

func TestNormalizeForClustering(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"Test", "test"},
		{"  Test  ", "test"},
		{"Test  String", "test string"},
		{"Test (Remix)", "test remix"},
		{"Test [Live]", "test live"},
		{"Test {Demo}", "test demo"},
		{"Rock & Roll", "rock and roll"},
		{"Rock + Roll", "rock and roll"},
		{"  Multiple   Spaces  ", "multiple spaces"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := NormalizeForClustering(tc.input)
			if result != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestGenerateClusterKey_EdgeCases(t *testing.T) {
	testCases := []struct {
		name     string
		metadata *store.Metadata
		srcPath  string
		expected string
	}{
		{
			name: "missing title only",
			metadata: &store.Metadata{
				TagArtist:  "Artist",
				TagTitle:   "",
				DurationMs: 100000,
			},
			srcPath:  "/music/song.mp3",
			expected: "artist||studio|99|disc0",
		},
		{
			name: "missing artist only",
			metadata: &store.Metadata{
				TagArtist:  "",
				TagTitle:   "Title",
				DurationMs: 100000,
			},
			srcPath:  "/music/song.mp3",
			expected: "|title|studio|99|disc0",
		},
		{
			name: "zero duration",
			metadata: &store.Metadata{
				TagArtist:  "Artist",
				TagTitle:   "Title",
				DurationMs: 0,
			},
			srcPath:  "/music/song.mp3",
			expected: "artist|title|studio|0|disc0",
		},
		{
			name: "very long duration",
			metadata: &store.Metadata{
				TagArtist:  "Artist",
				TagTitle:   "Epic Song",
				DurationMs: 3600000, // 1 hour
			},
			srcPath:  "/music/song.mp3",
			expected: "artist|epic song|studio|3600|disc0",
		},
		{
			name: "special characters in tags",
			metadata: &store.Metadata{
				TagArtist:  "AC/DC",
				TagTitle:   "Rock & Roll",
				DurationMs: 180000,
			},
			srcPath:  "/music/song.mp3",
			expected: "acdc|rock and roll|studio|180|disc0",
		},
		{
			name: "empty filename fallback",
			metadata: &store.Metadata{
				TagArtist:  "",
				TagTitle:   "",
				DurationMs: 100000,
			},
			srcPath:  "/music/.mp3",
			expected: "unknown|file_music|studio|99|disc0",
		},
		{
			name: "filename with multiple extensions",
			metadata: &store.Metadata{
				TagArtist:  "",
				TagTitle:   "",
				DurationMs: 100000,
			},
			srcPath:  "/music/track.backup.mp3",
			expected: "unknown|trackbackup|studio|99|disc0",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GenerateClusterKey(tc.metadata, tc.srcPath)
			if result != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestBucketDuration_EdgeCases(t *testing.T) {
	testCases := []struct {
		name           string
		durationMs     int
		expectedBucket int
	}{
		{"zero duration", 0, 0},
		{"negative duration", -1000, 0},
		{"very small duration", 100, 0},
		{"boundary case 1499ms", 1499, 0},
		{"boundary case 1500ms", 1500, 3},
		{"boundary case 1501ms", 1501, 3},
		{"large duration", 10000000, 9999}, // ~2.7 hours, rounds to 9999
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := bucketDuration(tc.durationMs)
			if result != tc.expectedBucket {
				t.Errorf("Duration %d: expected bucket %d, got %d", tc.durationMs, tc.expectedBucket, result)
			}
		})
	}
}

func TestGetDurationDelta_EdgeCases(t *testing.T) {
	testCases := []struct {
		name     string
		dur1     int
		dur2     int
		expected int
	}{
		{"both zero", 0, 0, 0},
		{"negative result", 100, 200, 100},
		{"positive result", 200, 100, 100},
		{"large difference", 1000000, 0, 1000000},
		{"same values", 5000, 5000, 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GetDurationDelta(tc.dur1, tc.dur2)
			if result != tc.expected {
				t.Errorf("GetDurationDelta(%d, %d): expected %d, got %d",
					tc.dur1, tc.dur2, tc.expected, result)
			}
		})
	}
}

func TestNormalizeForClustering_EdgeCases(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", ""},
		{"only spaces", "     ", ""},
		{"multiple brackets", "[[Test]]", "test"},
		{"nested brackets", "{[Test]}", "test"},
		{"multiple ampersands", "Rock & Roll & Blues", "rock and roll and blues"},
		{"mixed operators", "Rock & Blues + Jazz", "rock and blues and jazz"},
		{"unicode characters", "Café", "café"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := NormalizeForClustering(tc.input)
			if result != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, result)
			}
		})
	}
}
