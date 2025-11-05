package musicbrainz

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/franz/music-janitor/internal/util"
)

// Cache provides database-backed caching for MusicBrainz lookups
type Cache struct {
	db     *sql.DB
	client *Client
}

// CachedArtist represents a cached MusicBrainz artist lookup
type CachedArtist struct {
	SearchName    string
	CanonicalName string
	MBID          string
	Aliases       []string
	Score         int
	CachedAt      time.Time
}

// NewCache creates a new cache instance
func NewCache(db *sql.DB, client *Client) *Cache {
	return &Cache{
		db:     db,
		client: client,
	}
}

// EnsureSchema creates the cache table if it doesn't exist
func (c *Cache) EnsureSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS musicbrainz_cache (
		search_name TEXT PRIMARY KEY,
		canonical_name TEXT NOT NULL,
		mbid TEXT,
		aliases TEXT, -- JSON array of alias strings
		score INTEGER,
		cached_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		hit_count INTEGER DEFAULT 0
	);

	CREATE INDEX IF NOT EXISTS idx_mb_canonical ON musicbrainz_cache(canonical_name);
	CREATE INDEX IF NOT EXISTS idx_mb_mbid ON musicbrainz_cache(mbid);
	`

	_, err := c.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to create musicbrainz_cache table: %w", err)
	}

	return nil
}

// GetCanonicalName retrieves canonical name with cache support
// Checks cache first, falls back to API if not found
func (c *Cache) GetCanonicalName(ctx context.Context, artistName string) (canonical string, aliases []string, err error) {
	if artistName == "" {
		return "", nil, fmt.Errorf("artist name cannot be empty")
	}

	// Normalize search name for cache key
	searchKey := strings.ToLower(strings.TrimSpace(artistName))

	// Check cache first
	cached, err := c.getFromCache(searchKey)
	if err == nil && cached != nil {
		// Cache hit
		util.DebugLog("MusicBrainz cache hit: '%s' -> '%s'", artistName, cached.CanonicalName)
		c.incrementHitCount(searchKey)
		return cached.CanonicalName, cached.Aliases, nil
	}

	// Cache miss - query API
	util.DebugLog("MusicBrainz cache miss: '%s', querying API", artistName)
	canonical, aliases, err = c.client.GetCanonicalName(ctx, artistName)
	if err != nil {
		return "", nil, err
	}

	// Store in cache
	if err := c.storeInCache(searchKey, canonical, aliases, 100); err != nil {
		util.WarnLog("Failed to cache MusicBrainz result: %v", err)
		// Don't fail the operation if caching fails
	}

	return canonical, aliases, nil
}

// NormalizeArtistName normalizes an artist name using cached MusicBrainz data
func (c *Cache) NormalizeArtistName(ctx context.Context, artistName string) (string, error) {
	canonical, _, err := c.GetCanonicalName(ctx, artistName)
	return canonical, err
}

// getFromCache retrieves a cached lookup
func (c *Cache) getFromCache(searchName string) (*CachedArtist, error) {
	query := `
		SELECT canonical_name, mbid, aliases, score, cached_at
		FROM musicbrainz_cache
		WHERE search_name = ?
	`

	var cached CachedArtist
	var aliasesStr string
	var mbid sql.NullString

	err := c.db.QueryRow(query, searchName).Scan(
		&cached.CanonicalName,
		&mbid,
		&aliasesStr,
		&cached.Score,
		&cached.CachedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil // Not found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query cache: %w", err)
	}

	cached.SearchName = searchName
	if mbid.Valid {
		cached.MBID = mbid.String
	}

	// Parse aliases from comma-separated string
	if aliasesStr != "" {
		cached.Aliases = strings.Split(aliasesStr, ",")
	}

	return &cached, nil
}

// storeInCache stores a lookup result in the cache
func (c *Cache) storeInCache(searchName, canonicalName string, aliases []string, score int) error {
	// Convert aliases to comma-separated string
	aliasesStr := strings.Join(aliases, ",")

	query := `
		INSERT OR REPLACE INTO musicbrainz_cache
		(search_name, canonical_name, mbid, aliases, score, cached_at, hit_count)
		VALUES (?, ?, NULL, ?, ?, ?, COALESCE((SELECT hit_count FROM musicbrainz_cache WHERE search_name = ?), 0))
	`

	_, err := c.db.Exec(query, searchName, canonicalName, aliasesStr, score, time.Now(), searchName)
	if err != nil {
		return fmt.Errorf("failed to insert cache entry: %w", err)
	}

	return nil
}

// incrementHitCount increments the cache hit counter
func (c *Cache) incrementHitCount(searchName string) {
	query := `UPDATE musicbrainz_cache SET hit_count = hit_count + 1 WHERE search_name = ?`
	_, err := c.db.Exec(query, searchName)
	if err != nil {
		util.DebugLog("Failed to increment hit count: %v", err)
	}
}

// GetStats returns cache statistics
func (c *Cache) GetStats() (entries int, totalHits int64, err error) {
	query := `SELECT COUNT(*), COALESCE(SUM(hit_count), 0) FROM musicbrainz_cache`
	err = c.db.QueryRow(query).Scan(&entries, &totalHits)
	return
}

// ClearCache removes all cached entries
func (c *Cache) ClearCache() error {
	_, err := c.db.Exec("DELETE FROM musicbrainz_cache")
	return err
}

// ClearOldEntries removes cache entries older than the specified duration
func (c *Cache) ClearOldEntries(olderThan time.Duration) (int, error) {
	cutoff := time.Now().Add(-olderThan)
	result, err := c.db.Exec("DELETE FROM musicbrainz_cache WHERE cached_at < ?", cutoff)
	if err != nil {
		return 0, err
	}

	rows, _ := result.RowsAffected()
	return int(rows), nil
}

// PreloadArtists preloads multiple artist names into the cache in batch
// This is useful for scanning large libraries - fetch all unique artists first
func (c *Cache) PreloadArtists(ctx context.Context, artistNames []string) error {
	util.InfoLog("Preloading %d artists from MusicBrainz...", len(artistNames))

	successCount := 0
	errorCount := 0

	for i, name := range artistNames {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Skip if already cached
		searchKey := strings.ToLower(strings.TrimSpace(name))
		cached, _ := c.getFromCache(searchKey)
		if cached != nil {
			continue
		}

		// Fetch from API
		canonical, aliases, err := c.client.GetCanonicalName(ctx, name)
		if err != nil {
			util.WarnLog("Failed to fetch '%s': %v", name, err)
			errorCount++
			continue
		}

		// Cache it
		if err := c.storeInCache(searchKey, canonical, aliases, 100); err != nil {
			util.WarnLog("Failed to cache '%s': %v", name, err)
			errorCount++
			continue
		}

		successCount++

		// Progress reporting every 10 artists
		if (i+1)%10 == 0 {
			util.InfoLog("Progress: %d/%d artists preloaded", i+1, len(artistNames))
		}
	}

	util.SuccessLog("Preloaded %d artists (%d errors)", successCount, errorCount)
	return nil
}
