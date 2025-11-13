# MLC Process Details

This document provides a **detailed, technical explanation** of MLC's internal processing phases. It describes exactly how files flow through the system, what transformations occur, and how decisions are made.

---

## Table of Contents

1. [Overview: The Complete Pipeline](#overview-the-complete-pipeline)
2. [Phase 1: Scan](#phase-1-scan)
3. [Phase 2: Plan (Cluster → Score → Plan)](#phase-2-plan)
   - [2A: Clustering](#phase-2a-clustering)
   - [2B: Quality Scoring](#phase-2b-quality-scoring)
   - [2C: Destination Planning](#phase-2c-destination-planning)
4. [Phase 3: Execute](#phase-3-execute)
5. [Database State Transitions](#database-state-transitions)
6. [Resumability & Idempotency](#resumability--idempotency)

---

## Overview: The Complete Pipeline

MLC processes your music library in **three distinct phases**:

```
┌─────────┐      ┌───────────────────────┐      ┌─────────┐
│  SCAN   │─────▶│  PLAN                 │─────▶│ EXECUTE │
│         │      │  ├─ Cluster           │      │         │
│         │      │  ├─ Score             │      │         │
│         │      │  └─ Plan Destinations │      │         │
└─────────┘      └───────────────────────┘      └─────────┘
```

Each phase is:
- **Idempotent**: Can be run multiple times safely
- **Resumable**: Can be interrupted and restarted
- **Deterministic**: Same inputs produce same outputs
- **State-tracked**: Progress stored in SQLite database

---

## Phase 1: Scan

**Purpose**: Discover all audio files in the source directory and extract their metadata.

**Entry Point**: `mlc scan --source /path/to/music --db library.db`

**Code**: `internal/scan/scanner.go`, `internal/meta/ffprobe.go`

### 1.1 File Discovery

**Walking the Directory Tree**:
```go
// Uses filepath.WalkDir for efficient traversal
filepath.WalkDir(sourcePath, func(path string, d fs.DirEntry, err error) error {
    // Skip directories, hidden files, symlinks
    // Check file extension against whitelist
    // Send to processing channel
})
```

**Supported Extensions**:
- `.mp3`, `.flac`, `.m4a`, `.aac`
- `.ogg`, `.opus`, `.wav`, `.aiff`, `.aif`
- `.wma`, `.ape`, `.wv` (WavPack), `.mpc` (Musepack)

**Concurrency**: Uses worker pool (default: 4 workers) to process files in parallel.

### 1.2 File State Tracking

For each discovered file, MLC:

1. **Generates a file_key** (SHA1 of source path):
   ```go
   fileKey := util.HashString(absolutePath)
   ```

2. **Checks if file already exists** in database:
   - If exists AND mtime/size unchanged → **Skip** (use cached metadata)
   - If exists BUT mtime/size changed → **Re-extract** metadata
   - If new → **Insert** and extract metadata

3. **Stores file record** in `files` table:
   ```sql
   INSERT INTO files (file_key, src_path, size_bytes, mtime_unix, status)
   VALUES (?, ?, ?, ?, 'discovered')
   ```

### 1.3 Metadata Extraction

**Tool**: ffprobe (part of FFmpeg)

**For each file**:
```bash
ffprobe -v error -show_format -show_streams -of json "/path/to/file.mp3"
```

**Extracted Data**:
- **Format**: container (mp3, flac, m4a), codec (mp3, flac, aac, opus)
- **Audio Properties**:
  - Duration (milliseconds)
  - Bitrate (kbps)
  - Sample rate (Hz): 44100, 48000, 96000, 192000
  - Bit depth (bits): 16, 24, 32
  - Channels (mono/stereo)
  - Lossless flag (boolean)
- **Tags** (ID3, Vorbis Comments, MP4 atoms):
  - `artist`, `album`, `title`
  - `albumartist`, `date`
  - `disc`, `disc_total`, `track`, `track_total`
  - `compilation` flag
  - MusicBrainz IDs (if present)

**Normalization** (internal/meta/ffprobe.go):
- Tag encoding → UTF-8
- Whitespace trimming
- Date parsing (various formats → YYYY or YYYY-MM-DD)
- Disc/track numbers extracted from "1/12" format

**Storage**:
```sql
INSERT INTO metadata (file_id, format, codec, duration_ms, bitrate_kbps, ...)
VALUES (?, ?, ?, ?, ?, ...)
```

**File Status Update**:
- Success → `status = 'meta_ok'`
- Failure → `status = 'meta_error'`, error message stored

### 1.4 Progress Reporting

**Output Format**:
```
Scanning | 27315 found | 4413 new | 22902 cached | 2.1/s
[████████████████████░░░░░░░░░░░░] 65% | 27315 files
```

**Components**:
- **Found**: Total files discovered in this scan
- **New**: Files added to database (not seen before)
- **Cached**: Files skipped (unchanged since last scan)
- **Rate**: Files processed per second

**Visual Progress Bar**:
- Only shown if stdout is a terminal (TTY)
- Hidden if output is piped/redirected
- Updates every 200ms
- Cleared when scan completes

### 1.5 Scan Result

**Database State After Scan**:
```sql
-- Files table populated
SELECT COUNT(*) FROM files;                     -- All discovered files
SELECT COUNT(*) FROM files WHERE status = 'meta_ok';  -- Ready for clustering
SELECT COUNT(*) FROM files WHERE status = 'meta_error'; -- Failed extractions

-- Metadata table populated
SELECT COUNT(*) FROM metadata;  -- Should match meta_ok files
```

**Event Log** (`artifacts/events-YYYYMMDD-HHMMSS.jsonl`):
```json
{"timestamp":"...","event":"scan","file_key":"...","path":"...","status":"new"}
{"timestamp":"...","event":"metadata","file_key":"...","codec":"flac","bitrate":0,"lossless":true}
{"timestamp":"...","event":"scan","file_key":"...","path":"...","status":"cached"}
```

---

## Phase 2: Plan

**Purpose**: Analyze metadata, group duplicates, score quality, and determine destination paths.

**Entry Point**: `mlc plan --db library.db --dest /music`

**Duration**: For large libraries (100k+ files), this is the longest phase (hours).

### Phase 2A: Clustering

**Code**: `internal/cluster/cluster.go`

**Purpose**: Group files that represent the same recording (duplicates).

#### 2A.1 Clustering Algorithm

**Input**: All files with `status = 'meta_ok'`

**For each file**:

1. **Normalize Artist**:
   ```go
   artistNorm = NormalizeArtist(metadata.TagArtist)
   ```

   **Normalization Rules** (`internal/meta/normalize.go`):
   - Convert to lowercase
   - Unicode NFC normalization
   - Remove punctuation: `. , ! ? ' " : ; - /`
   - Replace `&` with `and`
   - Handle "Artist, The" → "the artist"
   - Collapse multiple spaces
   - Trim whitespace

   **MusicBrainz Integration** (optional):
   - If enabled, lookup artist on MusicBrainz
   - Use canonical artist name if found
   - Fallback to local rules if not found

   **Examples**:
   ```
   "The Beatles"      → "the beatles"
   "AC/DC"            → "acdc"
   "Beyoncé"          → "beyonce" (NFC normalization)
   "Artist & Friends" → "artist and friends"
   ```

2. **Detect Version Type** (before normalization):
   ```go
   versionType = DetectVersionType(metadata.TagTitle)
   ```

   **Version Type Detection** (with precedence):
   - **`live`**: Live performances, concerts, sessions
   - **`acoustic`**: Acoustic/unplugged versions
   - **`remix`**: Remixed versions, edits, extended versions
   - **`demo`**: Demo recordings, alternates, outtakes
   - **`instrumental`**: Instrumental/karaoke versions
   - **`studio`**: Original studio recordings (default, includes remasters)

   **Precedence**: `live > acoustic > remix > demo > instrumental > studio`

   **Examples**:
   ```
   "Song Title"                    → "studio"
   "Song Title (Remix)"            → "remix"
   "Song Title (Live)"             → "live"
   "Song Title (Acoustic)"         → "acoustic"
   "Song Title [2011 Remaster]"    → "studio"
   "Song Title (Live Acoustic)"    → "live"     (live wins)
   "Song Title (Acoustic Remix)"   → "acoustic" (acoustic wins)
   ```

3. **Normalize Title**:
   ```go
   titleNorm = NormalizeTitle(metadata.TagTitle)
   ```

   **Additional Title Rules**:
   - Remove ALL version suffixes to extract base title:
     - `(Remix)`, `(Live)`, `(Acoustic)`, `(Demo)`
     - `[Remaster]`, `[Deluxe]`, `[Bonus]`, `[Radio Edit]`
   - Version type is captured separately (step 2)
   - This extracts the base song title for clustering

   **Examples**:
   ```
   "Bohemian Rhapsody"              → "bohemian rhapsody"
   "Song Title (Remix)"             → "song title"
   "Track [2011 Remaster]"          → "track"
   "Song - Live"                    → "song"
   ```

4. **Duration Bucketing**:
   ```go
   durationBucket = bucketDuration(metadata.DurationMs)
   ```

   **Algorithm**:
   ```go
   // Round to nearest 3-second bucket
   durationSec := durationMs / 1000.0
   bucket := round(durationSec / 3.0) * 3.0
   ```

   **Purpose**: Group files with similar durations (±1.5 second tolerance)

   **Examples**:
   ```
   Duration (ms)    →  Bucket (seconds)
   ──────────────────────────────────────
   242000 (4:02)    →  243 (4:03)
   243500 (4:03.5)  →  243 (4:03)
   245000 (4:05)    →  246 (4:06)
   ```

   This allows slight duration differences (different rips, fade-outs) to cluster together.

5. **Generate Cluster Key**:
   ```go
   clusterKey = fmt.Sprintf("%s|%s|%s|%d", artistNorm, titleNorm, versionType, durationBucket)
   ```

   **Cluster Key Format**: `artist_norm|title_base|version_type|duration_bucket`

   **Examples**:
   ```
   "the beatles|hey jude|studio|423"
   "pink floyd|money|studio|382"
   "radiohead|creep|studio|237"
   "daft punk|get lucky|remix|248"
   "nirvana|smells like teen spirit|live|300"
   "eric clapton|layla|acoustic|247"
   ```

   **Why Include Version Type?**

   Different version types represent **distinct artistic works** and should NOT cluster together:
   - Studio recording vs. remix → different arrangements
   - Studio recording vs. live → different performances
   - Studio recording vs. acoustic → different instrumentation
   - Studio recording vs. demo → different quality/production

   However, **remastered versions** of the same studio recording SHOULD cluster:
   - `"Song Title"` → `studio`
   - `"Song Title (2011 Remaster)"` → `studio`
   - Both get same cluster key → correctly identified as duplicates

6. **Handle Missing Metadata**:

   If both artist AND title are empty:
   ```go
   // Use filename to prevent false clustering
   filename := filepath.Base(srcPath)          // "track01.mp3"
   filenameNoExt := removeExtension(filename)   // "track01"
   titleNorm = NormalizeTitle(filenameNoExt)   // "track01"
   artistNorm = "unknown"
   ```

   **Cluster Key**: `"unknown|track01|studio|245"`

   **Rationale**: Files without metadata should only cluster if they have identical filenames. This prevents grouping all untagged files together.

   **Version detection from filename**: If using filename fallback, version type is also detected from the filename (e.g., "track01 (Live).mp3" → `live`).

#### 2A.2 Clustering Process

**Step 1: Build Cluster Map** (in-memory):
```go
clusterMap := make(map[string][]*store.File)

for each file:
    clusterKey := GenerateClusterKey(metadata, srcPath)
    clusterMap[clusterKey] = append(clusterMap[clusterKey], file)
```

**Step 2: Write to Database**:
```sql
-- For each cluster
INSERT INTO clusters (cluster_key, hint) VALUES (?, ?);

-- For each file in cluster
INSERT INTO cluster_members (cluster_key, file_id, quality_score, preferred)
VALUES (?, ?, 0, 0);
```

**Cluster Hint** (for human readability):
```
"Pink Floyd - Money"
"The Beatles - Hey Jude"
```

#### 2A.3 Cluster Types

**Singleton Cluster** (1 file):
- Unique recording
- No duplicates found
- Will be copied to destination

**Duplicate Cluster** (2+ files):
- Multiple copies of the same recording
- Scoring phase will select winner
- Losers will be skipped

**Example**:
```
Cluster: "the beatles|hey jude|423"
  Members:
    - /music/old/The Beatles/Hey Jude.mp3 (128kbps MP3)
    - /music/new/Beatles/Hey Jude.flac (FLAC lossless)
    - /music/backup/hey_jude.m4a (256kbps AAC)
```

#### 2A.4 Resumable Clustering

**Progress Tracking** (Schema v3):
```sql
CREATE TABLE clustering_progress (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  last_processed_file_id INTEGER,
  total_files INTEGER,
  files_processed INTEGER,
  clusters_created INTEGER,
  started_at DATETIME,
  updated_at DATETIME
);
```

**Progress Saved**:
- Every 1000 files processed
- On Ctrl+C / SIGINT
- On crash / kill

**Resume Behavior**:
```go
if progress exists && !--force-recluster:
    // Resume from last checkpoint
    skip files where file.ID <= progress.last_processed_file_id
    rebuild clusterMap from existing clusters
else:
    // Start fresh
    ClearClusters()
    ClearClusteringProgress()
```

**Progress Output**:
```
Clustering | 125432/192880 grouped (65.0%) | 2.1 files/s
Writing clusters | 45234/89234 written (50.7%) | 3.5 clusters/s | 12483 duplicates
```

---

### Phase 2B: Quality Scoring

**Code**: `internal/score/scorer.go`

**Purpose**: Calculate quality scores for each file and select the best version.

#### 2B.1 Scoring Algorithm

**For each file** in each cluster:

```go
score := CalculateQualityScore(metadata, file)
```

**Scoring Components**:

##### 1. Codec Tier Score (40 points max)

**Lossless Codecs** (highest tier):
```
FLAC          → 40 points
ALAC (M4A)    → 40 points
WAV/AIFF PCM  → 40 points
APE           → 35 points
WavPack (.wv) → 35 points
TTA           → 30 points
```

**Lossy Codecs** (bitrate-dependent):

*AAC*:
```
≥ 256 kbps → 25 points
≥ 192 kbps → 22 points
≥ 128 kbps → 18 points
< 128 kbps → 15 points
```

*MP3*:
```
≥ 320 kbps → 20 points (CBR 320 or V0 VBR)
≥ 256 kbps → 18 points (V0 average)
≥ 192 kbps → 15 points (V2 VBR)
≥ 128 kbps → 12 points
< 128 kbps → 8 points
```

*Opus* (modern, efficient):
```
≥ 192 kbps → 24 points
≥ 128 kbps → 22 points
≥ 96 kbps  → 18 points
< 96 kbps  → 15 points
```

*Vorbis* (.ogg):
```
≥ 256 kbps → 22 points
≥ 192 kbps → 19 points
≥ 128 kbps → 16 points
< 128 kbps → 12 points
```

##### 2. Bit Depth Score (5 points max)
```
≥ 24-bit → +5 points
≥ 20-bit → +3 points
= 16-bit → 0 points (baseline)
< 16-bit → -2 points (penalty)
```

##### 3. Sample Rate Score (5 points max)
```
≥ 96kHz  → +5 points (hi-res: 96/192kHz)
≥ 48kHz  → +2 points (48kHz)
= 44.1kHz → 0 points (CD quality baseline)
≥ 32kHz  → -1 point
< 32kHz  → -3 points (penalty)
```

##### 4. Lossless Verification (+10 points)
```
if metadata.Lossless == true:
    score += 10
```

##### 5. Tag Completeness (5 points max)
```
Has Artist       → +1 point
Has Album        → +1 point
Has Title        → +1 point
Has Track Number → +1 point
All 4 present    → +1 bonus point
```

##### 6. File Size Bonus (lossless only, 2 points max)
```
if lossless:
    if fileSize > 50 MB → +2 points
    else if fileSize > 20 MB → +1 point
```

**Rationale**: Larger lossless files indicate less compression, higher quality.

#### 2B.2 Example Scores

**Scenario**: Three versions of "Hey Jude"

| File | Codec | Bitrate | Bit Depth | Sample Rate | Tags | Size | **Total Score** |
|------|-------|---------|-----------|-------------|------|------|----------------|
| `hey_jude.flac` | FLAC | N/A | 24-bit | 96kHz | Complete | 75 MB | **40 + 5 + 5 + 10 + 5 + 2 = 67** |
| `hey_jude.mp3` | MP3 | 320 kbps | 16-bit | 44.1kHz | Complete | 12 MB | **20 + 0 + 0 + 0 + 5 + 0 = 25** |
| `hey_jude.m4a` | AAC | 256 kbps | 16-bit | 44.1kHz | Partial | 8 MB | **25 + 0 + 0 + 0 + 3 + 0 = 28** |

**Winner**: `hey_jude.flac` (score: 67)

#### 2B.3 Winner Selection

**After scoring all files in a cluster**:

```go
winner := selectWinner(scoredMembers)
```

**Selection Algorithm** (tie-breakers):

1. **Highest Score** (primary criterion)
2. **Largest File Size** (if scores equal)
3. **Oldest mtime** (if size equal - prefer original rip)
4. **Lexical Path Order** (if mtime equal - deterministic)

**Example Tie-Breaker**:
```
File A: score=30, size=10MB, mtime=2020-01-01, path=/music/a/song.mp3
File B: score=30, size=10MB, mtime=2019-01-01, path=/music/b/song.mp3

Winner: File B (older mtime)
```

**Database Updates**:
```sql
-- Update all scores
UPDATE cluster_members SET quality_score = ? WHERE cluster_key = ? AND file_id = ?;

-- Mark winner
UPDATE cluster_members SET preferred = 1 WHERE cluster_key = ? AND file_id = ?;
```

#### 2B.4 Progress Output
```
Scoring: 45234/89234 clusters (50.7%) - 125678 files scored, 45234 winners selected
```

---

### Phase 2C: Destination Planning

**Code**: `internal/plan/planner.go`

**Purpose**: Determine destination paths for winner files and mark duplicates for skipping.

#### 2C.1 Path Generation

**For each cluster winner**:

```go
destPath := GenerateDestPath(destRoot, metadata, srcPath, isCompilation)
```

**Path Template**:
```
{dest_root}/{artist}/{album}/{disc}-{track} {title}.{ext}
```

**Field Processing**:

1. **Artist** (for directory):
   ```go
   artist := metadata.TagAlbumArtist  // Prefer AlbumArtist
   if artist == "":
       artist = metadata.TagArtist     // Fallback to Artist
   if artist == "":
       artist = "Unknown Artist"       // Last resort

   artist = SanitizeFilename(artist)   // Remove illegal characters
   ```

2. **Album** (for directory):
   ```go
   album := metadata.TagAlbum
   if album == "":
       album = "Unknown Album"

   album = SanitizeFilename(album)
   ```

3. **Compilation Handling**:
   ```go
   if metadata.TagCompilation && isRealCompilation():
       artist = "Compilations"  // Group all compilations together
   ```

   **Real Compilation Check**:
   - Must have `compilation` tag set
   - Must have ≥2 different artists in same album
   - Prevents single-artist albums mismarked as compilations

4. **Disc Number** (multi-disc sets):
   ```go
   if metadata.TagDiscTotal > 1:
       prefix = fmt.Sprintf("%d-", metadata.TagDisc)  // "2-"
   else:
       prefix = ""
   ```

5. **Track Number**:
   ```go
   trackNum := fmt.Sprintf("%02d", metadata.TagTrack)  // "01", "02", ..., "12"
   ```

6. **Title**:
   ```go
   title := metadata.TagTitle
   if title == "":
       // Use filename without extension
       title = filenameWithoutExt(srcPath)

   title = SanitizeFilename(title)
   ```

7. **Extension**:
   ```go
   ext := filepath.Ext(srcPath)  // Preserve original extension
   ```

**Filename Sanitization** (`internal/meta/normalize.go`):
```go
// Replace illegal characters
/ → -
\ → -
: → -
* → (removed)
? → (removed)
" → '
< → (removed)
> → (removed)
| → -

// Remove control characters
// Collapse whitespace
// Trim leading/trailing spaces and dots
```

#### 2C.2 Example Path Generation

**Input Metadata**:
```json
{
  "tag_artist": "Pink Floyd",
  "tag_albumartist": "Pink Floyd",
  "tag_album": "The Dark Side of the Moon",
  "tag_title": "Money",
  "tag_track": 6,
  "tag_disc": 1,
  "tag_disc_total": 1
}
```

**Generated Path**:
```
/music/Pink Floyd/The Dark Side of the Moon/06 Money.flac
```

**Multi-Disc Example**:
```json
{
  "tag_albumartist": "The Beatles",
  "tag_album": "The Beatles (White Album)",
  "tag_title": "Revolution 9",
  "tag_track": 8,
  "tag_disc": 2,
  "tag_disc_total": 2
}
```

**Generated Path**:
```
/music/The Beatles/The Beatles (White Album)/2-08 Revolution 9.mp3
```

**Compilation Example**:
```json
{
  "tag_artist": "Queen",
  "tag_album": "Now That's What I Call Music! 50",
  "tag_title": "Bohemian Rhapsody",
  "tag_compilation": 1,
  "tag_track": 3
}
```

**Generated Path** (if real compilation):
```
/music/Compilations/Now That's What I Call Music! 50/03 Bohemian Rhapsody.mp3
```

#### 2C.3 Plan Creation

**For each cluster**:

1. **Winner Plan**:
   ```sql
   INSERT INTO plans (file_id, action, dest_path, reason)
   VALUES (?, 'copy', '/music/Artist/Album/Track.flac', 'winner (score: 67.0)');
   ```

2. **Loser Plans** (duplicates):
   ```sql
   INSERT INTO plans (file_id, action, dest_path, reason)
   VALUES (?, 'skip', '', 'duplicate (score: 25.0, winner: 12345)');
   ```

**Action Types**:
- `copy`: Copy file to destination (default, safe)
- `move`: Move file to destination (removes from source)
- `hardlink`: Create hardlink (same filesystem only)
- `symlink`: Create symlink
- `skip`: Duplicate, do not copy

#### 2C.4 Path Collision Resolution

**Problem**: Two different files may generate the same destination path.

**Example**:
```
Cluster 1: "artist|song|240" → /music/Artist/Album/01 Song.mp3
Cluster 2: "artist|song remix|240" → /music/Artist/Album/01 Song.mp3
```

**Detection**:
```go
// Group plans by destination path (normalized)
pathMap := map[normalizedPath][]*store.Plan

// Check for case-insensitive collisions (macOS, Windows)
caseSensitive := DetectFilesystemCaseSensitivity(destRoot)
if !caseSensitive:
    normalizedPath = strings.ToLower(destPath)
```

**Resolution**:
```go
if len(pathMap[normalizedPath]) > 1:
    // Collision detected!
    // Keep file with highest quality score
    // Mark others as "skip"

    winner := selectHighestScoredFile(plans)
    for other in plans:
        if other != winner:
            UPDATE plans SET action = 'skip', reason = 'path collision (lower quality)'
```

**Output**:
```
[WARN] Path collision detected: 2 files -> /music/Artist/Album/01 Song.mp3
[WARN] Resolved 1 path collisions (kept highest quality files)
```

#### 2C.5 Planning Result

**Database State**:
```sql
-- All files have plans
SELECT action, COUNT(*) FROM plans GROUP BY action;

action  | count
─────────────────
copy    | 150000  (winners)
skip    | 42880   (duplicates + collisions)
```

**Event Log**:
```json
{"event":"plan","file_key":"...","src_path":"...","dest_path":"/music/...", "action":"copy","reason":"winner (score: 67.0)"}
{"event":"plan","file_key":"...","src_path":"...","dest_path":"","action":"skip","reason":"duplicate (score: 25.0, winner: 12345)"}
```

---

## Phase 3: Execute

**Purpose**: Copy/move files to destination according to the plan.

**Entry Point**: `mlc execute --db library.db --verify hash`

**Code**: `internal/execute/executor.go`

### 3.1 Execution Process

**For each plan** where `action != 'skip'`:

1. **Read Plan**:
   ```sql
   SELECT file_id, action, dest_path FROM plans WHERE action != 'skip';
   ```

2. **Create Destination Directory**:
   ```go
   destDir := filepath.Dir(destPath)
   os.MkdirAll(destDir, 0755)
   ```

3. **Execute Action**:

   **COPY** (default):
   ```go
   // Open source
   src, _ := os.Open(srcPath)
   defer src.Close()

   // Create destination with .part suffix
   tmpPath := destPath + ".part"
   dst, _ := os.Create(tmpPath)

   // Copy with progress tracking
   bytesWritten, _ := io.Copy(dst, src)

   // Sync to disk
   dst.Sync()
   dst.Close()

   // Atomic rename
   os.Rename(tmpPath, destPath)

   // Preserve mtime
   os.Chtimes(destPath, mtime, mtime)
   ```

   **MOVE**:
   ```go
   // Try rename first (same filesystem)
   if os.Rename(srcPath, destPath) != nil:
       // Fallback: copy + delete
       copy(srcPath, destPath)
       os.Remove(srcPath)
   ```

   **HARDLINK**:
   ```go
   os.Link(srcPath, destPath)
   ```

   **SYMLINK**:
   ```go
   os.Symlink(srcPath, destPath)
   ```

4. **Verification** (if `--verify hash`):
   ```go
   // Calculate SHA1 of source
   srcHash := sha1File(srcPath)

   // Calculate SHA1 of destination
   dstHash := sha1File(destPath)

   // Compare
   if srcHash != dstHash:
       return error("hash mismatch")
   ```

5. **Record Execution**:
   ```sql
   INSERT INTO executions (file_id, started_at, completed_at, bytes_written, verify_ok)
   VALUES (?, ?, ?, ?, 1);
   ```

### 3.2 Error Handling

**Partial Copy**:
- `.part` suffix prevents incomplete files in destination
- If execution fails, `.part` file is cleaned up
- Execution can be retried

**Verification Failure**:
- Destination file is **not** deleted
- Error logged for investigation
- Can be re-executed with `--verify none` to skip verification

### 3.3 Progress Output
```
Executing | 12500/150000 copied (8.3%) | 15.2 MB/s | 0 errors
```

### 3.4 Resumability

**Execution is resumable**:
- Tracks completed files in `executions` table
- Skips files already executed successfully
- Can be interrupted and restarted safely

```sql
-- Only execute files not yet done
SELECT p.* FROM plans p
LEFT JOIN executions e ON p.file_id = e.file_id
WHERE p.action != 'skip' AND e.file_id IS NULL;
```

---

## Database State Transitions

### File Status Lifecycle

```
discovered  →  meta_ok  (scan success)
           →  meta_error (scan failure)

meta_ok → (clustered) → (scored) → (planned) → (executed)
```

### Tables and Relationships

```
files (1) ──── (1) metadata
  │
  ├── (N) cluster_members ──── (1) clusters
  │
  ├── (1) plans
  │
  └── (1) executions
```

### State Invariants

**After Scan**:
```sql
-- Every file has exactly one status
SELECT COUNT(*) FROM files WHERE status IN ('discovered', 'meta_ok', 'meta_error');

-- Every meta_ok file has metadata
SELECT COUNT(*) FROM files f
JOIN metadata m ON f.id = m.file_id
WHERE f.status = 'meta_ok';
```

**After Clustering**:
```sql
-- Every meta_ok file is in exactly one cluster
SELECT COUNT(*) FROM cluster_members;  -- Should equal meta_ok files
```

**After Scoring**:
```sql
-- Every cluster has exactly one winner
SELECT cluster_key, COUNT(*) FROM cluster_members
WHERE preferred = 1
GROUP BY cluster_key
HAVING COUNT(*) != 1;  -- Should return 0 rows
```

**After Planning**:
```sql
-- Every clustered file has a plan
SELECT COUNT(*) FROM plans;  -- Should equal clustered files
```

**After Execution**:
```sql
-- Every non-skip plan has execution record
SELECT COUNT(*) FROM plans p
JOIN executions e ON p.file_id = e.file_id
WHERE p.action != 'skip';
```

---

## Resumability & Idempotency

### Idempotent Operations

**All phases can be run multiple times safely**:

- **Scan**: Updates existing files, adds new ones, skips unchanged
- **Clustering**: Clears clusters before rebuilding (unless resuming)
- **Scoring**: Recalculates scores, updates database
- **Planning**: Clears plans before regenerating
- **Execution**: Skips already-executed files

### Resume Points

**Clustering** (v1.6.0+):
- Progress saved every 1000 files
- Resume with: `mlc plan --db library.db --dest /music`
- Force restart: `mlc plan --db library.db --dest /music --force-recluster`

**Execution**:
- Resume automatically: `mlc execute --db library.db`
- Force re-execute: Delete from `executions` table, re-run

### Crash Recovery

**During Scan**:
- Next scan updates changed files
- No data loss (database writes are atomic)

**During Clustering**:
- Resume from last checkpoint (1000-file granularity)
- Rebuilds cluster map from database

**During Execution**:
- `.part` files cleaned up
- Re-run execution to complete

---

## Performance Characteristics

### Scan Phase
- **Speed**: 10-100 files/sec (I/O bound, ffprobe overhead)
- **Bottleneck**: Metadata extraction (ffprobe subprocess)
- **Optimization**: Worker pool (default 4 workers)

### Clustering Phase
- **Speed**: 1-5 files/sec (database writes)
- **Bottleneck**: SQLite inserts (192k files ≈ 10-15 hours)
- **Optimization**: Batch inserts, WAL mode, progress checkpoints

### Scoring Phase
- **Speed**: 20-50 clusters/sec (fast, in-memory calculations)
- **Bottleneck**: Database reads
- **Duration**: Minutes for 100k files

### Planning Phase
- **Speed**: 50-100 clusters/sec
- **Bottleneck**: Path collision detection
- **Duration**: Minutes for 100k files

### Execution Phase
- **Speed**: 10-50 MB/sec (network/disk I/O bound)
- **Bottleneck**: Network for NAS, disk for local
- **Optimization**: Concurrent workers (default 4)

---

## Summary

MLC's process is:

1. **Scan**: Discover files, extract metadata → `files` + `metadata` tables
2. **Cluster**: Group duplicates by normalized artist/title/duration → `clusters` + `cluster_members` tables
3. **Score**: Calculate quality scores, select winners → `quality_score` + `preferred` fields
4. **Plan**: Generate destination paths, mark duplicates → `plans` table
5. **Execute**: Copy winners to destination → `executions` table

Each phase is deterministic, resumable, and fully audited via event logs.
