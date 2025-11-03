# MLC v1.1.0 - Various Artists & Critical Bug Fixes

**Music Library Cleaner** - Infrastructure-grade media processing for personal collections

## 🎉 What's New in v1.1.0

This release adds **compilation album support** and fixes a **critical clustering bug** that caused files without metadata to be incorrectly treated as duplicates.

---

## 🆕 New Features

### 🎵 Various Artists / Compilation Album Support

MLC now automatically detects and properly organizes compilation albums!

**How it works:**
- Detects `compilation=1` tag (ID3v2 TCMP, MP4 cpil, Vorbis COMPILATION)
- Smart false-positive prevention: verifies album has 3+ different track artists
- Organizes under "Various Artists" folder with artist in filename

**Example output:**
```
Various Artists/
  2000 - Greatest Hits/
    01 - The Beatles - Hey Jude.mp3
    02 - Queen - Bohemian Rhapsody.mp3
    03 - Led Zeppelin - Stairway to Heaven.mp3
```

**vs normal albums:**
```
The Beatles/
  1969 - Abbey Road/
    01 - Come Together.mp3
    02 - Something.mp3
```

**Smart handling:** If compilation flag is set but all tracks have the same artist, MLC uses normal folder structure (prevents false positives).

**Commits:** `6c11673`, `4ad2881`, `3b3d562`

---

### 🔄 Metadata Re-scanning

New `mlc rescan` command to re-extract metadata for existing files.

**Usage:**
```bash
mlc rescan --db my-library.db
```

**Use cases:**
- Extract newly implemented fields (like compilation flag)
- Refresh metadata after editing tags with external tools
- Fix metadata extraction errors

**Features:**
- Concurrent processing with progress tracking
- Reports files processed, updated, and errors
- Respects `--verbose` and `--quiet` flags

**Commit:** `4ad2881`

---

## 🐛 Critical Bug Fixes

### ❌ False Duplicate Detection for Files Without Metadata

**Problem:** Files without artist/title tags were incorrectly clustered as duplicates if they had similar durations (~±1.5 seconds).

**Symptoms:**
```
Cluster Key: unknown|unknown|78

✗ [SKIP] 05 Track 05.wav (duplicate)
✗ [SKIP] 19 Track 19.wav (duplicate)
✓ [WINNER] 37 Track 37.wav (kept)
```

Different tracks from the same album were treated as duplicates, causing data loss!

**Root cause:** When both artist and title were missing, all files got cluster key `unknown|unknown|bucket`, making them appear as duplicates.

**Solution:** Now uses **normalized filename** in cluster key when metadata is missing. Files with different filenames won't cluster together.

**Impact:**
- ✅ No more false positive duplicates for untagged files
- ✅ Each untagged file treated as unique unless filename matches
- ✅ Files with metadata cluster correctly (no change)
- ✅ True duplicates (same filename + duration) still cluster

**Commit:** `bb86775`

---

## 📊 What's Changed Since v1.0.0

### Features
- ✅ Various Artists / Compilation album detection and organization
- ✅ `mlc rescan` command for metadata re-extraction
- ✅ Compilation flag extraction from all audio formats
- ✅ Smart false-positive prevention (multi-artist check)

### Bug Fixes
- 🐛 **CRITICAL**: Fixed false duplicate clustering for files without metadata
- 🐛 Path collision resolution (from v1.0.1)

### Tests & Quality
- 📈 Test coverage: 70+ tests across 10 packages
- ✅ All tests passing
- ✅ 0 linting issues (golangci-lint)

---

## 📥 Installation

### Download Pre-built Binaries

Choose the binary for your platform:

- **macOS (Apple Silicon)**: `mlc-v1.1.0-darwin-arm64`
- **macOS (Intel)**: `mlc-v1.1.0-darwin-amd64`
- **Linux (x86_64)**: `mlc-v1.1.0-linux-amd64`
- **Linux (ARM64)**: `mlc-v1.1.0-linux-arm64`

Verify checksums with `mlc-v1.1.0-checksums.txt`

### Install from Binary

```bash
# Download binary (example for macOS arm64)
curl -LO https://github.com/franz/music-janitor/releases/download/v1.1.0/mlc-v1.1.0-darwin-arm64

# Make executable
chmod +x mlc-v1.1.0-darwin-arm64

# Move to PATH
sudo mv mlc-v1.1.0-darwin-arm64 /usr/local/bin/mlc

# Verify installation
mlc --version
```

### Build from Source

```bash
git clone https://github.com/franz/music-janitor
cd music-janitor
git checkout v1.1.0
make build
sudo make install
```

---

## 🚨 Important: Re-scan Required for Bug Fix

**If you used v1.0.0** and have files without metadata tags, you should **re-cluster** your library to fix false duplicates:

```bash
# Option 1: Start fresh (recommended)
rm my-library.db
mlc scan --source /path/to/music --db my-library.db
mlc plan --dest /path/to/clean --db my-library.db
mlc execute --db my-library.db

# Option 2: Re-scan metadata only (if you want compilation flags)
mlc rescan --db my-library.db
# Then re-run plan and execute
mlc plan --dest /path/to/clean --db my-library.db
mlc execute --db my-library.db
```

---

## 🆙 Upgrade Path from v1.0.0

1. **Download new binary** or build from source
2. **Re-scan your library** to pick up fixes and new features
3. **Review plan** before executing (compilation albums will have new paths)
4. **Execute** to apply changes

---

## 📝 Full Changelog

### Commits Since v1.0.0
- `bb86775` - fix(cluster): use filename for clustering when metadata is missing
- `3b3d562` - docs: add Various Artists and rescan documentation
- `4ad2881` - feat(rescan): implement mlc rescan command for metadata re-extraction
- `6c11673` - feat(compilations): implement Various Artists handling for compilation albums
- `5ecb0d0` - chore: add releases/ to gitignore and include RELEASE_NOTES.md
- `4ab42bf` - docs(TODO): update to reflect v1.0.0 release and post-MVP status
- `2347a2d` - feat(plan): implement quality-based path collision resolution

---

## 🙏 Credits

Developed with assistance from Claude Code (Anthropic).

---

**Full Changelog**: https://github.com/franz/music-janitor/compare/v1.0.0...v1.1.0
