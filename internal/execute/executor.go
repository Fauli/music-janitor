package execute

import (
	"context"
	"crypto/sha1"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/franz/music-janitor/internal/meta"
	"github.com/franz/music-janitor/internal/report"
	"github.com/franz/music-janitor/internal/store"
	"github.com/franz/music-janitor/internal/util"
)

// Executor executes the planned actions (copy/move/link)
type Executor struct {
	store       *store.Store
	concurrency int
	verifyMode  string // "none", "size", "hash"
	dryRun      bool
	writeTags   bool // Write enriched metadata tags to destination files
	bufferSize  int // Buffer size for file copying (bytes)
	retryConfig *util.RetryConfig
	logger      *report.EventLogger
}

// Config holds executor configuration
type Config struct {
	Store       *store.Store
	Concurrency int
	VerifyMode  string // "none", "size", "hash"
	DryRun      bool
	WriteTags   bool // Write enriched metadata tags to destination files
	BufferSize  int         // Buffer size for file copying (0 = use default)
	RetryConfig *util.RetryConfig // Retry configuration (nil = use default)
	Logger      *report.EventLogger
}

// New creates a new Executor
func New(cfg *Config) *Executor {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 4
	}
	if cfg.VerifyMode == "" {
		cfg.VerifyMode = "size"
	}
	if cfg.BufferSize <= 0 {
		// Default 128KB - good balance for both local and network storage
		// Can be increased to 256KB+ for NAS via config or auto-tuning
		cfg.BufferSize = 128 * 1024
	}
	if cfg.RetryConfig == nil {
		// Use default retry config (no retries for local, can be overridden for NAS)
		cfg.RetryConfig = &util.RetryConfig{
			MaxAttempts: 1, // No retries by default (NAS will override)
			InitialWait: 0,
			MaxWait:     0,
		}
	}

	return &Executor{
		store:       cfg.Store,
		concurrency: cfg.Concurrency,
		verifyMode:  cfg.VerifyMode,
		dryRun:      cfg.DryRun,
		writeTags:   cfg.WriteTags,
		bufferSize:  cfg.BufferSize,
		retryConfig: cfg.RetryConfig,
		logger:      cfg.Logger,
	}
}

// Result represents execution results
type Result struct {
	Processed    int
	Succeeded    int
	Skipped      int
	Failed       int
	BytesWritten int64
	Errors       []error
}

// Execute executes all planned actions
func (e *Executor) Execute(ctx context.Context) (*Result, error) {
	util.InfoLog("Starting execution")

	// Get all plans with actions to execute (not "skip")
	allPlans, err := e.store.GetAllPlans()
	if err != nil {
		return nil, fmt.Errorf("failed to get plans: %w", err)
	}

	// Filter to only actionable plans
	var plans []*store.Plan
	for _, plan := range allPlans {
		if plan.Action != "skip" {
			plans = append(plans, plan)
		}
	}

	if len(plans) == 0 {
		util.InfoLog("No files to execute")
		return &Result{}, nil
	}

	totalPlans := len(plans)
	util.InfoLog("Found %d files to execute", totalPlans)

	if e.dryRun {
		util.InfoLog("DRY-RUN mode: no files will be copied/moved")
	}

	result := &Result{
		Errors: make([]error, 0),
	}

	// Counters for progress reporting
	var processed atomic.Int64
	var succeeded atomic.Int64
	var skipped atomic.Int64
	var failed atomic.Int64
	var bytesWritten atomic.Int64

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
				s := succeeded.Load()
				f := failed.Load()
				sk := skipped.Load()
				bw := bytesWritten.Load()

				if p > 0 {
					percentage := float64(p) / float64(totalPlans) * 100
					util.InfoLog("Executing: %d/%d (%.1f%%) - success: %d, failed: %d, skipped: %d, written: %s",
						p, totalPlans, percentage, s, f, sk, formatBytes(bw))
				}
			}
		}
	}()

	// Create worker pool
	plansChan := make(chan *store.Plan, e.concurrency*2)
	doneChan := make(chan struct{})

	// Start workers
	for i := 0; i < e.concurrency; i++ {
		go func() {
			for plan := range plansChan {
				select {
				case <-ctx.Done():
					return
				default:
				}

				processed.Add(1)

				// Execute the plan
				bytes, err := e.executePlan(ctx, plan)

				if err != nil {
					util.ErrorLog("Failed to execute plan for file %d: %v", plan.FileID, err)
					result.Errors = append(result.Errors, err)
					failed.Add(1)
				} else if bytes < 0 {
					// Negative bytes means skipped
					skipped.Add(1)
				} else {
					succeeded.Add(1)
					bytesWritten.Add(bytes)
				}
			}
			doneChan <- struct{}{}
		}()
	}

	// Send plans to workers
	go func() {
		for _, plan := range plans {
			select {
			case <-ctx.Done():
				return
			case plansChan <- plan:
			}
		}
		close(plansChan)
	}()

	// Wait for all workers to finish
	for i := 0; i < e.concurrency; i++ {
		<-doneChan
	}

	cancelProgress()

	// Update final counts
	result.Processed = int(processed.Load())
	result.Succeeded = int(succeeded.Load())
	result.Skipped = int(skipped.Load())
	result.Failed = int(failed.Load())
	result.BytesWritten = bytesWritten.Load()

	util.SuccessLog("Execution complete: %d processed, %d succeeded, %d skipped, %d failed, %s written",
		result.Processed, result.Succeeded, result.Skipped, result.Failed, formatBytes(result.BytesWritten))

	return result, nil
}

// executePlan executes a single plan
// Returns bytes written (or -1 if skipped) and error
func (e *Executor) executePlan(ctx context.Context, plan *store.Plan) (int64, error) {
	// Get file
	file, err := e.store.GetFileByID(plan.FileID)
	if err != nil {
		return 0, fmt.Errorf("failed to get file: %w", err)
	}

	// Check if already executed successfully
	execution, _ := e.store.GetExecution(plan.FileID)
	if execution != nil && execution.VerifyOK {
		util.DebugLog("File %d already executed successfully, skipping", plan.FileID)
		return -1, nil // Skipped
	}

	// Create execution record
	exec := &store.Execution{
		FileID:    plan.FileID,
		StartedAt: time.Now(),
	}

	// Execute based on action
	var bytesWritten int64

	if e.dryRun {
		// Dry run - just log what would happen
		util.DebugLog("DRY-RUN: Would %s %s -> %s", plan.Action, file.SrcPath, plan.DestPath)
		bytesWritten = file.SizeBytes
		exec.VerifyOK = true
	} else {
		switch plan.Action {
		case "copy":
			bytesWritten, err = e.copyFile(ctx, file.SrcPath, plan.DestPath)
		case "move":
			bytesWritten, err = e.moveFile(ctx, file.SrcPath, plan.DestPath)
		case "hardlink":
			bytesWritten, err = e.hardlinkFile(file.SrcPath, plan.DestPath)
		case "symlink":
			bytesWritten, err = e.symlinkFile(file.SrcPath, plan.DestPath)
		default:
			return 0, fmt.Errorf("unknown action: %s", plan.Action)
		}

		if err != nil {
			exec.Error = err.Error()
			exec.CompletedAt = time.Now()
			e.store.InsertOrUpdateExecution(exec)
			return 0, err
		}

		exec.BytesWritten = bytesWritten

		// Write enriched metadata tags to destination file (if enabled)
		if e.writeTags && (plan.Action == "copy" || plan.Action == "move") {
			if meta.CanWriteTags(plan.DestPath) {
				// Get metadata for this file
				metadata, metaErr := e.store.GetMetadata(file.ID)
				if metaErr != nil {
					util.WarnLog("Failed to get metadata for tag writing (file %d): %v", file.ID, metaErr)
				} else if metadata != nil {
					// Write tags to destination file
					if tagErr := meta.WriteTagsToFile(plan.DestPath, metadata); tagErr != nil {
						util.WarnLog("Failed to write tags to %s: %v", plan.DestPath, tagErr)
						// Don't fail the entire operation - just log the warning
					} else {
						util.DebugLog("Successfully wrote enriched tags to: %s", plan.DestPath)
					}
				}
			}
		}

		// Verify
		verifyOK := false
		switch e.verifyMode {
		case "size":
			verifyOK, err = e.verifySize(plan.DestPath, file.SizeBytes)
		case "hash":
			verifyOK, err = e.verifyHash(file.SrcPath, plan.DestPath)
		default:
			verifyOK = true // No verification
		}

		if err != nil {
			exec.Error = fmt.Sprintf("verification failed: %v", err)
			exec.VerifyOK = false
		} else {
			exec.VerifyOK = verifyOK
		}
	}

	exec.CompletedAt = time.Now()
	if err := e.store.InsertOrUpdateExecution(exec); err != nil {
		util.WarnLog("Failed to update execution record: %v", err)
	}

	// Log execution event
	if e.logger != nil {
		duration := exec.CompletedAt.Sub(exec.StartedAt)
		var execErr error
		if exec.Error != "" {
			execErr = fmt.Errorf("%s", exec.Error)
		}
		e.logger.LogExecute(file.FileKey, file.SrcPath, plan.DestPath, plan.Action, bytesWritten, duration, execErr)
	}

	// Update file status
	if exec.VerifyOK {
		e.store.UpdateFileStatus(plan.FileID, "executed", "")
	} else {
		e.store.UpdateFileStatus(plan.FileID, "error", exec.Error)
	}

	return bytesWritten, nil
}

// copyFile copies a file atomically using a .part temporary file
func (e *Executor) copyFile(ctx context.Context, srcPath, destPath string) (int64, error) {
	// Create destination directory with retry
	destDir := filepath.Dir(destPath)
	if err := util.RetryableMkdirAll(destDir, 0755, e.retryConfig); err != nil {
		return 0, fmt.Errorf("failed to create directory: %w", err)
	}

	// Open source file with retry
	src, err := util.RetryableOpen(srcPath, e.retryConfig)
	if err != nil {
		return 0, fmt.Errorf("failed to open source: %w", err)
	}
	defer src.Close()

	// Create temporary file (.part) with retry
	tempPath := destPath + ".part"
	dest, err := util.RetryableCreate(tempPath, e.retryConfig)
	if err != nil {
		return 0, fmt.Errorf("failed to create temp file: %w", err)
	}

	// Copy with context cancellation support
	bytesWritten, err := copyWithContext(ctx, dest, src, e.bufferSize)
	dest.Close()

	if err != nil {
		util.RetryableRemove(tempPath, e.retryConfig) // Clean up on error
		return 0, fmt.Errorf("failed to copy: %w", err)
	}

	// Atomic rename with retry
	if err := util.RetryableRename(tempPath, destPath, e.retryConfig); err != nil {
		util.RetryableRemove(tempPath, e.retryConfig) // Clean up on error
		return 0, fmt.Errorf("failed to rename: %w", err)
	}

	util.DebugLog("Copied: %s -> %s (%s)", srcPath, destPath, formatBytes(bytesWritten))
	return bytesWritten, nil
}

// moveFile moves a file (copy + delete source)
func (e *Executor) moveFile(ctx context.Context, srcPath, destPath string) (int64, error) {
	// First try rename (works if same filesystem) with retry
	destDir := filepath.Dir(destPath)
	if err := util.RetryableMkdirAll(destDir, 0755, e.retryConfig); err != nil {
		return 0, fmt.Errorf("failed to create directory: %w", err)
	}

	if err := util.RetryableRename(srcPath, destPath, e.retryConfig); err == nil {
		// Rename succeeded (same filesystem)
		stat, _ := util.RetryableStat(destPath, e.retryConfig)
		if stat != nil {
			return stat.Size(), nil
		}
		return 0, nil
	}

	// Rename failed (different filesystem), fall back to copy + delete
	bytesWritten, err := e.copyFile(ctx, srcPath, destPath)
	if err != nil {
		return 0, err
	}

	// Verify before deleting source
	if e.verifyMode != "none" {
		verifyOK := false
		switch e.verifyMode {
		case "size":
			stat, _ := util.RetryableStat(srcPath, e.retryConfig)
			if stat != nil {
				verifyOK, _ = e.verifySize(destPath, stat.Size())
			}
		case "hash":
			verifyOK, _ = e.verifyHash(srcPath, destPath)
		}

		if !verifyOK {
			return 0, fmt.Errorf("verification failed before deleting source")
		}
	}

	// Delete source with retry
	if err := util.RetryableRemove(srcPath, e.retryConfig); err != nil {
		util.WarnLog("Failed to delete source file %s: %v", srcPath, err)
		// Don't return error - file was copied successfully
	}

	util.DebugLog("Moved: %s -> %s", srcPath, destPath)
	return bytesWritten, nil
}

// hardlinkFile creates a hard link
func (e *Executor) hardlinkFile(srcPath, destPath string) (int64, error) {
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return 0, fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.Link(srcPath, destPath); err != nil {
		return 0, fmt.Errorf("failed to create hardlink: %w", err)
	}

	stat, _ := os.Stat(destPath)
	if stat != nil {
		return stat.Size(), nil
	}

	util.DebugLog("Hardlinked: %s -> %s", srcPath, destPath)
	return 0, nil
}

// symlinkFile creates a symbolic link
func (e *Executor) symlinkFile(srcPath, destPath string) (int64, error) {
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return 0, fmt.Errorf("failed to create directory: %w", err)
	}

	// Make source path absolute
	absSrc, err := filepath.Abs(srcPath)
	if err != nil {
		return 0, fmt.Errorf("failed to get absolute path: %w", err)
	}

	if err := os.Symlink(absSrc, destPath); err != nil {
		return 0, fmt.Errorf("failed to create symlink: %w", err)
	}

	util.DebugLog("Symlinked: %s -> %s", srcPath, destPath)
	return 0, nil
}

// verifySize verifies file size
func (e *Executor) verifySize(path string, expectedSize int64) (bool, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return false, err
	}

	return stat.Size() == expectedSize, nil
}

// verifyHash verifies file content using SHA1
func (e *Executor) verifyHash(srcPath, destPath string) (bool, error) {
	srcHash, err := hashFile(srcPath)
	if err != nil {
		return false, fmt.Errorf("failed to hash source: %w", err)
	}

	destHash, err := hashFile(destPath)
	if err != nil {
		return false, fmt.Errorf("failed to hash dest: %w", err)
	}

	return srcHash == destHash, nil
}

// hashFile computes SHA1 hash of a file
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// copyWithContext copies data with context cancellation support
func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader, bufferSize int) (int64, error) {
	if bufferSize <= 0 {
		bufferSize = 128 * 1024 // Default 128KB
	}

	buf := make([]byte, bufferSize)
	var written int64

	for {
		select {
		case <-ctx.Done():
			return written, ctx.Err()
		default:
		}

		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = fmt.Errorf("invalid write result")
				}
			}
			written += int64(nw)
			if ew != nil {
				return written, ew
			}
			if nr != nw {
				return written, io.ErrShortWrite
			}
		}
		if er != nil {
			if er != io.EOF {
				return written, er
			}
			break
		}
	}
	return written, nil
}

// formatBytes formats bytes in human-readable format
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
