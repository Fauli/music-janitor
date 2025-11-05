# MLC - Frequently Asked Questions (FAQ)

## Table of Contents
- [Resumability & State Management](#resumability--state-management)
- [Workflow & Operations](#workflow--operations)
- [Deduplication & Clustering](#deduplication--clustering)
- [Safety & Data Protection](#safety--data-protection)
- [Performance & Optimization](#performance--optimization)
- [MusicBrainz Integration](#musicbrainz-integration)
- [Troubleshooting](#troubleshooting)

---

## Resumability & State Management

### Q1: My library is huge. How does MLC know where to continue scanning from?

**A:** MLC uses SQLite to track every file it processes, making scanning fully resumable.

**How it works:**
1. Each file gets a unique `file_key` based on: `path + size + mtime`
2. When scanning, MLC checks if each file already exists in the database
3. If the file exists with matching `mtime` (modification time) and `size`, it's skipped
4. Only new or modified files are processed

**Example scenario:**
```bash
# Scan 50,000 files - interrupted after 30,000
mlc scan --source /huge/library --db library.db
# [Interrupted at 30,000 files]

# Resume - skips first 30,000, processes remaining 20,000
mlc scan --source /huge/library --db library.db
# ✓ Scanned: 30,000 (cached)
# ✓ New: 20,000
```

**What's tracked:**
- File path, size, modification time
- Scan status: `pending`, `meta_ok`, `meta_error`
- Metadata extraction results
- SHA1 hash (if enabled)
- Errors encountered

**Key insight:** You can safely stop and resume scanning at any time. MLC never re-processes files unless they've been modified (different mtime/size).

---

### Q2: What happens when I create a plan, re-scan with more files added, and then create another plan?

**A:** The new plan incorporates ALL files (old + new), ensuring proper deduplication across your entire library.

**Step-by-step example:**

**Initial state:**
```bash
# Scan 1,000 files
mlc scan --source /music --db library.db
# Files in DB: 1,000

# Create plan
mlc plan --dest /clean --db library.db
# Plan created for 1,000 files
```

**Add new files:**
```bash
# Add 200 new files to /music
cp -r /new-music/* /music/

# Re-scan (only processes the 200 new files)
mlc scan --source /music --db library.db
# ✓ Cached: 1,000
# ✓ New: 200
# Files in DB: 1,200
```

**Create new plan:**
```bash
# Create new plan - clusters ALL 1,200 files
mlc plan --dest /clean --db library.db
# ✓ Clustering: 1,200 files
# ✓ Old plans cleared
# ✓ New plan generated
```

**What happens:**
1. **Old plans are cleared** - Previous execution plans are deleted
2. **Re-clustering** - All 1,200 files are clustered together (old + new)
3. **Deduplication recalculated** - New files are deduplicated against existing ones
4. **Winners re-selected** - Quality scores may change if better versions were added

**Important:** This ensures new files are properly deduplicated against your entire library, not just against each other.

---

### Q3: What happens in multiple executions with only a few additionally added tracks?

**A:** Each execution is resumable and idempotent. Only unprocessed files from the plan are executed.

**Workflow for adding new tracks:**

**Scenario 1: New tracks during execution**
```bash
# Initial execution
mlc execute --db library.db
# Copying 1,000 files... (interrupted at 600)

# Add new tracks to source
cp /new-music/*.mp3 /music/

# Resume execution - continues from file 601
mlc execute --db library.db
# ✓ Already executed: 600
# ✓ Remaining: 400
# Note: New tracks NOT included yet (not in plan)
```

**Scenario 2: New tracks between executions**
```bash
# Complete execution #1
mlc execute --db library.db
# ✓ Executed: 1,000 files

# Add new tracks
cp /new-music/*.mp3 /music/

# To process new tracks, must scan + plan + execute:
mlc scan --source /music --db library.db
# ✓ New: 50

mlc plan --dest /clean --db library.db
# ✓ Re-clustered: 1,050 files
# ✓ New plan created

mlc execute --db library.db
# ✓ Already executed: 1,000 (skipped)
# ✓ New files: 50
```

**Key points:**
- Execute only processes files in the **current plan**
- Already-executed files are **skipped** (status = `executed`)
- New tracks require: **scan → plan → execute**
- Execution is **idempotent** - running it multiple times is safe

**Execution states tracked:**
- `pending`: Not yet executed
- `executed`: Successfully copied/moved
- `skipped`: Duplicate or excluded from plan
- `error`: Failed to execute

---

### Q4: Is there a scan+plan+execute shortcut?

**A:** No, but this is intentional design for safety and control.

**Why separate commands?**

1. **Review before executing**
   - Plans can be inspected before making any changes
   - Dry-run mode lets you preview actions
   - Mistakes are caught early

2. **Flexibility**
   - Re-plan without re-scanning
   - Change destination layout and re-plan
   - Adjust settings between phases

3. **Safety**
   - No accidental data operations
   - Clear separation of read (scan/plan) vs. write (execute)
   - Audit trail in database

**Recommended workflow:**

```bash
# Step 1: Scan (read-only, safe)
mlc scan --source /messy --db library.db -v

# Step 2: Plan with dry-run (preview, safe)
mlc plan --dest /clean --db library.db --dry-run

# Review the plan:
# - Check artifacts/events-*.jsonl
# - Review duplicate decisions
# - Verify destination paths

# Step 3: Execute (writes files, use with care)
mlc execute --db library.db --verify hash

# Step 4: Report (optional)
mlc report --db library.db
```

**Quick workflow (after first time):**
```bash
# For regular updates with new files:
mlc scan --source /messy --db library.db && \
mlc plan --dest /clean --db library.db && \
mlc execute --db library.db --verify hash
```

**Shell alias (optional):**
```bash
# Add to ~/.bashrc or ~/.zshrc
alias mlc-all='mlc scan --source /messy --db library.db && \
               mlc plan --dest /clean --db library.db && \
               mlc execute --db library.db --verify hash'
```

**Why not a single command?**
- MLC is designed for **large libraries** where mistakes are expensive
- Separate phases allow **review and adjustment**
- Database state allows **full auditability**
- You can **stop at any point** and resume later

---

## Workflow & Operations

### Q: What's the typical workflow for a first-time user?

**A:** Follow these steps:

```bash
# 1. Check prerequisites
mlc doctor --src /messy --dest /clean

# 2. Scan source library
mlc scan --source /messy --db library.db -v

# 3. Plan with dry-run (preview)
mlc plan --dest /clean --db library.db --dry-run

# 4. Review the plan
cat artifacts/events-*.jsonl | grep -E "cluster|winner|skip"

# 5. Execute (real operation)
mlc execute --db library.db --verify hash

# 6. Generate report
mlc report --db library.db
```

---

### Q: Can I change the destination folder structure after scanning?

**A:** Yes! Scanning and planning are separate operations.

```bash
# Scan once
mlc scan --source /messy --db library.db

# Try different layouts without re-scanning:

# Layout 1: Default (Artist/Year - Album/Disc/Track)
mlc plan --dest /clean --db library.db --layout default --dry-run

# Layout 2: Alternative (Artist/Album (Year)/Track)
mlc plan --dest /clean --db library.db --layout alt1 --dry-run

# Layout 3: With genre folders
mlc plan --dest /clean --db library.db --layout alt2 --dry-run

# Pick the one you like, then execute
mlc execute --db library.db
```

---

### Q: How do I update my clean library with new files?

**A:** Run scan → plan → execute again. Already-copied files are skipped.

```bash
# Add new files to source
cp -r /new-music/* /messy/

# Re-scan (only processes new files)
mlc scan --source /messy --db library.db

# Re-plan (clusters all files, old + new)
mlc plan --dest /clean --db library.db

# Execute (only copies new files)
mlc execute --db library.db --verify hash
```

---

## Deduplication & Clustering

### Q: How does MLC decide which duplicate to keep?

**A:** Quality-based scoring system with multiple factors.

**Scoring criteria (in priority order):**

1. **Codec/Container** (0-40 points)
   - FLAC/ALAC (lossless): 40 points
   - AAC VBR: 35 points
   - MP3 V0/320: 30 points
   - MP3 < 320: 20-25 points

2. **Lossless bonus** (+10 points)
   - Verified lossless encoding

3. **Sample rate / Bit depth** (+0-12 points)
   - 96 kHz / 24-bit: +12 points
   - 48 kHz / 16-bit: +6 points

4. **Duration proximity** (+6 or penalty)
   - Matches cluster median duration

5. **Tag completeness** (+4 points)
   - Has artist, album, title, track number

6. **Tie-breakers:**
   - Larger file size
   - Newer modification time
   - Lexical path order

**Example:**
```
Track: "Hey Jude" by The Beatles

File A: FLAC, 96 kHz/24-bit, complete tags → Score: 62
File B: MP3 320 kbps, 44.1 kHz/16-bit, complete tags → Score: 40
File C: MP3 128 kbps, 44.1 kHz/16-bit, missing tags → Score: 24

Winner: File A (FLAC) ✓
Skipped: File B, File C
```

---

### Q: What if MLC picks the wrong duplicate?

**A:** In MVP, you can manually adjust the database or exclude files.

**Workarounds:**

**Option 1: Exclude unwanted files**
```bash
# Delete low-quality versions from source before scanning
rm /messy/**/*128kbps*.mp3
mlc scan --source /messy --db library.db
```

**Option 2: Manual database edit**
```sql
-- Mark preferred file as winner
UPDATE cluster_members
SET preferred = 1
WHERE file_id = 12345;

-- Unmark old winner
UPDATE cluster_members
SET preferred = 0
WHERE cluster_key = 'artist|title|duration'
  AND file_id != 12345;
```

**Option 3: Use quality thresholds (future feature)**
```yaml
# Future: configs/my-library.yaml
min_bitrate: 192  # Skip < 192 kbps
prefer_lossless: true
```

**Note:** A web UI for reviewing and overriding duplicate decisions is planned for post-MVP.

---

## Safety & Data Protection

### Q: Will MLC delete or modify my original files?

**A:** **No, never** (in default `copy` mode).

**Safe modes:**
- `copy` (default): Copies files, source untouched
- `hardlink`: Creates hardlinks, source untouched
- `symlink`: Creates symlinks, source untouched

**Destructive mode (requires explicit flag):**
- `move`: Deletes source after successful copy + verification
  - Requires `--allow-move` flag in config
  - Uses hash verification by default

**Protection mechanisms:**
1. Copy/hardlink/symlink are **read-only** on source
2. Atomic operations (write to `.part`, then rename)
3. Verification (size or hash) before considering success
4. Database tracks every operation
5. Dry-run mode for previewing

---

### Q: What happens if execution is interrupted (power loss, Ctrl-C)?

**A:** MLC is designed to be crash-safe and resumable.

**Interruption handling:**

1. **Partial writes** - Files being copied are written to `.part` extension
2. **Database tracking** - Only completed files marked as `executed`
3. **Resume** - Re-running `mlc execute` skips completed files

**Example:**
```bash
# Execution interrupted at file 500/1000
mlc execute --db library.db
# [Power loss at file 500]

# Check filesystem:
# - Files 1-499: Fully copied
# - File 500: Partial copy (*.part file)
# - Files 501-1000: Not started

# Resume execution
mlc execute --db library.db
# ✓ Skipping files 1-499 (already executed)
# ✓ Re-copying file 500 (partial .part file detected)
# ✓ Copying files 501-1000
```

**Safety guarantees:**
- No corruption of completed files
- Partial files cleaned up or reprocessed
- Database stays consistent (atomic updates)
- Source files never touched (in copy mode)

---

## Performance & Optimization

### Q: How long does it take to process a large library?

**A:** Depends on library size, hardware, and storage type.

**Rough estimates (8-core machine, SSD):**

| Library Size | Scan Time | Plan Time | Execute Time (Copy) |
|--------------|-----------|-----------|---------------------|
| 1,000 files  | 1-2 min   | 5 sec     | 2-5 min             |
| 10,000 files | 10-20 min | 30 sec    | 20-50 min           |
| 50,000 files | 1-2 hours | 2-3 min   | 2-4 hours           |
| 100,000+ files | 2-4 hours | 5-10 min  | 4-8 hours           |

**Performance factors:**
- **Storage type:** SSD > HDD > NAS
- **Concurrency:** More workers = faster (default: 8)
- **Hashing:** SHA1 adds ~30% time, XXH3 adds ~10%
- **Network:** NAS is 5-10x slower (but auto-optimized)

**Optimization tips:**
```bash
# Fast scan (no hashing)
mlc scan --source /messy --db library.db --hashing none

# Fast execute (size verification only)
mlc execute --db library.db --verify size --concurrency 16

# NAS optimization (auto-detected)
mlc scan --source /nas/music --db library.db
# NAS detected → auto-tunes concurrency, buffers, retries
```

---

### Q: Can I speed up scanning by disabling certain features?

**A:** Yes, several options available:

```bash
# Minimal scan (fastest)
mlc scan --source /messy --db library.db --hashing none --fingerprinting false

# Skip MusicBrainz (saves time if large library)
mlc plan --dest /clean --db library.db  # (no --musicbrainz flag)

# Higher concurrency (if you have many cores)
mlc scan --source /messy --db library.db --concurrency 16

# Skip metadata enrichment
mlc execute --db library.db --write-tags=false
```

---

## MusicBrainz Integration

### Q: Should I use MusicBrainz for my library?

**A:** Depends on your library's artist consistency.

**Use MusicBrainz if:**
- ✅ Artist names are inconsistent ("Beatles" vs "The Beatles")
- ✅ International artists with multiple spellings
- ✅ Large library (>1,000 files) with time for preload
- ✅ Internet connectivity available

**Skip MusicBrainz if:**
- ❌ Artist tags are already clean and consistent
- ❌ Small library (<100 files)
- ❌ No internet connection
- ❌ Want fastest possible clustering

**Recommended approach:**
```bash
# First time: Preload all artists (takes time, but only once)
mlc plan --dest /clean --db library.db --musicbrainz --musicbrainz-preload

# Future runs: Use cached data (instant)
mlc plan --dest /clean --db library.db --musicbrainz
```

---

### Q: How long does MusicBrainz preload take?

**A:** ~1 second per unique artist (API rate limit).

**Examples:**
- 100 unique artists: ~2 minutes
- 500 unique artists: ~8 minutes
- 1,000 unique artists: ~17 minutes

**After preload:**
- All future runs are **instant** (uses cached data)
- Works **offline** (cache stored in database)
- Never re-queries the same artist

---

## Troubleshooting

### Q: How do I check what went wrong?

**A:** MLC provides multiple debugging tools:

**1. Event logs (detailed JSON logs):**
```bash
# View all events
cat artifacts/events-*.jsonl

# Find errors
cat artifacts/events-*.jsonl | grep '"error"'

# Find skipped files
cat artifacts/events-*.jsonl | grep '"skip"'

# Find clustering decisions
cat artifacts/events-*.jsonl | grep '"cluster"'
```

**2. Database queries:**
```bash
# Open database
sqlite3 library.db

# Check file statuses
SELECT status, COUNT(*) FROM files GROUP BY status;

# Find errors
SELECT src_path, error FROM files WHERE status = 'meta_error';

# Check execution status
SELECT action, COUNT(*) FROM plans GROUP BY action;
```

**3. Verbose mode:**
```bash
# Run with verbose logging
mlc scan --source /messy --db library.db -v
mlc plan --dest /clean --db library.db -v
```

---

### Q: "No plans found" error when running execute

**A:** You need to run `mlc plan` before `mlc execute`.

```bash
# Correct order:
mlc scan --source /messy --db library.db
mlc plan --dest /clean --db library.db  # Required step!
mlc execute --db library.db
```

---

### Q: Files not being detected as duplicates

**A:** Check these common causes:

**1. Duration difference >1.5 seconds**
```bash
# Files must have similar duration
# Example: "Hey Jude" - 7:05 vs 7:08 = different clusters
```

**2. Different artist/title tags**
```bash
# Check metadata
sqlite3 library.db "SELECT src_path, tag_artist, tag_title FROM metadata WHERE tag_title LIKE '%Hey Jude%';"
```

**3. Missing metadata**
```bash
# Files without metadata fall back to filename clustering
# Enable verbose mode to see clustering decisions
mlc plan --dest /clean --db library.db -v
```

**Solutions:**
- Use `--musicbrainz` for artist normalization
- Fix metadata tags before scanning
- Enable `--fingerprinting` (requires fpcalc) for acoustic matching

---

### Q: How do I start fresh / reset the database?

**A:** Delete the database file and re-scan.

```bash
# Delete database
rm library.db

# Start fresh
mlc scan --source /messy --db library.db
mlc plan --dest /clean --db library.db
mlc execute --db library.db
```

**Note:** This is safe - source files are never modified.

---

## Additional Questions?

**Documentation:**
- README.md - Quick start guide
- PLAN.md - Feature specification
- ARCHITECTURE.md - Technical design
- RELEASE_NOTES_v1.4.0.md - Latest updates

**Support:**
- GitHub Issues: https://github.com/franz/music-janitor/issues
- Documentation: https://github.com/franz/music-janitor#readme

**Reporting Bugs:**
Include:
1. MLC version (`mlc --version`)
2. Command you ran
3. Error message
4. Event log excerpt (`artifacts/events-*.jsonl`)
5. Platform (macOS/Linux) and storage type (SSD/NAS)
