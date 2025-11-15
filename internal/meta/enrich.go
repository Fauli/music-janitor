package meta

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/franz/music-janitor/internal/store"
)

// EnrichmentResult tracks what was enriched
type EnrichmentResult struct {
	Enriched      bool
	FieldsChanged []string
}

// EnrichFromPathAndSiblings attempts to fill in missing metadata fields using path analysis
// and sibling file inference. This is called after primary metadata extraction.
// This provides more advanced enrichment than the basic filename parsing.
func EnrichFromPathAndSiblings(metadata *store.Metadata, srcPath string, db *store.Store) (*EnrichmentResult, error) {
	result := &EnrichmentResult{
		Enriched:      false,
		FieldsChanged: make([]string, 0),
	}

	// Extract directory structure components
	dir := filepath.Dir(srcPath)
	filename := filepath.Base(srcPath)

	// Get parent directory names
	parts := strings.Split(filepath.ToSlash(dir), "/")
	if len(parts) == 0 {
		return result, nil
	}

	// Try to enrich from path structure
	enrichFromPath(metadata, parts, filename, result)

	// Try to enrich from sibling files (if we have database access)
	if db != nil {
		enrichFromSiblings(metadata, dir, db, result)
	}

	return result, nil
}

// enrichFromPath extracts metadata from directory and filename patterns
func enrichFromPath(metadata *store.Metadata, pathParts []string, filename string, result *EnrichmentResult) {
	// Common pattern: .../Artist/Album/Track.mp3
	// or: .../Artist/YYYY - Album/Track.mp3
	if len(pathParts) >= 2 {
		albumPart := pathParts[len(pathParts)-1]
		artistPart := pathParts[len(pathParts)-2]

		// Extract artist from path if missing
		if metadata.TagArtist == "" && artistPart != "" {
			// Clean up artist name (skip numeric folders like "02/")
			if !isNumericFolder(artistPart) && !isSpecialFolder(artistPart) {
				metadata.TagArtist = artistPart
				result.Enriched = true
				result.FieldsChanged = append(result.FieldsChanged, "artist_from_path")
			}
		}

		// Extract album from path if missing
		if metadata.TagAlbum == "" && albumPart != "" {
			// Try to parse "YYYY - Album Name" pattern
			album, year := parseYearAlbumPattern(albumPart)
			if album != "" {
				metadata.TagAlbum = album
				result.Enriched = true
				result.FieldsChanged = append(result.FieldsChanged, "album_from_path")

				// Also extract year if found and not already set
				if year != "" && metadata.TagDate == "" {
					metadata.TagDate = year
					result.FieldsChanged = append(result.FieldsChanged, "year_from_path")
				}
			} else if !isNumericFolder(albumPart) && !isSpecialFolder(albumPart) {
				// Use folder name as-is
				metadata.TagAlbum = albumPart
				result.Enriched = true
				result.FieldsChanged = append(result.FieldsChanged, "album_from_path")
			}
		}

		// Extract disc number from path if missing
		if metadata.TagDisc == 0 {
			disc := extractDiscNumber(albumPart)
			if disc > 0 {
				metadata.TagDisc = disc
				result.Enriched = true
				result.FieldsChanged = append(result.FieldsChanged, "disc_from_path")
			}
		}
	}

	// Extract track number and title from filename if missing
	if metadata.TagTrack == 0 || metadata.TagTitle == "" {
		track, title := parseTrackFilename(filename)
		if track > 0 && metadata.TagTrack == 0 {
			metadata.TagTrack = track
			result.Enriched = true
			result.FieldsChanged = append(result.FieldsChanged, "track_from_filename")
		}
		if title != "" && metadata.TagTitle == "" {
			metadata.TagTitle = title
			result.Enriched = true
			result.FieldsChanged = append(result.FieldsChanged, "title_from_filename")
		}
	}
}

// enrichFromSiblings infers missing metadata from files in the same directory
func enrichFromSiblings(metadata *store.Metadata, dir string, db *store.Store, result *EnrichmentResult) {
	// Get sibling files (files in same directory)
	siblings, err := db.GetFilesByDirectory(dir)
	if err != nil || len(siblings) < 2 {
		// Not enough siblings or error
		return
	}

	// Infer artist from most common artist among siblings
	if metadata.TagArtist == "" {
		artist := mostCommonArtist(siblings, db)
		if artist != "" {
			metadata.TagArtist = artist
			result.Enriched = true
			result.FieldsChanged = append(result.FieldsChanged, "artist_from_siblings")
		}
	}

	// Infer album from most common album among siblings
	if metadata.TagAlbum == "" {
		album := mostCommonAlbum(siblings, db)
		if album != "" {
			metadata.TagAlbum = album
			result.Enriched = true
			result.FieldsChanged = append(result.FieldsChanged, "album_from_siblings")
		}
	}

	// Infer album artist from most common album artist among siblings
	if metadata.TagAlbumArtist == "" {
		albumArtist := mostCommonAlbumArtist(siblings, db)
		if albumArtist != "" {
			metadata.TagAlbumArtist = albumArtist
			result.Enriched = true
			result.FieldsChanged = append(result.FieldsChanged, "album_artist_from_siblings")
		}
	}
}

// parseYearAlbumPattern extracts album and year from "YYYY - Album Name" pattern
func parseYearAlbumPattern(s string) (album string, year string) {
	// Pattern: "2013 - Album Name"
	pattern := regexp.MustCompile(`^(\d{4})\s*-\s*(.+)$`)
	matches := pattern.FindStringSubmatch(s)
	if len(matches) == 3 {
		return strings.TrimSpace(matches[2]), matches[1]
	}
	return "", ""
}

// extractDiscNumber extracts disc number from folder names like "CD1", "CD 1", "Disc 1", etc.
func extractDiscNumber(s string) int {
	patterns := []string{
		`(?i)cd\s*(\d+)`,
		`(?i)disc\s*(\d+)`,
		`(?i)disk\s*(\d+)`,
		`\(CD\s*(\d+)\)`,
		`\(Disc\s*(\d+)\)`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(s)
		if len(matches) >= 2 {
			if num, err := strconv.Atoi(matches[1]); err == nil {
				return num
			}
		}
	}
	return 0
}

// parseTrackFilename extracts track number and title from filename
// Examples: "01 - Song Title.mp3" -> (1, "Song Title")
//           "03 Song Title.mp3" -> (3, "Song Title")
func parseTrackFilename(filename string) (track int, title string) {
	// Remove extension
	nameNoExt := strings.TrimSuffix(filename, filepath.Ext(filename))

	// Pattern 1: "01 - Song Title" or "01- Song Title"
	pattern1 := regexp.MustCompile(`^(\d{1,3})\s*-\s*(.+)$`)
	matches := pattern1.FindStringSubmatch(nameNoExt)
	if len(matches) == 3 {
		if num, err := strconv.Atoi(matches[1]); err == nil {
			return num, strings.TrimSpace(matches[2])
		}
	}

	// Pattern 2: "01 Song Title" (space separated)
	pattern2 := regexp.MustCompile(`^(\d{1,3})\s+(.+)$`)
	matches = pattern2.FindStringSubmatch(nameNoExt)
	if len(matches) == 3 {
		if num, err := strconv.Atoi(matches[1]); err == nil {
			return num, strings.TrimSpace(matches[2])
		}
	}

	// Pattern 3: "Track 01 - Song Title"
	pattern3 := regexp.MustCompile(`(?i)track\s+(\d{1,3})\s*-\s*(.+)$`)
	matches = pattern3.FindStringSubmatch(nameNoExt)
	if len(matches) == 3 {
		if num, err := strconv.Atoi(matches[1]); err == nil {
			return num, strings.TrimSpace(matches[2])
		}
	}

	// Pattern 4: "Artist - Album - 01 - Song Title" (extract last segment after track number)
	// Handles complex filenames like: "Die Ärzte - Runter mit den Spendierhosen - 01 - Wie es geht"
	pattern4 := regexp.MustCompile(`-\s*(\d{1,3})\s*-\s*(.+)$`)
	matches = pattern4.FindStringSubmatch(nameNoExt)
	if len(matches) == 3 {
		if num, err := strconv.Atoi(matches[1]); err == nil {
			return num, strings.TrimSpace(matches[2])
		}
	}

	// Pattern 5: "Artist - 01 - Song Title" (extract last segment, simpler variant)
	// Handles: "Led Zeppelin - 05 - Dancing Days"
	pattern5 := regexp.MustCompile(`-\s*(\d{1,3})\s*-\s*([^-]+)$`)
	matches = pattern5.FindStringSubmatch(nameNoExt)
	if len(matches) == 3 {
		if num, err := strconv.Atoi(matches[1]); err == nil {
			title := strings.TrimSpace(matches[2])
			// Only accept if title is reasonably long (not just a single word fragment)
			if len(title) > 1 {
				return num, title
			}
		}
	}

	// Pattern 6: "01 Title" (just track number and title, no separators)
	// Handles: "05 Dancing Days"
	pattern6 := regexp.MustCompile(`^(\d{1,3})\s+([A-Za-z].+)$`)
	matches = pattern6.FindStringSubmatch(nameNoExt)
	if len(matches) == 3 {
		if num, err := strconv.Atoi(matches[1]); err == nil {
			return num, strings.TrimSpace(matches[2])
		}
	}

	return 0, ""
}

// isNumericFolder checks if folder name is purely numeric (like "02/", "03/")
func isNumericFolder(s string) bool {
	matched, _ := regexp.MatchString(`^\d+$`, s)
	return matched
}

// isSpecialFolder checks if folder should be skipped for metadata inference
func isSpecialFolder(s string) bool {
	special := map[string]bool{
		"_Singles":       true,
		"Various Artists": true,
		"Unknown Artist":  true,
		"Unknown Album":   true,
		".":               true,
		"..":              true,
	}
	return special[s]
}

// mostCommonArtist returns the most frequent artist name among siblings
func mostCommonArtist(siblings []*store.File, db *store.Store) string {
	counts := make(map[string]int)

	for _, sibling := range siblings {
		metadata, err := db.GetMetadataByFileID(sibling.ID)
		if err != nil || metadata == nil {
			continue
		}
		if metadata.TagArtist != "" {
			counts[metadata.TagArtist]++
		}
	}

	return mostFrequent(counts)
}

// mostCommonAlbum returns the most frequent album name among siblings
func mostCommonAlbum(siblings []*store.File, db *store.Store) string {
	counts := make(map[string]int)

	for _, sibling := range siblings {
		metadata, err := db.GetMetadataByFileID(sibling.ID)
		if err != nil || metadata == nil {
			continue
		}
		if metadata.TagAlbum != "" {
			counts[metadata.TagAlbum]++
		}
	}

	return mostFrequent(counts)
}

// mostCommonAlbumArtist returns the most frequent album artist among siblings
func mostCommonAlbumArtist(siblings []*store.File, db *store.Store) string {
	counts := make(map[string]int)

	for _, sibling := range siblings {
		metadata, err := db.GetMetadataByFileID(sibling.ID)
		if err != nil || metadata == nil {
			continue
		}
		if metadata.TagAlbumArtist != "" {
			counts[metadata.TagAlbumArtist]++
		}
	}

	return mostFrequent(counts)
}

// mostFrequent returns the key with the highest count
// Requires >50% consensus for safety
func mostFrequent(counts map[string]int) string {
	if len(counts) == 0 {
		return ""
	}

	var maxKey string
	var maxCount int
	var totalCount int

	for key, count := range counts {
		totalCount += count
		if count > maxCount {
			maxCount = count
			maxKey = key
		}
	}

	// Require majority consensus (>50%)
	if float64(maxCount)/float64(totalCount) > 0.5 {
		return maxKey
	}

	return ""
}
