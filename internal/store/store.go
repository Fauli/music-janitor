package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // SQLite driver
)

const (
	currentSchemaVersion = 2
)

// Store represents the application's persistent state
type Store struct {
	db *sql.DB
}

// OpenOptions holds options for opening a database
type OpenOptions struct {
	NetworkOptimized bool // Apply network-optimized pragmas
}

// Open opens or creates a SQLite database at the given path with default options
func Open(path string) (*Store, error) {
	return OpenWithOptions(path, nil)
}

// OpenWithOptions opens or creates a SQLite database with custom options
func OpenWithOptions(path string, opts *OpenOptions) (*Store, error) {
	if opts == nil {
		opts = &OpenOptions{}
	}

	// Open with pragmas for performance and reliability
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_timeout=5000&_busy_timeout=5000", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(1) // SQLite works best with a single writer
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	store := &Store{db: db}

	// Apply network-optimized pragmas if requested
	if opts.NetworkOptimized {
		if err := store.applyNetworkPragmas(); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to apply network pragmas: %w", err)
		}
	}

	// Run migrations
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	return store, nil
}

// applyNetworkPragmas applies SQLite optimizations for network filesystems
func (s *Store) applyNetworkPragmas() error {
	pragmas := []string{
		// Reduce fsync calls - NORMAL is safe with WAL mode
		// Instead of fsync on every commit (FULL), only fsync at checkpoints
		"PRAGMA synchronous = NORMAL",

		// Keep temp tables in memory instead of on network disk
		"PRAGMA temp_store = MEMORY",

		// Increase cache size to 64MB (reduce network round-trips)
		// Negative value = KB (64000 KB = ~64 MB)
		"PRAGMA cache_size = -64000",

		// Increase page size to 8KB (better for network, default is 4KB)
		// Must be set before any tables are created, so this may not apply
		// to existing databases
		"PRAGMA page_size = 8192",
	}

	for _, pragma := range pragmas {
		if _, err := s.db.Exec(pragma); err != nil {
			return fmt.Errorf("failed to execute %s: %w", pragma, err)
		}
	}

	return nil
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying database connection for custom queries
func (s *Store) DB() *sql.DB {
	return s.db
}

// SQLiteVersion returns the SQLite version string
func SQLiteVersion() string {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return ""
	}
	defer db.Close()

	var version string
	err = db.QueryRow("SELECT sqlite_version()").Scan(&version)
	if err != nil {
		return ""
	}
	return version
}

// CheckIntegrity runs PRAGMA integrity_check on the database
func (s *Store) CheckIntegrity() error {
	var result string
	err := s.db.QueryRow("PRAGMA integrity_check").Scan(&result)
	if err != nil {
		return fmt.Errorf("integrity check query failed: %w", err)
	}

	if result != "ok" {
		return fmt.Errorf("integrity check failed: %s", result)
	}

	return nil
}

// migrate applies database migrations
func (s *Store) migrate() error {
	// Check current schema version
	version, err := s.getSchemaVersion()
	if err != nil {
		return err
	}

	if version >= currentSchemaVersion {
		// Already at current version
		return nil
	}

	// Start transaction for migration
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Apply schema v1
	if version < 1 {
		if _, err := tx.Exec(schemaV1); err != nil {
			return fmt.Errorf("failed to apply schema v1: %w", err)
		}
		if err := s.setSchemaVersion(tx, 1); err != nil {
			return fmt.Errorf("failed to set schema version: %w", err)
		}
	}

	// Apply schema v2 - Performance indexes
	if version < 2 {
		if _, err := tx.Exec(schemaV2); err != nil {
			return fmt.Errorf("failed to apply schema v2: %w", err)
		}
		if err := s.setSchemaVersion(tx, 2); err != nil {
			return fmt.Errorf("failed to set schema version: %w", err)
		}
	}

	// Future migrations would go here:
	// if version < 3 { ... }

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration: %w", err)
	}

	return nil
}

// getSchemaVersion returns the current schema version
func (s *Store) getSchemaVersion() (int, error) {
	// Check if schema_version table exists
	var exists int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name='schema_version'
	`).Scan(&exists)
	if err != nil {
		return 0, err
	}

	if exists == 0 {
		// No schema yet
		return 0, nil
	}

	// Get latest version
	var version int
	err = s.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&version)
	if err != nil {
		return 0, err
	}

	return version, nil
}

// setSchemaVersion records a schema version in a transaction
func (s *Store) setSchemaVersion(tx *sql.Tx, version int) error {
	_, err := tx.Exec("INSERT INTO schema_version (version) VALUES (?)", version)
	return err
}

// Transaction executes a function within a transaction
func (s *Store) Transaction(fn func(*sql.Tx) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// File represents a discovered file
type File struct {
	ID          int64
	FileKey     string
	SrcPath     string
	SizeBytes   int64
	MtimeUnix   int64
	SHA1        string
	Status      string
	Error       string
	FirstSeenAt time.Time
	LastUpdate  time.Time
}

// Metadata represents extracted audio metadata
type Metadata struct {
	FileID                 int64
	Format                 string
	Codec                  string
	Container              string
	DurationMs             int
	SampleRate             int
	BitDepth               int
	Channels               int
	BitrateKbps            int
	Lossless               bool
	TagArtist              string
	TagAlbum               string
	TagTitle               string
	TagAlbumArtist         string
	TagDate                string
	TagDisc                int
	TagDiscTotal           int
	TagTrack               int
	TagTrackTotal          int
	TagCompilation         bool
	MusicBrainzRecordingID string
	MusicBrainzReleaseID   string
	RawTagsJSON            string
}

// ClusterMember represents a file in a duplicate cluster
type ClusterMember struct {
	ClusterKey   string
	FileID       int64
	QualityScore float64
	Preferred    bool
}

// Plan represents the planned action for a file
type Plan struct {
	FileID   int64
	Action   string
	DestPath string
	Reason   string
}

// Execution represents the execution result for a file
type Execution struct {
	FileID       int64
	StartedAt    time.Time
	CompletedAt  time.Time
	BytesWritten int64
	VerifyOK     bool
	Error        string
}
