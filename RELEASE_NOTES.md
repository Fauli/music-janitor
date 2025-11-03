# MLC v1.0.0 - MVP Release 🎉

**Music Library Cleaner** - Infrastructure-grade media processing for personal collections

## What's New

This is the first MVP release of MLC! After completing all core milestones (M0-M6), MLC is ready for real-world use.

### Core Features

✅ **Smart File Discovery & Metadata Extraction**
- Supports MP3, FLAC, M4A/AAC, OGG Vorbis, Opus, WAV, AIFF
- Hybrid extraction: tag libraries + ffprobe for comprehensive metadata
- Resumable scanning with SQLite state tracking

✅ **Intelligent Deduplication**
- Quality-based scoring system (codec, bitrate, sample rate, lossless detection)
- Duration-based clustering (±1.5s tolerance)
- Artist/title normalization with configurable rules
- Automatic winner selection with detailed scoring

✅ **Safe Execution**
- Multiple modes: copy, move, hardlink, symlink
- Atomic operations with `.part` temp files
- SHA1 hash verification (configurable: none/size/hash)
- Resumable execution (interrupt and restart safely)
- Worker pool with bounded concurrency

✅ **Comprehensive Reporting**
- JSONL event logs for audit trails
- Markdown summary reports with duplicate analysis
- Top errors and conflict tracking
- Configurable log levels (--verbose, --quiet)

✅ **User Experience**
- `mlc doctor` - System diagnostics (checks ffprobe, disk space, permissions)
- Progress indicators for long operations
- Color-coded terminal output
- Cross-filesystem move warnings
- Detailed troubleshooting documentation

✅ **Performance & Reliability**
- Database schema migrations with automatic upgrades
- Optimized indexes (v2 schema)
- 64+ tests across 9 packages
- golangci-lint compliant
- Production-ready error handling

## Installation

### Download Pre-built Binaries

Choose the binary for your platform:

- **macOS (Apple Silicon)**: `mlc-v1.0.0-darwin-arm64`
- **macOS (Intel)**: `mlc-v1.0.0-darwin-amd64`
- **Linux (x86_64)**: `mlc-v1.0.0-linux-amd64`
- **Linux (ARM64)**: `mlc-v1.0.0-linux-arm64`

Verify checksums with `mlc-v1.0.0-checksums.txt`

### Install from Binary

```bash
# Download binary (example for macOS arm64)
curl -LO https://github.com/franz/music-janitor/releases/download/v1.0.0/mlc-v1.0.0-darwin-arm64

# Make executable
chmod +x mlc-v1.0.0-darwin-arm64

# Move to PATH
sudo mv mlc-v1.0.0-darwin-arm64 /usr/local/bin/mlc

# Verify installation
mlc --version
```

### Build from Source

```bash
git clone https://github.com/franz/music-janitor
cd music-janitor
git checkout v1.0.0
make build
sudo make install
```

## Prerequisites

- **Required**: `ffprobe` (from FFmpeg package)
- **Optional**: `fpcalc` (for acoustic fingerprinting - future feature)

Install ffprobe:
```bash
# macOS
brew install ffmpeg

# Linux (Debian/Ubuntu)
sudo apt install ffmpeg
```

## Quick Start

```bash
# 1. Check your system
mlc doctor --src /path/to/messy-music --dest /path/to/clean-music

# 2. Scan your music collection
mlc scan --source /path/to/messy-music --db my-library.db

# 3. Create execution plan (dry-run)
mlc plan --dest /path/to/clean-music --db my-library.db --dry-run

# 4. Execute the plan
mlc execute --db my-library.db --verify hash

# 5. Generate summary report
mlc report --db my-library.db
```

## What's NOT in This Release

The following features are planned for future releases:

- Acoustic fingerprinting (M5 - optional)
- Web UI for cluster review
- MusicBrainz metadata enrichment
- Tag cleanup and normalization
- Album artwork extraction
- Case-insensitive filesystem collision handling
- Artist alias mapping
- Quality weight customization

See [TODO.md](TODO.md) for the full roadmap.

## Known Limitations

- Move mode across filesystems is slower (copy + verify + delete) - warning is shown
- No automatic detection of compilation albums
- Case-sensitive path handling only (case-insensitive FS support planned)
- No spectral analysis for transcode detection

## Documentation

- [README.md](README.md) - Full usage guide with troubleshooting
- [PLAN.md](docs/PLAN.md) - Product specification
- [ARCHITECTURE.md](docs/ARCHITECTURE.md) - Design and internals
- [TODO.md](TODO.md) - Development roadmap

## Contributing

MLC was developed with assistance from Claude Code (Anthropic). Contributions, bug reports, and feature requests are welcome!

## Testing

MLC has been tested with:
- 64+ unit and integration tests
- All major audio formats (MP3, FLAC, M4A, OGG, Opus, WAV, AIFF)
- Various edge cases (unicode filenames, long paths, etc.)
- Resumability and crash recovery scenarios

**Recommended**: Test on a small subset of your collection first, then review the generated plan before executing on your full library.

## License

[Add your license here]

## Credits

Developed with assistance from Claude Code (Anthropic).

---

**Full Changelog**: https://github.com/franz/music-janitor/commits/v1.0.0
