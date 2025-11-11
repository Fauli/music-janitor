package main

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/franz/music-janitor/internal/meta"
	"github.com/franz/music-janitor/internal/report"
	"github.com/franz/music-janitor/internal/store"
	"github.com/franz/music-janitor/internal/util"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var rescanCmd = &cobra.Command{
	Use:   "rescan",
	Short: "Re-extract metadata for existing files",
	Long: `Re-extract metadata for files that have already been scanned.

This command updates metadata for existing files without re-discovering them.
Useful for:
- Extracting newly implemented metadata fields (like compilation flag)
- Refreshing metadata after tag changes
- Retrying metadata extraction after fixing issues (e.g., installing ffprobe)

Processes files with status=meta_ok (refresh) or status=error (retry).
Files are processed concurrently.`,
	RunE: runRescan,
}

func init() {
	rootCmd.AddCommand(rescanCmd)
	rescanCmd.Flags().BoolP("metadata-only", "m", true, "Only re-extract metadata (don't re-discover files)")
}

func runRescan(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Get configuration
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

	util.InfoLog("Opening database: %s", dbPath)

	// Check if database is on network storage
	dbNetworkOptimized := false
	if dbInfo, err := util.DetectNetworkFilesystem(dbPath); err == nil && dbInfo.IsNetwork {
		dbNetworkOptimized = true
		util.InfoLog("Database on network storage (%s) - applying optimizations", dbInfo.Protocol)
	}

	// Open database with network optimizations if needed
	db, err := store.OpenWithOptions(dbPath, &store.OpenOptions{
		NetworkOptimized: dbNetworkOptimized,
	})
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Create event logger
	logLevel := report.LevelInfo
	if quiet {
		logLevel = report.LevelWarning
	} else if verbose {
		logLevel = report.LevelDebug
	}

	logger, err := report.NewEventLogger("artifacts", logLevel)
	if err != nil {
		return fmt.Errorf("failed to create event logger: %w", err)
	}
	defer logger.Close()

	// Get all files with existing metadata
	util.InfoLog("Finding files to rescan...")
	files, err := db.GetAllFiles()
	if err != nil {
		return fmt.Errorf("failed to get files: %w", err)
	}

	// Filter to files with metadata OR error status (to retry failed extractions)
	var filesToRescan []*store.File
	for _, file := range files {
		if file.Status == "meta_ok" || file.Status == "error" {
			filesToRescan = append(filesToRescan, file)
		}
	}

	if len(filesToRescan) == 0 {
		util.InfoLog("No files to rescan")
		return nil
	}

	// Count files by status for user feedback
	metaOKCount := 0
	errorCount := 0
	for _, file := range filesToRescan {
		if file.Status == "meta_ok" {
			metaOKCount++
		} else if file.Status == "error" {
			errorCount++
		}
	}

	util.InfoLog("Rescanning metadata for %d files...", len(filesToRescan))
	if metaOKCount > 0 {
		util.InfoLog("  Files with metadata: %d", metaOKCount)
	}
	if errorCount > 0 {
		util.InfoLog("  Previously failed files: %d (retrying)", errorCount)
	}

	// Progress tracking
	startTime := time.Now()
	var processed atomic.Int64
	var updated atomic.Int64
	var errors atomic.Int64

	// Progress reporter
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
				if p > 0 {
					percentage := float64(p) / float64(len(filesToRescan)) * 100
					util.InfoLog("Progress: %d/%d files (%.1f%%) - %d updated, %d errors",
						p, len(filesToRescan), percentage, updated.Load(), errors.Load())
				}
			}
		}
	}()

	// Process files concurrently
	semaphore := make(chan struct{}, concurrency)
	done := make(chan struct{})

	go func() {
		for _, file := range filesToRescan {
			select {
			case <-ctx.Done():
				return
			case semaphore <- struct{}{}:
				go func(f *store.File) {
					defer func() { <-semaphore }()

					// Re-extract metadata using the hybrid approach
					newMetadata, err := meta.ExtractFromPath(f.SrcPath)
					if err != nil {
						util.ErrorLog("Failed to re-extract metadata for %s: %v", f.SrcPath, err)
						errors.Add(1)
						processed.Add(1)
						// Update status to error
						db.UpdateFileStatus(f.ID, "error", err.Error())
						return
					}

					// Get old metadata (may not exist for previously failed files)
					oldMetadata, err := db.GetMetadata(f.ID)
					wasError := f.Status == "error"

					// If file previously had error status, this is a successful recovery
					if wasError {
						updated.Add(1)
						util.DebugLog("Successfully extracted metadata for previously failed file: %s", f.SrcPath)
					} else if err == nil && oldMetadata != nil {
						// Check if compilation flag changed
						if oldMetadata.TagCompilation != newMetadata.TagCompilation {
							updated.Add(1)
							util.DebugLog("Updated compilation flag for %s: %v -> %v",
								f.SrcPath, oldMetadata.TagCompilation, newMetadata.TagCompilation)
						}
					}

					// Update metadata
					newMetadata.FileID = f.ID
					if err := db.InsertMetadata(newMetadata); err != nil {
						util.ErrorLog("Failed to update metadata for file %d: %v", f.ID, err)
						errors.Add(1)
						processed.Add(1)
						return
					}

					// Update status to meta_ok
					db.UpdateFileStatus(f.ID, "meta_ok", "")

					processed.Add(1)
				}(file)
			}
		}

		// Wait for all goroutines to complete
		for i := 0; i < concurrency; i++ {
			semaphore <- struct{}{}
		}
		close(done)
	}()

	// Wait for completion or cancellation
	select {
	case <-ctx.Done():
		cancelProgress()
		return ctx.Err()
	case <-done:
		cancelProgress()
	}

	elapsed := time.Since(startTime)

	// Final summary
	util.SuccessLog("Rescan complete!")
	util.InfoLog("Files processed: %d", processed.Load())
	util.InfoLog("Metadata updated: %d", updated.Load())
	if errors.Load() > 0 {
		util.WarnLog("Errors: %d", errors.Load())
	}
	util.InfoLog("Time elapsed: %s", elapsed.Round(time.Second))

	return nil
}
