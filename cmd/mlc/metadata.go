package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/franz/music-janitor/internal/store"
	"github.com/franz/music-janitor/internal/util"
	"github.com/spf13/cobra"
)

var metadataCmd = &cobra.Command{
	Use:   "metadata",
	Short: "Query and display metadata for files",
	Long: `Query and display metadata stored in the database for audio files.

Supports flexible filtering by artist, album, title, format, and more.
Can output in human-readable format, JSONL, or CSV.

Examples:
  # Show all files with empty titles
  mlc metadata --empty-title

  # Show all files by an artist
  mlc metadata --artist "Die Ärzte"

  # Show all FLAC files
  mlc metadata --format flac

  # Export to CSV
  mlc metadata --output csv > metadata.csv

  # Show files from specific path
  mlc metadata --path "*Ärzte*" --limit 20
`,
	RunE: runMetadata,
}

func init() {
	rootCmd.AddCommand(metadataCmd)

	// Filter flags
	metadataCmd.Flags().StringP("artist", "a", "", "Filter by artist name")
	metadataCmd.Flags().StringP("album", "b", "", "Filter by album name")
	metadataCmd.Flags().StringP("title", "t", "", "Filter by title (supports partial match)")
	metadataCmd.Flags().StringP("path", "p", "", "Filter by file path pattern")
	metadataCmd.Flags().StringP("format", "f", "", "Filter by format (mp3, flac, m4a, etc.)")
	metadataCmd.Flags().String("codec", "", "Filter by codec")
	metadataCmd.Flags().Bool("lossless", false, "Show only lossless files")
	metadataCmd.Flags().Bool("lossy", false, "Show only lossy files")
	metadataCmd.Flags().Bool("empty-artist", false, "Show files with missing artist tag")
	metadataCmd.Flags().Bool("empty-album", false, "Show files with missing album tag")
	metadataCmd.Flags().Bool("empty-title", false, "Show files with missing title tag")
	metadataCmd.Flags().Bool("errors", false, "Show files with metadata extraction errors")

	// Output flags
	metadataCmd.Flags().StringP("output", "o", "human", "Output format: human, jsonl, csv")
	metadataCmd.Flags().IntP("limit", "l", 0, "Limit number of results (0 = no limit)")
	metadataCmd.Flags().String("sort", "path", "Sort by: artist, album, title, path, duration, bitrate")
}

func runMetadata(cmd *cobra.Command, args []string) error {
	// Get database path
	dbPath, _ := cmd.Flags().GetString("db")
	if dbPath == "" {
		dbPath = ".mlc/mlc-state.db"
	}

	// Open database
	db, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Build query options from flags
	opts := &store.MetadataQueryOptions{}

	opts.Artist, _ = cmd.Flags().GetString("artist")
	opts.Album, _ = cmd.Flags().GetString("album")
	opts.Title, _ = cmd.Flags().GetString("title")
	opts.PathPattern, _ = cmd.Flags().GetString("path")
	opts.Format, _ = cmd.Flags().GetString("format")
	opts.Codec, _ = cmd.Flags().GetString("codec")
	opts.LosslessOnly, _ = cmd.Flags().GetBool("lossless")
	opts.LossyOnly, _ = cmd.Flags().GetBool("lossy")
	opts.EmptyArtist, _ = cmd.Flags().GetBool("empty-artist")
	opts.EmptyAlbum, _ = cmd.Flags().GetBool("empty-album")
	opts.EmptyTitle, _ = cmd.Flags().GetBool("empty-title")
	opts.ShowErrors, _ = cmd.Flags().GetBool("errors")
	opts.Limit, _ = cmd.Flags().GetInt("limit")
	opts.SortBy, _ = cmd.Flags().GetString("sort")

	// Query database
	results, err := db.QueryFilesWithMetadata(opts)
	if err != nil {
		return fmt.Errorf("failed to query metadata: %w", err)
	}

	// Output results
	outputFormat, _ := cmd.Flags().GetString("output")

	switch outputFormat {
	case "jsonl":
		return outputJSONL(results)
	case "csv":
		return outputCSV(results)
	default:
		return outputHuman(results, opts)
	}
}

func outputHuman(results []*store.FileWithMetadata, opts *store.MetadataQueryOptions) error {
	if len(results) == 0 {
		util.InfoLog("No files found matching criteria")
		return nil
	}

	// Print header
	fmt.Printf("\n=== Metadata Query Results ===\n")
	if opts.Artist != "" {
		fmt.Printf("Artist: %s\n", opts.Artist)
	}
	if opts.Album != "" {
		fmt.Printf("Album: %s\n", opts.Album)
	}
	if opts.EmptyTitle {
		fmt.Printf("Filter: Empty titles only\n")
	}
	fmt.Printf("Found: %d files\n\n", len(results))

	// Print each file
	var totalSize int64
	for i, result := range results {
		f := result.File
		m := result.Metadata

		fmt.Printf("[%d] Path: %s\n", i+1, f.SrcPath)
		fmt.Printf("    Artist:   %s\n", formatStringOrEmpty(m.TagArtist))
		fmt.Printf("    Album:    %s\n", formatStringOrEmpty(m.TagAlbum))
		fmt.Printf("    Title:    %s\n", formatStringOrEmpty(m.TagTitle))

		if m.TagTrack > 0 {
			if m.TagTrackTotal > 0 {
				fmt.Printf("    Track:    %d/%d\n", m.TagTrack, m.TagTrackTotal)
			} else {
				fmt.Printf("    Track:    %d\n", m.TagTrack)
			}
		}

		if m.TagDisc > 0 {
			if m.TagDiscTotal > 0 {
				fmt.Printf("    Disc:     %d/%d\n", m.TagDisc, m.TagDiscTotal)
			} else {
				fmt.Printf("    Disc:     %d\n", m.TagDisc)
			}
		}

		// Format info
		losslessStr := "No"
		if m.Lossless {
			losslessStr = "Yes"
		}
		fmt.Printf("    Format:   %s (%s) @ %dkbps\n", strings.ToUpper(m.Format), m.Codec, m.BitrateKbps)
		fmt.Printf("    Lossless: %s\n", losslessStr)

		if m.DurationMs > 0 {
			fmt.Printf("    Duration: %s\n", formatDuration(m.DurationMs))
		}

		fmt.Printf("    Size:     %s\n", formatBytes(f.SizeBytes))

		fmt.Println()

		totalSize += f.SizeBytes
	}

	// Summary
	fmt.Printf("=== Summary ===\n")
	fmt.Printf("Files: %d\n", len(results))
	fmt.Printf("Total Size: %s\n\n", formatBytes(totalSize))

	return nil
}

func outputJSONL(results []*store.FileWithMetadata) error {
	encoder := json.NewEncoder(os.Stdout)

	for _, result := range results {
		obj := map[string]interface{}{
			"file_id":      result.File.ID,
			"path":         result.File.SrcPath,
			"size_bytes":   result.File.SizeBytes,
			"artist":       result.Metadata.TagArtist,
			"album":        result.Metadata.TagAlbum,
			"title":        result.Metadata.TagTitle,
			"track":        result.Metadata.TagTrack,
			"disc":         result.Metadata.TagDisc,
			"format":       result.Metadata.Format,
			"codec":        result.Metadata.Codec,
			"duration_ms":  result.Metadata.DurationMs,
			"bitrate_kbps": result.Metadata.BitrateKbps,
			"lossless":     result.Metadata.Lossless,
		}

		if err := encoder.Encode(obj); err != nil {
			return fmt.Errorf("failed to encode JSON: %w", err)
		}
	}

	return nil
}

func outputCSV(results []*store.FileWithMetadata) error {
	writer := csv.NewWriter(os.Stdout)
	defer writer.Flush()

	// Write header
	header := []string{
		"path", "artist", "album", "title", "track", "disc",
		"format", "codec", "bitrate_kbps", "duration_ms",
		"lossless", "size_bytes",
	}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write rows
	for _, result := range results {
		m := result.Metadata
		f := result.File

		losslessStr := "false"
		if m.Lossless {
			losslessStr = "true"
		}

		row := []string{
			f.SrcPath,
			m.TagArtist,
			m.TagAlbum,
			m.TagTitle,
			fmt.Sprintf("%d", m.TagTrack),
			fmt.Sprintf("%d", m.TagDisc),
			m.Format,
			m.Codec,
			fmt.Sprintf("%d", m.BitrateKbps),
			fmt.Sprintf("%d", m.DurationMs),
			losslessStr,
			fmt.Sprintf("%d", f.SizeBytes),
		}

		if err := writer.Write(row); err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	return nil
}

func formatStringOrEmpty(s string) string {
	if s == "" {
		return "(empty)"
	}
	return s
}

func formatDuration(ms int) string {
	seconds := ms / 1000
	minutes := seconds / 60
	seconds = seconds % 60
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
