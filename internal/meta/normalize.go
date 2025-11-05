package meta

import (
	"context"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// MusicBrainzNormalizer interface for MusicBrainz lookups
// This allows dependency injection and testing without circular imports
type MusicBrainzNormalizer interface {
	NormalizeArtistName(ctx context.Context, artistName string) (string, error)
}

var (
	// GlobalMBNormalizer is the optional MusicBrainz normalizer
	// Set this when MusicBrainz integration is enabled
	GlobalMBNormalizer MusicBrainzNormalizer
)

// NormalizeArtist normalizes an artist name for comparison
// Uses MusicBrainz if available, falls back to local rules
func NormalizeArtist(artist string) string {
	if artist == "" {
		return ""
	}

	// Try MusicBrainz normalization if available
	if GlobalMBNormalizer != nil {
		ctx := context.Background()
		canonical, err := GlobalMBNormalizer.NormalizeArtistName(ctx, artist)
		if err == nil && canonical != "" {
			// MusicBrainz succeeded - use canonical name but still apply local normalization
			artist = canonical
		}
		// If MusicBrainz fails, fall through to local rules
	}

	// Apply local normalization rules
	return normalizeArtistLocal(artist)
}

// normalizeArtistLocal performs local normalization without MusicBrainz
func normalizeArtistLocal(artist string) string {
	if artist == "" {
		return ""
	}

	// Unicode NFC normalization
	artist = norm.NFC.String(artist)

	// Lowercase
	artist = strings.ToLower(artist)

	// Trim whitespace
	artist = strings.TrimSpace(artist)

	// Handle "Artist, The" -> "the artist"
	if strings.HasSuffix(artist, ", the") {
		artist = "the " + strings.TrimSuffix(artist, ", the")
	}

	// Remove common punctuation
	artist = removePunctuation(artist)

	// Collapse multiple spaces
	artist = collapseWhitespace(artist)

	return artist
}

// NormalizeArtistWithoutMB normalizes an artist name without MusicBrainz
// Useful for fallback or when MusicBrainz is disabled
func NormalizeArtistWithoutMB(artist string) string {
	return normalizeArtistLocal(artist)
}

// NormalizeTitle normalizes a song title for comparison
func NormalizeTitle(title string) string {
	if title == "" {
		return ""
	}

	// Unicode NFC normalization
	title = norm.NFC.String(title)

	// Lowercase
	title = strings.ToLower(title)

	// Trim whitespace
	title = strings.TrimSpace(title)

	// Remove version suffixes in parentheses for clustering (keep base title)
	// e.g., "Song (Remix)" -> "song"
	// But we'll keep this in raw data
	title = removeVersionSuffixes(title)

	// Remove common punctuation
	title = removePunctuation(title)

	// Collapse whitespace
	title = collapseWhitespace(title)

	return title
}

// NormalizeAlbum normalizes an album name
func NormalizeAlbum(album string) string {
	if album == "" {
		return ""
	}

	// Unicode NFC normalization
	album = norm.NFC.String(album)

	// Lowercase
	album = strings.ToLower(album)

	// Trim whitespace
	album = strings.TrimSpace(album)

	// Collapse whitespace
	album = collapseWhitespace(album)

	return album
}

// CleanString performs basic string cleaning (Unicode, trim, collapse)
func CleanString(s string) string {
	if s == "" {
		return ""
	}

	// Unicode NFC normalization
	s = norm.NFC.String(s)

	// Trim whitespace
	s = strings.TrimSpace(s)

	// Collapse whitespace
	s = collapseWhitespace(s)

	return s
}

// removePunctuation removes common punctuation characters
func removePunctuation(s string) string {
	// Remove: . , ! ? ' " : ; - /
	replacer := strings.NewReplacer(
		".", "",
		",", "",
		"!", "",
		"?", "",
		"'", "",
		"\"", "",
		":", "",
		";", "",
		"-", " ",
		"_", " ",
		"&", "and",
		"/", "",
	)
	return replacer.Replace(s)
}

// collapseWhitespace replaces multiple spaces with a single space
func collapseWhitespace(s string) string {
	re := regexp.MustCompile(`\s+`)
	return strings.TrimSpace(re.ReplaceAllString(s, " "))
}

// removeVersionSuffixes removes common version suffixes for clustering
// e.g., (Remix), (Live), (Acoustic), [Remaster], etc.
func removeVersionSuffixes(s string) string {
	patterns := []string{
		`\s*\(.*?(remix|live|acoustic|demo|instrumental|radio edit|extended|version|mix).*?\)`,
		`\s*\[.*?(remaster|deluxe|bonus|edit|live|remix).*?\]`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(`(?i)` + pattern)
		s = re.ReplaceAllString(s, "")
	}

	return strings.TrimSpace(s)
}

// SanitizeFilename removes or replaces characters that are unsafe in filenames
func SanitizeFilename(s string) string {
	if s == "" {
		return ""
	}

	// Unicode NFC normalization
	s = norm.NFC.String(s)

	// Replace illegal characters with safe alternatives
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "",
		"?", "",
		"!", "",
		"\"", "'",
		"<", "",
		">", "",
		"|", "-",
	)
	s = replacer.Replace(s)

	// Remove control characters
	s = removeControlChars(s)

	// Collapse whitespace
	s = collapseWhitespace(s)

	// Trim whitespace and dots (dots at end can cause issues on Windows)
	s = strings.Trim(s, " .")

	return s
}

// removeControlChars removes non-printable control characters
func removeControlChars(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return -1
		}
		return r
	}, s)
}

// GetArtistForPath returns the artist to use for path generation
// Prefers AlbumArtist over Artist
func GetArtistForPath(albumArtist, artist string) string {
	if albumArtist != "" {
		return albumArtist
	}
	if artist != "" {
		return artist
	}
	return "Unknown Artist"
}

// GetAlbumForPath returns the album to use for path generation
func GetAlbumForPath(album string) string {
	if album != "" {
		return album
	}
	return "Unknown Album"
}

// GetTitleForPath returns the title to use for path generation
func GetTitleForPath(title, filename string) string {
	if title != "" {
		return title
	}
	// Use filename without extension as fallback
	return strings.TrimSuffix(filename, "")
}
