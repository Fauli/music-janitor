package meta

import (
	"testing"

	"github.com/franz/music-janitor/internal/store"
)

func TestBuildMetadataArgs(t *testing.T) {
	testCases := []struct {
		name     string
		metadata *store.Metadata
		expected int // number of metadata arguments (each field = 2 args: -metadata key=value)
	}{
		{
			name: "complete metadata",
			metadata: &store.Metadata{
				TagTitle:       "Test Title",
				TagArtist:      "Test Artist",
				TagAlbum:       "Test Album",
				TagAlbumArtist: "Album Artist",
				TagDate:        "2023",
				TagTrack:       1,
				TagTrackTotal:  10,
				TagDisc:        1,
				TagDiscTotal:   2,
			},
			expected: 14, // 7 fields * 2 args each (title, artist, album, album_artist, date, track, disc)
		},
		{
			name: "minimal metadata",
			metadata: &store.Metadata{
				TagTitle:  "Title Only",
				TagArtist: "Artist Only",
			},
			expected: 4, // 2 fields * 2 args each
		},
		{
			name:     "empty metadata",
			metadata: &store.Metadata{},
			expected: 0,
		},
		{
			name: "track without total",
			metadata: &store.Metadata{
				TagTitle: "Title",
				TagTrack: 5,
			},
			expected: 4, // title + track = 2 fields * 2 args
		},
		{
			name: "compilation flag",
			metadata: &store.Metadata{
				TagCompilation: true,
			},
			expected: 2, // compilation flag = 1 field * 2 args
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			args := buildMetadataArgs(tc.metadata)

			if len(args) != tc.expected {
				t.Errorf("Expected %d args, got %d", tc.expected, len(args))
			}

			// Verify args are in pairs of -metadata key=value
			if len(args)%2 != 0 {
				t.Error("Expected even number of arguments (-metadata flag followed by key=value)")
			}

			// Verify all odd indices are "-metadata"
			for i := 0; i < len(args); i += 2 {
				if args[i] != "-metadata" {
					t.Errorf("Expected '-metadata' at index %d, got %s", i, args[i])
				}
			}
		})
	}
}

func TestCanWriteTags(t *testing.T) {
	testCases := []struct {
		path     string
		expected bool
	}{
		{"/path/to/file.mp3", true},
		{"/path/to/file.MP3", true}, // Case insensitive
		{"/path/to/file.flac", true},
		{"/path/to/file.FLAC", true},
		{"/path/to/file.m4a", true},
		{"/path/to/file.ogg", true},
		{"/path/to/file.opus", true},
		{"/path/to/file.wav", true},
		{"/path/to/file.aiff", true},
		{"/path/to/file.ape", true},
		{"/path/to/file.wv", true},
		{"/path/to/file.tta", true},
		{"/path/to/file.mpc", true},
		{"/path/to/file.wma", true},
		{"/path/to/file.txt", false},
		{"/path/to/file.jpg", false},
		{"/path/to/file", false},
		{"", false},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			result := CanWriteTags(tc.path)
			if result != tc.expected {
				t.Errorf("CanWriteTags(%s): expected %v, got %v", tc.path, tc.expected, result)
			}
		})
	}
}

func TestBuildMetadataArgs_Format(t *testing.T) {
	metadata := &store.Metadata{
		TagTitle:  "Test Song",
		TagArtist: "Test Artist",
		TagTrack:  3,
		TagTrackTotal: 12,
		TagDisc: 2,
		TagDiscTotal: 3,
	}

	args := buildMetadataArgs(metadata)

	// Check that args contain expected key=value pairs
	expectedPairs := map[string]bool{
		"title=Test Song":    false,
		"artist=Test Artist": false,
		"track=3/12":         false,
		"disc=2/3":           false,
	}

	for i := 1; i < len(args); i += 2 {
		value := args[i]
		for pair := range expectedPairs {
			if value == pair {
				expectedPairs[pair] = true
			}
		}
	}

	for pair, found := range expectedPairs {
		if !found {
			t.Errorf("Expected to find metadata pair %s, but it was not found", pair)
		}
	}
}
