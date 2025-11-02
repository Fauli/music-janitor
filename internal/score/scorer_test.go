package score

import (
	"testing"

	"github.com/franz/music-janitor/internal/store"
)

func TestCalculateQualityScore(t *testing.T) {
	testCases := []struct {
		name         string
		metadata     *store.Metadata
		file         *store.File
		expectedMin  float64
		expectedMax  float64
		description  string
	}{
		{
			name: "FLAC 24/96 with complete tags",
			metadata: &store.Metadata{
				Codec:      "flac",
				Lossless:   true,
				BitDepth:   24,
				SampleRate: 96000,
				TagArtist:  "Artist",
				TagAlbum:   "Album",
				TagTitle:   "Title",
				TagTrack:   1,
			},
			file: &store.File{
				SizeBytes: 50 * 1024 * 1024, // 50MB
			},
			expectedMin: 60.0, // 40 (FLAC) + 10 (lossless) + 5 (bit depth) + 5 (sample rate) + 5 (tags) + 2 (size)
			expectedMax: 70.0,
			description: "Highest quality lossless with hi-res and complete tags",
		},
		{
			name: "FLAC 16/44.1 with complete tags",
			metadata: &store.Metadata{
				Codec:      "flac",
				Lossless:   true,
				BitDepth:   16,
				SampleRate: 44100,
				TagArtist:  "Artist",
				TagAlbum:   "Album",
				TagTitle:   "Title",
				TagTrack:   1,
			},
			file: &store.File{
				SizeBytes: 30 * 1024 * 1024,
			},
			expectedMin: 54.0, // 40 (FLAC) + 10 (lossless) + 0 (16-bit baseline) + 0 (44.1k baseline) + 5 (tags) + 1 (size 20-50MB)
			expectedMax: 58.0,
			description: "CD quality FLAC",
		},
		{
			name: "MP3 320 CBR with complete tags",
			metadata: &store.Metadata{
				Codec:       "mp3",
				Lossless:    false,
				BitrateKbps: 320,
				SampleRate:  44100,
				TagArtist:   "Artist",
				TagAlbum:    "Album",
				TagTitle:    "Title",
				TagTrack:    1,
			},
			file: &store.File{
				SizeBytes: 10 * 1024 * 1024,
			},
			expectedMin: 23.0, // 20 (MP3 320) + 0 (44.1k) + 4 (tags) - size bonus doesn't apply to lossy
			expectedMax: 26.0,
			description: "High quality MP3",
		},
		{
			name: "MP3 128 CBR with no tags",
			metadata: &store.Metadata{
				Codec:       "mp3",
				Lossless:    false,
				BitrateKbps: 128,
				SampleRate:  44100,
			},
			file: &store.File{
				SizeBytes: 4 * 1024 * 1024,
			},
			expectedMin: 10.0, // 12 (MP3 128) + 0 (44.1k) + 0 (no tags) - size adjustments
			expectedMax: 13.0,
			description: "Low quality MP3 with no tags",
		},
		{
			name: "AAC 256 VBR",
			metadata: &store.Metadata{
				Codec:       "aac",
				Lossless:    false,
				BitrateKbps: 256,
				SampleRate:  44100,
				TagArtist:   "Artist",
				TagTitle:    "Title",
			},
			file: &store.File{
				SizeBytes: 8 * 1024 * 1024,
			},
			expectedMin: 25.0, // 25 (AAC 256) + 0 (44.1k) + 2 (partial tags) - adjustments
			expectedMax: 28.0,
			description: "High quality AAC",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			score := CalculateQualityScore(tc.metadata, tc.file)

			if score < tc.expectedMin || score > tc.expectedMax {
				t.Errorf("%s: expected score between %.1f and %.1f, got %.1f",
					tc.description, tc.expectedMin, tc.expectedMax, score)
			}

			t.Logf("%s: score = %.1f", tc.description, score)
		})
	}
}

func TestGetCodecScore(t *testing.T) {
	testCases := []struct {
		codec       string
		lossless    bool
		bitrateKbps int
		expectedMin float64
		expectedMax float64
	}{
		{"flac", true, 0, 40.0, 40.0},
		{"alac", true, 0, 40.0, 40.0},
		{"pcm_s16le", true, 0, 40.0, 40.0},
		{"mp3", false, 320, 20.0, 20.0},
		{"mp3", false, 256, 18.0, 18.0},
		{"mp3", false, 192, 15.0, 15.0},
		{"mp3", false, 128, 12.0, 12.0},
		{"aac", false, 256, 25.0, 25.0},
		{"aac", false, 192, 22.0, 22.0},
		{"opus", false, 128, 22.0, 22.0},
		{"vorbis", false, 256, 22.0, 22.0},
	}

	for _, tc := range testCases {
		t.Run(tc.codec, func(t *testing.T) {
			score := getCodecScore(tc.codec, tc.lossless, tc.bitrateKbps)

			if score < tc.expectedMin || score > tc.expectedMax {
				t.Errorf("Codec %s (lossless=%v, bitrate=%d): expected %.1f-%.1f, got %.1f",
					tc.codec, tc.lossless, tc.bitrateKbps, tc.expectedMin, tc.expectedMax, score)
			}
		})
	}
}

func TestGetBitDepthScore(t *testing.T) {
	testCases := []struct {
		bitDepth int
		expected float64
	}{
		{24, 5.0},
		{20, 3.0},
		{16, 0.0},
		{8, -2.0},
		{0, -2.0},
	}

	for _, tc := range testCases {
		result := getBitDepthScore(tc.bitDepth)
		if result != tc.expected {
			t.Errorf("Bit depth %d: expected %.1f, got %.1f", tc.bitDepth, tc.expected, result)
		}
	}
}

func TestGetSampleRateScore(t *testing.T) {
	testCases := []struct {
		sampleRate int
		expected   float64
	}{
		{96000, 5.0},
		{192000, 5.0},
		{48000, 2.0},
		{44100, 0.0},
		{32000, -1.0},
		{22050, -3.0},
	}

	for _, tc := range testCases {
		result := getSampleRateScore(tc.sampleRate)
		if result != tc.expected {
			t.Errorf("Sample rate %d: expected %.1f, got %.1f", tc.sampleRate, tc.expected, result)
		}
	}
}

func TestGetTagCompletenessScore(t *testing.T) {
	testCases := []struct {
		name     string
		metadata *store.Metadata
		expected float64
	}{
		{
			name: "complete tags",
			metadata: &store.Metadata{
				TagArtist: "Artist",
				TagAlbum:  "Album",
				TagTitle:  "Title",
				TagTrack:  1,
			},
			expected: 5.0, // 1 + 1 + 1 + 1 + 1 (bonus)
		},
		{
			name: "partial tags",
			metadata: &store.Metadata{
				TagArtist: "Artist",
				TagTitle:  "Title",
			},
			expected: 2.0, // 1 + 1
		},
		{
			name:     "no tags",
			metadata: &store.Metadata{},
			expected: 0.0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getTagCompletenessScore(tc.metadata)
			if result != tc.expected {
				t.Errorf("Expected %.1f, got %.1f", tc.expected, result)
			}
		})
	}
}

func TestSelectWinner(t *testing.T) {
	// Create test members
	members := []scoredMember{
		{
			score: 50.0,
			file:  &store.File{ID: 1, SizeBytes: 10000, MtimeUnix: 1000, SrcPath: "/a.mp3"},
		},
		{
			score: 60.0, // Highest score - should win
			file:  &store.File{ID: 2, SizeBytes: 20000, MtimeUnix: 2000, SrcPath: "/b.flac"},
		},
		{
			score: 40.0,
			file:  &store.File{ID: 3, SizeBytes: 15000, MtimeUnix: 1500, SrcPath: "/c.mp3"},
		},
	}

	winner := selectWinner(members)

	if winner.file.ID != 2 {
		t.Errorf("Expected file ID 2 to win (highest score), got ID %d", winner.file.ID)
	}
}

func TestSelectWinnerTieBreakers(t *testing.T) {
	// Test tie-breakers when scores are equal
	members := []scoredMember{
		{
			score: 50.0,
			file:  &store.File{ID: 1, SizeBytes: 10000, MtimeUnix: 2000, SrcPath: "/a.mp3"},
		},
		{
			score: 50.0, // Same score, but larger file - should win
			file:  &store.File{ID: 2, SizeBytes: 20000, MtimeUnix: 2000, SrcPath: "/b.mp3"},
		},
		{
			score: 50.0,
			file:  &store.File{ID: 3, SizeBytes: 15000, MtimeUnix: 2000, SrcPath: "/c.mp3"},
		},
	}

	winner := selectWinner(members)

	if winner.file.ID != 2 {
		t.Errorf("Expected file ID 2 to win (largest file), got ID %d", winner.file.ID)
	}

	// Test mtime tie-breaker (when score and size are equal)
	membersTime := []scoredMember{
		{
			score: 50.0,
			file:  &store.File{ID: 1, SizeBytes: 10000, MtimeUnix: 2000, SrcPath: "/a.mp3"},
		},
		{
			score: 50.0,
			file:  &store.File{ID: 2, SizeBytes: 10000, MtimeUnix: 1000, SrcPath: "/b.mp3"}, // Older - should win
		},
	}

	winnerTime := selectWinner(membersTime)

	if winnerTime.file.ID != 2 {
		t.Errorf("Expected file ID 2 to win (older mtime), got ID %d", winnerTime.file.ID)
	}
}

func TestGetDurationProximityScore(t *testing.T) {
	testCases := []struct {
		dur1     int
		dur2     int
		expected float64
	}{
		{100000, 100000, 6.0},  // Exact match
		{100000, 101000, 6.0},  // 1s difference (within 1.5s)
		{100000, 101500, 6.0},  // 1.5s difference
		{100000, 103000, 3.0},  // 3s difference
		{100000, 105000, 1.0},  // 5s difference
		{100000, 110000, -2.0}, // 10s difference (penalty)
	}

	for _, tc := range testCases {
		result := GetDurationProximityScore(tc.dur1, tc.dur2)
		if result != tc.expected {
			t.Errorf("Duration delta %d: expected %.1f, got %.1f",
				abs(tc.dur1-tc.dur2)/1000, tc.expected, result)
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
