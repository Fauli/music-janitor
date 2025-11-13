package scan

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/franz/music-janitor/internal/report"
	"github.com/franz/music-janitor/internal/store"
	"github.com/franz/music-janitor/internal/util"
	"github.com/schollz/progressbar/v3"
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
	".wv",  // WavPack
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
	Store          *store.Store
	AdditionalExts []string
	Concurrency    int
	Logger         *report.EventLogger
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

	// Pre-load existing file keys for quick duplicate detection
	util.InfoLog("Pre-loading existing file keys...")
	existingKeys, err := s.store.GetAllFileKeysMap()
	if err != nil {
		return nil, fmt.Errorf("failed to load existing file keys: %w", err)
	}
	util.InfoLog("Loaded %d existing file keys", len(existingKeys))

	// Thread-safe map for tracking existing keys
	var keysMutex sync.RWMutex

	// Channel for discovered file paths
	filePaths := make(chan string, 100)

	// Channel for new files to batch insert
	newFiles := make(chan *store.File, 1000)

	// Counters for progress reporting (using atomic for thread-safety)
	var filesFound atomic.Int64
	var filesProcessed atomic.Int64
	var filesNew atomic.Int64
	var filesSkipped atomic.Int64

	// WaitGroup for workers
	var wg sync.WaitGroup

	// Start progress reporter with visual progress bar
	progressCtx, cancelProgress := context.WithCancel(ctx)
	defer cancelProgress()

	// Check if stdout is a terminal (disable progress bar if piped/redirected)
	isTTY := util.IsTerminal(os.Stdout.Fd())
	var bar *progressbar.ProgressBar
	var lastRate float64
	var lastUpdate time.Time

	if isTTY && !util.IsQuiet() {
		// Create indeterminate progress bar (we don't know total yet)
		bar = progressbar.NewOptions(-1,
			progressbar.OptionSetDescription("Scanning"),
			progressbar.OptionSetWidth(40),
			progressbar.OptionShowCount(),
			progressbar.OptionShowIts(),
			progressbar.OptionSetItsString("files"),
			progressbar.OptionThrottle(200*time.Millisecond),
			progressbar.OptionClearOnFinish(),
			progressbar.OptionSetRenderBlankState(true),
		)
		lastUpdate = time.Now()
	}

	go func() {
		ticker := time.NewTicker(1 * time.Second) // Update more frequently for smooth bar
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

				if bar != nil && found > 0 {
					// Calculate rate
					now := time.Now()
					elapsed := now.Sub(lastUpdate).Seconds()
					if elapsed > 0 {
						lastRate = float64(processed) / time.Since(lastUpdate).Seconds()
					}

					// Update progress bar description with stats
					description := fmt.Sprintf("Scanning | %d found | %d new | %d cached | %.1f/s",
						found, new, skipped, lastRate)
					bar.Describe(description)
					bar.Set64(processed)
				} else if found > 0 {
					// Fallback to text output if not a TTY
					util.InfoLog("Progress: found %d audio files, processed %d (new: %d, skipped: %d)",
						found, processed, new, skipped)
				}
			}
		}
	}()

	// Start batch writer goroutine
	var writerWg sync.WaitGroup
	writerWg.Add(1)
	go func() {
		defer writerWg.Done()
		batch := make([]*store.File, 0, 1000)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		flush := func() {
			if len(batch) == 0 {
				return
			}
			if err := s.store.InsertFileBatch(batch); err != nil {
				util.ErrorLog("Failed to batch insert files: %v", err)
				result.Errors = append(result.Errors, err)
			}
			batch = batch[:0] // Reset batch
		}

		for {
			select {
			case file, ok := <-newFiles:
				if !ok {
					// Channel closed, flush remaining
					flush()
					return
				}
				batch = append(batch, file)
				if len(batch) >= 1000 {
					flush()
				}
			case <-ticker.C:
				// Periodic flush
				flush()
			case <-ctx.Done():
				flush()
				return
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

				isNew, err := s.processFileOptimized(path, existingKeys, &keysMutex, newFiles)
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

	// Close new files channel and wait for batch writer
	close(newFiles)
	writerWg.Wait()

	cancelProgress()

	// Finish progress bar
	if bar != nil {
		bar.Finish()
		// Print final summary
		util.SuccessLog("Scan complete: %d files found, %d new, %d cached",
			filesFound.Load(), filesNew.Load(), filesSkipped.Load())
	}

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

// processFileOptimized processes a single file using pre-loaded keys and batch inserts
// Returns (isNew, error) where isNew indicates if the file was newly inserted
func (s *Scanner) processFileOptimized(path string, existingKeys map[string]bool, keysMutex *sync.RWMutex, newFiles chan<- *store.File) (bool, error) {
	// Generate file key
	fileKey, err := util.GenerateFileKey(path)
	if err != nil {
		return false, fmt.Errorf("failed to generate file key: %w", err)
	}

	// Check if file already exists (using pre-loaded map)
	keysMutex.RLock()
	exists := existingKeys[fileKey]
	keysMutex.RUnlock()

	if exists {
		// File already scanned, skip
		util.DebugLog("File already scanned: %s", path)
		return false, nil
	}

	// Get file metadata
	size, mtime, err := util.GetFileMetadata(path)
	if err != nil {
		return false, fmt.Errorf("failed to get file metadata: %w", err)
	}

	// Create file record
	file := &store.File{
		FileKey:   fileKey,
		SrcPath:   path,
		SizeBytes: size,
		MtimeUnix: mtime,
		Status:    "discovered",
	}

	// Send to batch writer
	newFiles <- file

	// Add to existing keys map to prevent duplicates within same scan
	keysMutex.Lock()
	existingKeys[fileKey] = true
	keysMutex.Unlock()

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
