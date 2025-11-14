package meta

import (
	"regexp"
	"strings"

	"github.com/franz/music-janitor/internal/store"
)

// PatternCleaningResult tracks what was cleaned
type PatternCleaningResult struct {
	Changed       bool
	FieldsCleaned []string
	Warnings      []string
}

// ApplyPatternCleaning applies tree.txt-derived cleaning patterns to metadata
// This removes common artifacts found in messy music libraries
func ApplyPatternCleaning(metadata *store.Metadata, srcPath string) *PatternCleaningResult {
	result := &PatternCleaningResult{
		Changed:       false,
		FieldsCleaned: make([]string, 0),
		Warnings:      make([]string, 0),
	}

	// Clean album name
	if metadata.TagAlbum != "" {
		original := metadata.TagAlbum
		metadata.TagAlbum = cleanAlbumName(metadata.TagAlbum, result)
		if metadata.TagAlbum != original {
			result.Changed = true
			result.FieldsCleaned = append(result.FieldsCleaned, "album")
		}
	}

	// Clean artist name
	if metadata.TagArtist != "" {
		original := metadata.TagArtist
		metadata.TagArtist = cleanArtistName(metadata.TagArtist, result)
		if metadata.TagArtist != original {
			result.Changed = true
			result.FieldsCleaned = append(result.FieldsCleaned, "artist")
		}
	}

	// Clean title
	if metadata.TagTitle != "" {
		original := metadata.TagTitle
		metadata.TagTitle = cleanTitleName(metadata.TagTitle, result)
		if metadata.TagTitle != original {
			result.Changed = true
			result.FieldsCleaned = append(result.FieldsCleaned, "title")
		}
	}

	// Detect compilation from path/tags
	if !metadata.TagCompilation {
		if detectCompilation(metadata, srcPath) {
			metadata.TagCompilation = true
			result.Changed = true
			result.FieldsCleaned = append(result.FieldsCleaned, "compilation_flag")
		}
	}

	// Extract catalog number from album if present
	catalogNum := extractCatalogNumber(metadata.TagAlbum)
	if catalogNum != "" {
		result.Warnings = append(result.Warnings, "catalog_number:"+catalogNum)
	}

	return result
}

// cleanAlbumName removes common artifacts from album names
// Based on 30+ WEB, 20+ VINYL, 30+ EP examples from tree.txt
func cleanAlbumName(album string, result *PatternCleaningResult) string {
	original := album

	// Remove release format indicators (WEB, VINYL, EP, CD)
	// Pattern: "-WEB", "_WEB", " WEB", "(WEB)", "[WEB]" at end
	formatMarkers := []string{
		"-WEB", "_WEB", " WEB", "(WEB)", "[WEB]",
		"-VINYL", "_VINYL", " VINYL", "(VINYL)", "[VINYL]",
		"(CD)", "[CD]", " CD",
		"-EP", "_EP",
	}
	for _, marker := range formatMarkers {
		if strings.HasSuffix(album, marker) {
			album = strings.TrimSuffix(album, marker)
			album = strings.TrimSpace(album)
		}
	}

	// Remove catalog numbers: (CAT123), [BMR008], -(TIGER967BP)-
	// Pattern: 3-15 alphanumeric characters in brackets/parens
	catalogPattern := regexp.MustCompile(`[-\s]*[\(\[]([A-Z0-9]{3,15})[\)\]][-\s]*`)
	album = catalogPattern.ReplaceAllString(album, " ")

	// Remove website attribution brackets: [www.clubtone.net], [by Esprit03]
	webPattern := regexp.MustCompile(`\[(?:www\.|by\s|http)[^\]]+\]`)
	album = webPattern.ReplaceAllString(album, "")

	// Remove bootleg/promo markers (but keep in warning)
	bootlegPattern := regexp.MustCompile(`(?i)\s*[-_\(]\s*(bootleg|promo|promotion)\s*[-_\)]?\s*`)
	if bootlegPattern.MatchString(album) {
		result.Warnings = append(result.Warnings, "bootleg_or_promo")
		album = bootlegPattern.ReplaceAllString(album, " ")
	}

	// Remove "Only for promotion" type strings
	promoPattern := regexp.MustCompile(`(?i)only\s+for\s+promotion`)
	album = promoPattern.ReplaceAllString(album, "")

	// Collapse multiple spaces/dashes
	album = collapseWhitespace(album)
	album = strings.ReplaceAll(album, "--", "-")
	album = strings.ReplaceAll(album, "__", "_")

	// Trim separators
	album = strings.Trim(album, " -_")

	// If album became empty or is a URL, warn
	if album == "" || isURLBased(album) {
		result.Warnings = append(result.Warnings, "suspicious_album_name:"+original)
		return original // Keep original if cleaning made it empty
	}

	return album
}

// cleanArtistName removes artifacts from artist names
func cleanArtistName(artist string, result *PatternCleaningResult) string {
	// Remove "Unknown Artist" literal
	if strings.ToLower(artist) == "unknown artist" {
		result.Warnings = append(result.Warnings, "unknown_artist")
		return "" // Clear it so enrichment can try
	}

	// Trim whitespace
	artist = strings.TrimSpace(artist)

	return artist
}

// cleanTitleName removes artifacts from track titles
func cleanTitleName(title string, result *PatternCleaningResult) string {
	// Extract and clean featured artists from title
	// Pattern: "(feat. Artist)", "(ft. Artist)", "(Feat Artist)"
	featPattern := regexp.MustCompile(`\s*[\(\[]\s*(?:feat\.?|ft\.?|featuring)\s+([^)\]]+)[\)\]]`)
	if featPattern.MatchString(title) {
		matches := featPattern.FindStringSubmatch(title)
		if len(matches) > 1 {
			result.Warnings = append(result.Warnings, "featured_artist:"+strings.TrimSpace(matches[1]))
		}
		// Keep the feat. in title for now (preserves information)
	}

	// Trim whitespace
	title = strings.TrimSpace(title)

	return title
}

// detectCompilation checks if this should be marked as a compilation
// Based on 50+ Various Artists, Compilation examples from tree.txt
func detectCompilation(metadata *store.Metadata, srcPath string) bool {
	pathLower := strings.ToLower(srcPath)

	// Check for "Various Artists" in path
	if strings.Contains(pathLower, "various artists") ||
		strings.Contains(pathLower, "variousartists") ||
		strings.Contains(pathLower, "various_artists") {
		return true
	}

	// Check for "Compilation" in path
	if strings.Contains(pathLower, "compilation") {
		return true
	}

	// Check for "Mixed by" in path (DJ mixes)
	if strings.Contains(pathLower, "mixed by") ||
		strings.Contains(pathLower, "compiled by") ||
		strings.Contains(pathLower, "compiled & mixed") {
		return true
	}

	// Check for "_Singles" in path
	if strings.Contains(pathLower, "_singles") {
		return true
	}

	// Check album name
	if metadata.TagAlbum != "" {
		albumLower := strings.ToLower(metadata.TagAlbum)
		if strings.Contains(albumLower, "various") ||
			strings.Contains(albumLower, "compilation") ||
			strings.Contains(albumLower, "mixed by") {
			return true
		}
	}

	return false
}

// extractCatalogNumber extracts catalog number from album name
// Returns empty string if none found
func extractCatalogNumber(album string) string {
	// Pattern: [ABC123], (XYZ789), [MST027]
	pattern := regexp.MustCompile(`[\(\[]([A-Z]{2,5}\d{3,5})[\)\]]`)
	matches := pattern.FindStringSubmatch(album)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// isURLBased checks if string looks like a URL-based folder name
func isURLBased(s string) bool {
	lowerS := strings.ToLower(s)
	return strings.HasPrefix(lowerS, "http") ||
		strings.Contains(lowerS, "_soundcloud_") ||
		strings.Contains(lowerS, "_facebook_") ||
		strings.Contains(lowerS, "_myspace_") ||
		strings.Contains(lowerS, "www_") ||
		strings.Contains(lowerS, "blogspot") ||
		strings.Contains(lowerS, "djsoundtop")
}
