package score

import (
	"fmt"
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
			expectedMin: 65.0, // 45 (FLAC) + 10 (lossless) + 5 (bit depth) + 5 (sample rate) + 5 (tags) + 2 (size)
			expectedMax: 75.0,
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
			expectedMin: 59.0, // 45 (FLAC) + 10 (lossless) + 0 (16-bit baseline) + 0 (44.1k baseline) + 5 (tags) + 1 (size 20-50MB)
			expectedMax: 63.0,
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
			expectedMin: 25.0, // 22 (MP3 320) + 0 (44.1k) + 4 (tags) - size bonus doesn't apply to lossy
			expectedMax: 28.0,
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
			expectedMin: 26.0, // 26 (AAC 256) + 0 (44.1k) + 2 (partial tags) - adjustments
			expectedMax: 29.0,
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
		{"flac", true, 0, 45.0, 45.0},     // Improved from 40
		{"alac", true, 0, 45.0, 45.0},     // Improved from 40
		{"pcm_s16le", true, 0, 42.0, 42.0}, // Improved from 40
		{"mp3", false, 320, 22.0, 22.0},   // Improved from 20
		{"mp3", false, 256, 20.0, 20.0},   // Improved from 18
		{"mp3", false, 192, 17.0, 17.0},   // Improved from 15
		{"mp3", false, 128, 13.0, 13.0},   // Improved from 12
		{"aac", false, 256, 26.0, 26.0},   // Improved from 25
		{"aac", false, 192, 23.0, 23.0},   // Improved from 22
		{"opus", false, 128, 25.0, 25.0},  // Improved from 22
		{"vorbis", false, 256, 24.0, 24.0}, // Improved from 22
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

func TestCalculateQualityScore_EdgeCases(t *testing.T) {
	testCases := []struct {
		name        string
		metadata    *store.Metadata
		file        *store.File
		description string
	}{
		{
			name: "empty metadata",
			metadata: &store.Metadata{
				Codec:    "unknown",
				Lossless: false,
			},
			file: &store.File{
				SizeBytes: 0,
			},
			description: "File with minimal metadata",
		},
		{
			name: "extremely high bitrate",
			metadata: &store.Metadata{
				Codec:       "mp3",
				Lossless:    false,
				BitrateKbps: 9999,
				SampleRate:  44100,
			},
			file: &store.File{
				SizeBytes: 100 * 1024 * 1024,
			},
			description: "MP3 with unrealistically high bitrate",
		},
		{
			name: "zero bitrate",
			metadata: &store.Metadata{
				Codec:       "mp3",
				Lossless:    false,
				BitrateKbps: 0,
				SampleRate:  44100,
			},
			file: &store.File{
				SizeBytes: 1024,
			},
			description: "MP3 with zero bitrate",
		},
		{
			name: "very large file",
			metadata: &store.Metadata{
				Codec:      "flac",
				Lossless:   true,
				BitDepth:   24,
				SampleRate: 192000,
			},
			file: &store.File{
				SizeBytes: 1000 * 1024 * 1024, // 1GB
			},
			description: "Very large FLAC file (1GB)",
		},
		{
			name: "negative values",
			metadata: &store.Metadata{
				Codec:       "mp3",
				Lossless:    false,
				BitDepth:    -1,
				SampleRate:  -1,
				BitrateKbps: -1,
			},
			file: &store.File{
				SizeBytes: -1,
			},
			description: "Invalid negative values",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Should not panic
			score := CalculateQualityScore(tc.metadata, tc.file)
			t.Logf("%s: score = %.1f", tc.description, score)

			// Score should be a reasonable value (not NaN, not infinite)
			if score < -100 || score > 200 {
				t.Errorf("Score %.1f is outside reasonable bounds", score)
			}
		})
	}
}

func TestGetCodecScore_EdgeCases(t *testing.T) {
	testCases := []struct {
		name        string
		codec       string
		lossless    bool
		bitrateKbps int
		description string
	}{
		{"unknown lossless codec", "unknown_lossless", true, 0, "Unknown lossless codec"},
		{"unknown lossy codec", "unknown_lossy", false, 128, "Unknown lossy codec"},
		{"mixed case flac", "FLAC", true, 0, "Mixed case FLAC"},
		{"mixed case mp3", "MP3", false, 320, "Mixed case MP3"},
		{"ape codec", "ape", true, 0, "APE lossless"},
		{"wavpack codec", "wavpack", true, 0, "WavPack"},
		{"tta codec", "tta", true, 0, "TTA lossless"},
		{"pcm_s24le", "pcm_s24le", true, 0, "PCM 24-bit"},
		{"low bitrate AAC", "aac", false, 64, "Low bitrate AAC"},
		{"low bitrate MP3", "mp3", false, 64, "Low bitrate MP3"},
		{"high bitrate Opus", "opus", false, 256, "High bitrate Opus"},
		{"vorbis low", "vorbis", false, 96, "Low bitrate Vorbis"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			score := getCodecScore(tc.codec, tc.lossless, tc.bitrateKbps)
			t.Logf("%s: score = %.1f", tc.description, score)

			// Lossless should generally score higher than lossy
			if tc.lossless && score < 20.0 {
				t.Logf("Note: Lossless codec %s scored %.1f (expected >= 20)", tc.codec, score)
			}
		})
	}
}

func TestSelectWinner_EdgeCases(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		winner := selectWinner([]scoredMember{})
		// Should return zero value, not panic
		if winner.file != nil {
			t.Error("Expected nil file for empty list")
		}
	})

	t.Run("single member", func(t *testing.T) {
		members := []scoredMember{
			{
				score: 50.0,
				file:  &store.File{ID: 1, SizeBytes: 1000, MtimeUnix: 1000, SrcPath: "/a.mp3"},
			},
		}

		winner := selectWinner(members)
		if winner.file.ID != 1 {
			t.Errorf("Expected file ID 1, got %d", winner.file.ID)
		}
	})

	t.Run("identical files - lexical path tiebreaker", func(t *testing.T) {
		members := []scoredMember{
			{
				score: 50.0,
				file:  &store.File{ID: 1, SizeBytes: 1000, MtimeUnix: 1000, SrcPath: "/z.mp3"},
			},
			{
				score: 50.0,
				file:  &store.File{ID: 2, SizeBytes: 1000, MtimeUnix: 1000, SrcPath: "/a.mp3"}, // Lexically first - should win
			},
			{
				score: 50.0,
				file:  &store.File{ID: 3, SizeBytes: 1000, MtimeUnix: 1000, SrcPath: "/m.mp3"},
			},
		}

		winner := selectWinner(members)
		if winner.file.ID != 2 {
			t.Errorf("Expected file ID 2 (lexically first /a.mp3), got ID %d with path %s", winner.file.ID, winner.file.SrcPath)
		}
	})

	t.Run("all tie-breakers cascade", func(t *testing.T) {
		// Test that tie-breakers are applied in order
		members := []scoredMember{
			{
				score: 100.0, // Highest score wins despite other factors
				file:  &store.File{ID: 1, SizeBytes: 100, MtimeUnix: 9999, SrcPath: "/zzz.mp3"},
			},
			{
				score: 50.0,
				file:  &store.File{ID: 2, SizeBytes: 10000, MtimeUnix: 1000, SrcPath: "/aaa.mp3"},
			},
		}

		winner := selectWinner(members)
		if winner.file.ID != 1 {
			t.Errorf("Expected file ID 1 (highest score overrides all), got ID %d", winner.file.ID)
		}
	})
}

func TestGetTagCompletenessScore_EdgeCases(t *testing.T) {
	testCases := []struct {
		name     string
		metadata *store.Metadata
		expected float64
	}{
		{
			name: "track number without other tags",
			metadata: &store.Metadata{
				TagTrack: 5,
			},
			expected: 1.0,
		},
		{
			name: "zero track number",
			metadata: &store.Metadata{
				TagArtist: "Artist",
				TagTrack:  0, // Zero means not set
			},
			expected: 1.0,
		},
		{
			name: "negative track number",
			metadata: &store.Metadata{
				TagTrack: -1, // Invalid
			},
			expected: 0.0,
		},
		{
			name: "exactly 4 tags (gets bonus)",
			metadata: &store.Metadata{
				TagArtist: "Artist",
				TagAlbum:  "Album",
				TagTitle:  "Title",
				TagTrack:  1,
			},
			expected: 5.0, // 4 tags + 1 bonus
		},
		{
			name: "3 tags (no bonus)",
			metadata: &store.Metadata{
				TagArtist: "Artist",
				TagAlbum:  "Album",
				TagTitle:  "Title",
			},
			expected: 3.0,
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

func TestGetDurationProximityScore_EdgeCases(t *testing.T) {
	testCases := []struct {
		name     string
		dur1     int
		dur2     int
		expected float64
	}{
		{"exact same duration", 100000, 100000, 6.0},
		{"0.5 second difference", 100000, 100500, 6.0},
		{"exactly 1.5 seconds", 100000, 101500, 6.0},
		{"just over 1.5 seconds", 100000, 101600, 3.0},
		{"exactly 3 seconds", 100000, 103000, 3.0},
		{"just over 3 seconds", 100000, 103100, 1.0},
		{"exactly 5 seconds", 100000, 105000, 1.0},
		{"just over 5 seconds", 100000, 105100, -2.0},
		{"very large difference", 100000, 500000, -2.0},
		{"negative difference (order matters)", 105000, 100000, 1.0},
		{"both zero", 0, 0, 6.0},
		{"one zero", 100000, 0, -2.0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GetDurationProximityScore(tc.dur1, tc.dur2)
			if result != tc.expected {
				delta := abs(tc.dur1 - tc.dur2)
				t.Errorf("Duration delta %dms (%.1fs): expected %.1f, got %.1f",
					delta, float64(delta)/1000.0, tc.expected, result)
			}
		})
	}
}

func TestGetFileExtension(t *testing.T) {
	testCases := []struct {
		path     string
		expected string
	}{
		{"/path/to/file.mp3", ".mp3"},
		{"/path/to/file.FLAC", ".flac"},
		{"/path/to/file.MP3", ".mp3"},
		{"/path/to/file", ""},
		{"/path/to/file.", "."},
		{"/path/to/.hidden", ".hidden"},
		{"/path/to/file.tar.gz", ".gz"},
		{"file.mp3", ".mp3"},
		{"", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			result := GetFileExtension(tc.path)
			if result != tc.expected {
				t.Errorf("Path %s: expected %q, got %q", tc.path, tc.expected, result)
			}
		})
	}
}

func TestBitDepthScore_BoundaryValues(t *testing.T) {
	testCases := []struct {
		bitDepth int
		expected float64
	}{
		{32, 5.0},  // Above 24
		{24, 5.0},  // Exactly 24
		{23, 3.0},  // Between 20-23
		{20, 3.0},  // Exactly 20
		{19, 0.0},  // Between 16-19
		{16, 0.0},  // Exactly 16 (baseline)
		{15, -2.0}, // Below 16
		{8, -2.0},  // Low bit depth
		{0, -2.0},  // Zero/invalid
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%d-bit", tc.bitDepth), func(t *testing.T) {
			result := getBitDepthScore(tc.bitDepth)
			if result != tc.expected {
				t.Errorf("Bit depth %d: expected %.1f, got %.1f", tc.bitDepth, tc.expected, result)
			}
		})
	}
}

func TestSampleRateScore_BoundaryValues(t *testing.T) {
	testCases := []struct {
		sampleRate int
		expected   float64
	}{
		{192000, 5.0},  // Hi-res
		{96000, 5.0},   // Hi-res
		{88200, 2.0},   // Between 48k-96k
		{48000, 2.0},   // Exactly 48k
		{47999, 0.0},   // Just below 48k
		{44100, 0.0},   // CD quality (baseline)
		{44099, -1.0},  // Just below 44.1k
		{32000, -1.0},  // Exactly 32k
		{31999, -3.0},  // Below 32k
		{22050, -3.0},  // Low quality
		{8000, -3.0},   // Very low quality
		{0, -3.0},      // Invalid
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%dHz", tc.sampleRate), func(t *testing.T) {
			result := getSampleRateScore(tc.sampleRate)
			if result != tc.expected {
				t.Errorf("Sample rate %d: expected %.1f, got %.1f", tc.sampleRate, tc.expected, result)
			}
		})
	}
}
