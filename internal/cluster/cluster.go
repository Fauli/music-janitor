package cluster

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/franz/music-janitor/internal/meta"
	"github.com/franz/music-janitor/internal/report"
	"github.com/franz/music-janitor/internal/store"
	"github.com/franz/music-janitor/internal/util"
)

// Clusterer groups files into duplicate clusters
type Clusterer struct {
	store          *store.Store
	logger         *report.EventLogger
	forceRecluster bool
}

// Config holds clusterer configuration
type Config struct {
	Store          *store.Store
	Logger         *report.EventLogger
	ForceRecluster bool // If true, discards resume state and starts fresh
}

// New creates a new Clusterer
func New(cfg *Config) *Clusterer {
	return &Clusterer{
		store:          cfg.Store,
		logger:         cfg.Logger,
		forceRecluster: cfg.ForceRecluster,
	}
}

// Result represents clustering results
type Result struct {
	ClustersCreated  int
	SingletonClusters int
	DuplicateClusters int
	FilesGrouped      int
	Errors            []error
}

// Cluster performs duplicate detection clustering
func (c *Clusterer) Cluster(ctx context.Context) (*Result, error) {
	util.InfoLog("Starting clustering")

	// Check for existing progress
	progress, err := c.store.GetClusteringProgress()
	if err != nil {
		return nil, fmt.Errorf("failed to check clustering progress: %w", err)
	}

	var resuming bool
	var lastProcessedID int64

	if progress != nil && !c.forceRecluster {
		// Resume from previous run
		resuming = true
		lastProcessedID = progress.LastProcessedFileID
		util.InfoLog("Resuming clustering from file ID %d (%d/%d files processed)",
			lastProcessedID, progress.FilesProcessed, progress.TotalFiles)
	} else if progress != nil && c.forceRecluster {
		// Force recluster - clear everything
		util.InfoLog("Force re-clustering: clearing previous state")
		if err := c.store.ClearClusters(); err != nil {
			return nil, fmt.Errorf("failed to clear clusters: %w", err)
		}
		if err := c.store.ClearClusteringProgress(); err != nil {
			return nil, fmt.Errorf("failed to clear progress: %w", err)
		}
	} else if !resuming {
		// Starting fresh - clear any existing clusters
		if err := c.store.ClearClusters(); err != nil {
			return nil, fmt.Errorf("failed to clear clusters: %w", err)
		}
	}

	// Get all files with status "meta_ok"
	files, err := c.store.GetFilesByStatus("meta_ok")
	if err != nil {
		return nil, fmt.Errorf("failed to get files: %w", err)
	}

	if len(files) == 0 {
		util.InfoLog("No files to cluster")
		return &Result{}, nil
	}

	if !resuming {
		util.InfoLog("Found %d files to cluster", len(files))
		// Initialize progress tracking
		if err := c.store.InitClusteringProgress(len(files)); err != nil {
			return nil, fmt.Errorf("failed to init progress: %w", err)
		}
	}

	result := &Result{
		Errors: make([]error, 0),
	}

	// Group files by cluster key
	clusterMap := make(map[string][]*store.File)

	// If resuming, rebuild cluster map from existing clusters
	if resuming {
		util.InfoLog("Rebuilding cluster map from existing clusters...")
		existingClusters, err := c.store.GetAllClusters()
		if err != nil {
			return nil, fmt.Errorf("failed to load existing clusters: %w", err)
		}

		for _, cluster := range existingClusters {
			members, err := c.store.GetClusterMembers(cluster.ClusterKey)
			if err != nil {
				util.WarnLog("Failed to load members for cluster %s: %v", cluster.ClusterKey, err)
				continue
			}

			// Load file details for each member
			for _, member := range members {
				file, err := c.store.GetFileByID(member.FileID)
				if err != nil {
					util.WarnLog("Failed to load file %d: %v", member.FileID, err)
					continue
				}
				if file != nil {
					clusterMap[cluster.ClusterKey] = append(clusterMap[cluster.ClusterKey], file)
				}
			}
		}
		util.InfoLog("Loaded %d existing clusters with %d files", len(clusterMap), progress.FilesProcessed)
	}

	// Progress reporting for grouping phase
	util.InfoLog("Grouping files into clusters...")

	var processed int64
	startTime := time.Now()
	lastUpdateTime := time.Now()
	var lastRate float64

	// Progress ticker
	progressTicker := time.NewTicker(1 * time.Second)
	defer progressTicker.Stop()

	progressDone := make(chan struct{})
	go func() {
		defer close(progressDone)
		for {
			select {
			case <-ctx.Done():
				return
			case <-progressTicker.C:
				p := processed
				if p > 0 {
					elapsed := time.Since(lastUpdateTime).Seconds()
					if elapsed > 0 {
						lastRate = float64(p) / time.Since(startTime).Seconds()
					}
					percentage := float64(p) / float64(len(files)) * 100
					util.InfoLog("Clustering | %d/%d grouped (%.1f%%) | %.1f files/s",
						p, len(files), percentage, lastRate)
					lastUpdateTime = time.Now()
				}
			}
		}
	}()

	for _, file := range files {
		select {
		case <-ctx.Done():
			// Save progress before exiting
			_ = c.store.UpdateClusteringProgress(file.ID, int(processed), len(clusterMap))
			return result, ctx.Err()
		default:
		}

		// Skip files we've already processed (resume logic)
		if resuming && file.ID <= lastProcessedID {
			processed++
			continue
		}

		// Get metadata for file
		metadata, err := c.store.GetMetadata(file.ID)
		if err != nil {
			util.ErrorLog("Failed to get metadata for file %d: %v", file.ID, err)
			result.Errors = append(result.Errors, err)
			continue
		}

		if metadata == nil {
			util.WarnLog("No metadata found for file %d", file.ID)
			continue
		}

		// Generate cluster key (pass source path for filename fallback)
		clusterKey := GenerateClusterKey(metadata, file.SrcPath)

		// Add to cluster map
		clusterMap[clusterKey] = append(clusterMap[clusterKey], file)
		result.FilesGrouped++
		processed++

		// Periodically save progress (every 1000 files)
		if processed%1000 == 0 {
			if err := c.store.UpdateClusteringProgress(file.ID, int(processed), len(clusterMap)); err != nil {
				util.WarnLog("Failed to save clustering progress: %v", err)
			}
		}
	}

	// Stop progress reporting
	progressTicker.Stop()
	<-progressDone

	util.InfoLog("Grouped %d files into %d potential clusters", result.FilesGrouped, len(clusterMap))

	// Insert clusters and members
	util.InfoLog("Writing clusters to database...")

	var clustersProcessed int64
	totalClusters := len(clusterMap)
	startTime = time.Now()
	lastUpdateTime = time.Now()
	lastRate = 0

	// Progress ticker for writing phase
	progressTicker = time.NewTicker(1 * time.Second)
	defer progressTicker.Stop()

	progressDone = make(chan struct{})
	go func() {
		defer close(progressDone)
		for {
			select {
			case <-ctx.Done():
				return
			case <-progressTicker.C:
				p := clustersProcessed
				if p > 0 {
					elapsed := time.Since(lastUpdateTime).Seconds()
					if elapsed > 0 {
						lastRate = float64(p) / time.Since(startTime).Seconds()
					}
					percentage := float64(p) / float64(totalClusters) * 100
					util.InfoLog("Writing clusters | %d/%d written (%.1f%%) | %.1f clusters/s | %d duplicates",
						p, totalClusters, percentage, lastRate, result.DuplicateClusters)
					lastUpdateTime = time.Now()
				}
			}
		}
	}()

	for clusterKey, members := range clusterMap {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		// Create hint for cluster (first file's metadata)
		hint := ""
		if len(members) > 0 {
			metadata, _ := c.store.GetMetadata(members[0].ID)
			if metadata != nil {
				hint = fmt.Sprintf("%s - %s", metadata.TagArtist, metadata.TagTitle)
			}
		}

		// Insert cluster
		cluster := &store.Cluster{
			ClusterKey: clusterKey,
			Hint:       hint,
		}

		if err := c.store.InsertCluster(cluster); err != nil {
			util.ErrorLog("Failed to insert cluster %s: %v", clusterKey, err)
			result.Errors = append(result.Errors, err)
			continue
		}

		result.ClustersCreated++

		// Track cluster type
		if len(members) == 1 {
			result.SingletonClusters++
		} else {
			result.DuplicateClusters++
		}

		// Insert cluster members
		for _, file := range members {
			member := &store.ClusterMember{
				ClusterKey:   clusterKey,
				FileID:       file.ID,
				QualityScore: 0, // Will be set by scorer
				Preferred:    false,
			}

			if err := c.store.InsertClusterMember(member); err != nil {
				util.ErrorLog("Failed to insert cluster member: %v", err)
				result.Errors = append(result.Errors, err)
			}

			// Log cluster event
			if c.logger != nil {
				c.logger.LogCluster(file.FileKey, file.SrcPath, clusterKey, len(members))
			}
		}

		clustersProcessed++
	}

	// Stop progress reporting
	progressTicker.Stop()
	<-progressDone

	util.SuccessLog("Clustering complete: %d clusters created (%d singletons, %d duplicates)",
		result.ClustersCreated, result.SingletonClusters, result.DuplicateClusters)

	// Clear progress tracking since we completed successfully
	if err := c.store.ClearClusteringProgress(); err != nil {
		util.WarnLog("Failed to clear clustering progress: %v", err)
	}

	return result, nil
}

// GenerateClusterKey creates a cluster key from metadata
// Key format: artist_norm|title_base|version_type|duration_bucket
//
// The version_type separates different artistic works:
//   - "studio" = original studio recording (includes remasters, deluxe editions)
//   - "remix" = remixed versions (radio edit, club mix, etc.)
//   - "live" = live performances
//   - "acoustic" = acoustic/unplugged versions
//   - "demo" = demo recordings
//   - "instrumental" = instrumental/karaoke versions
//
// Duration bucketing naturally separates versions with different lengths,
// while version_type ensures separation even when durations are similar.
func GenerateClusterKey(m *store.Metadata, srcPath string) string {
	// Normalize artist and title
	artistNorm := meta.NormalizeArtist(m.TagArtist)

	// Detect version type BEFORE normalizing title (need original text)
	versionType := meta.DetectVersionType(m.TagTitle)

	// Normalize title (this removes ALL version suffixes for base title)
	titleNorm := meta.NormalizeTitle(m.TagTitle)

	// If both are empty, use filename to prevent false duplicates
	// Files without metadata should only cluster if they have the same filename
	if artistNorm == "" && titleNorm == "" {
		// Extract filename without extension
		filename := filepath.Base(srcPath)
		ext := filepath.Ext(filename)
		filenameNoExt := strings.TrimSuffix(filename, ext)

		// Detect version type from filename
		versionType = meta.DetectVersionType(filenameNoExt)

		// Use filename as title (normalized)
		titleNorm = meta.NormalizeTitle(filenameNoExt)
		artistNorm = "unknown"

		// If filename is also empty/generic, use path hash to ensure uniqueness
		if titleNorm == "" {
			titleNorm = fmt.Sprintf("file_%s", filepath.Base(filepath.Dir(srcPath)))
		}
	}

	// Duration bucket (±1.5s tolerance)
	// Round to nearest 3-second bucket to group similar durations
	durationBucket := bucketDuration(m.DurationMs)

	// Generate cluster key with version type
	return fmt.Sprintf("%s|%s|%s|%d", artistNorm, titleNorm, versionType, durationBucket)
}

// bucketDuration rounds duration to nearest 3-second bucket
// This allows files with durations within ±1.5s to cluster together
func bucketDuration(durationMs int) int {
	if durationMs <= 0 {
		return 0
	}

	// Convert to seconds
	durationSec := float64(durationMs) / 1000.0

	// Round to nearest 3-second bucket
	bucketSize := 3.0
	bucket := math.Round(durationSec/bucketSize) * bucketSize

	return int(bucket)
}

// GetDurationDelta calculates the absolute difference in duration (ms)
func GetDurationDelta(duration1, duration2 int) int {
	delta := duration1 - duration2
	if delta < 0 {
		return -delta
	}
	return delta
}

// NormalizeForClustering applies additional normalization for clustering
// Removes common patterns that shouldn't affect clustering
func NormalizeForClustering(text string) string {
	text = strings.ToLower(text)
	text = strings.TrimSpace(text)

	// Remove common patterns
	replacements := map[string]string{
		"  ": " ",  // Collapse multiple spaces
		"(": " ",   // Remove parentheses
		")": " ",
		"[": " ",   // Remove brackets
		"]": " ",
		"{": " ",
		"}": " ",
		"&": "and", // Normalize ampersand
		"+": "and",
	}

	for old, new := range replacements {
		text = strings.ReplaceAll(text, old, new)
	}

	// Collapse whitespace again
	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}

	return strings.TrimSpace(text)
}
