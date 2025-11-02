package store

import (
	"database/sql"
)

// InsertPlan inserts a plan for a file
func (s *Store) InsertPlan(plan *Plan) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO plans (file_id, action, dest_path, reason)
		VALUES (?, ?, ?, ?)
	`, plan.FileID, plan.Action, plan.DestPath, plan.Reason)

	return err
}

// GetPlan gets the plan for a file
func (s *Store) GetPlan(fileID int64) (*Plan, error) {
	var p Plan
	err := s.db.QueryRow(`
		SELECT file_id, action, COALESCE(dest_path, ''), COALESCE(reason, '')
		FROM plans
		WHERE file_id = ?
	`, fileID).Scan(&p.FileID, &p.Action, &p.DestPath, &p.Reason)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	return &p, err
}

// GetAllPlans returns all plans
func (s *Store) GetAllPlans() ([]*Plan, error) {
	rows, err := s.db.Query(`
		SELECT file_id, action, COALESCE(dest_path, ''), COALESCE(reason, '')
		FROM plans
		ORDER BY file_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plans []*Plan
	for rows.Next() {
		var p Plan
		err := rows.Scan(&p.FileID, &p.Action, &p.DestPath, &p.Reason)
		if err != nil {
			return nil, err
		}
		plans = append(plans, &p)
	}

	return plans, rows.Err()
}

// GetPlansByAction returns plans with a specific action
func (s *Store) GetPlansByAction(action string) ([]*Plan, error) {
	rows, err := s.db.Query(`
		SELECT file_id, action, COALESCE(dest_path, ''), COALESCE(reason, '')
		FROM plans
		WHERE action = ?
		ORDER BY file_id
	`, action)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plans []*Plan
	for rows.Next() {
		var p Plan
		err := rows.Scan(&p.FileID, &p.Action, &p.DestPath, &p.Reason)
		if err != nil {
			return nil, err
		}
		plans = append(plans, &p)
	}

	return plans, rows.Err()
}

// CountPlansByAction returns the number of plans with a specific action
func (s *Store) CountPlansByAction(action string) (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM plans WHERE action = ?
	`, action).Scan(&count)

	return count, err
}

// ClearPlans removes all plans (for idempotent re-planning)
func (s *Store) ClearPlans() error {
	_, err := s.db.Exec(`DELETE FROM plans`)
	return err
}
