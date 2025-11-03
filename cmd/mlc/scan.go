package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/franz/music-janitor/internal/meta"
	"github.com/franz/music-janitor/internal/report"
	"github.com/franz/music-janitor/internal/scan"
	"github.com/franz/music-janitor/internal/store"
	"github.com/franz/music-janitor/internal/util"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan source directory and extract metadata",
	Long: `Scan the source directory for audio files and extract metadata.

This command performs two operations:
1. Discovery: Walks the source directory and finds all audio files
2. Extraction: Reads tags and audio properties from each file

Files are tracked in the database with their metadata. The scan can be
resumed if interrupted.`,
	RunE: runScan,
}

func init() {
	rootCmd.AddCommand(scanCmd)
}

func runScan(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Get configuration from viper (flags override config file)
	source := viper.GetString("source")
	if source == "" {
		return fmt.Errorf("source directory is required (use --source/-s or set in config)")
	}

	concurrency := viper.GetInt("concurrency")
	if concurrency <= 0 {
		concurrency = 8
	}

	dbPath := viper.GetString("db")
	verbose := viper.GetBool("verbose")
	quiet := viper.GetBool("quiet")

	// Set log level
	util.SetVerbose(verbose)
	util.SetQuiet(quiet)

	// Verify source exists
	if _, err := os.Stat(source); os.IsNotExist(err) {
		return fmt.Errorf("source directory does not exist: %s", source)
	}

	util.InfoLog("Opening database: %s", dbPath)

	// Open database
	db, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Create event logger with appropriate log level
	logLevel := report.LevelInfo // Default
	if quiet {
		logLevel = report.LevelWarning // Only warnings and errors
	} else if verbose {
		logLevel = report.LevelDebug // Everything
	}

	logger, err := report.NewEventLogger("artifacts", logLevel)
	if err != nil {
		util.WarnLog("Failed to create event logger: %v", err)
		logger = report.NullLogger()
	}
	defer logger.Close()

	if logger.Path() != "" {
		util.InfoLog("Event log: %s", logger.Path())
	}

	// Phase 1: Discovery
	util.InfoLog("=== Phase 1: File Discovery ===")
	util.InfoLog("Source: %s", source)
	util.InfoLog("Concurrency: %d", concurrency)

	scanner := scan.New(&scan.Config{
		Store:       db,
		Concurrency: concurrency,
		Logger:      logger,
	})

	startTime := time.Now()

	scanResult, err := scanner.Scan(ctx, source)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	scanDuration := time.Since(startTime)

	util.SuccessLog("Discovery complete in %v", scanDuration.Round(time.Millisecond))
	util.InfoLog("  Files discovered: %d", scanResult.FilesDiscovered)
	util.InfoLog("  Files skipped: %d", scanResult.FilesSkipped)
	if len(scanResult.Errors) > 0 {
		util.WarnLog("  Errors: %d", len(scanResult.Errors))
	}

	// Phase 2: Metadata Extraction
	util.InfoLog("")
	util.InfoLog("=== Phase 2: Metadata Extraction ===")

	// Check for ffprobe
	if !meta.CheckFFprobeAvailable() {
		util.WarnLog("ffprobe not found in PATH - using tag library only")
		util.WarnLog("Install ffmpeg for best results: https://ffmpeg.org/")
	}

	extractor := meta.New(&meta.Config{
		Store:       db,
		Concurrency: concurrency,
		Logger:      logger,
	})

	extractStart := time.Now()

	extractResult, err := extractor.Extract(ctx)
	if err != nil {
		return fmt.Errorf("metadata extraction failed: %w", err)
	}

	extractDuration := time.Since(extractStart)

	util.SuccessLog("Extraction complete in %v", extractDuration.Round(time.Millisecond))
	util.InfoLog("  Files processed: %d", extractResult.Processed)
	util.InfoLog("  Success: %d", extractResult.Success)
	if len(extractResult.Errors) > 0 {
		util.WarnLog("  Errors: %d", len(extractResult.Errors))
	}

	// Summary
	util.InfoLog("")
	util.SuccessLog("=== Scan Summary ===")
	util.InfoLog("Total time: %v", (scanDuration + extractDuration).Round(time.Millisecond))
	util.InfoLog("Database: %s", dbPath)

	// Show status counts
	discovered, _ := db.CountFilesByStatus("discovered")
	metaOK, _ := db.CountFilesByStatus("meta_ok")
	errors, _ := db.CountFilesByStatus("error")

	util.InfoLog("")
	util.InfoLog("Current database status:")
	util.InfoLog("  Ready for planning: %d files", metaOK)
	if discovered > 0 {
		util.InfoLog("  Pending metadata: %d files", discovered)
	}
	if errors > 0 {
		util.WarnLog("  Errors: %d files", errors)
	}

	util.InfoLog("")
	util.InfoLog("Next step: mlc plan --dest <destination> --dry-run")

	return nil
}
