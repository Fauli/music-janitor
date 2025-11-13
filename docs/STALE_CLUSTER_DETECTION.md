# Stale Cluster Detection Implementation Guide

## Problem Statement

When users run `mlc rescan`, metadata is re-extracted and updated in the database. If artist names, song titles, or durations change, existing clusters become **stale** (incorrect groupings). Currently, there's no automatic way to detect this, requiring users to manually decide when to use `--force-recluster`.

### Example Scenario

```bash
# Initial scan - ffprobe not installed, duration extraction fails
mlc scan --source /music --db library.db
# Result: Many files have duration_ms = 0

# User runs plan - clusters created with duration=0
mlc plan --db library.db --dest /output

# User installs ffprobe
sudo apt install ffmpeg

# User rescans to fix metadata
mlc rescan --db library.db --errors-only
# Result: duration_ms now populated with correct values (e.g., 180000ms)

# User runs plan again WITHOUT --force-recluster
mlc plan --db library.db --dest /output
# Problem: Clusters still based on old duration_ms=0 values!
# Files that should cluster together are now separated
# Wrong duplicates selected, incorrect winners chosen
```

---

## Current Behavior Analysis

### What `rescan` Updates

Location: `cmd/mlc/rescan.go`, `internal/meta/extractor.go`

```go
// Rescan re-extracts metadata and REPLACES all fields
store.InsertMetadata() // Uses ON CONFLICT DO UPDATE
```

**Metadata fields that affect clustering:**
- `tag_artist` → normalized to `artistNorm` in cluster key
- `tag_title` → normalized to `titleNorm` in cluster key
- `duration_ms` → bucketed to nearest 3 seconds in cluster key

**Metadata fields that affect scoring only:**
- `codec`, `bitrate_kbps`, `lossless` (codec tier scoring)
- `bit_depth`, `sample_rate` (quality bonuses)
- `tag_album`, `tag_albumartist`, `tag_track`, `tag_date` (completeness)

### What `rescan` Does NOT Update

- Does not touch `clusters` table
- Does not touch `cluster_members` table
- Does not regenerate cluster keys
- Does not recalculate quality scores
- Does not update plans

### Cluster Key Generation

Location: `internal/cluster/cluster.go:336-368`

```go
func GenerateClusterKey(m *store.Metadata, srcPath string) string {
    artistNorm := meta.NormalizeArtist(m.TagArtist)
    titleNorm := meta.NormalizeTitle(m.TagTitle)
    durationBucket := bucketDuration(m.DurationMs)

    return fmt.Sprintf("%s|%s|%d", artistNorm, titleNorm, durationBucket)
}
```

**Critical observation**: Cluster keys are generated from metadata but NOT stored. They must be recomputed to detect staleness.

---

## Implementation Options

### Option 1: Simple Warning (No Schema Changes)

**Pros:**
- No database migration required
- Works immediately
- Low implementation complexity

**Cons:**
- Cannot detect actual staleness
- May show false positives
- No persistent tracking

**Implementation:**

1. **Add warning check in `cmd/mlc/plan.go`** before clustering phase:

```go
// In runPlan(), after database open, before Phase 1: Clustering

if !forceRecluster {
    existingClusters, err := db.CountClusters()
    if err == nil && existingClusters > 0 {
        util.WarnLog("⚠️  WARNING: Existing clusters detected (%d clusters)", existingClusters)
        util.WarnLog("   If you've run 'rescan' and metadata changed (artist/title/duration),")
        util.WarnLog("   clusters may be STALE and produce incorrect results.")
        util.WarnLog("   ")
        util.WarnLog("   Consider using: mlc plan --force-recluster")
        util.InfoLog("")

        // Optional: Add small delay so user sees warning
        time.Sleep(3 * time.Second)
    }
}
```

2. **Add documentation** in `docs/OPERATIONS.md`:

```markdown
## When to Use --force-recluster

### Always use after:
- Running `rescan` that fixes extraction errors
- Installing/upgrading ffprobe
- Batch tag edits to artist/title fields

### Safe default:
If unsure, use `--force-recluster` - it's idempotent and guarantees correctness.
```

**Estimated effort**: 1 hour

---

### Option 2: Smart Detection with Timestamps (Schema v4)

**Pros:**
- Accurate detection of when reclustering is needed
- No false positives from non-clustering metadata changes
- Provides clear guidance to users

**Cons:**
- Requires database migration
- More complex implementation
- Need to handle backward compatibility

**Implementation:**

#### Step 1: Add Schema v4

File: `internal/store/schema.go`

```go
// Schema v4 - Add cluster staleness detection
const schemaV4 = `
-- Track when clusters were created
ALTER TABLE clusters ADD COLUMN created_at DATETIME DEFAULT CURRENT_TIMESTAMP;

-- Track when metadata was last extracted
ALTER TABLE metadata ADD COLUMN extracted_at DATETIME DEFAULT CURRENT_TIMESTAMP;
`
```

Update migration in `internal/store/store.go`:

```go
const (
    currentSchemaVersion = 4
)

// In migrate() function, add:
if version < 4 {
    if _, err := tx.Exec(schemaV4); err != nil {
        return fmt.Errorf("failed to apply schema v4: %w", err)
    }
    if err := s.setSchemaVersion(tx, 4); err != nil {
        return fmt.Errorf("failed to set schema version: %w", err)
    }
}
```

#### Step 2: Update Clustering to Set Timestamp

File: `internal/cluster/cluster.go`

```go
// After InsertCluster()
if err := c.store.InsertCluster(cluster); err != nil {
    return err
}

// Set created_at to now (already done by DEFAULT in schema, but explicit is safer)
```

#### Step 3: Update Rescan to Update Timestamp

File: `internal/meta/extractor.go` or similar

```go
// After metadata extraction
metadata.ExtractedAt = time.Now()

// InsertMetadata will update extracted_at via ON CONFLICT clause
```

Modify `internal/store/metadata.go`:

```go
func (s *Store) InsertMetadata(metadata *Metadata) error {
    _, err := s.db.Exec(`
        INSERT INTO metadata (file_id, format, codec, ..., extracted_at)
        VALUES (?, ?, ?, ..., ?)
        ON CONFLICT(file_id) DO UPDATE SET
            format = excluded.format,
            codec = excluded.codec,
            ...
            extracted_at = excluded.extracted_at  -- IMPORTANT: update timestamp
    `, ...)
    return err
}
```

#### Step 4: Add Staleness Check in Plan Command

File: `cmd/mlc/plan.go`

```go
// Before Phase 1: Clustering
if !forceRecluster {
    staleCount, err := db.CountStaleClusterMembers()
    if err == nil && staleCount > 0 {
        util.WarnLog("⚠️  WARNING: %d files have stale clusters", staleCount)
        util.WarnLog("   Metadata was re-extracted after clustering, affecting duplicate detection.")
        util.WarnLog("   ")
        util.WarnLog("   Clusters will be automatically regenerated (use --force-recluster to force).")
        util.InfoLog("")

        // Auto-enable force recluster
        forceRecluster = true
    }
}
```

#### Step 5: Add Database Query for Staleness

File: `internal/store/clusters.go`

```go
// CountStaleClusterMembers counts cluster members where metadata was
// extracted AFTER the cluster was created (indicating staleness)
func (s *Store) CountStaleClusterMembers() (int, error) {
    var count int
    err := s.db.QueryRow(`
        SELECT COUNT(DISTINCT cm.file_id)
        FROM cluster_members cm
        JOIN clusters c ON cm.cluster_key = c.cluster_key
        JOIN metadata m ON cm.file_id = m.file_id
        WHERE m.extracted_at > c.created_at
    `).Scan(&count)

    if err == sql.ErrNoRows {
        return 0, nil
    }

    return count, err
}

// GetStaleClusterDetails returns detailed info about stale clusters
func (s *Store) GetStaleClusterDetails() ([]*StaleClusterInfo, error) {
    rows, err := s.db.Query(`
        SELECT
            c.cluster_key,
            c.hint,
            c.created_at,
            COUNT(DISTINCT cm.file_id) as stale_members,
            MAX(m.extracted_at) as latest_extraction
        FROM clusters c
        JOIN cluster_members cm ON c.cluster_key = cm.cluster_key
        JOIN metadata m ON cm.file_id = m.file_id
        WHERE m.extracted_at > c.created_at
        GROUP BY c.cluster_key
        ORDER BY stale_members DESC
        LIMIT 10
    `)
    // ... parse results
}
```

#### Step 6: Optional - Add Validation Command

File: `cmd/mlc/validate.go` (new file)

```go
var validateCmd = &cobra.Command{
    Use:   "validate",
    Short: "Validate database integrity and detect issues",
    RunE:  runValidate,
}

func init() {
    rootCmd.AddCommand(validateCmd)
    validateCmd.Flags().Bool("clusters", false, "Check for stale clusters")
}

func runValidate(cmd *cobra.Command, args []string) error {
    checkClusters, _ := cmd.Flags().GetBool("clusters")

    if checkClusters {
        staleCount, err := db.CountStaleClusterMembers()
        if err != nil {
            return err
        }

        if staleCount > 0 {
            util.WarnLog("⚠️  Found %d files with stale clusters", staleCount)

            // Show details
            details, _ := db.GetStaleClusterDetails()
            util.InfoLog("\nTop stale clusters:")
            for _, info := range details {
                util.InfoLog("  %s (%d files)", info.Hint, info.StaleMembers)
                util.InfoLog("    Clustered: %s", info.CreatedAt)
                util.InfoLog("    Last extraction: %s", info.LatestExtraction)
            }

            util.InfoLog("\nRun: mlc plan --force-recluster")
        } else {
            util.SuccessLog("✓ All clusters are up-to-date")
        }
    }

    return nil
}
```

**Estimated effort**: 4-6 hours

---

### Option 3: Cluster Key Hash Validation (Most Accurate)

**Pros:**
- Detects ONLY actual cluster key changes
- No false positives from non-clustering metadata updates
- Most precise detection

**Cons:**
- Requires storing cluster key hash
- More complex logic
- Requires recomputing cluster keys

**Implementation:**

#### Step 1: Add Schema v4 with Hash Column

```go
const schemaV4 = `
-- Store hash of cluster key components for staleness detection
ALTER TABLE cluster_members ADD COLUMN cluster_key_hash TEXT;

-- Index for fast lookups
CREATE INDEX IF NOT EXISTS idx_cluster_members_hash
ON cluster_members(cluster_key_hash);
`
```

#### Step 2: Compute and Store Hash During Clustering

File: `internal/cluster/cluster.go`

```go
// When inserting cluster member
member := &store.ClusterMember{
    ClusterKey:     clusterKey,
    FileID:         file.ID,
    ClusterKeyHash: computeClusterKeyHash(metadata),
}

func computeClusterKeyHash(m *store.Metadata) string {
    // Hash the three components used in cluster key
    artistNorm := meta.NormalizeArtist(m.TagArtist)
    titleNorm := meta.NormalizeTitle(m.TagTitle)
    durationBucket := bucketDuration(m.DurationMs)

    data := fmt.Sprintf("%s|%s|%d", artistNorm, titleNorm, durationBucket)
    hash := sha1.Sum([]byte(data))
    return hex.EncodeToString(hash[:])
}
```

#### Step 3: Check Hash During Plan

```go
// Before clustering, check if stored hash matches current metadata
staleMembers := []int64{}

for each cluster member:
    currentHash := computeClusterKeyHash(currentMetadata)
    storedHash := clusterMember.ClusterKeyHash

    if currentHash != storedHash:
        staleMembers = append(staleMembers, clusterMember.FileID)

if len(staleMembers) > 0:
    util.WarnLog("⚠️  %d files have clustering metadata changes", len(staleMembers))
    util.WarnLog("   Forcing re-cluster to ensure correct groupings")
    forceRecluster = true
```

**Estimated effort**: 6-8 hours

---

## Recommended Implementation Order

### Phase 1 (Immediate)
1. Implement **Option 1** (simple warning) - 1 hour
2. Add user documentation explaining when to use `--force-recluster`
3. Add to release notes

### Phase 2 (v1.7.0)
1. Implement **Option 2** (timestamp-based detection) - 4-6 hours
2. Add `validate --clusters` command
3. Automatic force-recluster when staleness detected

### Phase 3 (Future)
1. Consider **Option 3** (hash-based) if timestamp approach has issues
2. Add `mlc doctor` command for comprehensive health checks

---

## Testing Strategy

### Manual Testing

1. **Test stale detection**:
   ```bash
   # Create initial database
   mlc scan --source /music --db test.db
   mlc plan --db test.db --dest /output

   # Manually update metadata to simulate rescan
   sqlite3 test.db "UPDATE metadata SET tag_artist='Different Artist' WHERE file_id=1"

   # Run plan again - should detect staleness
   mlc plan --db test.db --dest /output
   # Expected: Warning about stale clusters
   ```

2. **Test false positive prevention**:
   ```bash
   # Update non-clustering metadata
   sqlite3 test.db "UPDATE metadata SET bitrate_kbps=320 WHERE file_id=1"

   # Should NOT trigger warning (Option 2 only)
   mlc plan --db test.db --dest /output
   ```

3. **Test force-recluster**:
   ```bash
   # Should regenerate regardless of staleness
   mlc plan --db test.db --dest /output --force-recluster
   ```

### Integration Tests

File: `internal/cluster/cluster_test.go`

```go
func TestStaleClusterDetection(t *testing.T) {
    // Setup database with clusters
    // Update metadata
    // Verify staleness detected
}

func TestForceReclusterOverride(t *testing.T) {
    // Verify --force-recluster regenerates clusters
}
```

---

## User-Facing Changes

### New CLI Behavior

**Before** (v1.5.x):
```bash
mlc plan --db library.db --dest /music
# Silent - may use stale clusters
```

**After Option 1** (v1.6.x):
```bash
mlc plan --db library.db --dest /music
# WARNING: Existing clusters detected (89234 clusters)
#   If you've run 'rescan' and metadata changed (artist/title/duration),
#   clusters may be STALE and produce incorrect results.
#
#   Consider using: mlc plan --force-recluster
#
# [continues after 3 second delay]
```

**After Option 2** (v1.7.0):
```bash
mlc plan --db library.db --dest /music
# WARNING: 1,234 files have stale clusters
#   Metadata was re-extracted after clustering, affecting duplicate detection.
#
#   Clusters will be automatically regenerated.
#
# === Phase 1: Clustering ===
# [automatically uses force-recluster]
```

### New Commands

```bash
# Check cluster health (Option 2+)
mlc validate --clusters --db library.db

# Output:
# ⚠️  Found 1,234 files with stale clusters
#
# Top stale clusters:
#   Pink Floyd - Money (3 files)
#     Clustered: 2024-01-01 10:00:00
#     Last extraction: 2024-01-02 15:30:00
#
# Run: mlc plan --force-recluster
```

---

## Migration Path

### Upgrading from v1.5.x to v1.6.x (Option 1)

**No migration required** - warning is purely informational.

User action: Use `--force-recluster` when prompted.

### Upgrading from v1.6.x to v1.7.0 (Option 2)

**Schema migration v4**:
- Adds `created_at` to `clusters` table
- Adds `extracted_at` to `metadata` table

**Backward compatibility**:
- Existing clusters will have `created_at = NULL`
- Existing metadata will have `extracted_at = NULL`
- Staleness check handles NULL (treats as "unknown, assume stale")

**User action after upgrade**:
```bash
# First plan after upgrade will show warning about NULL timestamps
mlc plan --db library.db --dest /music --force-recluster

# Populates created_at for all clusters
# Future rescans will populate extracted_at
```

---

## Documentation Updates Required

1. **README.md**: Add note about `--force-recluster` flag
2. **docs/OPERATIONS.md**: Document when to use force-recluster
3. **docs/TROUBLESHOOTING.md**: Add section on stale cluster issues
4. **docs/PROCESS_DETAILS.md**: Update clustering section with staleness info
5. **Release notes**: Document new behavior

---

## Open Questions

1. **Should force-recluster be automatic or require user confirmation?**
   - Current plan: Automatic with warning (Option 2)
   - Alternative: Prompt user (requires interactive mode)

2. **How to handle NULL timestamps in migration?**
   - Current plan: Treat as "unknown, assume stale"
   - Alternative: Backfill with "oldest timestamp" to avoid false positives

3. **Should we add a dry-run mode for validation?**
   - Current plan: Yes, via `validate --clusters` command
   - Shows what would be reclustered without actually doing it

---

## Priority and Timeline

**Priority**: HIGH (prevents incorrect duplicate detection)

**Target version**:
- Option 1: v1.6.1 (patch release)
- Option 2: v1.7.0 (minor release)

**Dependencies**:
- None for Option 1
- Schema v4 for Option 2

**Breaking changes**: None (backward compatible)
