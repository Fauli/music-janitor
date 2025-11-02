# Music Library Cleaner — PLAN.md

## 1) Problem & Goals

**Goal:** Consolidate a historically grown music archive into a clean, deduplicated, and well‑structured library.

**High‑level tasks**

1. Ingest a *source* directory with arbitrary nested folders and many audio files.
2. Parse and normalize metadata (tags, filename hints, audio properties).
3. Decide canonical file organization and move/copy into a *destination* library.
4. Deduplicate intelligently (keep highest quality), and generate an auditable report.
5. Be resumable, idempotent, and safe for terabytes of data.

**Non‑goals (v1):** artwork editing, tag editing UI, streaming server, auto‑download of missing tags from the internet (optional add‑on later).

---

## 2) Proposed Tech Stack

* **Language:** Go 1.22+

  * Rationale: fast, static binary, great concurrency, single cross‑platform build (macOS/Linux/Windows). Fits Franz’s Go ecosystem.
* **Tag/Container parsing:**

  * Primary: [`go-music-tagger` or `tag`](https://github.com/dhowden/tag) for common formats (MP3/FLAC/M4A/OGG/Opus/WAV/AIFF).
  * Fallback: parse with `ffprobe` JSON (from FFmpeg) for stubborn containers.
* **Audio quality/properties:** `ffprobe` (external dependency) to read bit depth, sample rate, bitrate, channel layout, duration, lossless/lossy indicators.
* **Acoustic fingerprinting (optional v1.1):** `fpcalc` (Chromaprint) to cluster true duplicates even with divergent tags.
* **State/Resumability:** Embedded **SQLite** DB (via `modernc.org/sqlite` for CGO‑less builds) or `mattn/go-sqlite3` (CGO) if acceptable.
* **CLI UX:** Cobra/Viper (commands + config file); TUI progress via `mpb` or `bubbletea` (optional).
* **Logging/Reports:** structured JSONL + human Markdown/HTML summary.

**External tools required:** `ffprobe` (mandatory), `fpcalc` (optional but recommended for robust duplicate detection).

---

## 3) Architecture Overview

```
cmd/
  mlc/                  # main binary (Music Library Cleaner)
internal/
  scan/                 # walks source FS, discovers files
  meta/                 # metadata extraction (tags + ffprobe)
  score/                # quality scoring & duplicate arbitration
  plan/                 # dry‑run planning (what to do)
  execute/              # perform copy/move/link, atomic renames
  layout/               # destination path rules
  report/               # JSONL events + summary
  store/                # SQLite state + migrations
  util/                 # hash, safe io, concurrency helpers
configs/
  example.yaml          # sample configuration

artifacts/
  reports/              # generated reports (timestamped)
```

**Pipeline (per file):**

1. **Discover** → 2. **Extract metadata** (tags → ffprobe) → 3. **Normalize/Infer** → 4. **Score** quality → 5. **Group** by Work (Track) identity → 6. **Arbitrate** duplicates → 7. **Plan** destination path → 8. **Execute** (copy/move/hardlink/symlink) → 9. **Record** event → 10. **Checkpoint** in DB.

All steps are retriable and recorded with a deterministic **FileKey** (stable across runs) to support resume.

---

## 4) Data Model (SQLite)

```sql
-- files discovered in source
CREATE TABLE files (
  id INTEGER PRIMARY KEY,
  file_key TEXT UNIQUE,            -- stable key (see below)
  src_path TEXT NOT NULL,
  size_bytes INTEGER,
  mtime_unix INTEGER,
  sha1 TEXT,                       -- optional; large files may be hashed lazily
  status TEXT NOT NULL,            -- discovered|meta_ok|planned|executed|skipped|error
  error TEXT,
  first_seen_at DATETIME,
  last_update_at DATETIME
);

-- extracted metadata (one row per file)
CREATE TABLE metadata (
  file_id INTEGER PRIMARY KEY REFERENCES files(id) ON DELETE CASCADE,
  format TEXT, codec TEXT, container TEXT,
  duration_ms INTEGER,
  sample_rate INTEGER, bit_depth INTEGER, channels INTEGER,
  bitrate_kbps INTEGER,
  lossless INTEGER,
  tag_artist TEXT, tag_album TEXT, tag_title TEXT,
  tag_albumartist TEXT, tag_date TEXT, tag_disc INTEGER, tag_disc_total INTEGER,
  tag_track INTEGER, tag_track_total INTEGER,
  tag_compilation INTEGER,
  musicbrainz_recording_id TEXT, musicbrainz_release_id TEXT,
  raw_tags_json TEXT
);

-- groups of files believed to be the same recording (dedup cluster)
CREATE TABLE clusters (
  cluster_key TEXT PRIMARY KEY,    -- derived from normalized artist+title+duration bucket (+ fp if available)
  hint TEXT
);

CREATE TABLE cluster_members (
  cluster_key TEXT REFERENCES clusters(cluster_key) ON DELETE CASCADE,
  file_id INTEGER REFERENCES files(id) ON DELETE CASCADE,
  quality_score REAL,
  preferred INTEGER,               -- 1 if chosen as winner
  PRIMARY KEY (cluster_key, file_id)
);

-- planned destination mapping per winning file
CREATE TABLE plans (
  file_id INTEGER PRIMARY KEY REFERENCES files(id) ON DELETE CASCADE,
  action TEXT NOT NULL,            -- copy|move|link|skip
  dest_path TEXT,
  reason TEXT                      -- e.g., "duplicate (lower score)", "unsupported format", etc.
);

-- execution results
CREATE TABLE executions (
  file_id INTEGER PRIMARY KEY REFERENCES files(id) ON DELETE CASCADE,
  started_at DATETIME,
  completed_at DATETIME,
  bytes_written INTEGER,
  verify_ok INTEGER,
  error TEXT
);
```

**Stable `file_key`:** SHA1 of `(device,inode,size,mtime)` for speed; optionally upgrade to content hash (SHA1/xxh3) when file selected for keep. This ensures idempotency and robust resume.

---

## 5) Destination Layout Rules

Default: `Artist/Year - Album/Disc XX/NN - Title.ext`

* **Artist selection:** Prefer `AlbumArtist` if present, else `Artist`.
* **Various artists:** If `Compilation=1` or album has many artists, use `Various Artists/Year - Album/...` with individual track artists in filenames: `NN - Artist - Title.ext`.
* **Singles/EPs/No Album:** Place under `Artist/_Singles` or `Artist/_Misc`, grouping by tag `Date` (YYYY) when possible: `YYYY - Title.ext`.
* **Unknown metadata:** `Unknown Artist/Unknown Album/...` retaining a `src_basename` suffix until corrected.
* **Multi‑disc:** create `Disc 01`, `Disc 02` subfolders if `DiscTotal>1`.
* **Track numbers:** zero‑pad to 2 (or 3 for >99).
* **Characters:** normalize Unicode (NFC), strip control chars, replace `/\:*?"<>|` with safe hyphens/underscores, collapse whitespace.
* **Case‑style:** Title Case for names, preserve acronyms.

**Alternative layouts (configurable):**

* `AlbumArtist/Album (Year)/NN - Title.ext`
* `Genre/AlbumArtist/Album/NN - Title.ext`
* `Artist/Album/NN - Title [Format@Samplerate-Bitrate].ext` (power‑user)

---

## 6) Quality Scoring & Duplicate Arbitration

**Goal:** If multiple files represent the same track, keep the highest quality and discard/skip the rest.

**Quality score (0–100), example weights:**

* Container/Codec:

  * FLAC/ALAC (lossless): +40
  * AAC/M4A (VBR ≥ 256): +25
  * MP3 (LAME V0/V2): +18
  * MP3 320 CBR: +20 (≈ AAC 256)
  * Ogg Vorbis q8+: +22
  * Opus 192+: +22
* Bit depth & sample rate (lossless): +10 if ≥16‑bit/44.1k; +2 bonus for 24‑bit/96k (capped to avoid “hi‑res” upsample bias).
* Verified lossless (via codec/container): +10
* Duration proximity to cluster median (|Δ| ≤ 1.5s): +6 else penalty proportional to deviation.
* Tag completeness (Artist/Album/Title/Track present): +4
* ReplayGain/peak present: +1
* Loudness normalization not used for scoring.

**Tie‑breakers:**

* Prefer lower generation (no transcode signs detected via spectral check, optional)
* Larger file size (within same codec/params)
* Newer mtime (assume curated)
* Deterministic: lexical order of src_path

**Cluster identity (same recording):**

* Primary key: normalized `(artist_norm, title_norm, approx_duration_bucket)`
* Secondary hints: album name (if same), MusicBrainz IDs (if present)
* Optional v1.1: Chromaprint fingerprint match.

**Actions for lower‑scored duplicates:**

* Default: **skip** (do not delete) but record in report with pointer to kept file.
* Optional: move to `destination/_duplicates/…` mirroring final layout.

---

## 7) Metadata Extraction & Inference

**Order of trust:**

1. Explicit tags (AlbumArtist > Artist, Title, Album, Track, Disc, Date, MusicBrainz IDs).
2. Container properties (duration, channels, sample rate, bitrate, bit depth).
3. Filename heuristics: parse patterns like `01 - Artist - Title`, `Artist/Album/01 Title`, `NN.Title`, `CD1`/`Disc 1`/`Part A`.
4. Folder context: sibling files, shared album folder, common prefix.

**Normalization:**

* Unicode NFC; trim whitespace; collapse spaces; unify separators (`-`, `_`, `.`); common stop‑words handled.
* Map common artist aliases (configurable alias table).

**Unsupported or broken files:**

* Mark as `error` with reason (`unsupported container`, `zero duration`, `corrupt`). Do not block processing.

---

## 8) Execution Strategy (Safe & Resumable)

* **Dry‑run first:** Build full plan (`plans` table + `artifacts/reports/<ts>/plan.jsonl`) without touching destination.
* **Transactional write:**

  * Copy to temp path inside destination (`.mlc_tmp/<file_key>.part`).
  * Verify size and, if configured, content hash.
  * Atomic rename to final `dest_path` (same filesystem) or `rename + fsync` fallback.
* **Mode:** `--mode=copy|move|hardlink|symlink` (default `copy`). Hardlink only when same filesystem and `--allow-hardlinks`.
* **Idempotency:** If `dest_path` exists with same content hash → mark executed; if different → log conflict (`conflict_existing`) and keep both via suffix ` (2)` unless `--prefer-existing`.
* **Resume:** On restart, pick up files with `status IN ('discovered','meta_ok','planned')` and no successful `executions`.
* **Parallelism:** Worker pool for IO‑bound tasks with bounded concurrency (`--concurrency=N`).

---

## 9) Reporting & Auditability

**Per‑event JSONL** (one line per action):

```json
{
  "ts": "2025-11-02T20:15:03Z",
  "level": "info|warn|error",
  "event": "plan|execute|skip|duplicate|conflict|error",
  "file_key": "…",
  "src_path": "…",
  "dest_path": "…",
  "cluster_key": "…",
  "quality_score": 87.5,
  "action": "copy|move|skip",
  "reason": "duplicate lower score"
}
```

**Human summary** (Markdown & HTML):

* Totals: scanned, valid, errors, duplicates found, winners kept, bytes moved, time spent.
* Top issues (e.g., missing tags, corrupt files).
* Duplicates table per cluster with chosen winner and score breakdown.
* Conflicts encountered and resolutions.

**Preview/What‑If:** `--dry-run` prints a concise table and writes full plan files for review.

---

## 10) CLI & Config

**Commands**

* `mlc scan --src <path> --db <state.db>`
* `mlc plan --dest <path> [--mode copy|move|link] [--layout default|alt1] [--dry-run]`
* `mlc execute [--verify hash] [--concurrency 8]`
* `mlc report --out artifacts/reports/<ts>`
* `mlc resume` (alias for `execute` continuing unfinished)

**Global flags**

* `--config configs/example.yaml`
* `--hashing none|sha1|xxh3` (default `sha1` on winners only)
* `--duplicates keep|quarantine|delete` (default `keep`)
* `--prefer-existing` (do not overwrite differing files in dest)
* `--fingerprinting on|off` (Chromaprint)

**Config example (`configs/example.yaml`)**

```yaml
source: "/Volumes/MessyMusic"
destination: "/Volumes/MusicClean"
mode: copy
layout: default
concurrency: 8
hashing: sha1
fingerprinting: off
alias_map:
  "The Weeknd": ["Weeknd, The"]
  "AC/DC": ["ACDC"]
duplicate_policy: keep
min_aac_bitrate_kbps: 192
min_mp3_bitrate_kbps: 192
quality_weights:
  lossless_bonus: 40
  bitrate_weight: 0.04
  duration_penalty_per_s: 2.0
  tag_completeness_bonus: 4
```

---

## 11) Edge Cases & Rules

* **Hidden tracks / pregap / live medleys:** rely on duration clustering; do not force to album if ambiguous.
* **Same title different songs:** cluster requires artist + duration proximity; optional fingerprint to disambiguate.
* **Remixes / versions:** include `(Remix)`, `(Live)`, `(Acoustic)` from tags in filename suffix.
* **Non‑music audio:** move to `/_Other/` by MIME/heuristics (podcasts, audiobooks) with separate layout.
* **Very short clips (< 10 s):** move to `/_Clips/` unless part of album with track numbers.
* **CUE sheets / ISO / DSF:** label as unsupported in v1; plan future handlers.
* **Artwork:** extract front cover to `folder.jpg` in album dir when available (optional `--artwork on`).

---

## 12) Performance Considerations

* Use a bounded worker pool; prioritize metadata extraction, then planning, then writing.
* Use mmap or buffered IO where beneficial.
* Hash winners only to minimize disk reads; enable full‑hash mode for final verification if desired.
* Batch `ffprobe` calls; reuse process pool or use library bindings.

---

## 13) Testing Strategy

* **Golden fixtures** for common formats and messy names.
* **Property tests** for path normalization (round‑trip safety, illegal char removal).
* **Load test** on synthetic tree (100k files) to validate concurrency & resume.
* **Chaos resume**: kill process mid‑copy; ensure no corruption and full resume.

---

## 14) Milestones

1. **M1 — Scanner + Metadata**: discover files, read tags+ffprobe, persist to DB.
2. **M2 — Clustering & Scoring**: build clusters, choose winners, write plan.
3. **M3 — Executor**: safe copy/move with verification; JSONL logging.
4. **M4 — Reporting**: Markdown/HTML report; dry‑run previews.
5. **M5 — Fingerprinting (optional)**: Chromaprint duplicate confirmation.
6. **M6 — Polishing**: config options, TUI progress, docs.

---

## 15) Deliverables

* `mlc` single binary (macOS/ARM64, Linux/AMD64; others on request).
* `PLAN.md` (this document), `README.md` (usage), `configs/example.yaml`.
* Example reports and sample dataset for verification.

---

## 16) Risks & Mitigations

* **Incorrect duplicate merges** → start with conservative clustering; require multiple signals (artist/title/duration ± fingerprint when on).
* **Filesystem differences** → default `copy` to new volume; allow `hardlink` only when safe.
* **Huge datasets** → resumable pipeline, lazy hashing, concurrency tuning.
* **Tag chaos** → filename heuristics + folder context; surface uncertainties in report for manual fix.

---

## 17) Acceptance Criteria

* Dry‑run on a messy corpus produces a deterministic plan with ≥95% files mapped to a sensible destination.
* Executing the plan is resumable, crash‑safe, and idempotent.
* Duplicates are identified and winners chosen with transparent scoring; lower‑scored files are preserved or quarantined per policy.
* Summary report clearly shows what changed and why.

---

## 18) Nice‑to‑Haves (post‑v1)

* MusicBrainz/Discogs lookups to enrich/repair tags.
* Web UI for reviewing clusters and overriding decisions.
* ReplayGain calculation.
* Artwork normalization and de‑duplication.
* NAS friendly mode (SMB quirks, case‑sensitivity guards).
