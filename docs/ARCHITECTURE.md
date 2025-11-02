# ARCHITECTURE.md — Music Library Cleaner (mlc)

This document explains the internal architecture of **mlc**, the CLI that ingests a messy music archive and produces a clean, deduplicated library. It complements `PLAN.md` and is implementation‑oriented for contributors.

---

## 1. High‑Level Overview

**Core idea:** A staged, resumable pipeline that scans, extracts metadata, clusters likely‑identical recordings, selects a canonical winner per cluster, plans a destination layout, and executes safe file operations — all recorded in a SQLite state store and JSONL event logs.

```
[Filesystem] → scan → meta → normalize → cluster → score → plan → execute → verify → report
                                      ↘────────────── state (SQLite) ─────────────↗
```

**Properties**

* Idempotent: every file gets a stable `file_key` and can be reprocessed safely.
* Crash‑safe: writes go to temp files then atomically rename; progress is checkpointed.
* Transparent: decisions (e.g., duplicate arbitration) are persisted and reportable.

---

## 2. Components & Packages

```
cmd/mlc                # Cobra CLI entrypoint
internal/scan          # parallel directory walker with filters
internal/meta          # tag & ffprobe extractors, normalization helpers
internal/score         # quality scoring & tie‑breakers
internal/cluster       # grouping files into candidate recordings
internal/layout        # destination path rules & sanitization
internal/plan          # builds action plan (copy/move/link/skip)
internal/execute       # safe file ops, hashing, verification
internal/report        # JSONL emitter + markdown/html summaries
internal/store         # SQLite schema, migrations, queries
internal/util          # io/fs utils, concurrency, error types, logging
```

Each stage reads from the store and appends/updates rows, allowing resume and partial re‑runs.

---

## 3. Data Flow & State Transitions

### 3.1 File lifecycle state machine

```
DISCOVERED → META_OK → PLANNED → EXECUTED
     │           │          │        └─► DONE (verify_ok=1)
     │           │          └─► SKIPPED (duplicate lower score / policy)
     │           └─► ERROR(meta)
     └─► ERROR(scan)
```

* `files.status` drives resume logic. Any `ERROR` remains resumable after fixing root cause.
* `plans` and `executions` are append‑once per file_id; updates are monotonic.

### 3.2 Sequence (happy path)

```
User → CLI(scan) → walk FS → files(row)
      → CLI(meta) → read tags/ffprobe → metadata(row)
      → CLI(plan) → cluster + score → clusters/members + plans(row)
      → CLI(execute) → copy/move → executions(row) + verify
      → CLI(report) → JSONL → Markdown/HTML summary
```

All steps can be combined under `mlc execute` with internal sub‑stages, but we maintain explicit commands for observability and dry‑runs.

---

## 4. Identifiers & Keys

* **file_key**: fast stable key `hash(dev,inode,size,mtime)`; upgraded to content hash when selected as a winner (configurable).
* **cluster_key**: normalized `(artist_norm,title_norm,duration_bucket)`; may include MusicBrainz IDs if present. Optionally extended by Chromaprint.

These keys keep the pipeline deterministic and aid conflict resolution.

---

## 5. SQLite Schema (essentials)

See `PLAN.md` for full DDL. Tables used most frequently:

* `files(id,file_key,src_path,size_bytes,mtime_unix,sha1,status,error,first_seen_at,last_update_at)`
* `metadata(file_id,format,codec,duration_ms,sample_rate,bit_depth,channels,bitrate_kbps,lossless,tag_*,raw_tags_json)`
* `clusters(cluster_key,hint)` and `cluster_members(cluster_key,file_id,quality_score,preferred)`
* `plans(file_id,action,dest_path,reason)`
* `executions(file_id,started_at,completed_at,bytes_written,verify_ok,error)`

Indexes: `(files.file_key UNIQUE)`, `(metadata.file_id PK)`, `(cluster_members.cluster_key)`, `(plans.dest_path)`.

---

## 6. Metadata Extraction Strategy

1. **Tags first**: `dhowden/tag` (MP3/ID3, M4A/AAC/ALAC, FLAC/Vorbis, OGG/Opus, WAV/AIFF where applicable).
2. **Fallback**: spawn `ffprobe -v quiet -show_format -show_streams -print_format json` and parse.
3. **Normalization**: Unicode NFC; trim; normalize separators; standardize case; coalesce AlbumArtist/Artist; parse `Disc/Track/Date`.
4. **Filename heuristics**: regex parse common patterns when tags are missing; use parent folders for Album/Artist context.

Failures store `error` on `files` and proceed.

---

## 7. Clustering & Scoring

### 7.1 Clustering

* Key = `(artist_norm, title_norm, duration_bucket)` where `duration_bucket = round(duration_seconds, 1.5)`.
* Secondary hints: same album, disc/track numbers, MusicBrainz IDs, identical fingerprints (if enabled).
* Conservative merge policy to avoid false positives; ambiguous files can form singleton clusters.

### 7.2 Quality Score (0–100)

* Container/codec tier (lossless > high‑quality lossy > legacy MP3, etc.)
* Bit depth / sample rate bonuses (bounded)
* Bitrate (lossy) versus thresholds
* Duration proximity to cluster median
* Tag completeness bonus
* Tie‑breakers: likely generation, file size, mtime, lexical path

Lower‑scored cluster members are **not deleted** by default; they become `SKIPPED` with a reason.

---

## 8. Destination Layout & Sanitization

Default path template:

```
{AlbumArtistOrArtist}/{YYYY - Album}/Disc {DD}/{NN} - {Title}{suffix}.{ext}
```

Rules:

* Choose AlbumArtist over Artist; Various Artists → special handling.
* Unknowns routed to `Unknown Artist/Unknown Album/…` with source basename suffix.
* Replace illegal chars, collapse whitespace, normalize Unicode; ensure case‑stable collisions handled (`name (2)` policy unless `--prefer-existing`).

`internal/layout` exposes a pure function `BuildPath(meta) (path, issues)` for deterministic results.

---

## 9. Execution Engine

* **Mode**: `copy|move|hardlink|symlink` (default copy).
* **Atomicity**: write into `dest/.mlc_tmp/<file_key>.part`, `fsync`, then `rename()`.
* **Verification**: size always; optional content hash; optional re‑probe.
* **Concurrency**: worker pool with back‑pressure; tune via `--concurrency`.
* **Conflicts**: if `dest_path` exists with different content, apply policy: keep existing (`--prefer-existing`), suffix new name, or quarantine.

Failures are recorded on `executions.error`; resume picks up remaining items safely.

---

## 10. Resumability & Idempotency

* State transitions are monotonic; re‑running any stage recomputes or fills missing rows.
* `scan` is additive; existing `file_key` prevents duplicates.
* `plan` can be recomputed — it deactivates previous winner flags before computing new ones.
* `execute` is idempotent: if a destination file already matches, it marks `verify_ok=1` without copying.

---

## 11. Configuration & CLI

* Config file (YAML) + flags; Viper merges env/flags/config.
* Key options: `source`, `destination`, `mode`, `layout`, `hashing`, `duplicates` policy, `fingerprinting`, `concurrency`, `min_*_bitrate_kbps`, weights.
* Subcommands: `scan`, `plan`, `execute`, `resume`, `report` (+ hidden `doctor` for environment checks like ffprobe presence).

---

## 12. Observability

* **Logs**: human logs (stderr) + structured JSONL event stream (`artifacts/events-YYYYMMDD.jsonl`).
* **Metrics (optional)**: Prometheus via expvar/http if `--metrics` enabled: processed files, bytes written, queue depths, error counts.
* **Reports**: Markdown/HTML roll‑up with stats, duplicates table, conflicts, top errors.

---

## 13. Error Handling & Policy

* Use sentinel error types (`ErrUnsupported`, `ErrCorrupt`, `ErrConflict`) for branch decisions.
* Fail‑open strategy: one bad file never blocks the batch.
* Retries for transient IO.
* Quarantine folder for unexpected conflicts when `--quarantine on`.

---

## 14. Performance Design

* Parallel scan using a bounded channel of discovered paths.
* Batched `ffprobe` invocations via a worker pool.
* Hash only winners by default; full‑hash verification optional.
* Avoid small writes; use large buffers and `sendfile`/`copy_file_range` where available.

Target: 100k files/day on modest NAS with concurrency 8–16.

---

## 15. Security & Safety

* Never delete sources by default; `move` only on explicit flag.
* Do not execute tag contents; treat all metadata as untrusted input.
* Path traversal guard: destination must remain inside `destination` root after sanitization.
* Symlink/hardlink policies controlled via flags; disabled across filesystems unless explicit.

---

## 16. Extensibility

* New format readers implement `meta.Reader` interface; registry pattern selects appropriate reader.
* Alternative layouts implement `layout.Builder` interface.
* New scoring rules added via weighted functions; unit tests ensure regressions don’t reshuffle winners unexpectedly.
* Future: plugin hooks (e.g., MusicBrainz enrichment) executed after `META_OK` and before `PLAN`.

---

## 17. Testing Strategy

* Unit tests per package (normalization, layout, scoring tie‑breakers).
* Golden file fixtures for parser correctness across formats.
* Integration test harness builds a synthetic messy tree → asserts deterministic plan and layout.
* Chaos tests: SIGKILL during `execute` → ensure no partial files except `.part` which are auto‑reclaimed.

---

## 18. Deployment & Build

* Pure Go build (no CGO) using `modernc.org/sqlite` by default; optional tag `cgo_sqlite` switches to `mattn/go-sqlite3`.
* Single static binary; distribute for macOS arm64 & Linux amd64.
* Runtime deps: `ffprobe` (required), `fpcalc` (optional).

---

## 19. Directory Layout (repo)

```
.
├── cmd/mlc
├── internal/{scan,meta,cluster,score,layout,plan,execute,report,store,util}
├── configs/example.yaml
├── artifacts/ (gitignored)
├── PLAN.md
└── ARCHITECTURE.md
```

---

## 20. Open Questions / Future Decisions

* Do we flip default `mode` to `move` after sufficient confidence? (Currently `copy`.)
* Enable Chromaprint by default once performance is acceptable?
* Add a minimal web UI for reviewing and overriding cluster winners before execute?

---

## 21. Quick Start for Developers

1. Install `ffmpeg` (for `ffprobe`).
2. `go run ./cmd/mlc --help`
3. Point to a small sample source and run: `mlc scan --src ~/Messy --db state.db && mlc plan --dest ~/Clean --dry-run`
4. Inspect `artifacts/` JSONL and the generated report; iterate on config weights.

---

