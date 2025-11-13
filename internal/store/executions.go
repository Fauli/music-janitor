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

// BatchInsertOrUpdateExecution inserts or updates multiple execution records in a single transaction
func (s *Store) BatchInsertOrUpdateExecution(executions []*Execution) error {
	if len(executions) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO executions
		(file_id, started_at, completed_at, bytes_written, verify_ok, error)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, exec := range executions {
		verifyOK := 0
		if exec.VerifyOK {
			verifyOK = 1
		}

		_, err := stmt.Exec(exec.FileID, exec.StartedAt, exec.CompletedAt, exec.BytesWritten, verifyOK, exec.Error)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetAllExecutionsMap returns all execution records as a map indexed by file_id
func (s *Store) GetAllExecutionsMap() (map[int64]*Execution, error) {
	rows, err := s.db.Query(`
		SELECT file_id, started_at, completed_at, bytes_written, verify_ok, COALESCE(error, '')
		FROM executions
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int64]*Execution)
	for rows.Next() {
		var exec Execution
		var verifyOK int

		err := rows.Scan(&exec.FileID, &exec.StartedAt, &exec.CompletedAt, &exec.BytesWritten, &verifyOK, &exec.Error)
		if err != nil {
			return nil, err
		}

		exec.VerifyOK = verifyOK == 1
		result[exec.FileID] = &exec
	}

	return result, rows.Err()
}
