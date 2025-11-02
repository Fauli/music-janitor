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

🚧 **Under Development** — Currently in M0 (Project Setup & Foundation)

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

### Example Workflow

```bash
# Full pipeline with config file
mlc scan --config configs/my-library.yaml
mlc plan --dry-run
# Review the plan...
mlc execute --verify hash
mlc report
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

- [x] **M0** — Project Setup & Foundation
- [x] **M1** — Scanner + Metadata Extraction
- [ ] **M2** — Clustering & Scoring
- [ ] **M3** — Executor (Safe Copy/Move)
- [ ] **M4** — Reporting & Observability
- [ ] **M5** — Fingerprinting (Optional)
- [ ] **M6** — Polishing & Documentation

See [TODO.md](TODO.md) for detailed task breakdown.

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
