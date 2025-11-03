# TODO.md — Music Library Cleaner (mlc)

**Status tracking for MVP development**

Legend: `[ ]` Not started · `[~]` In progress · `[x]` Done · `[—]` Blocked/Deferred

---

## Current Focus

> **Active Milestone:** M4 — Reporting & Observability

---

## M0 — Project Setup & Foundation ✅ **COMPLETE**

**Goal:** Establish project structure, dependencies, and development tooling

### Project Structure
- [x] Initialize Go module (`go.mod`)
- [x] Create package structure (`cmd/mlc`, `internal/*`)
- [x] Set up basic CLI skeleton with Cobra
- [x] Add `.gitignore` (artifacts/, *.db, vendor/)
- [x] Create `configs/example.yaml` template

### Dependencies
- [x] Add core deps: Cobra, Viper, SQLite driver (`modernc.org/sqlite`)
- [x] Add tag parsing: `github.com/dhowden/tag`
- [x] Add utilities: logging, error types
- [x] Verify `ffprobe` availability check utility

### Development Infrastructure
- [x] Set up `go test ./...` baseline
- [x] Create `Makefile` with build/test/clean targets
- [—] Add GitHub Actions or CI config (optional for local dev)
- [x] Document quick start in README.md

### Database Foundation
- [x] Design initial migration system (`internal/store/migrations`)
- [x] Implement schema v1 (files, metadata, clusters, plans, executions)
- [x] Add basic store helpers (open/close, transaction wrappers)
- [x] Write migration tests

---

## M1 — Scanner + Metadata Extraction ✅ **COMPLETE**

**Goal:** Discover files, extract tags & audio properties, persist to DB

### Scanner (`internal/scan`)
- [x] Implement directory walker with bounded concurrency
- [x] Add file filters (audio extensions: `.mp3`, `.flac`, `.m4a`, `.ogg`, `.opus`, `.wav`, `.aiff`)
- [x] Generate stable `file_key` from `(dev, inode, size, mtime)`
- [x] Insert discovered files into `files` table with `status=discovered`
- [x] Handle symlinks and permission errors gracefully
- [x] Write unit tests for path filtering
- [x] Integration test: scan sample fixture tree
- [x] Add progress indicators for file discovery

### Metadata Extraction (`internal/meta`)
- [x] Implement tag reader using `dhowden/tag`
- [x] Implement `ffprobe` JSON parser as fallback
- [x] Extract: format, codec, duration, sample rate, bit depth, channels, bitrate, lossless flag
- [x] Extract tags: artist, album, title, albumartist, date, disc, track, compilation, MusicBrainz IDs
- [x] Store raw tags as JSON blob
- [x] Update `files.status=meta_ok` or `error` on completion
- [x] Add normalization helpers (Unicode NFC, trim, collapse whitespace)
- [x] Write tests for each format (MP3/ID3v2, FLAC/Vorbis, M4A/AAC, OGG)
- [x] Integration test: parse diverse sample files
- [x] Fix AIFF bit depth parsing bug (IntOrString custom type)
- [x] Add progress indicators for metadata extraction

### Filename & Folder Heuristics
- [x] Parse common filename patterns: `NN - Title`, `Artist - Title`, `NN.Title`
- [x] Infer disc/track numbers from filenames when tags missing
- [x] Use parent folder names as Album/Artist hints
- [x] Write regex tests with edge cases

### CLI Commands
- [x] `mlc scan --src <path> --db <state.db>`
- [x] Progress output: files discovered, metadata extracted, errors
- [x] Resume support: skip already-processed files by `file_key`
- [x] Add command-line flag overrides (--source, --dest, --mode, etc.)

### Testing & Validation
- [x] Golden fixtures for MP3, FLAC, M4A, OGG (with/without tags)
- [x] Test corrupt file handling (zero duration, unsupported container)
- [x] Verify idempotency: re-scan same tree produces same `file_key`s
- [x] Generate test fixtures with ffmpeg (25+ files in testdata/)
- [x] Integration tests for all audio formats including WAV, AIFF, Opus

---

## M2 — Clustering & Scoring ✅ **COMPLETE**

**Goal:** Group files into duplicate clusters, score quality, choose winners

### Clustering (`internal/cluster`)
- [x] Implement cluster key generation: `(artist_norm, title_norm, duration_bucket)`
- [x] Duration bucketing logic (±1.5s tolerance using 3-second buckets)
- [x] Normalize artist/title (lowercase, trim, collapse spaces, remove common stopwords)
- [x] Insert clusters into `clusters` and `cluster_members` tables
- [x] Handle singleton clusters (unique files)
- [x] Write tests for normalization edge cases (unicode, punctuation, "The" prefix)
- [x] Add progress indicators for clustering

### Quality Scoring (`internal/score`)
- [x] Implement codec tier scoring (FLAC/ALAC +40, AAC VBR +25, MP3 V0/320 +18-20, etc.)
- [x] Bit depth & sample rate bonuses (16/44.1k baseline, 24/96k bonus)
- [x] Lossless verification bonus (+10)
- [x] Duration proximity scoring (±1.5s → +6, penalty for larger deltas)
- [x] Tag completeness bonus (+5 if artist/album/title/track present)
- [—] ReplayGain presence bonus (+1) - deferred
- [x] Implement tie-breakers: file size, mtime, lexical path order
- [x] Store quality scores in `cluster_members.quality_score`
- [x] Mark winner with `preferred=1`
- [x] Write unit tests for score calculation with known inputs
- [x] Add progress indicators for scoring

### Planning (`internal/plan`)
- [x] Build action plan for each file (copy/move/link/skip)
- [x] Winners → `action=copy|move` with `dest_path`
- [x] Losers → `action=skip` with `reason="duplicate (lower score)"`
- [x] Handle edge case: all cluster members have same score (use tie-breaker)
- [x] Insert into `plans` table
- [x] Write tests for plan generation logic
- [x] Implement destination path generation with year, multi-disc support
- [x] Path sanitization (illegal characters, length limits)
- [x] Add progress indicators for planning

### CLI Commands
- [x] `mlc plan --dest <path> [--mode copy|move] [--dry-run]`
- [x] Dry-run mode: show plan summary without execution
- [x] Output: clusters found, winners selected, duplicates skipped
- [x] Three-phase operation (cluster → score → plan)

### Testing & Validation
- [x] Test with intentional duplicates (tagged MP3s with same metadata)
- [x] Verify highest quality wins (FLAC > MP3 > AAC for same song)
- [x] Test artist/title normalization
- [x] Test duration clustering (±1.5s tolerance with 3s buckets)
- [x] Test destination path generation edge cases
- [x] Test path sanitization (illegal characters, unicode)

**Test Results:** All tests passing (cluster, score, plan packages)

---

## M3 — Executor (Safe Copy/Move) ✅ **COMPLETE**

**Goal:** Safely copy/move files to destination with verification

### Destination Layout (`internal/layout`)
- [ ] Implement path template: `{AlbumArtist}/{YYYY - Album}/Disc {DD}/{NN} - {Title}.{ext}`
  - [x] AlbumArtist fallback to Artist (implemented in planner)
  - [x] Singles/no-album → `Artist/_Singles/` (implemented in planner)
  - [x] Multi-disc folder creation (`Disc 01`, `Disc 02`) (implemented in planner)
  - [x] Track number zero-padding (01-99 → 2 digits, 100+ → 3 digits) (implemented in planner)
  - [x] Character sanitization: strip `/\:*?"<>|`, normalize unicode NFC (implemented in planner)
- [ ] Various Artists handling (Compilation=1 → `Various Artists/`)
- [ ] Collision handling: append ` (2)`, ` (3)` if file exists with different content
- [ ] Write pure function tests for path generation
- [ ] Property test: no path traversal, no illegal chars in output

### Execution Engine (`internal/execute`)
- [x] Implement atomic copy: write to `.part`, then `rename()`
- [x] Support modes: copy, move, hardlink, symlink
- [x] Size verification after copy
- [x] Content hash verification (SHA1)
- [x] Update `executions` table with timing, bytes written, verify status
- [x] Update `files.status=executed` on success
- [x] Handle write errors gracefully (disk full, permissions)
- [x] Implement worker pool with bounded concurrency (`--concurrency=N`)
- [x] Write unit tests for all operations
- [x] Integration test: copy sample files, verify integrity

### Resumability
- [x] On resume, skip files with `executions.verify_ok=1`
- [x] Handle partial executions (mid-copy crash)
- [ ] Recover orphaned `.part` files (delete or resume) - deferred

### Conflict Resolution
- [ ] If `dest_path` exists with same hash → mark as `verify_ok=1`, skip copy
- [ ] If `dest_path` exists with different hash:
  - Default: suffix new file ` (2)`
  - `--prefer-existing`: skip copy, log conflict
  - `--quarantine`: move conflict to `_conflicts/` folder
- [ ] Log conflicts to JSONL and summary

### CLI Commands
- [x] `mlc execute [--verify hash|size] [--concurrency N]`
- [x] Progress output: files copied, bytes written, errors
- [ ] `mlc resume` (alias for execute with resume logic) - works with execute

### Testing & Validation
- [x] Unit tests for all executor operations (copy, move, hardlink, symlink, verify)
- [x] Integration test: plan + execute on sample tree
- [ ] Chaos test: SIGKILL during copy → verify no partial files, resume works
- [ ] Test conflict scenarios (existing dest file with different content)
- [x] Verify `move` mode deletes source only after successful verify

---

## M4 — Reporting & Observability

**Goal:** Generate JSONL event logs and human-readable reports

### Event Logging (`internal/report`)
- [ ] Implement JSONL event emitter
- [ ] Event types: `scan`, `meta`, `plan`, `execute`, `skip`, `duplicate`, `conflict`, `error`
- [ ] Fields: `ts`, `level`, `event`, `file_key`, `src_path`, `dest_path`, `cluster_key`, `quality_score`, `action`, `reason`
- [ ] Write to `artifacts/events-YYYYMMDD-HHMMSS.jsonl`
- [ ] Emit events from each pipeline stage
- [ ] Write tests for JSONL serialization

### Summary Reports
- [ ] Generate Markdown report with:
  - Total files scanned, valid, errors
  - Duplicates found, winners kept, losers skipped
  - Bytes copied/moved, time elapsed
  - Top errors and warnings
  - Conflicts encountered
  - Duplicates table per cluster (show all candidates + scores + winner)
- [ ] Optional: HTML report with same content (styled)
- [ ] Write to `artifacts/reports/<timestamp>/summary.md`
- [ ] Include sample invocation and config used

### Dry-Run Preview
- [ ] `--dry-run` mode: generate plan, output preview table, write JSONL, do not execute
- [ ] Preview table: cluster_key, winner, score, losers (count), dest_path
- [ ] Save dry-run plan to `artifacts/plans/<timestamp>/plan.jsonl`

### CLI Commands
- [ ] `mlc report --out artifacts/reports/<timestamp>` (post-execution)
- [ ] Auto-generate report after `execute` completes

### Testing & Validation
- [ ] Integration test: full pipeline → verify JSONL structure
- [ ] Verify Markdown report parses and contains expected sections
- [ ] Test dry-run: no files written to destination

---

## M5 — Fingerprinting (Optional)

**Goal:** Use Chromaprint to enhance duplicate detection

### Chromaprint Integration (`internal/fingerprint`)
- [ ] Check for `fpcalc` binary availability
- [ ] Extract acoustic fingerprints for audio files
- [ ] Store fingerprints in DB (extend `metadata` or new `fingerprints` table)
- [ ] Compare fingerprints for cluster candidates (Hamming distance threshold)
- [ ] Enhance cluster key with fingerprint similarity
- [ ] Handle fingerprint extraction failures gracefully (fallback to duration clustering)
- [ ] Add `--fingerprinting on|off` config option
- [ ] Write tests with known duplicate/non-duplicate pairs

### CLI Integration
- [ ] Add `--fingerprinting` flag to `scan` and `plan` commands
- [ ] Show fingerprint match confidence in reports

### Performance
- [ ] Parallelize `fpcalc` calls (worker pool)
- [ ] Skip fingerprinting for already-clustered singletons (optimization)

**Note:** This milestone is optional for MVP. Defer if time-constrained.

---

## M6 — Polishing & Documentation

**Goal:** Finalize config, improve UX, write user docs

### Configuration & Flexibility
- [x] Validate `configs/example.yaml` with all options documented
- [x] Support env var overrides (`MLC_SOURCE`, `MLC_DEST`, etc.)
- [x] Add `--config` flag to all commands
- [ ] Implement alias map for artist normalization
- [ ] Add quality weight overrides in config
- [ ] Add min bitrate thresholds (`min_aac_bitrate_kbps`, `min_mp3_bitrate_kbps`)
- [ ] Write config validation tests

### User Experience
- [x] Add progress bars for long operations (scan, meta, execute)
- [ ] Improve error messages (actionable, include context)
- [ ] Add `mlc doctor` command: check `ffprobe`, `fpcalc`, disk space, permissions
- [x] Add `--verbose` and `--quiet` flags for log levels
- [x] Color-coded output for terminal (success/warning/error)

### Documentation
- [x] Write `README.md` with quick start, installation, basic usage
- [x] Document CLI commands and flags
- [ ] Add troubleshooting section (common errors, FAQ)
- [ ] Provide example workflows (dry-run → review → execute)
- [ ] Link to PLAN.md and ARCHITECTURE.md for advanced users

### Edge Cases & Hardening
- [x] Handle very long filenames (truncate intelligently) - 200 char limit
- [ ] Handle case-insensitive filesystems (macOS, Windows)
- [x] Guard against path traversal in `dest_path` generation
- [ ] Add warning for cross-filesystem moves (suggest copy instead)
- [ ] Test with files >4GB (large FLAC/ALAC)
- [x] Test with unicode filenames (emoji, CJK, accents) - unicode test in integration tests

### Performance Tuning
- [ ] Benchmark scan + meta extraction on 10k files
- [ ] Optimize DB queries (add indexes if missing)
- [ ] Profile memory usage and optimize allocations
- [ ] Test concurrency scaling (1, 4, 8, 16 workers)

### Testing & Quality
- [~] Achieve >80% test coverage (in progress - core packages covered)
- [x] Add integration test suite (end-to-end happy path)
- [ ] Add chaos/resilience tests (disk full, permission denied, SIGKILL)
- [ ] Run `go vet`, `golangci-lint` without errors
- [ ] Load test: 100k synthetic files

---

## Post-MVP Backlog (BACKLOG.md)

**Features deferred after MVP:**

### Enhancement Ideas
- [ ] Web UI for reviewing clusters and overriding winners
- [ ] TUI (Terminal UI) with interactive cluster review
- [ ] MusicBrainz / Discogs lookup and tag enrichment
- [ ] Artwork extraction and deduplication (`folder.jpg` per album)
- [ ] ReplayGain calculation and normalization
- [ ] CUE sheet parsing for multi-track FLAC files
- [ ] Tag editing and cleanup (remove junk, fix case, unify formats)
- [ ] Playlist migration (import .m3u, update paths to new dest)
- [ ] NAS optimization mode (SMB quirks, case-sensitivity guards, network retry)
- [ ] Incremental sync mode (update dest when source changes)
- [ ] Plugin system for custom metadata enrichers
- [ ] Spectral analysis for transcode detection (avoid upscaled lossy files)
- [ ] Metrics endpoint (Prometheus/expvar) for monitoring
- [ ] Docker image for portable execution
- [ ] Cross-platform GUI (Electron / Tauri)

### Non-Audio Extensions
- [ ] Support for video files (music videos, concerts)
- [ ] Podcast / audiobook handling (separate layout rules)
- [ ] Support for DSF/DFF (DSD formats)

---

## Testing Checklist (Cross-Cutting)

**Must pass before v1.0 release:**

- [x] Unit tests pass (`go test ./...`) - 21 tests across 6 packages
- [ ] Integration tests pass (happy path: scan → plan → execute → report)
- [ ] Chaos tests pass (SIGKILL mid-copy, resume works)
- [x] Golden file tests for parsers (all supported formats) - 25+ test fixtures
- [ ] Property tests for normalization and layout rules
- [ ] Load test: 100k files processed without crashes
- [ ] Manual smoke test on real messy music collection
- [ ] Verify no data loss (source files untouched in copy mode)
- [ ] Verify resume works after interruption at any stage
- [ ] Dry-run produces deterministic plan (same inputs → same outputs)

---

## Release Checklist

**Before tagging v1.0:**

- [ ] All M1-M4 tasks complete (M5-M6 optional polish)
- [ ] README.md complete with usage examples
- [ ] CHANGELOG.md created
- [ ] Version string in CLI (`mlc --version`)
- [ ] Build binaries for macOS (arm64/amd64) and Linux (amd64)
- [ ] Test binaries on fresh machines (no dev deps)
- [ ] Create GitHub release with binaries
- [ ] Tag release in git (`v1.0.0`)

---

## Progress Summary

**Completed Milestones:**
- ✅ **M0** - Project Setup & Foundation (Go module, CLI, DB, dependencies)
- ✅ **M1** - Scanner + Metadata Extraction (file discovery, tag parsing, ffprobe integration)
- ✅ **M2** - Clustering & Scoring (duplicate detection, quality scoring, planning)
- ✅ **M3** - Executor (atomic copy/move, verification, worker pool, resumability)

**In Progress:**
- 🔨 **M4** - Reporting & Observability - NEXT UP

**Remaining:**
- ⏳ **M5** - Fingerprinting (Optional)
- ⏳ **M6** - Polishing & Documentation

**Test Coverage:**
- 33 tests passing across 7 packages (cluster, execute, meta, plan, scan, score, store)
- Integration tests for all major audio formats
- Test fixtures: 25+ generated audio files (MP3, FLAC, M4A, OGG, Opus, WAV, AIFF)
- Executor tests: atomic copy, move, hardlink, symlink, verification, resumability

---

## Notes & Decisions

**Key decisions made:**

- Default mode is `copy` (safest; `move` requires explicit flag)
- Default hashing is SHA1 on winners only (performance vs. safety balance)
- Fingerprinting is optional (fpcalc not required for MVP)
- SQLite with `modernc.org/sqlite` (no CGO by default)
- Dry-run generates full plan before any execution
- Duplicates are skipped (not deleted) by default
- Resumability is a first-class requirement (checkpoint after every stage)
- Progress indicators report every 2 seconds during long operations
- Duration clustering uses 3-second buckets for ±1.5s tolerance
- Quality scoring favors lossless > high bitrate lossy > standard lossy
- Tie-breakers: score → file size → mtime → lexical path

**Implementation highlights:**

- Hybrid metadata extraction (tag library for tags + ffprobe for audio properties)
- Custom IntOrString JSON unmarshaling for ffprobe quirks (AIFF bit depth)
- Atomic counters with progress goroutines for thread-safe reporting
- Comprehensive path sanitization (illegal chars, length limits, unicode handling)
- Store layer with complete CRUD operations for all entities
- Atomic file operations: write to `.part` temp file, then atomic rename
- Worker pool pattern with bounded concurrency for parallel execution
- SHA1 hash verification with context-aware cancellation support
- Resumability via database tracking (skips verify_ok=1 executions)

**Open questions:**

- Should we auto-detect compilation albums by analyzing all files in a folder?
- How to handle remixes/live versions in clustering? (Current: include in title normalization)
- Should `move` mode be allowed by default in v1.0? (Current: require explicit flag)

---

**Last Updated:** 2025-11-03
