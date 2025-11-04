package meta

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/franz/music-janitor/internal/store"
	"github.com/franz/music-janitor/internal/util"
)

// WriteTagsToFile writes metadata tags to an audio file
// Uses ffmpeg to write tags to the file in-place
// This modifies the destination file to include enriched metadata
func WriteTagsToFile(filePath string, metadata *store.Metadata) error {
	if metadata == nil {
		return fmt.Errorf("metadata is nil")
	}

	// Check if file exists
	if _, err := os.Stat(filePath); err != nil {
		return fmt.Errorf("file does not exist: %w", err)
	}

	// Build ffmpeg metadata arguments
	metadataArgs := buildMetadataArgs(metadata)
	if len(metadataArgs) == 0 {
		util.DebugLog("No metadata to write for %s", filePath)
		return nil // Nothing to write
	}

	// Create temporary output file
	tempPath := filePath + ".tagged"

	// Build ffmpeg command
	// ffmpeg -i input.mp3 -metadata title="Title" -metadata artist="Artist" -c copy output.mp3
	args := []string{
		"-i", filePath,
	}
	args = append(args, metadataArgs...)
	args = append(args,
		"-c", "copy", // Copy codec (don't re-encode)
		"-y",         // Overwrite output
		tempPath,
	)

	// Run ffmpeg
	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg failed: %w (output: %s)", err, string(output))
	}

	// Replace original file with tagged version
	if err := os.Remove(filePath); err != nil {
		os.Remove(tempPath) // Clean up temp file
		return fmt.Errorf("failed to remove original file: %w", err)
	}

	if err := os.Rename(tempPath, filePath); err != nil {
		return fmt.Errorf("failed to rename tagged file: %w", err)
	}

	util.DebugLog("Wrote tags to: %s", filePath)
	return nil
}

// buildMetadataArgs builds ffmpeg -metadata arguments from store.Metadata
func buildMetadataArgs(m *store.Metadata) []string {
	var args []string

	// Helper to add metadata field
	addMeta := func(key, value string) {
		if value != "" {
			args = append(args, "-metadata", fmt.Sprintf("%s=%s", key, value))
		}
	}

	// Core tags
	addMeta("title", m.TagTitle)
	addMeta("artist", m.TagArtist)
	addMeta("album", m.TagAlbum)
	addMeta("album_artist", m.TagAlbumArtist)
	addMeta("date", m.TagDate)

	// Track/disc numbers
	if m.TagTrack > 0 {
		if m.TagTrackTotal > 0 {
			addMeta("track", fmt.Sprintf("%d/%d", m.TagTrack, m.TagTrackTotal))
		} else {
			addMeta("track", fmt.Sprintf("%d", m.TagTrack))
		}
	}

	if m.TagDisc > 0 {
		if m.TagDiscTotal > 0 {
			addMeta("disc", fmt.Sprintf("%d/%d", m.TagDisc, m.TagDiscTotal))
		} else {
			addMeta("disc", fmt.Sprintf("%d", m.TagDisc))
		}
	}

	// Compilation flag
	if m.TagCompilation {
		addMeta("compilation", "1")
	}

	// MusicBrainz IDs
	addMeta("musicbrainz_trackid", m.MusicBrainzRecordingID)
	addMeta("musicbrainz_albumid", m.MusicBrainzReleaseID)

	return args
}

// CanWriteTags checks if we can write tags for this file format
func CanWriteTags(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))

	// Formats that support metadata tagging via ffmpeg
	supportedFormats := map[string]bool{
		".mp3":  true,
		".m4a":  true,
		".flac": true,
		".ogg":  true,
		".opus": true,
		".wma":  true,
		".wav":  true, // WAV supports ID3v2 tags
		".aiff": true,
		".ape":  true,
		".wv":   true, // WavPack
		".tta":  true,
		".mpc":  true,
	}

	return supportedFormats[ext]
}

// ValidateFFmpeg checks if ffmpeg is available
func ValidateFFmpeg() error {
	cmd := exec.Command("ffmpeg", "-version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg not found or not executable: %w", err)
	}
	return nil
}
