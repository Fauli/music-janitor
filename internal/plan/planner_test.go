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
			result := GenerateDestPath(tc.destRoot, tc.metadata, tc.srcPath)

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
