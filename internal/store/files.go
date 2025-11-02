package store

import (
	"database/sql"
	"fmt"
	"time"
)

// InsertFile inserts or updates a file record
func (s *Store) InsertFile(f *File) error {
	result, err := s.db.Exec(`
		INSERT INTO files (file_key, src_path, size_bytes, mtime_unix, status)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(file_key) DO UPDATE SET
			src_path = excluded.src_path,
			size_bytes = excluded.size_bytes,
			mtime_unix = excluded.mtime_unix,
			last_update_at = CURRENT_TIMESTAMP
		`, f.FileKey, f.SrcPath, f.SizeBytes, f.MtimeUnix, f.Status)

	if err != nil {
		return fmt.Errorf("failed to insert file: %w", err)
	}

	// Get the inserted ID if this was a new insert
	if f.ID == 0 {
		id, err := result.LastInsertId()
		if err == nil {
			f.ID = id
		} else {
			// On conflict update, fetch the existing ID
			err = s.db.QueryRow("SELECT id FROM files WHERE file_key = ?", f.FileKey).Scan(&f.ID)
			if err != nil {
				return fmt.Errorf("failed to get file ID: %w", err)
			}
		}
	}

	return nil
}

// GetFileByKey retrieves a file by its file_key
func (s *Store) GetFileByKey(fileKey string) (*File, error) {
	f := &File{}
	err := s.db.QueryRow(`
		SELECT id, file_key, src_path, size_bytes, mtime_unix,
		       COALESCE(sha1, ''), status, COALESCE(error, ''),
		       first_seen_at, last_update_at
		FROM files WHERE file_key = ?
	`, fileKey).Scan(
		&f.ID, &f.FileKey, &f.SrcPath, &f.SizeBytes, &f.MtimeUnix,
		&f.SHA1, &f.Status, &f.Error,
		&f.FirstSeenAt, &f.LastUpdate,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get file: %w", err)
	}

	return f, nil
}

// UpdateFileStatus updates the status of a file
func (s *Store) UpdateFileStatus(fileID int64, status string, errorMsg string) error {
	_, err := s.db.Exec(`
		UPDATE files SET status = ?, error = ?, last_update_at = ?
		WHERE id = ?
	`, status, errorMsg, time.Now(), fileID)

	if err != nil {
		return fmt.Errorf("failed to update file status: %w", err)
	}

	return nil
}

// UpdateFileSHA1 updates the SHA1 hash of a file
func (s *Store) UpdateFileSHA1(fileID int64, sha1 string) error {
	_, err := s.db.Exec(`
		UPDATE files SET sha1 = ?, last_update_at = ?
		WHERE id = ?
	`, sha1, time.Now(), fileID)

	if err != nil {
		return fmt.Errorf("failed to update file SHA1: %w", err)
	}

	return nil
}

// GetFilesByStatus retrieves files with a given status
func (s *Store) GetFilesByStatus(status string) ([]*File, error) {
	rows, err := s.db.Query(`
		SELECT id, file_key, src_path, size_bytes, mtime_unix,
		       COALESCE(sha1, ''), status, COALESCE(error, ''),
		       first_seen_at, last_update_at
		FROM files WHERE status = ?
		ORDER BY id
	`, status)

	if err != nil {
		return nil, fmt.Errorf("failed to query files: %w", err)
	}
	defer rows.Close()

	var files []*File
	for rows.Next() {
		f := &File{}
		err := rows.Scan(
			&f.ID, &f.FileKey, &f.SrcPath, &f.SizeBytes, &f.MtimeUnix,
			&f.SHA1, &f.Status, &f.Error,
			&f.FirstSeenAt, &f.LastUpdate,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file: %w", err)
		}
		files = append(files, f)
	}

	return files, rows.Err()
}

// CountFilesByStatus returns the number of files with a given status
func (s *Store) CountFilesByStatus(status string) (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM files WHERE status = ?", status).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count files: %w", err)
	}
	return count, nil
}

// GetAllFiles retrieves all files
func (s *Store) GetAllFiles() ([]*File, error) {
	rows, err := s.db.Query(`
		SELECT id, file_key, src_path, size_bytes, mtime_unix,
		       COALESCE(sha1, ''), status, COALESCE(error, ''),
		       first_seen_at, last_update_at
		FROM files
		ORDER BY id
	`)

	if err != nil {
		return nil, fmt.Errorf("failed to query files: %w", err)
	}
	defer rows.Close()

	var files []*File
	for rows.Next() {
		f := &File{}
		err := rows.Scan(
			&f.ID, &f.FileKey, &f.SrcPath, &f.SizeBytes, &f.MtimeUnix,
			&f.SHA1, &f.Status, &f.Error,
			&f.FirstSeenAt, &f.LastUpdate,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file: %w", err)
		}
		files = append(files, f)
	}

	return files, rows.Err()
}

// GetFileByID retrieves a file by its ID
func (s *Store) GetFileByID(id int64) (*File, error) {
	f := &File{}
	err := s.db.QueryRow(`
		SELECT id, file_key, src_path, size_bytes, mtime_unix,
		       COALESCE(sha1, ''), status, COALESCE(error, ''),
		       first_seen_at, last_update_at
		FROM files WHERE id = ?
	`, id).Scan(
		&f.ID, &f.FileKey, &f.SrcPath, &f.SizeBytes, &f.MtimeUnix,
		&f.SHA1, &f.Status, &f.Error,
		&f.FirstSeenAt, &f.LastUpdate,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get file: %w", err)
	}

	return f, nil
}
