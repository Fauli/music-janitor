package store

import (
	"database/sql"
	"fmt"
)

// Cluster represents a group of duplicate files
type Cluster struct {
	ClusterKey string
	Hint       string
}

// InsertCluster inserts a new cluster
func (s *Store) InsertCluster(cluster *Cluster) error {
	_, err := s.db.Exec(`
		INSERT INTO clusters (cluster_key, hint)
		VALUES (?, ?)
	`, cluster.ClusterKey, cluster.Hint)

	return err
}

// InsertClusterMember inserts a cluster member
func (s *Store) InsertClusterMember(member *ClusterMember) error {
	preferred := 0
	if member.Preferred {
		preferred = 1
	}

	_, err := s.db.Exec(`
		INSERT INTO cluster_members (cluster_key, file_id, quality_score, preferred)
		VALUES (?, ?, ?, ?)
	`, member.ClusterKey, member.FileID, member.QualityScore, preferred)

	return err
}

// UpdateClusterMemberScore updates the quality score for a cluster member
func (s *Store) UpdateClusterMemberScore(clusterKey string, fileID int64, score float64) error {
	_, err := s.db.Exec(`
		UPDATE cluster_members
		SET quality_score = ?
		WHERE cluster_key = ? AND file_id = ?
	`, score, clusterKey, fileID)

	return err
}

// UpdateClusterMemberPreferred sets a cluster member as preferred (winner)
func (s *Store) UpdateClusterMemberPreferred(clusterKey string, fileID int64, preferred bool) error {
	preferredInt := 0
	if preferred {
		preferredInt = 1
	}

	_, err := s.db.Exec(`
		UPDATE cluster_members
		SET preferred = ?
		WHERE cluster_key = ? AND file_id = ?
	`, preferredInt, clusterKey, fileID)

	return err
}

// GetClusterMembers returns all members of a cluster
func (s *Store) GetClusterMembers(clusterKey string) ([]*ClusterMember, error) {
	rows, err := s.db.Query(`
		SELECT cluster_key, file_id, quality_score, preferred
		FROM cluster_members
		WHERE cluster_key = ?
		ORDER BY quality_score DESC, file_id ASC
	`, clusterKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []*ClusterMember
	for rows.Next() {
		var m ClusterMember
		var preferredInt int

		err := rows.Scan(&m.ClusterKey, &m.FileID, &m.QualityScore, &preferredInt)
		if err != nil {
			return nil, err
		}

		m.Preferred = preferredInt == 1
		members = append(members, &m)
	}

	return members, rows.Err()
}

// GetAllClusters returns all clusters
func (s *Store) GetAllClusters() ([]*Cluster, error) {
	rows, err := s.db.Query(`
		SELECT cluster_key, hint
		FROM clusters
		ORDER BY cluster_key
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clusters []*Cluster
	for rows.Next() {
		var c Cluster
		err := rows.Scan(&c.ClusterKey, &c.Hint)
		if err != nil {
			return nil, err
		}
		clusters = append(clusters, &c)
	}

	return clusters, rows.Err()
}

// GetClusterByKey gets a cluster by its key
func (s *Store) GetClusterByKey(clusterKey string) (*Cluster, error) {
	var c Cluster
	err := s.db.QueryRow(`
		SELECT cluster_key, hint
		FROM clusters
		WHERE cluster_key = ?
	`, clusterKey).Scan(&c.ClusterKey, &c.Hint)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	return &c, err
}

// ClearClusters removes all clusters and members (for idempotent re-clustering)
func (s *Store) ClearClusters() error {
	// Delete cluster members first (in case foreign keys aren't enabled)
	if _, err := s.db.Exec(`DELETE FROM cluster_members`); err != nil {
		return fmt.Errorf("failed to clear cluster members: %w", err)
	}

	// Then delete clusters
	if _, err := s.db.Exec(`DELETE FROM clusters`); err != nil {
		return fmt.Errorf("failed to clear clusters: %w", err)
	}

	return nil
}

// CountClusters returns the total number of clusters
func (s *Store) CountClusters() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM clusters`).Scan(&count)
	return count, err
}

// CountDuplicateClusters returns the number of clusters with multiple members
func (s *Store) CountDuplicateClusters() (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(DISTINCT cluster_key)
		FROM cluster_members
		GROUP BY cluster_key
		HAVING COUNT(*) > 1
	`).Scan(&count)

	if err == sql.ErrNoRows {
		return 0, nil
	}

	return count, err
}

// GetClusterMember gets a specific cluster member
func (s *Store) GetClusterMember(clusterKey string, fileID int64) (*ClusterMember, error) {
	var m ClusterMember
	var preferredInt int

	err := s.db.QueryRow(`
		SELECT cluster_key, file_id, quality_score, preferred
		FROM cluster_members
		WHERE cluster_key = ? AND file_id = ?
	`, clusterKey, fileID).Scan(&m.ClusterKey, &m.FileID, &m.QualityScore, &preferredInt)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	m.Preferred = preferredInt == 1
	return &m, nil
}

// InsertClusterBatch inserts multiple clusters in a single transaction
func (s *Store) InsertClusterBatch(clusters []*Cluster) error {
	if len(clusters) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO clusters (cluster_key, hint) VALUES (?, ?)`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, cluster := range clusters {
		if _, err := stmt.Exec(cluster.ClusterKey, cluster.Hint); err != nil {
			return fmt.Errorf("failed to insert cluster %s: %w", cluster.ClusterKey, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// InsertClusterMemberBatch inserts multiple cluster members in a single transaction
func (s *Store) InsertClusterMemberBatch(members []*ClusterMember) error {
	if len(members) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO cluster_members (cluster_key, file_id, quality_score, preferred) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, member := range members {
		preferred := 0
		if member.Preferred {
			preferred = 1
		}
		if _, err := stmt.Exec(member.ClusterKey, member.FileID, member.QualityScore, preferred); err != nil {
			return fmt.Errorf("failed to insert cluster member: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
