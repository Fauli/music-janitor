package store

import (
	"database/sql"
)

// InsertOrUpdateExecution inserts or updates an execution record
func (s *Store) InsertOrUpdateExecution(exec *Execution) error {
	verifyOK := 0
	if exec.VerifyOK {
		verifyOK = 1
	}

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO executions
		(file_id, started_at, completed_at, bytes_written, verify_ok, error)
		VALUES (?, ?, ?, ?, ?, ?)
	`, exec.FileID, exec.StartedAt, exec.CompletedAt, exec.BytesWritten, verifyOK, exec.Error)

	return err
}

// GetExecution gets the execution record for a file
func (s *Store) GetExecution(fileID int64) (*Execution, error) {
	var exec Execution
	var verifyOK int

	err := s.db.QueryRow(`
		SELECT file_id, started_at, completed_at, bytes_written, verify_ok, COALESCE(error, '')
		FROM executions
		WHERE file_id = ?
	`, fileID).Scan(&exec.FileID, &exec.StartedAt, &exec.CompletedAt, &exec.BytesWritten, &verifyOK, &exec.Error)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	exec.VerifyOK = verifyOK == 1
	return &exec, nil
}

// GetAllExecutions returns all execution records
func (s *Store) GetAllExecutions() ([]*Execution, error) {
	rows, err := s.db.Query(`
		SELECT file_id, started_at, completed_at, bytes_written, verify_ok, COALESCE(error, '')
		FROM executions
		ORDER BY completed_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var executions []*Execution
	for rows.Next() {
		var exec Execution
		var verifyOK int

		err := rows.Scan(&exec.FileID, &exec.StartedAt, &exec.CompletedAt, &exec.BytesWritten, &verifyOK, &exec.Error)
		if err != nil {
			return nil, err
		}

		exec.VerifyOK = verifyOK == 1
		executions = append(executions, &exec)
	}

	return executions, rows.Err()
}

// CountSuccessfulExecutions returns the count of successfully verified executions
func (s *Store) CountSuccessfulExecutions() (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM executions WHERE verify_ok = 1
	`).Scan(&count)

	return count, err
}

// GetTotalBytesWritten returns the total bytes written across all executions
func (s *Store) GetTotalBytesWritten() (int64, error) {
	var total int64
	err := s.db.QueryRow(`
		SELECT COALESCE(SUM(bytes_written), 0) FROM executions WHERE verify_ok = 1
	`).Scan(&total)

	return total, err
}
