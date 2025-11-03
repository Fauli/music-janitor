package cluster

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/franz/music-janitor/internal/meta"
	"github.com/franz/music-janitor/internal/report"
	"github.com/franz/music-janitor/internal/store"
	"github.com/franz/music-janitor/internal/util"
)

// Clusterer groups files into duplicate clusters
type Clusterer struct {
	store  *store.Store
	logger *report.EventLogger
}

// Config holds clusterer configuration
type Config struct {
	Store  *store.Store
	Logger *report.EventLogger
}

// New creates a new Clusterer
func New(cfg *Config) *Clusterer {
	return &Clusterer{
		store:  cfg.Store,
		logger: cfg.Logger,
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

	// Get all files with status "meta_ok"
	files, err := c.store.GetFilesByStatus("meta_ok")
	if err != nil {
		return nil, fmt.Errorf("failed to get files: %w", err)
	}

	if len(files) == 0 {
		util.InfoLog("No files to cluster")
		return &Result{}, nil
	}

	util.InfoLog("Found %d files to cluster", len(files))

	result := &Result{
		Errors: make([]error, 0),
	}

	// Clear existing clusters (idempotent operation)
	if err := c.store.ClearClusters(); err != nil {
		return nil, fmt.Errorf("failed to clear clusters: %w", err)
	}

	// Group files by cluster key
	clusterMap := make(map[string][]*store.File)

	for _, file := range files {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
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

		// Generate cluster key
		clusterKey := GenerateClusterKey(metadata)

		// Add to cluster map
		clusterMap[clusterKey] = append(clusterMap[clusterKey], file)
		result.FilesGrouped++
	}

	// Insert clusters and members
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
	}

	util.SuccessLog("Clustering complete: %d clusters created (%d singletons, %d duplicates)",
		result.ClustersCreated, result.SingletonClusters, result.DuplicateClusters)

	return result, nil
}

// GenerateClusterKey creates a cluster key from metadata
// Key format: artist_norm|title_norm|duration_bucket
func GenerateClusterKey(m *store.Metadata) string {
	// Normalize artist and title
	artistNorm := meta.NormalizeArtist(m.TagArtist)
	titleNorm := meta.NormalizeTitle(m.TagTitle)

	// If both are empty, fall back to filename-based normalization
	if artistNorm == "" && titleNorm == "" {
		artistNorm = "unknown"
		titleNorm = "unknown"
	}

	// Duration bucket (±1.5s tolerance)
	// Round to nearest 3-second bucket to group similar durations
	durationBucket := bucketDuration(m.DurationMs)

	// Generate cluster key
	return fmt.Sprintf("%s|%s|%d", artistNorm, titleNorm, durationBucket)
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
