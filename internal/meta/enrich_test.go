package meta

import (
	"testing"

	"github.com/franz/music-janitor/internal/store"
)

func TestParseYearAlbumPattern(t *testing.T) {
	tests := []struct {
		input       string
		wantAlbum   string
		wantYear    string
		description string
	}{
		{"2013 - Egofm Vol. 2", "Egofm Vol. 2", "2013", "Standard year-album pattern"},
		{"2004 - Hello_ Is This Thing On_ - Single", "Hello_ Is This Thing On_ - Single", "2004", "Year with complex album name"},
		{"1998 - Room 112", "Room 112", "1998", "Simple year-album"},
		{"Album Without Year", "", "", "No year prefix"},
		{"2024-Album", "Album", "2024", "Year with dash no spaces"},
		{"2020 -  Album", "Album", "2020", "Extra spaces after dash"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			gotAlbum, gotYear := parseYearAlbumPattern(tt.input)
			if gotAlbum != tt.wantAlbum {
				t.Errorf("parseYearAlbumPattern(%q) album = %q, want %q", tt.input, gotAlbum, tt.wantAlbum)
			}
			if gotYear != tt.wantYear {
				t.Errorf("parseYearAlbumPattern(%q) year = %q, want %q", tt.input, gotYear, tt.wantYear)
			}
		})
	}
}

func TestExtractDiscNumber(t *testing.T) {
	tests := []struct {
		input       string
		want        int
		description string
	}{
		{"CD1", 1, "CD with digit"},
		{"CD 1", 1, "CD with space"},
		{"CD 2", 2, "CD 2"},
		{"Disc 1", 1, "Disc with space"},
		{"Disc1", 1, "Disc no space"},
		{"(CD 1)", 1, "CD in parens"},
		{"(Disc 2)", 2, "Disc in parens"},
		{"Album Name", 0, "No disc number"},
		{"CD", 0, "CD without number"},
		{"1998 - Greatest Hits (CD 1)", 1, "Disc in album name"},
		{"All Eyez On Me (CD1)", 1, "Disc at end"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			got := extractDiscNumber(tt.input)
			if got != tt.want {
				t.Errorf("extractDiscNumber(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseTrackFilename(t *testing.T) {
	tests := []struct {
		filename    string
		wantTrack   int
		wantTitle   string
		description string
	}{
		{"01 - Song Title.mp3", 1, "Song Title", "Standard track - title format"},
		{"03 Song Title.mp3", 3, "Song Title", "Track space title format"},
		{"12 - Artist - Song.flac", 12, "Artist - Song", "Track with artist"},
		{"Track 05 - Title.m4a", 5, "Title", "Track keyword format"},
		{"Song Without Number.mp3", 0, "", "No track number"},
		{"02- Title.mp3", 2, "Title", "Dash without space"},
		{"100 - Final Track.wav", 100, "Final Track", "Three digit track"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			gotTrack, gotTitle := parseTrackFilename(tt.filename)
			if gotTrack != tt.wantTrack {
				t.Errorf("parseTrackFilename(%q) track = %d, want %d", tt.filename, gotTrack, tt.wantTrack)
			}
			if gotTitle != tt.wantTitle {
				t.Errorf("parseTrackFilename(%q) title = %q, want %q", tt.filename, gotTitle, tt.wantTitle)
			}
		})
	}
}

func TestIsNumericFolder(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"02", true},
		{"03", true},
		{"123", true},
		{"Album", false},
		{"2013", true},
		{"02_", false},
		{"", false},
	}

	for _, tt := range tests {
		got := isNumericFolder(tt.input)
		if got != tt.want {
			t.Errorf("isNumericFolder(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestIsSpecialFolder(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"_Singles", true},
		{"Various Artists", true},
		{"Unknown Artist", true},
		{"Unknown Album", true},
		{".", true},
		{"..", true},
		{"Regular Album", false},
		{"Artist Name", false},
	}

	for _, tt := range tests {
		got := isSpecialFolder(tt.input)
		if got != tt.want {
			t.Errorf("isSpecialFolder(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestMostFrequent(t *testing.T) {
	tests := []struct {
		name   string
		counts map[string]int
		want   string
	}{
		{
			name:   "Clear majority (>50%)",
			counts: map[string]int{"Artist A": 7, "Artist B": 2, "Artist C": 1},
			want:   "Artist A", // 7/10 = 70%
		},
		{
			name:   "No majority",
			counts: map[string]int{"Artist A": 3, "Artist B": 3, "Artist C": 2},
			want:   "", // 3/8 = 37.5%, no >50%
		},
		{
			name:   "Exact 50% (should fail)",
			counts: map[string]int{"Artist A": 5, "Artist B": 5},
			want:   "", // 5/10 = 50%, need >50%
		},
		{
			name:   "51% majority",
			counts: map[string]int{"Artist A": 51, "Artist B": 49},
			want:   "Artist A", // 51/100 = 51%
		},
		{
			name:   "Empty counts",
			counts: map[string]int{},
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mostFrequent(tt.counts)
			if got != tt.want {
				t.Errorf("mostFrequent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEnrichFromPath(t *testing.T) {
	tests := []struct {
		name           string
		pathParts      []string
		filename       string
		initialMeta    *store.Metadata
		wantArtist     string
		wantAlbum      string
		wantYear       string
		wantDisc       int
		wantTrack      int
		wantTitle      string
		wantEnriched   bool
		wantFieldCount int
	}{
		{
			name:         "Standard Artist/Album/Track structure",
			pathParts:    []string{"/", "music", "16 Bit Lolitas", "2005 - Helen Savage"},
			filename:     "01 - Helen Savage (Original Mix).mp3",
			initialMeta:  &store.Metadata{},
			wantArtist:   "16 Bit Lolitas",
			wantAlbum:    "Helen Savage",
			wantYear:     "2005",
			wantTrack:    1,
			wantTitle:    "Helen Savage (Original Mix)",
			wantEnriched: true,
		},
		{
			name:         "Multi-disc album",
			pathParts:    []string{"/", "music", "2Pac", "1998 - Greatest Hits (CD 1)"},
			filename:     "02 - California Love.mp3",
			initialMeta:  &store.Metadata{},
			wantArtist:   "2Pac",
			wantAlbum:    "Greatest Hits (CD 1)",
			wantYear:     "1998",
			wantDisc:     1,
			wantTrack:    2,
			wantTitle:    "California Love",
			wantEnriched: true,
		},
		{
			name:         "Numeric folder (skip artist inference)",
			pathParts:    []string{"/", "music", "02", "Album Name"},
			filename:     "track.mp3",
			initialMeta:  &store.Metadata{},
			wantArtist:   "",
			wantAlbum:    "Album Name",
			wantEnriched: true, // Album enriched
		},
		{
			name:         "Already has metadata (no overwrite)",
			pathParts:    []string{"/", "music", "Artist", "Album"},
			filename:     "track.mp3",
			initialMeta:  &store.Metadata{TagArtist: "Existing Artist", TagAlbum: "Existing Album"},
			wantArtist:   "Existing Artist",
			wantAlbum:    "Existing Album",
			wantEnriched: false, // Nothing enriched
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &EnrichmentResult{
				Enriched:      false,
				FieldsChanged: make([]string, 0),
			}

			enrichFromPath(tt.initialMeta, tt.pathParts, tt.filename, result)

			if tt.wantArtist != "" && tt.initialMeta.TagArtist != tt.wantArtist {
				t.Errorf("enrichFromPath() artist = %q, want %q", tt.initialMeta.TagArtist, tt.wantArtist)
			}
			if tt.wantAlbum != "" && tt.initialMeta.TagAlbum != tt.wantAlbum {
				t.Errorf("enrichFromPath() album = %q, want %q", tt.initialMeta.TagAlbum, tt.wantAlbum)
			}
			if tt.wantYear != "" && tt.initialMeta.TagDate != tt.wantYear {
				t.Errorf("enrichFromPath() year = %q, want %q", tt.initialMeta.TagDate, tt.wantYear)
			}
			if tt.wantDisc != 0 && tt.initialMeta.TagDisc != tt.wantDisc {
				t.Errorf("enrichFromPath() disc = %d, want %d", tt.initialMeta.TagDisc, tt.wantDisc)
			}
			if tt.wantTrack != 0 && tt.initialMeta.TagTrack != tt.wantTrack {
				t.Errorf("enrichFromPath() track = %d, want %d", tt.initialMeta.TagTrack, tt.wantTrack)
			}
			if tt.wantTitle != "" && tt.initialMeta.TagTitle != tt.wantTitle {
				t.Errorf("enrichFromPath() title = %q, want %q", tt.initialMeta.TagTitle, tt.wantTitle)
			}
			if result.Enriched != tt.wantEnriched {
				t.Errorf("enrichFromPath() enriched = %v, want %v", result.Enriched, tt.wantEnriched)
			}
		})
	}
}
