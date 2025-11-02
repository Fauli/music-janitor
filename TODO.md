# TODO.md — Music Library Cleaner (mlc)

**Status tracking for MVP development**

Legend: `[ ]` Not started · `[~]` In progress · `[x]` Done · `[—]` Blocked/Deferred

---

## Current Focus

> **Active Milestone:** M0 — Project Setup & Foundation

---

## M0 — Project Setup & Foundation

**Goal:** Establish project structure, dependencies, and development tooling

### Project Structure
- [ ] Initialize Go module (`go.mod`)
- [ ] Create package structure (`cmd/mlc`, `internal/*`)
- [ ] Set up basic CLI skeleton with Cobra
- [ ] Add `.gitignore` (artifacts/, *.db, vendor/)
- [ ] Create `configs/example.yaml` template

### Dependencies
- [ ] Add core deps: Cobra, Viper, SQLite driver (`modernc.org/sqlite`)
- [ ] Add tag parsing: `github.com/dhowden/tag`
- [ ] Add utilities: logging, error types
- [ ] Verify `ffprobe` availability check utility

### Development Infrastructure
- [ ] Set up `go test ./...` baseline
- [ ] Create `Makefile` with build/test/clean targets
- [ ] Add GitHub Actions or CI config (optional for local dev)
- [ ] Document quick start in README.md

### Database Foundation
- [ ] Design initial migration system (`internal/store/migrations`)
- [ ] Implement schema v1 (files, metadata, clusters, plans, executions)
- [ ] Add basic store helpers (open/close, transaction wrappers)
- [ ] Write migration tests

---

## M1 — Scanner + Metadata Extraction

**Goal:** Discover files, extract tags & audio properties, persist to DB

### Scanner (`internal/scan`)
- [ ] Implement directory walker with bounded concurrency
- [ ] Add file filters (audio extensions: `.mp3`, `.flac`, `.m4a`, `.ogg`, `.opus`, `.wav`, `.aiff`)
- [ ] Generate stable `file_key` from `(dev, inode, size, mtime)`
- [ ] Insert discovered files into `files` table with `status=discovered`
- [ ] Handle symlinks and permission errors gracefully
- [ ] Write unit tests for path filtering
- [ ] Integration test: scan sample fixture tree

### Metadata Extraction (`internal/meta`)
- [ ] Implement tag reader using `dhowden/tag`
- [ ] Implement `ffprobe` JSON parser as fallback
- [ ] Extract: format, codec, duration, sample rate, bit depth, channels, bitrate, lossless flag
- [ ] Extract tags: artist, album, title, albumartist, date, disc, track, compilation, MusicBrainz IDs
- [ ] Store raw tags as JSON blob
- [ ] Update `files.status=meta_ok` or `error` on completion
- [ ] Add normalization helpers (Unicode NFC, trim, collapse whitespace)
- [ ] Write tests for each format (MP3/ID3v2, FLAC/Vorbis, M4A/AAC, OGG)
- [ ] Integration test: parse diverse sample files

### Filename & Folder Heuristics
- [ ] Parse common filename patterns: `NN - Title`, `Artist - Title`, `NN.Title`
- [ ] Infer disc/track numbers from filenames when tags missing
- [ ] Use parent folder names as Album/Artist hints
- [ ] Write regex tests with edge cases

### CLI Commands
- [ ] `mlc scan --src <path> --db <state.db>`
- [ ] Progress output: files discovered, metadata extracted, errors
- [ ] Resume support: skip already-processed files by `file_key`

### Testing & Validation
- [ ] Golden fixtures for MP3, FLAC, M4A, OGG (with/without tags)
- [ ] Test corrupt file handling (zero duration, unsupported container)
- [ ] Verify idempotency: re-scan same tree produces same `file_key`s

---

## M2 — Clustering & Scoring

**Goal:** Group files into duplicate clusters, score quality, choose winners

### Clustering (`internal/cluster`)
- [ ] Implement cluster key generation: `(artist_norm, title_norm, duration_bucket)`
- [ ] Duration bucketing logic (±1.5s tolerance)
- [ ] Normalize artist/title (lowercase, trim, collapse spaces, remove common stopwords)
- [ ] Insert clusters into `clusters` and `cluster_members` tables
- [ ] Handle singleton clusters (unique files)
- [ ] Write tests for normalization edge cases (unicode, punctuation, "The" prefix)

### Quality Scoring (`internal/score`)
- [ ] Implement codec tier scoring (FLAC/ALAC +40, AAC VBR +25, MP3 V0/320 +18-20, etc.)
- [ ] Bit depth & sample rate bonuses (16/44.1k baseline, 24/96k bonus)
- [ ] Lossless verification bonus (+10)
- [ ] Duration proximity scoring (±1.5s → +6, penalty for larger deltas)
- [ ] Tag completeness bonus (+4 if artist/album/title/track present)
- [ ] ReplayGain presence bonus (+1)
- [ ] Implement tie-breakers: file size, mtime, lexical path order
- [ ] Store quality scores in `cluster_members.quality_score`
- [ ] Mark winner with `preferred=1`
- [ ] Write unit tests for score calculation with known inputs
- [ ] Property test: scores are deterministic and monotonic

### Planning (`internal/plan`)
- [ ] Build action plan for each file (copy/move/link/skip)
- [ ] Winners → `action=copy|move` with `dest_path`
- [ ] Losers → `action=skip` with `reason="duplicate (lower score)"`
- [ ] Handle edge case: all cluster members have same score (use tie-breaker)
- [ ] Insert into `plans` table
- [ ] Write tests for plan generation logic

### CLI Commands
- [ ] `mlc plan --dest <path> [--mode copy|move] [--layout default] [--dry-run]`
- [ ] Dry-run mode: show plan summary without execution
- [ ] Output: clusters found, winners selected, duplicates skipped

### Testing & Validation
- [ ] Fixture with intentional duplicates (FLAC + MP3 320 + MP3 128 of same track)
- [ ] Verify highest quality wins
- [ ] Test artist/title normalization ("The Beatles" vs "Beatles, The")
- [ ] Test duration clustering (same song at 3:42 and 3:43)

---

## M3 — Executor (Safe Copy/Move)

**Goal:** Safely copy/move files to destination with verification

### Destination Layout (`internal/layout`)
- [ ] Implement path template: `{AlbumArtist}/{YYYY - Album}/Disc {DD}/{NN} - {Title}.{ext}`
- [ ] AlbumArtist fallback to Artist
- [ ] Various Artists handling (Compilation=1 → `Various Artists/`)
- [ ] Singles/no-album → `Artist/_Singles/` or `Artist/_Misc/`
- [ ] Multi-disc folder creation (`Disc 01`, `Disc 02`)
- [ ] Track number zero-padding (01-99 → 2 digits, 100+ → 3 digits)
- [ ] Character sanitization: strip `/\:*?"<>|`, normalize unicode NFC
- [ ] Collision handling: append ` (2)`, ` (3)` if file exists with different content
- [ ] Write pure function tests for path generation
- [ ] Property test: no path traversal, no illegal chars in output

### Execution Engine (`internal/execute`)
- [ ] Implement atomic copy: write to `.mlc_tmp/<file_key>.part`, then `rename()`
- [ ] Support modes: copy, move (with `--allow-move` safety flag)
- [ ] Optional: hardlink/symlink modes (same filesystem only)
- [ ] Size verification after copy
- [ ] Optional: content hash verification (SHA1)
- [ ] Update `executions` table with timing, bytes written, verify status
- [ ] Update `files.status=executed` on success
- [ ] Handle write errors gracefully (disk full, permissions)
- [ ] Implement worker pool with bounded concurrency (`--concurrency=N`)
- [ ] Write unit tests for atomic write helper
- [ ] Integration test: copy sample files, verify integrity

### Resumability
- [ ] On resume, skip files with `executions.verify_ok=1`
- [ ] Recover orphaned `.part` files (delete or resume)
- [ ] Handle partial executions (mid-copy crash)

### Conflict Resolution
- [ ] If `dest_path` exists with same hash → mark as `verify_ok=1`, skip copy
- [ ] If `dest_path` exists with different hash:
  - Default: suffix new file ` (2)`
  - `--prefer-existing`: skip copy, log conflict
  - `--quarantine`: move conflict to `_conflicts/` folder
- [ ] Log conflicts to JSONL and summary

### CLI Commands
- [ ] `mlc execute [--verify hash|size] [--concurrency 8] [--mode copy|move]`
- [ ] `mlc resume` (alias for execute with resume logic)
- [ ] Progress output: files copied, bytes written, conflicts, errors

### Testing & Validation
- [ ] Integration test: plan + execute on sample tree
- [ ] Chaos test: SIGKILL during copy → verify no partial files, resume works
- [ ] Test conflict scenarios (existing dest file with different content)
- [ ] Verify `move` mode deletes source only after successful verify

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
- [ ] Validate `configs/example.yaml` with all options documented
- [ ] Support env var overrides (`MLC_SOURCE`, `MLC_DEST`, etc.)
- [ ] Add `--config` flag to all commands
- [ ] Implement alias map for artist normalization
- [ ] Add quality weight overrides in config
- [ ] Add min bitrate thresholds (`min_aac_bitrate_kbps`, `min_mp3_bitrate_kbps`)
- [ ] Write config validation tests

### User Experience
- [ ] Add progress bars for long operations (scan, meta, execute)
- [ ] Improve error messages (actionable, include context)
- [ ] Add `mlc doctor` command: check `ffprobe`, `fpcalc`, disk space, permissions
- [ ] Add `--verbose` and `--quiet` flags for log levels
- [ ] Color-coded output for terminal (success/warning/error)

### Documentation
- [ ] Write `README.md` with quick start, installation, basic usage
- [ ] Document CLI commands and flags
- [ ] Add troubleshooting section (common errors, FAQ)
- [ ] Provide example workflows (dry-run → review → execute)
- [ ] Link to PLAN.md and ARCHITECTURE.md for advanced users

### Edge Cases & Hardening
- [ ] Handle very long filenames (truncate intelligently)
- [ ] Handle case-insensitive filesystems (macOS, Windows)
- [ ] Guard against path traversal in `dest_path` generation
- [ ] Add warning for cross-filesystem moves (suggest copy instead)
- [ ] Test with files >4GB (large FLAC/ALAC)
- [ ] Test with unicode filenames (emoji, CJK, accents)

### Performance Tuning
- [ ] Benchmark scan + meta extraction on 10k files
- [ ] Optimize DB queries (add indexes if missing)
- [ ] Profile memory usage and optimize allocations
- [ ] Test concurrency scaling (1, 4, 8, 16 workers)

### Testing & Quality
- [ ] Achieve >80% test coverage
- [ ] Add integration test suite (end-to-end happy path)
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

- [ ] Unit tests pass (`go test ./...`)
- [ ] Integration tests pass (happy path: scan → plan → execute → report)
- [ ] Chaos tests pass (SIGKILL mid-copy, resume works)
- [ ] Golden file tests for parsers (all supported formats)
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

## Notes & Decisions

**Key decisions made:**

- Default mode is `copy` (safest; `move` requires explicit flag)
- Default hashing is SHA1 on winners only (performance vs. safety balance)
- Fingerprinting is optional (fpcalc not required for MVP)
- SQLite with `modernc.org/sqlite` (no CGO by default)
- Dry-run generates full plan before any execution
- Duplicates are skipped (not deleted) by default
- Resumability is a first-class requirement (checkpoint after every stage)

**Open questions:**

- Should we auto-detect compilation albums by analyzing all files in a folder?
- How to handle remixes/live versions in clustering? (Current: include in title normalization)
- Should `move` mode be allowed by default in v1.0? (Current: require explicit flag)

---

**Last Updated:** 2025-11-02
