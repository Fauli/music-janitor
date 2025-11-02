package meta

import (
	"path/filepath"
	"testing"
)

func TestParseFilename(t *testing.T) {
	tests := []struct {
		path          string
		expectedTrack int
		expectedTitle string
		expectedArtist string
		minConfidence float64
	}{
		{
			path:          "/music/Artist/Album/01 - Artist - Title.mp3",
			expectedTrack: 1,
			expectedTitle: "Title",
			expectedArtist: "Artist",
			minConfidence: 0.7,
		},
		{
			path:          "/music/Artist/Album/01 - Title.flac",
			expectedTrack: 1,
			expectedTitle: "Title",
			minConfidence: 0.6,
		},
		{
			path:          "/music/Artist - Title.mp3",
			expectedTitle: "Title",
			expectedArtist: "Artist",
			minConfidence: 0.4,
		},
		{
			path:          "/music/01.Title.mp3",
			expectedTrack: 1,
			expectedTitle: "Title",
			minConfidence: 0.5,
		},
		{
			path:          "/music/Random Song.mp3",
			expectedTitle: "Random Song",
			minConfidence: 0.1,
		},
	}

	for _, tt := range tests {
		result := ParseFilename(tt.path)

		if result.Track != tt.expectedTrack {
			t.Errorf("ParseFilename(%q).Track = %d, expected %d",
				tt.path, result.Track, tt.expectedTrack)
		}

		if result.Title != tt.expectedTitle {
			t.Errorf("ParseFilename(%q).Title = %q, expected %q",
				tt.path, result.Title, tt.expectedTitle)
		}

		if tt.expectedArtist != "" && result.Artist != tt.expectedArtist {
			t.Errorf("ParseFilename(%q).Artist = %q, expected %q",
				tt.path, result.Artist, tt.expectedArtist)
		}

		if result.Confidence < tt.minConfidence {
			t.Errorf("ParseFilename(%q).Confidence = %f, expected >= %f",
				tt.path, result.Confidence, tt.minConfidence)
		}
	}
}

func TestInferFromPath(t *testing.T) {
	tests := []struct {
		path           string
		expectedArtist string
		expectedAlbum  string
		expectedDisc   int
	}{
		{
			path:           "/music/The Beatles/Abbey Road/01 - Come Together.mp3",
			expectedArtist: "The Beatles",
			expectedAlbum:  "Abbey Road",
		},
		{
			path:           "/music/Artist/2023 - Album/track.mp3",
			expectedArtist: "Artist",
			expectedAlbum:  "Album",
		},
		{
			path:           "/music/Artist/Album (2023)/track.mp3",
			expectedArtist: "Artist",
			expectedAlbum:  "Album",
		},
		{
			path:           "/music/Artist/Album/Disc 2/track.mp3",
			expectedArtist: "Artist",
			expectedAlbum:  "Album",
			expectedDisc:   2,
		},
	}

	for _, tt := range tests {
		meta := ParseFilename(tt.path)

		if meta.Artist != tt.expectedArtist {
			t.Errorf("Path %q: Artist = %q, expected %q",
				tt.path, meta.Artist, tt.expectedArtist)
		}

		if meta.Album != tt.expectedAlbum {
			t.Errorf("Path %q: Album = %q, expected %q",
				tt.path, meta.Album, tt.expectedAlbum)
		}

		if meta.Disc != tt.expectedDisc {
			t.Errorf("Path %q: Disc = %d, expected %d",
				tt.path, meta.Disc, tt.expectedDisc)
		}
	}
}

func TestYearExtraction(t *testing.T) {
	tests := []struct {
		path         string
		expectedYear string
	}{
		{
			path:         "/music/Artist/2023 - Album/track.mp3",
			expectedYear: "2023",
		},
		{
			path:         "/music/Artist/Album (2020)/track.mp3",
			expectedYear: "2020",
		},
		{
			path:         "/music/Artist/Album/track.mp3",
			expectedYear: "",
		},
	}

	for _, tt := range tests {
		meta := ParseFilename(tt.path)

		if meta.Year != tt.expectedYear {
			t.Errorf("Path %q: Year = %q, expected %q",
				tt.path, meta.Year, tt.expectedYear)
		}
	}
}

func TestDiscNumberExtraction(t *testing.T) {
	tests := []struct {
		dir          string
		expectedDisc int
	}{
		{"Disc 1", 1},
		{"Disc 2", 2},
		{"CD1", 1},
		{"CD 3", 3},
		{"Disk 4", 4},
		{"Album", 0},
	}

	for _, tt := range tests {
		path := filepath.Join("/music/Artist/Album", tt.dir, "track.mp3")
		meta := ParseFilename(path)

		if meta.Disc != tt.expectedDisc {
			t.Errorf("Directory %q: Disc = %d, expected %d",
				tt.dir, meta.Disc, tt.expectedDisc)
		}
	}
}
