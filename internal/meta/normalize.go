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

// removeVersionSuffixes removes ALL version suffixes for clustering
// This includes remasters, remixes, live versions, acoustic, demos, etc.
// The version type is captured separately in the cluster key
func removeVersionSuffixes(s string) string {
	patterns := []string{
		// Parentheses: (Remix), (Live), (Remaster), (Radio Edit), etc.
		`\s*\([^)]*?(remix|live|acoustic|demo|instrumental|radio|edit|extended|version|mix|remaster|deluxe|bonus|anniversary|edition|unplugged|session|concert|recording|alternate|original|single|album|explicit|clean|vocal|karaoke|cover).*?\)`,

		// Brackets: [Remaster], [Deluxe Edition], [Live], etc.
		`\s*\[[^\]]*?(remix|live|acoustic|demo|instrumental|radio|edit|extended|version|mix|remaster|deluxe|bonus|anniversary|edition|unplugged|session|concert|recording|alternate|original|single|album|explicit|clean|vocal|karaoke|cover).*?\]`,

		// Trailing patterns without punctuation: "Song Title Remastered", "Song Title Live"
		`\s+(remastered|remix|live|acoustic|demo|instrumental|unplugged)$`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(`(?i)` + pattern)
		s = re.ReplaceAllString(s, "")
	}

	return strings.TrimSpace(s)
}

// DetectVersionType detects the version type from a title string
// Returns: "remix", "live", "acoustic", "demo", "instrumental", or "studio" (default)
// Precedence: live > acoustic > remix > demo > instrumental > studio
func DetectVersionType(title string) string {
	if title == "" {
		return "studio"
	}

	lowerTitle := strings.ToLower(title)

	// Live performances (highest precedence - often explicitly marked)
	liveKeywords := []string{
		"live", "concert", "session", "unplugged live",
	}
	for _, keyword := range liveKeywords {
		if strings.Contains(lowerTitle, keyword) {
			return "live"
		}
	}

	// Acoustic/unplugged versions (second precedence)
	acousticKeywords := []string{
		"acoustic", "unplugged",
	}
	for _, keyword := range acousticKeywords {
		if strings.Contains(lowerTitle, keyword) {
			return "acoustic"
		}
	}

	// Remixes and edits (third precedence)
	remixKeywords := []string{
		"remix", " mix", "edit", "dub", "bootleg", "mashup",
		"radio", "club", "extended",
	}
	for _, keyword := range remixKeywords {
		// Exclude "remaster" which contains "remix"
		// Exclude "edition" which contains "edit"
		if strings.Contains(lowerTitle, keyword) &&
		   !strings.Contains(lowerTitle, "remaster") &&
		   !strings.Contains(lowerTitle, "edition") {
			return "remix"
		}
	}

	// Demo versions
	demoKeywords := []string{
		"demo", "rough", "alternate", "outtake", "unreleased",
	}
	for _, keyword := range demoKeywords {
		if strings.Contains(lowerTitle, keyword) {
			return "demo"
		}
	}

	// Instrumental/karaoke versions
	instrumentalKeywords := []string{
		"instrumental", "karaoke", "backing track",
	}
	for _, keyword := range instrumentalKeywords {
		if strings.Contains(lowerTitle, keyword) {
			return "instrumental"
		}
	}

	// Everything else is "studio" (includes remasters, deluxe, etc.)
	// Remaster/deluxe are considered same as studio version
	return "studio"
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

// CanonicalizeArtistName applies consistent capitalization rules to artist names
// This ensures that "&me" and "&ME" both become "&ME" in destination paths
func CanonicalizeArtistName(artist string) string {
	if artist == "" {
		return ""
	}

	//Unicode NFC normalization
	artist = norm.NFC.String(artist)

	// Trim whitespace
	artist = strings.TrimSpace(artist)

	// Handle special cases where ALL CAPS is the norm
	allCapsExceptions := map[string]string{
		"ac/dc":   "AC/DC",
		"acdc":    "AC/DC",
		"ac_dc":   "AC/DC",
		"abba":    "ABBA",
		"mgmt":    "MGMT",
		"mstrkrft": "MSTRKRFT",
		"unkle":   "UNKLE",
	}

	// Check if it's an all-caps exception
	lowerArtist := strings.ToLower(artist)
	if canonical, ok := allCapsExceptions[lowerArtist]; ok {
		return canonical
	}

	// Handle ampersand-prefixed artists (keep ampersand lowercase, capitalize rest)
	if strings.HasPrefix(artist, "&") || strings.HasPrefix(artist, "_&") {
		// Examples: "&me" -> "&ME", "&lez" -> "&lez"
		// Rule: If name is 2-3 chars after &, make it uppercase
		trimmed := strings.TrimPrefix(strings.TrimPrefix(artist, "_"), "&")
		if len(trimmed) <= 3 {
			return"&" + strings.ToUpper(trimmed)
		}
		// Otherwise apply title case
		return "&" + toTitleCase(trimmed)
	}

	// Handle numeric-prefix artists
	if len(artist) > 0 && artist[0] >= '0' && artist[0] <= '9' {
		// Examples: "2pac" -> "2Pac", "2raumwohnung" -> "2raumwohnung"
		return toTitleCase(artist)
	}

	// Default: Apply title case
	return toTitleCase(artist)
}

// toTitleCase applies smart title casing to a string
// Handles special cases like "feat.", "the", "and", etc.
func toTitleCase(s string) string {
	if s == "" {
		return ""
	}

	// Split on spaces and process each word
	words := strings.Fields(s)
	result := make([]string, len(words))

	// Words that should stay lowercase (unless first word)
	lowercaseWords := map[string]bool{
		"a": true, "an": true, "the": true,
		"and": true, "or": true, "but": true,
		"of": true, "in": true, "on": true, "at": true, "to": true, "for": true,
		"feat": true, "feat.": true, "ft": true, "ft.": true,
		"vs": true, "vs.": true,
	}

	for i, word := range words {
		lowerWord := strings.ToLower(word)

		// First word always capitalized
		if i == 0 {
			result[i] = capitalizeWord(word)
			continue
		}

		// Keep lowercase words lowercase (unless first)
		if lowercaseWords[lowerWord] {
			result[i] = lowerWord
			continue
		}

		// Capitalize all other words
		result[i] = capitalizeWord(word)
	}

	return strings.Join(result, " ")
}

// capitalizeWord capitalizes the first letter of a word
// Handles all-lowercase, all-uppercase, and mixed case intelligently
func capitalizeWord(word string) string {
	if word == "" {
		return ""
	}

	runes := []rune(word)

	// Check if word is all lowercase or all uppercase
	hasLower := false
	hasUpper := false
	for _, r := range runes {
		if unicode.IsLetter(r) {
			if unicode.IsLower(r) {
				hasLower = true
			}
			if unicode.IsUpper(r) {
				hasUpper = true
			}
		}
	}

	// If all uppercase or all lowercase, convert to title case
	if (hasUpper && !hasLower) || (hasLower && !hasUpper) {
		runes[0] = unicode.ToUpper(runes[0])
		for i := 1; i < len(runes); i++ {
			runes[i] = unicode.ToLower(runes[i])
		}
	} else {
		// Mixed case - just ensure first letter is uppercase (preserve rest like "McCartney")
		runes[0] = unicode.ToUpper(runes[0])
	}

	return string(runes)
}

// CleanAlbumName removes URLs, catalog numbers, and normalizes release type markers
// Examples:
//   "https_soundcloud.com_artist" -> ""
//   "Album Name-(CATALOG123)-WEB" -> "Album Name"
//   "Album Name EP-WEB" -> "Album Name EP"
func CleanAlbumName(album string) string {
	if album == "" {
		return ""
	}

	// Trim whitespace
	album = strings.TrimSpace(album)

	// Remove year prefix (YYYY - ) to check the actual album name
	// We'll add it back at the end if it exists
	var yearPrefix string
	yearPattern := regexp.MustCompile(`^(\d{4}\s*-\s*)`)
	if match := yearPattern.FindString(album); match != "" {
		yearPrefix = strings.TrimSpace(match)
		album = strings.TrimPrefix(album, match)
		album = strings.TrimSpace(album)
	}

	// Detect URL-based folder names
	if strings.HasPrefix(album, "http") || strings.Contains(album, "_soundcloud_") ||
		strings.Contains(album, "_facebook_") || strings.Contains(album, "www_") {
		// This is a URL-based name, return empty to trigger "Unknown Album"
		return ""
	}

	// Remove catalog numbers in parentheses or brackets
	// Examples: (CATALOG123), [BMR008], -(TIGER967BP)-
	catalogPattern := regexp.MustCompile(`[-\s]*[\(\[]?[A-Z0-9]{3,15}[\)\]]?[-\s]*`)
	album = catalogPattern.ReplaceAllString(album, " ")

	// Remove standalone WEB, VINYL markers (but preserve "WEB" in "Spider-Man: Into the Spider-Web")
	// Pattern: Match -WEB, _WEB, (WEB), [WEB] at end or as separate word
	releaseMarkers := []string{"-WEB", "_WEB", " WEB", "(WEB)", "[WEB]", "-VINYL", "_VINYL", " VINYL", "(VINYL)", "[VINYL]", "(CD)", "[CD]"}
	for _, marker := range releaseMarkers {
		album = strings.TrimSuffix(album, marker)
	}

	// Remove website attribution brackets
	// Examples: [www.clubtone.net], [by Esprit03]
	webPattern := regexp.MustCompile(`\[(?:www\.|by\s)[^\]]+\]`)
	album = webPattern.ReplaceAllString(album, "")

	// Collapse multiple spaces/dashes
	album = collapseWhitespace(album)
	album = strings.ReplaceAll(album, "--", "-")
	album = strings.ReplaceAll(album, "__", "_")

	// Trim trailing separators
	album = strings.Trim(album, " -_")

	// Restore year prefix if it existed and album is not empty
	if yearPrefix != "" && album != "" {
		return yearPrefix + " " + album
	}

	return album
}
