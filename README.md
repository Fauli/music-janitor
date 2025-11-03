# MLC — Music Library Cleaner

**Infrastructure-grade media processing for personal collections**

MLC is a deterministic, resumable music library cleaner that takes a large, messy archive of audio files and produces a clean, deduplicated, normalized destination library with audit logs, safe copies, format scoring, and duplicate arbitration.

## Features

- **Deterministic & Resumable**: Crash-safe operations with SQLite state tracking
- **Smart Deduplication**: Quality-based scoring to keep the best version of each track
- **Safe by Default**: Copy mode prevents data loss; dry-run before execution
- **Metadata Extraction**: Support for MP3, FLAC, M4A/AAC, OGG, Opus, WAV, AIFF
- **Flexible Layout**: Customizable destination folder structure
- **Transparent**: JSONL event logs and detailed reports
- **Fast**: Concurrent processing with bounded worker pools

## Status

✅ **MVP Complete** — Ready for real-world use!

- ✅ Scanner + Metadata Extraction (MP3, FLAC, M4A, OGG, Opus, WAV, AIFF)
- ✅ Smart Deduplication (quality-based scoring)
- ✅ Safe Execution (copy/move/hardlink/symlink with verification)
- ✅ Event Logging & Markdown Reports
- ✅ Diagnostics & Troubleshooting (`mlc doctor`)
- ✅ Performance Optimizations (indexed queries, cross-filesystem warnings)
- ✅ Comprehensive Documentation (README, troubleshooting, workflows, FAQ)
- ✅ 64+ tests across 9 packages, golangci-lint passing

See [TODO.md](TODO.md) for development progress and [docs/PLAN.md](docs/PLAN.md) for full specification.

## Quick Start

### Prerequisites

- Go 1.22 or later
- `ffprobe` (from FFmpeg) — **required** for metadata extraction
- `fpcalc` (from Chromaprint) — optional for acoustic fingerprinting

#### Install ffprobe (macOS)

```bash
brew install ffmpeg
```

#### Install ffprobe (Linux)

```bash
# Debian/Ubuntu
sudo apt install ffmpeg

# Fedora/RHEL
sudo dnf install ffmpeg
```

### Installation

#### From Source

```bash
git clone https://github.com/franz/music-janitor
cd music-janitor
make build
sudo make install
```

The `mlc` binary will be installed to `$GOPATH/bin`.

#### Using Go

```bash
go install github.com/franz/music-janitor/cmd/mlc@latest
```

### Usage

#### 1. Check Your Environment

```bash
make doctor
# or
mlc doctor
```

This verifies that Go, ffprobe, and other dependencies are installed.

#### 2. Configure

MLC supports three configuration methods (in order of precedence):

1. **Config file** (YAML) - persistent settings
2. **Environment variables** (`MLC_*`) - user/system defaults
3. **Command-line flags** - one-off overrides (highest priority)

**Option A: Use a config file**

```bash
cp configs/example.yaml configs/my-library.yaml
# Edit configs/my-library.yaml with your paths
mlc scan --config configs/my-library.yaml
```

**Option B: Use command-line flags**

```bash
mlc scan -s /Volumes/MessyMusic -d /Volumes/MusicClean --db my-library.db
```

**Option C: Mix both** (flags override config file)

```bash
# Use config defaults but override concurrency
mlc scan --config my-library.yaml -c 16
```

See [docs/CONFIGURATION.md](docs/CONFIGURATION.md) for complete details.

#### 3. Scan Source Files

```bash
# With config file
mlc scan --config my-library.yaml

# Or with flags
mlc scan -s /Volumes/MessyMusic --db my-library.db -v
```

This discovers all audio files and stores them in the database.

#### 4. Plan Destination Layout (Dry-Run)

```bash
mlc plan --dest /Volumes/MusicClean --db my-library.db --dry-run
```

Review the generated plan in `artifacts/plans/<timestamp>/plan.jsonl`.

#### 5. Execute (Copy Files)

```bash
mlc execute --db my-library.db --verify hash --concurrency 8
```

This copies files to the destination according to the plan, with hash verification.

#### 6. Review Report

```bash
mlc report --out artifacts/reports/$(date +%Y%m%d)
```

Generates a summary report showing duplicates, conflicts, and errors.

## Example Workflows

### Basic Workflow: Clean Your Music Library

This is the recommended workflow for first-time users:

```bash
# 1. Check your system is ready
mlc doctor --src /Volumes/MessyMusic --dest /Volumes/MusicClean

# 2. Scan your messy music collection
mlc scan --source /Volumes/MessyMusic --db my-library.db --verbose

# 3. Create execution plan (dry-run to preview)
mlc plan --dest /Volumes/MusicClean --db my-library.db --dry-run

# 4. Review the plan output
# Check artifacts/events-*.jsonl for details
# Look at how many duplicates were found, what will be kept/skipped

# 5. Execute the plan (safe copy mode)
mlc execute --db my-library.db --verify hash --concurrency 4

# 6. Generate summary report
mlc report --db my-library.db
# Report saved to artifacts/reports/<timestamp>/summary.md
```

### Advanced Workflow: Resume After Interruption

If execution was interrupted (crash, ctrl-c, etc.), you can safely resume:

```bash
# Resume will skip already-completed files
mlc execute --db my-library.db --verify hash

# Check what's left to do
mlc report --db my-library.db
```

### Advanced Workflow: Move Instead of Copy

**⚠️ WARNING:** Move mode deletes source files after successful verification. Use with caution!

```bash
# Scan and plan as usual
mlc scan -s /Volumes/MessyMusic --db my-library.db
mlc plan --dest /Volumes/MusicClean --db my-library.db --dry-run

# Execute with move mode (DESTRUCTIVE - deletes source after copy)
mlc execute --mode move --verify hash --db my-library.db

# Verify everything succeeded
mlc report --db my-library.db
# Check that "Files Failed: 0" before deleting database
```

### Advanced Workflow: Using Config Files

For repeated operations, use a config file:

```bash
# Create your config
cp configs/example.yaml configs/my-library.yaml

# Edit configs/my-library.yaml:
# source: /Volumes/MessyMusic
# dest: /Volumes/MusicClean
# mode: copy
# concurrency: 8

# Run with config (flags still override)
mlc scan --config configs/my-library.yaml
mlc plan --config configs/my-library.yaml --dry-run
mlc execute --config configs/my-library.yaml --verify hash
mlc report --config configs/my-library.yaml
```

### Command-Line Flags Quick Reference

**Common flags:**
- `-s, --source <path>` — Source directory to scan
- `-d, --dest <path>` — Destination directory for clean library
- `-c, --concurrency <n>` — Number of parallel workers (default: 8)
- `--db <path>` — State database file (default: mlc-state.db)
- `-v, --verbose` — Verbose output (debug logs)
- `-q, --quiet` — Quiet mode (errors only)

**Execution options:**
- `--mode <mode>` — copy, move, hardlink, symlink (default: copy)
- `--dry-run` — Plan without executing
- `--layout <layout>` — default, alt1, alt2

**Quality & verification:**
- `--hashing <algo>` — sha1, xxh3, none (default: sha1)
- `--verify <mode>` — size, hash, full (default: hash)
- `--fingerprinting` — Enable acoustic fingerprinting

**Duplicate handling:**
- `--duplicates <policy>` — keep, quarantine, delete (default: keep)
- `--prefer-existing` — Prefer existing files on conflict

See `mlc --help` for complete list.

## Troubleshooting

### Common Errors and Solutions

#### "ffprobe not found" or "metadata extraction failed"

**Problem:** MLC can't find or run `ffprobe`.

**Solution:**
```bash
# macOS
brew install ffmpeg

# Linux (Debian/Ubuntu)
sudo apt install ffmpeg

# Verify installation
ffprobe -version

# Run diagnostics
mlc doctor
```

#### "permission denied" when scanning source directory

**Problem:** MLC doesn't have read permission for source files.

**Solution:**
```bash
# Check permissions
ls -la /path/to/source

# Fix permissions (if you own the files)
chmod -R u+r /path/to/source

# On macOS, grant Full Disk Access to Terminal in System Preferences
```

#### "disk space" warnings

**Problem:** Not enough free space on destination.

**Solution:**
```bash
# Check available space
df -h /path/to/destination

# Use mlc doctor to see space requirements
mlc doctor --src /source --dest /destination

# Consider:
# - Using a larger disk
# - Using --mode hardlink (same filesystem only)
# - Using --mode symlink (no space used)
```

#### Database is locked / "database is busy"

**Problem:** Another MLC process is running or crashed without releasing the lock.

**Solution:**
```bash
# Check for running mlc processes
ps aux | grep mlc

# Kill any stuck processes
killall mlc

# If that doesn't work, the database may be corrupted
mlc doctor --db my-library.db

# Last resort: start fresh (backup first!)
mv my-library.db my-library.db.backup
mlc scan --source /path/to/source --db my-library.db
```

#### "no plans found" when running execute

**Problem:** You need to run `mlc plan` before `mlc execute`.

**Solution:**
```bash
# Create the execution plan first
mlc plan --dest /path/to/destination --db my-library.db

# Then execute
mlc execute --db my-library.db
```

#### Files not being detected as duplicates

**Problem:** Similar files are not being clustered together.

**Possible causes:**
- Different artist/title tags → Check metadata: `mlc report`
- Duration difference >1.5s → Files are actually different versions
- Missing metadata → MLC falls back to filename parsing

**Solution:**
```bash
# Enable verbose logging to see clustering decisions
mlc plan --dest /dest --db my-library.db --verbose

# Check the event log for clustering details
cat artifacts/events-*.jsonl | grep cluster

# Consider using fingerprinting (optional, requires fpcalc)
mlc scan --fingerprinting
mlc plan --fingerprinting
```

#### Execution fails with "verification failed"

**Problem:** Hash verification detected file corruption during copy.

**Possible causes:**
- Disk errors (source or destination)
- Network issues (if using NAS/network drives)
- Bad RAM

**Solution:**
```bash
# Check disk health
# macOS: Disk Utility → First Aid
# Linux: sudo fsck /dev/sdX

# Retry with size-only verification (faster, less strict)
mlc execute --verify size

# Or disable verification (not recommended)
mlc execute --verify none

# For network drives, reduce concurrency
mlc execute --concurrency 1 --verify hash
```

#### "cross-device link" error with hardlink mode

**Problem:** Can't create hardlinks across different filesystems.

**Solution:**
```bash
# Check if source and dest are on same filesystem
df /path/to/source
df /path/to/dest

# If different, use copy mode instead
mlc execute --mode copy
```

### FAQ

**Q: Will MLC delete my original files?**

A: Only if you explicitly use `--mode move`. The default mode is `copy`, which is safe and non-destructive.

**Q: What happens if I interrupt MLC (Ctrl-C) during execution?**

A: MLC is designed to be resumable. Just run `mlc execute` again and it will skip already-completed files. Partial writes are stored as `.part` files and will be cleaned up.

**Q: How does MLC decide which duplicate to keep?**

A: MLC uses a quality scoring system that prefers:
1. Lossless formats (FLAC, ALAC) over lossy
2. Higher bitrates
3. Better metadata completeness
4. Larger file size (as tiebreaker)

See the "Quality Scoring" section for details.

**Q: Can I override which duplicate MLC keeps?**

A: Not in MVP. This feature is planned for a future web UI. For now, you can:
- Manually edit the database `cluster_members` table to change `preferred` flag
- Delete unwanted files from source before scanning
- Use `--verify hash` and manually verify winners after execution

**Q: What audio formats does MLC support?**

A: MP3, FLAC, M4A/AAC, OGG Vorbis, Opus, WAV, AIFF. Support is based on what `ffprobe` can read.

**Q: Does MLC modify my audio files or metadata?**

A: No. MLC only reads metadata and copies files. It never modifies the audio content or tags. Original files remain untouched (in `copy` mode).

**Q: How much disk space does the database use?**

A: Approximately 2-5 KB per file. A library with 100,000 files uses ~200-500 MB.

**Q: Can I run multiple MLC instances in parallel?**

A: Not on the same database. SQLite uses file locking. Use separate database files for different libraries.

**Q: How do I clean up the artifacts/ directory?**

A: The `artifacts/` directory contains event logs and reports. You can safely delete old logs:
```bash
# Delete logs older than 30 days
find artifacts/ -mtime +30 -delete

# Or clean up manually
rm -rf artifacts/events-*.jsonl
rm -rf artifacts/reports/
```

**Q: Where can I find more details about how MLC works?**

A: See:
- [PLAN.md](docs/PLAN.md) — Feature specification and design decisions
- [ARCHITECTURE.md](docs/ARCHITECTURE.md) — Internal architecture and data flow
- [TODO.md](TODO.md) — Development progress and roadmap

## Project Structure

```
.
├── cmd/mlc/                # CLI entrypoint
├── internal/
│   ├── scan/              # Directory scanner
│   ├── meta/              # Metadata extraction
│   ├── cluster/           # Duplicate clustering
│   ├── score/             # Quality scoring
│   ├── layout/            # Destination path rules
│   ├── plan/              # Action planning
│   ├── execute/           # Safe file operations
│   ├── report/            # JSONL and report generation
│   ├── store/             # SQLite database
│   └── util/              # Utilities and helpers
├── configs/               # Configuration files
├── docs/                  # Documentation
│   ├── PLAN.md           # Product specification
│   └── ARCHITECTURE.md   # Design & internals
├── TODO.md               # Development roadmap
└── CLAUDE.md             # AI pair programming guide
```

## Development

### Build and Test

```bash
# Run tests
make test

# Run tests with coverage
make test-coverage

# Build binary
make build

# Run the binary
make run

# Format code
make fmt

# Run linter
make lint

# Check environment
make doctor
```

### Development Milestones

**MVP Complete! 🎉**

- [x] **M0** — Project Setup & Foundation
- [x] **M1** — Scanner + Metadata Extraction
- [x] **M2** — Clustering & Scoring
- [x] **M3** — Executor (Safe Copy/Move)
- [x] **M4** — Reporting & Observability
- [x] **M6** — Polishing & Documentation
- [ ] **M5** — Fingerprinting (Optional - post-MVP)

See [TODO.md](TODO.md) for detailed task breakdown and post-MVP roadmap.

## Documentation

- [PLAN.md](docs/PLAN.md) — Product specification and feature details
- [ARCHITECTURE.md](docs/ARCHITECTURE.md) — Design, data flow, and internals
- [TODO.md](TODO.md) — Development roadmap and task tracking
- [CLAUDE.md](CLAUDE.md) — AI collaboration guide for development

## Configuration

MLC uses a YAML configuration file. See [configs/example.yaml](configs/example.yaml) for all available options.

Key configuration options:

| Option | Default | Description |
|--------|---------|-------------|
| `mode` | `copy` | Execution mode: `copy`, `move`, `hardlink`, `symlink` |
| `layout` | `default` | Destination folder layout template |
| `concurrency` | `8` | Number of parallel workers |
| `hashing` | `sha1` | Hash algorithm: `sha1`, `xxh3`, `none` |
| `fingerprinting` | `false` | Enable acoustic fingerprinting (requires `fpcalc`) |
| `duplicate_policy` | `keep` | What to do with duplicates: `keep`, `quarantine`, `delete` |

## Quality Scoring

MLC uses a multi-factor quality scoring system to choose the best version of each track:

- **Codec/Container** (0-40 points): FLAC/ALAC > AAC VBR > MP3 V0 > MP3 320
- **Lossless** (+10 points): Verified lossless encoding
- **Sample Rate/Bit Depth** (+0-12 points): Higher quality audio properties
- **Duration Proximity** (+6 or penalty): Matches cluster median duration
- **Tag Completeness** (+4 points): Has artist, album, title, track number
- **Tie-breakers**: File size, modification time, lexical path order

## Safety Features

- **No destructive defaults**: `copy` mode by default; `move` requires explicit flag
- **Atomic operations**: Files are written to temp locations, then atomically renamed
- **Verification**: Size and hash verification after copy
- **Resumability**: Interrupted operations can be safely resumed
- **Dry-run**: Review planned actions before execution
- **Audit logs**: All actions recorded in JSONL format
- **Conflict handling**: Existing files are preserved or renamed

## Roadmap (Post-MVP)

- Web UI for reviewing clusters and overriding decisions
- MusicBrainz/Discogs metadata enrichment
- Tag editing and cleanup
- Album artwork extraction and deduplication
- ReplayGain calculation
- Playlist migration
- NAS-optimized mode

## Contributing

This project is currently in active development. See [TODO.md](TODO.md) for tasks and [CLAUDE.md](CLAUDE.md) for development guidelines.

## License

[Add license here]

## Credits

Developed with assistance from Claude Code (Anthropic).
