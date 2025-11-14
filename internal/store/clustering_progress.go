package store

import (
	"database/sql"
	"time"
)

// ClusteringProgress tracks the state of clustering operation for resumability
type ClusteringProgress struct {
	LastProcessedFileID int64
	TotalFiles          int
	FilesProcessed      int
	ClustersCreated     int
	StartedAt           time.Time
	UpdatedAt           time.Time
}

// GetClusteringProgress retrieves the current clustering progress
func (s *Store) GetClusteringProgress() (*ClusteringProgress, error) {
	var p ClusteringProgress
	var startedAt, updatedAt sql.NullString

	err := s.db.QueryRow(`
		SELECT last_processed_file_id, total_files, files_processed,
		       clusters_created, started_at, updated_at
		FROM clustering_progress
		WHERE id = 1
	`).Scan(&p.LastProcessedFileID, &p.TotalFiles, &p.FilesProcessed,
		&p.ClustersCreated, &startedAt, &updatedAt)

	if err == sql.ErrNoRows {
		// No progress tracked yet
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	// Parse timestamps
	if startedAt.Valid {
		p.StartedAt, _ = time.Parse("2006-01-02 15:04:05", startedAt.String)
	}
	if updatedAt.Valid {
		p.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt.String)
	}

	return &p, nil
}

// InitClusteringProgress initializes or resets clustering progress
func (s *Store) InitClusteringProgress(totalFiles int) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO clustering_progress
		(id, last_processed_file_id, total_files, files_processed, clusters_created, started_at, updated_at)
		VALUES (1, 0, ?, 0, 0, datetime('now'), datetime('now'))
	`, totalFiles)
	return err
}

// UpdateClusteringProgress updates progress during clustering
func (s *Store) UpdateClusteringProgress(lastFileID int64, filesProcessed, clustersCreated int) error {
	_, err := s.db.Exec(`
		UPDATE clustering_progress
		SET last_processed_file_id = ?,
		    files_processed = ?,
		    clusters_created = ?,
		    updated_at = datetime('now')
		WHERE id = 1
	`, lastFileID, filesProcessed, clustersCreated)
	return err
}

// ClearClusteringProgress removes progress tracking (called when clustering completes successfully)
func (s *Store) ClearClusteringProgress() error {
	_, err := s.db.Exec(`DELETE FROM clustering_progress WHERE id = 1`)
	return err
}

// HasInProgressClustering checks if there's a clustering operation in progress
func (s *Store) HasInProgressClustering() (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM clustering_progress WHERE id = 1`).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// DetectStaleClusters checks if clusters exist but metadata was updated after clustering
// Returns: (isStale bool, clusterTime time.Time, newestMetadataTime time.Time, error)
func (s *Store) DetectStaleClusters() (bool, time.Time, time.Time, error) {
	// Check if clusters exist
	var clusterCount int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM clusters`).Scan(&clusterCount)
	if err != nil {
		return false, time.Time{}, time.Time{}, err
	}

	if clusterCount == 0 {
		// No clusters exist, so can't be stale
		return false, time.Time{}, time.Time{}, nil
	}

	// Get the most recent file update timestamp (when metadata was last modified)
	var newestUpdateStr sql.NullString
	err = s.db.QueryRow(`
		SELECT MAX(last_update_at)
		FROM files
		WHERE status = 'meta_ok'
	`).Scan(&newestUpdateStr)

	if err != nil || !newestUpdateStr.Valid {
		// No metadata files found or error
		return false, time.Time{}, time.Time{}, err
	}

	newestUpdate, err := time.Parse("2006-01-02 15:04:05", newestUpdateStr.String)
	if err != nil {
		return false, time.Time{}, time.Time{}, err
	}

	// Check if we have clustering progress record (indicates when clustering started)
	// This is cleared when clustering completes, so we need to check cluster_members for timestamps
	// Since we don't have created_at on clusters table in current schema,
	// we use a heuristic: if any file was updated in the last hour and clusters exist,
	// clusters might be stale

	// Get oldest file that's part of a cluster
	var oldestClusteredFileStr sql.NullString
	err = s.db.QueryRow(`
		SELECT MIN(f.last_update_at)
		FROM files f
		INNER JOIN cluster_members cm ON f.id = cm.file_id
	`).Scan(&oldestClusteredFileStr)

	if err != nil || !oldestClusteredFileStr.Valid {
		// Can't determine cluster age
		return false, time.Time{}, time.Time{}, nil
	}

	oldestClusteredFile, err := time.Parse("2006-01-02 15:04:05", oldestClusteredFileStr.String)
	if err != nil {
		return false, time.Time{}, time.Time{}, err
	}

	// If newest metadata update is more recent than oldest clustered file update,
	// it suggests metadata changed after clustering
	// Add 1-second buffer to account for timing precision
	isStale := newestUpdate.After(oldestClusteredFile.Add(1 * time.Second))

	return isStale, oldestClusteredFile, newestUpdate, nil
}
