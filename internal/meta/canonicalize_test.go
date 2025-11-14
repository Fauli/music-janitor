package meta

import (
	"testing"
)

func TestCanonicalizeArtistName(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		// Ampersand-prefixed artists
		{"&me", "&ME"},
		{"&ME", "&ME"},
		{"&Me", "&ME"},
		{"&lez", "&LEZ"},
		{"&LEZ", "&LEZ"},

		// AC/DC variations
		{"ac/dc", "AC/DC"},
		{"AC/DC", "AC/DC"},
		{"ac_dc", "AC/DC"},
		{"AC_DC", "AC/DC"},
		{"acdc", "AC/DC"},
		{"ACDC", "AC/DC"},

		// ABBA
		{"abba", "ABBA"},
		{"ABBA", "ABBA"},
		{"Abba", "ABBA"},

		// Other all-caps bands
		{"mgmt", "MGMT"},
		{"MGMT", "MGMT"},

		// Numeric-prefix artists
		{"2pac", "2pac"},        // Already lowercase, preserved
		{"2Pac", "2Pac"},        // Mixed case, preserved
		{"2PAC", "2pac"},        // All caps, converted to lowercase after digit
		{"2raumwohnung", "2raumwohnung"},    // All lowercase, preserved
		{"2Raumwohnung", "2Raumwohnung"},    // Mixed case, preserved
		{"2RAUMWOHNUNG", "2raumwohnung"},    // All caps, converted

		// Regular artists with title case
		{"the beatles", "The Beatles"},
		{"The Beatles", "The Beatles"},
		{"THE BEATLES", "The Beatles"},
		{"pink floyd", "Pink Floyd"},
		{"PINK FLOYD", "Pink Floyd"},

		// Artists with "the"
		{"the rolling stones", "The Rolling Stones"},

		// Artists with "and"
		{"simon and garfunkel", "Simon and Garfunkel"},
		{"SIMON AND GARFUNKEL", "Simon and Garfunkel"},

		// Artists with "feat."
		{"artist feat. other", "Artist feat. Other"},
		{"ARTIST FEAT. OTHER", "Artist feat. Other"},

		// Empty string
		{"", ""},

		// Single word
		{"madonna", "Madonna"},
		{"MADONNA", "Madonna"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := CanonicalizeArtistName(tc.input)
			if result != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestCleanAlbumName(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		// URL-based names (should return empty -> "Unknown Album")
		{"https_soundcloud.com_artist", ""},
		{"https_facebook.com_page", ""},
		{"www_djxiz_blogspot_com", ""},
		// This one has a year prefix but URL album, we get empty after cleanup
		// The year would be handled separately in path generation
		{"2013 - https_soundcloud.com_rootaccess", ""},

		// Catalog numbers
		{"Album Name-(CATALOG123)-WEB", "Album Name"},
		{"Andromeda EP-(BMR008)-WEB", "Andromeda EP"},
		{"FB-(TIGER967BP)-WEB", "FB"},

		// Release markers
		{"Clubland Vol.7-WEB", "Clubland Vol.7"},
		{"Album Name-WEB", "Album Name"},
		{"Album Name_WEB", "Album Name"},
		{"Album Name (WEB)", "Album Name"},
		{"Album Name [WEB]", "Album Name"},
		{"Album Name-VINYL", "Album Name"},
		{"Album Name (CD)", "Album Name"},

		// Website attribution
		{"Album [www.clubtone.net]", "Album"},
		{"Album [by Esprit03]", "Album"},

		// Complex case
		{"Andromeda EP-(BMR008)-WEB", "Andromeda EP"},

		// Normal album names (should be unchanged)
		{"Abbey Road", "Abbey Road"},
		{"The Dark Side of the Moon", "The Dark Side of the Moon"},

		// Empty string
		{"", ""},

		// Multiple spaces/dashes
		{"Album  Name--Test", "Album Name-Test"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := CleanAlbumName(tc.input)
			if result != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestToTitleCase(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		// Basic title case
		{"hello world", "Hello World"},
		{"HELLO WORLD", "Hello World"},

		// Articles and conjunctions
		{"the quick brown fox", "The Quick Brown Fox"},
		{"a day in the life", "A Day in the Life"},
		{"song of the year", "Song of the Year"},

		// feat. and ft.
		{"artist feat. other", "Artist feat. Other"},
		{"artist ft. other", "Artist ft. Other"},

		// vs/vs.
		{"artist vs other", "Artist vs Other"},
		{"artist vs. other", "Artist vs. Other"},

		// Empty string
		{"", ""},

		// Single word
		{"hello", "Hello"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := toTitleCase(tc.input)
			if result != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestCapitalizeWord(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		// Basic capitalization
		{"hello", "Hello"},
		{"HELLO", "Hello"},
		{"Hello", "Hello"},

		// Preserve internal caps (like McCartney)
		{"mcCartney", "McCartney"},
		{"McCartney", "McCartney"},

		// Empty string
		{"", ""},

		// Single letter
		{"a", "A"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := capitalizeWord(tc.input)
			if result != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, result)
			}
		})
	}
}
