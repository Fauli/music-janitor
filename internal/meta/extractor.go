package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dhowden/tag"
	"github.com/franz/music-janitor/internal/report"
	"github.com/franz/music-janitor/internal/store"
	"github.com/franz/music-janitor/internal/util"
)

// Extractor extracts metadata from audio files
type Extractor struct {
	store       *store.Store
	concurrency int
	logger      *report.EventLogger
}

// Config holds extractor configuration
type Config struct {
	Store       *store.Store
	Concurrency int
	Logger      *report.EventLogger
}

// New creates a new metadata extractor
func New(cfg *Config) *Extractor {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 4
	}

	return &Extractor{
		store:       cfg.Store,
		concurrency: cfg.Concurrency,
		logger:      cfg.Logger,
	}
}

// Result represents extraction results
type Result struct {
	Processed int
	Success   int
	Errors    []error
}

// Extract extracts metadata for all discovered files
func (e *Extractor) Extract(ctx context.Context) (*Result, error) {
	util.InfoLog("Starting metadata extraction")

	// Get files with status "discovered"
	files, err := e.store.GetFilesByStatus("discovered")
	if err != nil {
		return nil, fmt.Errorf("failed to get files: %w", err)
	}

	if len(files) == 0 {
		util.InfoLog("No files to process")
		return &Result{}, nil
	}

	totalFiles := len(files)
	util.InfoLog("Found %d files to process", totalFiles)

	result := &Result{
		Errors: make([]error, 0),
	}

	// Counters for progress reporting
	var processed atomic.Int64
	var success atomic.Int64
	var errors atomic.Int64

	// Start progress reporter
	progressCtx, cancelProgress := context.WithCancel(ctx)
	defer cancelProgress()

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-progressCtx.Done():
				return
			case <-ticker.C:
				p := processed.Load()
				s := success.Load()
				e := errors.Load()

				if p > 0 {
					percentage := float64(p) / float64(totalFiles) * 100
					util.InfoLog("Extracting metadata: %d/%d (%.1f%%) - success: %d, errors: %d",
						p, totalFiles, percentage, s, e)
				}
			}
		}
	}()

	// Process files (TODO: add worker pool for concurrency)
	for _, file := range files {
		select {
		case <-ctx.Done():
			result.Processed = int(processed.Load())
			result.Success = int(success.Load())
			return result, ctx.Err()
		default:
		}

		processed.Add(1)

		metadata, err := e.extractFile(file)
		if err != nil {
			util.ErrorLog("Failed to extract metadata for %s: %v", file.SrcPath, err)
			result.Errors = append(result.Errors, err)
			errors.Add(1)

			// Log error event
			if e.logger != nil {
				e.logger.LogMeta(file.FileKey, file.SrcPath, "", false, err)
			}

			// Update file status to error
			e.store.UpdateFileStatus(file.ID, "error", err.Error())
		} else {
			success.Add(1)

			// Log success event
			if e.logger != nil {
				e.logger.LogMeta(file.FileKey, file.SrcPath, metadata.Codec, metadata.Lossless, nil)
			}

			// Update file status to meta_ok
			e.store.UpdateFileStatus(file.ID, "meta_ok", "")
		}
	}

	cancelProgress()

	// Update result with final counts
	result.Processed = int(processed.Load())
	result.Success = int(success.Load())

	util.SuccessLog("Metadata extraction complete: %d processed, %d success, %d errors",
		result.Processed, result.Success, len(result.Errors))

	return result, nil
}

// extractFile extracts metadata from a single file
func (e *Extractor) extractFile(file *store.File) (*store.Metadata, error) {
	util.DebugLog("Extracting metadata: %s", file.SrcPath)

	var metadata *store.Metadata

	// Try tag library for tags
	tagMetadata, tagErr := e.extractWithTag(file.SrcPath)

	// Always try ffprobe for audio properties (codec, bitrate, sample rate, etc.)
	// ffprobe provides comprehensive audio properties that tag libraries don't
	ffprobeMetadata, ffprobeErr := e.extractWithFFprobe(file.SrcPath)

	if tagErr != nil && ffprobeErr != nil {
		return nil, fmt.Errorf("all extraction methods failed: tag: %v, ffprobe: %v", tagErr, ffprobeErr)
	}

	// Merge results: prefer tag library for tags, ffprobe for audio properties
	if ffprobeMetadata != nil {
		metadata = ffprobeMetadata // Start with ffprobe (has audio properties)

		// Overlay tags from tag library if available (often more accurate for tags)
		if tagMetadata != nil {
			if tagMetadata.TagTitle != "" {
				metadata.TagTitle = tagMetadata.TagTitle
			}
			if tagMetadata.TagArtist != "" {
				metadata.TagArtist = tagMetadata.TagArtist
			}
			if tagMetadata.TagAlbum != "" {
				metadata.TagAlbum = tagMetadata.TagAlbum
			}
			if tagMetadata.TagAlbumArtist != "" {
				metadata.TagAlbumArtist = tagMetadata.TagAlbumArtist
			}
			if tagMetadata.TagDate != "" {
				metadata.TagDate = tagMetadata.TagDate
			}
			if tagMetadata.TagTrack > 0 {
				metadata.TagTrack = tagMetadata.TagTrack
				metadata.TagTrackTotal = tagMetadata.TagTrackTotal
			}
			if tagMetadata.TagDisc > 0 {
				metadata.TagDisc = tagMetadata.TagDisc
				metadata.TagDiscTotal = tagMetadata.TagDiscTotal
			}
			if tagMetadata.Format != "" {
				metadata.Format = tagMetadata.Format
			}
		}
	} else if tagMetadata != nil {
		metadata = tagMetadata // Fallback to tag-only if ffprobe failed
	} else {
		return nil, fmt.Errorf("no metadata could be extracted")
	}

	// Set file ID
	metadata.FileID = file.ID

	// Enrich with filename-based hints for missing fields
	EnrichMetadata(metadata, file.SrcPath)

	// Store metadata
	if err := e.store.InsertMetadata(metadata); err != nil {
		return nil, fmt.Errorf("failed to store metadata: %w", err)
	}

	return metadata, nil
}

// extractWithTag uses dhowden/tag library to extract metadata
func (e *Extractor) extractWithTag(path string) (*store.Metadata, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read tags: %w", err)
	}

	// Build metadata struct
	metadata := &store.Metadata{
		Format:    string(m.Format()),
		TagArtist: m.Artist(),
		TagAlbum:  m.Album(),
		TagTitle:  m.Title(),
		TagAlbumArtist: m.AlbumArtist(),
	}

	// Extract year from various formats
	if m.Year() > 0 {
		metadata.TagDate = fmt.Sprintf("%d", m.Year())
	}

	// Track and disc numbers
	track, total := m.Track()
	metadata.TagTrack = track
	metadata.TagTrackTotal = total

	disc, discTotal := m.Disc()
	metadata.TagDisc = disc
	metadata.TagDiscTotal = discTotal

	// Extract compilation flag from raw tags
	// Different formats use different keys: TCMP (ID3v2), cpil (MP4), COMPILATION (Vorbis)
	if rawMap := m.Raw(); rawMap != nil {
		// Check common compilation tag keys
		for _, key := range []string{"TCMP", "cpil", "COMPILATION", "compilation", "Compilation"} {
			if val, ok := rawMap[key]; ok {
				// Handle different value types
				switch v := val.(type) {
				case string:
					metadata.TagCompilation = (v == "1" || v == "true" || v == "TRUE")
				case int:
					metadata.TagCompilation = (v == 1)
				case bool:
					metadata.TagCompilation = v
				}
				if metadata.TagCompilation {
					break // Found it
				}
			}
		}
	}

	// Store raw tags as JSON
	rawTags := map[string]interface{}{
		"format":       m.Format(),
		"file_type":    m.FileType(),
		"artist":       m.Artist(),
		"album":        m.Album(),
		"title":        m.Title(),
		"album_artist": m.AlbumArtist(),
		"composer":     m.Composer(),
		"genre":        m.Genre(),
		"year":         m.Year(),
		"track":        track,
		"track_total":  total,
		"disc":         disc,
		"disc_total":   discTotal,
		"compilation":  metadata.TagCompilation,
	}

	rawJSON, _ := json.Marshal(rawTags)
	metadata.RawTagsJSON = string(rawJSON)

	// Note: dhowden/tag doesn't provide audio properties (bitrate, sample rate, etc.)
	// We'll need ffprobe for that
	return metadata, nil
}

// extractWithFFprobe uses ffprobe to extract metadata (fallback)
func (e *Extractor) extractWithFFprobe(path string) (*store.Metadata, error) {
	// Get ffprobe info
	info, err := RunFFprobe(path)
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	metadata := &store.Metadata{}

	// Extract format info
	if info.Format != nil {
		metadata.Container = info.Format.FormatName
		if info.Format.Duration != "" {
			// Parse duration (in seconds)
			var durationSec float64
			fmt.Sscanf(info.Format.Duration, "%f", &durationSec)
			metadata.DurationMs = int(durationSec * 1000)
		}

		// Parse bitrate
		if info.Format.BitRate != "" {
			var bitrate int
			fmt.Sscanf(info.Format.BitRate, "%d", &bitrate)
			metadata.BitrateKbps = bitrate / 1000
		}

		// Extract tags from format
		if tags := info.Format.Tags; tags != nil {
			metadata.TagArtist = getTag(tags, "artist", "ARTIST")
			metadata.TagAlbum = getTag(tags, "album", "ALBUM")
			metadata.TagTitle = getTag(tags, "title", "TITLE")
			metadata.TagAlbumArtist = getTag(tags, "album_artist", "ALBUM_ARTIST", "albumartist")
			metadata.TagDate = getTag(tags, "date", "DATE", "year", "YEAR")

			// Parse compilation flag
			compilationTag := getTag(tags, "compilation", "COMPILATION", "Compilation")
			metadata.TagCompilation = (compilationTag == "1" || compilationTag == "true")

			// Parse track/disc numbers
			if trackStr := getTag(tags, "track", "TRACK"); trackStr != "" {
				fmt.Sscanf(trackStr, "%d", &metadata.TagTrack)
			}
			if discStr := getTag(tags, "disc", "DISC"); discStr != "" {
				fmt.Sscanf(discStr, "%d", &metadata.TagDisc)
			}
		}
	}

	// Extract stream info (audio properties)
	if len(info.Streams) > 0 {
		stream := info.Streams[0] // First audio stream
		metadata.Codec = stream.CodecName
		metadata.SampleRate = stream.SampleRate
		metadata.Channels = stream.Channels

		// Determine if lossless
		metadata.Lossless = isLosslessCodec(stream.CodecName)

		// Bit depth (if available)
		if stream.BitsPerSample.Value > 0 {
			metadata.BitDepth = stream.BitsPerSample.Value
		} else if stream.BitsPerRawSample.Value > 0 {
			metadata.BitDepth = stream.BitsPerRawSample.Value
		}
	}

	// Store raw ffprobe output
	rawJSON, _ := json.Marshal(info)
	metadata.RawTagsJSON = string(rawJSON)

	return metadata, nil
}

// getTag retrieves a tag value from a map, trying multiple keys
func getTag(tags map[string]string, keys ...string) string {
	for _, key := range keys {
		if val, ok := tags[key]; ok && val != "" {
			return val
		}
	}
	return ""
}

// isLosslessCodec checks if a codec is lossless
func isLosslessCodec(codec string) bool {
	codec = strings.ToLower(codec)
	lossless := map[string]bool{
		"flac":    true,
		"alac":    true,
		"ape":     true,
		"wavpack": true,
		"wv":      true,
		"tta":     true,
		"pcm":     true,
		"wav":     true,
		"aiff":    true,
	}
	// Also check for PCM variants (pcm_s16le, pcm_s16be, pcm_s24le, etc.)
	if strings.HasPrefix(codec, "pcm_") {
		return true
	}
	return lossless[codec]
}
