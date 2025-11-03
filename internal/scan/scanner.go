package scan

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/franz/music-janitor/internal/report"
	"github.com/franz/music-janitor/internal/store"
	"github.com/franz/music-janitor/internal/util"
)

// AudioExtensions are the default supported audio file extensions
var AudioExtensions = []string{
	".mp3",
	".flac",
	".m4a",
	".aac",
	".ogg",
	".opus",
	".wav",
	".aiff",
	".aif",
	".wma",
	".ape",
	".wv", // WavPack
	".mpc", // Musepack
}

// Scanner discovers audio files in a directory tree
type Scanner struct {
	store       *store.Store
	extensions  map[string]bool
	concurrency int
	logger      *report.EventLogger
}

// Config holds scanner configuration
type Config struct {
	Store              *store.Store
	AdditionalExts     []string
	Concurrency        int
	Logger             *report.EventLogger
}

// New creates a new Scanner
func New(cfg *Config) *Scanner {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 4
	}

	// Build extension map (case-insensitive)
	extMap := make(map[string]bool)
	for _, ext := range AudioExtensions {
		extMap[strings.ToLower(ext)] = true
	}
	for _, ext := range cfg.AdditionalExts {
		extMap[strings.ToLower(ext)] = true
	}

	return &Scanner{
		store:       cfg.Store,
		extensions:  extMap,
		concurrency: cfg.Concurrency,
		logger:      cfg.Logger,
	}
}

// Result represents a scan result
type Result struct {
	FilesDiscovered int
	FilesSkipped    int
	Errors          []error
}

// Scan walks the source directory and discovers audio files
func (s *Scanner) Scan(ctx context.Context, sourcePath string) (*Result, error) {
	util.InfoLog("Starting scan of: %s", sourcePath)

	result := &Result{
		Errors: make([]error, 0),
	}

	// Channel for discovered file paths
	filePaths := make(chan string, 100)

	// Counters for progress reporting (using atomic for thread-safety)
	var filesFound atomic.Int64
	var filesProcessed atomic.Int64
	var filesNew atomic.Int64
	var filesSkipped atomic.Int64

	// WaitGroup for workers
	var wg sync.WaitGroup

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
				found := filesFound.Load()
				processed := filesProcessed.Load()
				new := filesNew.Load()
				skipped := filesSkipped.Load()

				if found > 0 {
					util.InfoLog("Progress: found %d audio files, processed %d (new: %d, skipped: %d)",
						found, processed, new, skipped)
				}
			}
		}
	}()

	// Start worker pool
	for i := 0; i < s.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range filePaths {
				// Check for cancellation
				select {
				case <-ctx.Done():
					return
				default:
				}

				isNew, err := s.processFile(path)
				filesProcessed.Add(1)

				if err != nil {
					util.ErrorLog("Failed to process %s: %v", path, err)
					result.Errors = append(result.Errors, err)
				} else if isNew {
					filesNew.Add(1)
				} else {
					filesSkipped.Add(1)
				}
			}
		}()
	}

	// Walk directory tree
	walkErr := filepath.WalkDir(sourcePath, func(path string, d fs.DirEntry, err error) error {
		// Check for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err != nil {
			util.WarnLog("Error accessing path %s: %v", path, err)
			result.Errors = append(result.Errors, fmt.Errorf("access error: %s: %w", path, err))
			return nil // Continue walking
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Check if it's an audio file
		if s.isAudioFile(path) {
			filesFound.Add(1)
			select {
			case filePaths <- path:
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		return nil
	})

	// Close channel and wait for workers
	close(filePaths)
	wg.Wait()
	cancelProgress()

	// Update result with final counts
	result.FilesDiscovered = int(filesNew.Load())
	result.FilesSkipped = int(filesSkipped.Load())

	if walkErr != nil && walkErr != context.Canceled {
		return result, fmt.Errorf("walk error: %w", walkErr)
	}

	util.SuccessLog("Scan complete: %d files discovered, %d skipped, %d errors",
		result.FilesDiscovered, result.FilesSkipped, len(result.Errors))

	return result, nil
}


// processFile processes a single file and stores it in the database
// Returns (isNew, error) where isNew indicates if the file was newly inserted
func (s *Scanner) processFile(path string) (bool, error) {
	// Generate file key
	fileKey, err := util.GenerateFileKey(path)
	if err != nil {
		return false, fmt.Errorf("failed to generate file key: %w", err)
	}

	// Check if file already exists in database
	existing, err := s.store.GetFileByKey(fileKey)
	if err != nil {
		return false, fmt.Errorf("failed to check existing file: %w", err)
	}

	if existing != nil {
		// File already scanned, skip
		util.DebugLog("File already scanned: %s", path)
		return false, nil
	}

	// Get file metadata
	size, mtime, err := util.GetFileMetadata(path)
	if err != nil {
		return false, fmt.Errorf("failed to get file metadata: %w", err)
	}

	// Insert into database
	file := &store.File{
		FileKey:   fileKey,
		SrcPath:   path,
		SizeBytes: size,
		MtimeUnix: mtime,
		Status:    "discovered",
	}

	if err := s.store.InsertFile(file); err != nil {
		return false, fmt.Errorf("failed to insert file: %w", err)
	}

	// Log scan event
	if s.logger != nil {
		s.logger.LogScan(fileKey, path, size)
	}

	util.DebugLog("Discovered: %s (key: %s)", path, fileKey[:8])
	return true, nil
}

// isAudioFile checks if a file has a supported audio extension
func (s *Scanner) isAudioFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return s.extensions[ext]
}

// GetSupportedExtensions returns the list of supported extensions
func (s *Scanner) GetSupportedExtensions() []string {
	exts := make([]string, 0, len(s.extensions))
	for ext := range s.extensions {
		exts = append(exts, ext)
	}
	return exts
}
