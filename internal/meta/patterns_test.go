package meta

import (
	"strings"
	"testing"

	"github.com/franz/music-janitor/internal/store"
)

func TestCleanAlbumNamePatterns(t *testing.T) {
	tests := []struct {
		input       string
		want        string
		description string
	}{
		// Format markers
		{"2014 - Clubland Vol.7-WEB", "2014 - Clubland Vol.7", "Remove -WEB suffix"},
		{"Album Name_WEB", "Album Name", "Remove _WEB suffix"},
		{"Album (WEB)", "Album", "Remove (WEB) suffix"},
		{"Album [WEB]", "Album", "Remove [WEB] suffix"},
		{"2013 - Hyperfine Interaction VINYL", "2013 - Hyperfine Interaction", "Remove VINYL suffix"},
		{"Album Name EP", "Album Name EP", "Keep EP in middle"},
		{"Album Name-EP", "Album Name", "Remove -EP suffix"},

		// Catalog numbers
		{"2022 - AH [HEAR0053]", "2022 - AH", "Remove catalog in brackets"},
		{"Album (MST027)", "Album", "Remove catalog in parens"},
		{"Album Name [ABC123]", "Album Name", "Remove catalog number"},

		// Website attribution
		{"Album [www.clubtone.net]", "Album", "Remove website bracket"},
		{"Album [by Esprit03]", "Album", "Remove artist attribution"},

		// Bootleg/Promo markers (these may stay if not in specific pattern)
		{"Live Bootleg Album", "Live Bootleg Album", "Bootleg in middle stays"},
		{"Album-Promo", "Album", "Remove promo marker"},
		{"Album (Promo)", "Album", "Remove promo in parens"},

		// Multiple cleanings
		{"Album [CAT123] WEB", "Album", "Remove multiple artifacts"},
		{"2014 - Album [CATALOG]", "2014 - Album", "Year + catalog"},

		// Edge cases
		{"", "", "Empty string"},
		{"Album", "Album", "No cleaning needed"},
		{"   Album   ", "Album", "Trim whitespace"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := &PatternCleaningResult{
				Changed:       false,
				FieldsCleaned: make([]string, 0),
				Warnings:      make([]string, 0),
			}
			got := cleanAlbumName(tt.input, result)
			if got != tt.want {
				t.Errorf("cleanAlbumName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDetectCompilation(t *testing.T) {
	tests := []struct {
		name        string
		metadata    *store.Metadata
		srcPath     string
		want        bool
		description string
	}{
		{
			name:        "Various Artists in path",
			metadata:    &store.Metadata{},
			srcPath:     "/music/Various Artists/2014 - Album/track.mp3",
			want:        true,
			description: "Path contains 'Various Artists'",
		},
		{
			name:        "Compilation in album name",
			metadata:    &store.Metadata{TagAlbum: "Kitsune Maison Compilation 15"},
			srcPath:     "/music/Album/track.mp3",
			want:        true,
			description: "Album contains 'compilation'",
		},
		{
			name:        "Mixed by in path",
			metadata:    &store.Metadata{},
			srcPath:     "/music/Artist/Album (Mixed by DJ)/track.mp3",
			want:        true,
			description: "Path contains 'mixed by'",
		},
		{
			name:        "_Singles folder",
			metadata:    &store.Metadata{},
			srcPath:     "/music/Artist/_Singles/track.mp3",
			want:        true,
			description: "Singles collection",
		},
		{
			name:        "Regular album",
			metadata:    &store.Metadata{TagAlbum: "Regular Album"},
			srcPath:     "/music/Artist/Album/track.mp3",
			want:        false,
			description: "Not a compilation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			got := detectCompilation(tt.metadata, tt.srcPath)
			if got != tt.want {
				t.Errorf("detectCompilation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractCatalogNumber(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Album [ABC123]", "ABC123"},
		{"Album (MST027)", "MST027"},
		{"Album [HEAR0053]", "HEAR0053"},
		{"Album [CAT12345]", "CAT12345"},
		{"Album Name", ""},
		{"Album [AB1]", ""}, // Too short
		{"Album [ABCDEFGH1234567]", ""}, // Too long
	}

	for _, tt := range tests {
		got := extractCatalogNumber(tt.input)
		if got != tt.want {
			t.Errorf("extractCatalogNumber(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsURLBased(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"https_soundcloud.com_artist", true},
		{"http_www.myspace.com_artist", true},
		{"www_facebook_com_artist", true},
		{"https://soundcloud.com/artist", true},
		{"djsoundtop.com", true},
		{"blogspot.com", true},
		{"Regular Album Name", false},
		{"Artist Name", false},
	}

	for _, tt := range tests {
		got := isURLBased(tt.input)
		if got != tt.want {
			t.Errorf("isURLBased(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestApplyPatternCleaning(t *testing.T) {
	tests := []struct {
		name         string
		metadata     *store.Metadata
		srcPath      string
		wantChanged  bool
		wantFields   []string
		wantWarnings []string
	}{
		{
			name: "Clean WEB album and detect compilation",
			metadata: &store.Metadata{
				TagAlbum: "Album Name-WEB",
				TagArtist: "Artist",
			},
			srcPath:     "/music/Various Artists/Album/track.mp3",
			wantChanged: true,
			wantFields:  []string{"album", "compilation_flag"},
		},
		{
			name: "Featured artist in title",
			metadata: &store.Metadata{
				TagTitle: "Song Title (feat. Guest Artist)",
			},
			srcPath:      "/music/track.mp3",
			wantChanged:  false, // Title cleaning doesn't modify feat. yet
			wantWarnings: []string{"featured_artist:Guest Artist"},
		},
		{
			name: "Catalog number extraction",
			metadata: &store.Metadata{
				TagAlbum: "Album [MST027]",
			},
			srcPath:     "/music/track.mp3",
			wantChanged: true,
			// Note: catalog number is extracted before cleaning, so warning should appear
			wantWarnings: []string{"catalog_number:MST027"},
		},
		{
			name: "Unknown Artist clearing",
			metadata: &store.Metadata{
				TagArtist: "Unknown Artist",
			},
			srcPath:      "/music/track.mp3",
			wantChanged:  true,
			wantFields:   []string{"artist"},
			wantWarnings: []string{"unknown_artist"},
		},
		{
			name: "No changes needed",
			metadata: &store.Metadata{
				TagAlbum:  "Clean Album",
				TagArtist: "Clean Artist",
			},
			srcPath:     "/music/track.mp3",
			wantChanged: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ApplyPatternCleaning(tt.metadata, tt.srcPath)

			if result.Changed != tt.wantChanged {
				t.Errorf("ApplyPatternCleaning() changed = %v, want %v", result.Changed, tt.wantChanged)
			}

			if tt.wantFields != nil {
				for _, field := range tt.wantFields {
					found := false
					for _, cleaned := range result.FieldsCleaned {
						if cleaned == field {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("ApplyPatternCleaning() missing field %q in cleaned fields %v", field, result.FieldsCleaned)
					}
				}
			}

			if tt.wantWarnings != nil {
				for _, warning := range tt.wantWarnings {
					found := false
					for _, w := range result.Warnings {
						if strings.Contains(w, warning) {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("ApplyPatternCleaning() missing warning %q in warnings %v", warning, result.Warnings)
					}
				}
			}
		})
	}
}

func TestCleanTitleName(t *testing.T) {
	tests := []struct {
		input       string
		want        string
		wantWarning bool
		description string
	}{
		{
			input:       "Song Title (feat. Guest)",
			want:        "Song Title (feat. Guest)",
			wantWarning: true,
			description: "Featured artist detected",
		},
		{
			input:       "Song Title (ft. Artist)",
			want:        "Song Title (ft. Artist)",
			wantWarning: true,
			description: "ft. variant",
		},
		{
			input:       "Regular Song Title",
			want:        "Regular Song Title",
			wantWarning: false,
			description: "No featured artist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := &PatternCleaningResult{
				Warnings: make([]string, 0),
			}
			got := cleanTitleName(tt.input, result)
			if got != tt.want {
				t.Errorf("cleanTitleName(%q) = %q, want %q", tt.input, got, tt.want)
			}
			hasWarning := len(result.Warnings) > 0
			if hasWarning != tt.wantWarning {
				t.Errorf("cleanTitleName(%q) warning = %v, want %v", tt.input, hasWarning, tt.wantWarning)
			}
		})
	}
}
