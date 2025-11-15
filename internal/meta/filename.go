package meta

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/franz/music-janitor/internal/store"
)

// FilenameMeta holds metadata parsed from filename and path
type FilenameMeta struct {
	Artist    string
	Album     string
	Title     string
	Track     int
	Disc      int
	Year      string
	Confidence float64 // 0.0-1.0 how confident we are in the parse
}

// ParseFilename attempts to extract metadata from a filename
func ParseFilename(path string) *FilenameMeta {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	dir := filepath.Dir(path)

	meta := &FilenameMeta{
		Confidence: 0.3, // Default low confidence
	}

	// Try various patterns
	patterns := []struct {
		re   *regexp.Regexp
		parse func(*FilenameMeta, []string)
		confidence float64
	}{
		{
			// Pattern: "01 - Artist - Title.mp3"
			re: regexp.MustCompile(`^(\d+)\s*[-_.]\s*(.+?)\s*[-_.]\s*(.+)$`),
			parse: func(m *FilenameMeta, matches []string) {
				m.Track, _ = strconv.Atoi(matches[1])
				m.Artist = strings.TrimSpace(matches[2])
				m.Title = strings.TrimSpace(matches[3])
			},
			confidence: 0.8,
		},
		{
			// Pattern: "01 - Title.mp3"
			re: regexp.MustCompile(`^(\d+)\s*[-_.]\s*(.+)$`),
			parse: func(m *FilenameMeta, matches []string) {
				m.Track, _ = strconv.Atoi(matches[1])
				m.Title = strings.TrimSpace(matches[2])
			},
			confidence: 0.7,
		},
		{
			// Pattern: "Artist - Title.mp3"
			re: regexp.MustCompile(`^(.+?)\s*[-_.]\s*(.+)$`),
			parse: func(m *FilenameMeta, matches []string) {
				m.Artist = strings.TrimSpace(matches[1])
				m.Title = strings.TrimSpace(matches[2])
			},
			confidence: 0.5,
		},
		{
			// Pattern: "01.Title.mp3" or "01_Title.mp3"
			re: regexp.MustCompile(`^(\d+)[._](.+)$`),
			parse: func(m *FilenameMeta, matches []string) {
				m.Track, _ = strconv.Atoi(matches[1])
				m.Title = strings.ReplaceAll(strings.TrimSpace(matches[2]), "_", " ")
			},
			confidence: 0.6,
		},
	}

	// Try each pattern
	for _, p := range patterns {
		if matches := p.re.FindStringSubmatch(name); matches != nil {
			p.parse(meta, matches)
			meta.Confidence = p.confidence
			break
		}
	}

	// If no pattern matched, use filename as title
	if meta.Title == "" {
		meta.Title = name
		meta.Confidence = 0.2
	}

	// Confidence bonuses for structured filenames
	// Track number presence indicates well-organized files
	if meta.Track > 0 {
		meta.Confidence += 0.15 // Boost confidence when track number detected
		if meta.Confidence > 1.0 {
			meta.Confidence = 1.0 // Cap at 1.0
		}
	}

	// Bonus for filenames with separators (well-structured)
	if strings.Contains(name, " - ") || strings.Contains(name, " _ ") {
		meta.Confidence += 0.05
		if meta.Confidence > 1.0 {
			meta.Confidence = 1.0
		}
	}

	// Try to extract album and artist from directory structure
	meta.inferFromPath(dir)

	return meta
}

// inferFromPath tries to extract album/artist from directory path
func (m *FilenameMeta) inferFromPath(dir string) {
	// Get parent directories
	parts := strings.Split(filepath.Clean(dir), string(filepath.Separator))
	if len(parts) == 0 {
		return
	}

	// Common pattern: /Artist/Album/tracks
	if len(parts) >= 2 {
		parentDir := parts[len(parts)-1] // Album
		grandParent := parts[len(parts)-2] // Artist

		// Try to parse "Artist - Album" or "Artist/Album"
		if m.Album == "" {
			m.Album = parentDir

			// Remove disc folder if present
			discPattern := regexp.MustCompile(`^(?i)(disc|cd|disk)\s*\d+$`)
			if discPattern.MatchString(m.Album) && len(parts) >= 3 {
				m.Album = parts[len(parts)-2]
				grandParent = parts[len(parts)-3]
			}
		}

		if m.Artist == "" {
			m.Artist = grandParent
		}

		// Try to extract year from album folder
		// Pattern: "2023 - Album Name" or "Album Name (2023)"
		if yearMatch := regexp.MustCompile(`^(\d{4})\s*[-_.]\s*(.+)$`).FindStringSubmatch(m.Album); yearMatch != nil {
			m.Year = yearMatch[1]
			m.Album = strings.TrimSpace(yearMatch[2])
		} else if yearMatch := regexp.MustCompile(`^(.+?)\s*\((\d{4})\)$`).FindStringSubmatch(m.Album); yearMatch != nil {
			m.Album = strings.TrimSpace(yearMatch[1])
			m.Year = yearMatch[2]
		}
	}

	// Try to detect disc number from directory
	if len(parts) >= 1 {
		lastDir := parts[len(parts)-1]
		if discMatch := regexp.MustCompile(`(?i)(disc|cd|disk)\s*(\d+)`).FindStringSubmatch(lastDir); discMatch != nil {
			m.Disc, _ = strconv.Atoi(discMatch[2])
		}
	}
}

// EnrichMetadata enriches store metadata with filename-based hints
// Only fills in missing fields
func EnrichMetadata(meta *store.Metadata, path string) {
	fileMeta := ParseFilename(path)

	// Use different confidence thresholds for different fields
	// Title is critical - use lower threshold (0.3) if title is empty
	// Artist requires higher confidence (0.5) to avoid false matches

	// Artist enrichment - require standard confidence threshold
	if fileMeta.Confidence >= 0.5 {
		if meta.TagArtist == "" && fileMeta.Artist != "" {
			meta.TagArtist = fileMeta.Artist
		}
	}

	// Title enrichment - use lower threshold for empty titles (safety net)
	// This prevents data loss when tags are completely missing
	titleConfidenceThreshold := 0.5
	if meta.TagTitle == "" {
		// Lower the bar for empty titles - better to have something from filename
		// than lose the track entirely due to path collisions
		titleConfidenceThreshold = 0.3
	}
	if fileMeta.Confidence >= titleConfidenceThreshold {
		if meta.TagTitle == "" && fileMeta.Title != "" {
			meta.TagTitle = fileMeta.Title
		}
	}

	// Album from path is usually reliable
	if meta.TagAlbum == "" && fileMeta.Album != "" {
		meta.TagAlbum = fileMeta.Album
	}

	// Track/Disc numbers from filename are quite reliable
	if meta.TagTrack == 0 && fileMeta.Track > 0 {
		meta.TagTrack = fileMeta.Track
	}
	if meta.TagDisc == 0 && fileMeta.Disc > 0 {
		meta.TagDisc = fileMeta.Disc
	}

	// Year from path
	if meta.TagDate == "" && fileMeta.Year != "" {
		meta.TagDate = fileMeta.Year
	}
}
