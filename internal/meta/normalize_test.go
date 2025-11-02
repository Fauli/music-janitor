package meta

import (
	"testing"
)

func TestNormalizeArtist(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"The Beatles", "the beatles"},
		{"Beatles, The", "the beatles"},
		{"AC/DC", "acdc"},
		{"  Artist Name  ", "artist name"},
		{"Artist-Name", "artist name"},
		{"Artist_Name", "artist name"},
		{"Björk", "björk"},
		{"", ""},
	}

	for _, tt := range tests {
		result := NormalizeArtist(tt.input)
		if result != tt.expected {
			t.Errorf("NormalizeArtist(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Song Title", "song title"},
		{"Song (Remix)", "song"},
		{"Song [Live]", "song"},
		{"Song (Acoustic Version)", "song"},
		{"Song - Remix", "song remix"},
		{"  Title  ", "title"},
		{"", ""},
	}

	for _, tt := range tests {
		result := NormalizeTitle(tt.input)
		if result != tt.expected {
			t.Errorf("NormalizeTitle(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Normal Title", "Normal Title"},
		{"Artist/Album", "Artist-Album"},
		{"Title: Subtitle", "Title- Subtitle"},
		{"Title? Yes!", "Title Yes"},
		{"Title\"Quote\"", "Title'Quote'"},
		{"Title|Pipe", "Title-Pipe"},
		{"Title<>", "Title"},
		{"Artist*", "Artist"},
		{"  Title  ", "Title"},
		{"Title...", "Title"},
		{"", ""},
	}

	for _, tt := range tests {
		result := SanitizeFilename(tt.input)
		if result != tt.expected {
			t.Errorf("SanitizeFilename(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestCleanString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  multiple   spaces  ", "multiple spaces"},
		{"tabs\t\there", "tabs here"},
		{"newlines\n\nhere", "newlines here"},
		{"Café", "Café"},
		{"", ""},
	}

	for _, tt := range tests {
		result := CleanString(tt.input)
		if result != tt.expected {
			t.Errorf("CleanString(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestGetArtistForPath(t *testing.T) {
	tests := []struct {
		albumArtist string
		artist      string
		expected    string
	}{
		{"Album Artist", "Track Artist", "Album Artist"},
		{"", "Track Artist", "Track Artist"},
		{"", "", "Unknown Artist"},
	}

	for _, tt := range tests {
		result := GetArtistForPath(tt.albumArtist, tt.artist)
		if result != tt.expected {
			t.Errorf("GetArtistForPath(%q, %q) = %q, expected %q",
				tt.albumArtist, tt.artist, result, tt.expected)
		}
	}
}

func TestIsLosslessCodec(t *testing.T) {
	tests := []struct {
		codec    string
		expected bool
	}{
		{"flac", true},
		{"FLAC", true},
		{"alac", true},
		{"ape", true},
		{"wavpack", true},
		{"pcm", true},
		{"mp3", false},
		{"aac", false},
		{"opus", false},
		{"", false},
	}

	for _, tt := range tests {
		result := isLosslessCodec(tt.codec)
		if result != tt.expected {
			t.Errorf("isLosslessCodec(%q) = %v, expected %v", tt.codec, result, tt.expected)
		}
	}
}
