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
		// Basic normalization
		{"Song Title", "song title"},
		{"SONG TITLE", "song title"},
		{"  Song  Title  ", "song title"},

		// Version suffix removal (ALL suffixes now removed)
		{"Song (Remix)", "song"},
		{"Song [Live]", "song"},
		{"Song (Acoustic Version)", "song"},
		{"Song [2011 Remaster]", "song"},
		{"Song (Deluxe Edition)", "song"},
		{"Song - Remix", "song"},       // Trailing "Remix" pattern removed

		// Punctuation removal
		{"Song: Title!", "song title"},
		{"Song, Title?", "song title"},
		{"Song-Title", "song title"},
		{"Song_Title", "song title"},
		{"Song & Title", "song and title"},

		// Unicode normalization (NFC)
		{"Café", "café"}, // NFC normalization preserves composed characters

		// Empty/whitespace
		{"", ""},
		{"  Title  ", "title"},
	}

	for _, tt := range tests {
		result := NormalizeTitle(tt.input)
		if result != tt.expected {
			t.Errorf("NormalizeTitle(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestDetectVersionType(t *testing.T) {
	tests := []struct {
		title    string
		expected string
	}{
		// Studio versions (default, includes remasters)
		{"Song Title", "studio"},
		{"", "studio"},
		{"Song Title [2011 Remaster]", "studio"},
		{"Song Title (Remastered)", "studio"},
		{"Song Title [Deluxe Edition]", "studio"},
		{"Song Title (Anniversary Edition)", "studio"},
		{"Song Title [Bonus Track]", "studio"},

		// Live versions
		{"Song Title (Live)", "live"},
		{"Song Title [Live at Wembley]", "live"},
		{"Song Title (Live in Tokyo)", "live"},
		{"Song Title - Live", "live"},
		{"Song Title (Concert Version)", "live"},
		{"Song Title [Live Session]", "live"},

		// Acoustic/unplugged versions
		{"Song Title (Acoustic)", "acoustic"},
		{"Song Title [Acoustic Version]", "acoustic"},
		{"Song Title (Unplugged)", "acoustic"},
		{"Song Title - Unplugged", "acoustic"},
		{"Song Title (Acoustic Live)", "live"}, // Live takes precedence

		// Remixes and edits
		{"Song Title (Remix)", "remix"},
		{"Song Title [Radio Edit]", "remix"},
		{"Song Title (Club Mix)", "remix"},
		{"Song Title (Extended Version)", "remix"},
		{"Song Title [Dub Mix]", "remix"},
		{"Song Title (Bootleg)", "remix"},
		{"Song Title (Mashup)", "remix"},
		{"Song Title (DJ Edit)", "remix"},

		// Demo versions
		{"Song Title (Demo)", "demo"},
		{"Song Title [Demo Version]", "demo"},
		{"Song Title (Rough)", "demo"},
		{"Song Title (Alternate Take)", "demo"},
		{"Song Title [Outtake]", "demo"},
		{"Song Title (Unreleased)", "demo"},

		// Instrumental/karaoke
		{"Song Title (Instrumental)", "instrumental"},
		{"Song Title [Karaoke Version]", "instrumental"},
		{"Song Title (Backing Track)", "instrumental"},

		// Edge cases and precedence
		{"Song Title (Live Acoustic)", "live"},         // Live > Acoustic
		{"Song Title (Acoustic Remix)", "acoustic"},    // Acoustic > Remix
		{"Song Title (Demo Remix)", "remix"},           // Remix wins (more specific transformation)
		{"Song Title (Instrumental Live)", "live"},     // Live > Instrumental
		{"Song Title Remastered", "studio"},            // Remastered = Studio
		{"Song Title (2011 Remaster)", "studio"},       // Remaster not detected as remix
		{"Song Title (Radio Edit Remix)", "remix"},     // Has remix keyword
		{"Song Title [Extended Mix Version]", "remix"}, // Multiple remix keywords
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			result := DetectVersionType(tt.title)
			if result != tt.expected {
				t.Errorf("DetectVersionType(%q) = %q, want %q", tt.title, result, tt.expected)
			}
		})
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
