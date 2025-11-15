# MLC v1.9.2 - Multi-Disc Album Clustering Fix

**Release Date**: 2024-11-15
**Type**: Patch Release (Bug Fix)

---

## 🐛 Critical Fix

This release fixes a **critical clustering bug** where different disc tracks from multi-disc albums were incorrectly grouped as duplicates.

### Issue Fixed

When planning multi-disc albums, tracks with the same number from different discs were being clustered together, causing path collisions:

```
[WARN]  Path collision: 3 files -> Track No03.mp3
[INFO]   Keeping: Disco 1/Track No03.mp3 (score: 14.0)
[INFO]   Skipping: Disco 2/Track No03.mp3 (score: 14.0)
[INFO]   Skipping: Disco 3/Track No03.mp3 (score: 14.0)
```

**This is incorrect behavior** - Track 3 from Disc 1, Disc 2, and Disc 3 are different songs and should NOT be treated as duplicates.

### Root Cause

The cluster key generation function was not including the disc number, causing all tracks with matching artist/title/duration to cluster together regardless of which disc they came from:

```
# Before (wrong)
artist|title|version|duration

# After (correct)
artist|title|version|duration|discN
```

### Fix Applied

Modified `GenerateClusterKey()` in `internal/cluster/cluster.go` to include disc number:

```go
// Include disc number in cluster key to prevent different discs from clustering
// This fixes the issue where Track 3 from Disc 1, 2, and 3 would cluster together
discNum := m.TagDisc

// Generate cluster key with version type and disc number
// Disc number is critical for multi-disc albums to prevent false duplicates
return fmt.Sprintf("%s|%s|%s|%d|disc%d", artistNorm, titleNorm, versionType, durationBucket, discNum)
```

---

## 📝 Changes

### Fixed
- **Multi-disc album clustering** - Different discs no longer cluster together
- **Cluster key format** - Now includes disc number: `artist|title|version|duration|discN`
- **Path collisions** - Eliminated false duplicate warnings for multi-disc albums

### Added
- 3 new test cases for multi-disc album scenarios (Disc 1, 2, 3)
- Updated 17 existing test cases to match new cluster key format

### Tests
All cluster tests pass:
```bash
go test ./internal/cluster -v
# PASS: TestGenerateClusterKey (13 test cases)
# PASS: TestGenerateClusterKey_EdgeCases (7 test cases)
# PASS: TestBucketDuration
# PASS: TestGetDurationDelta
# PASS: TestNormalizeForClustering
```

---

## 🚀 Upgrade from v1.9.1

**This fix is CRITICAL if you have multi-disc albums in your library.**

### Who Should Upgrade?

**Upgrade immediately if:**
- ❌ You have multi-disc albums (compilations, box sets, double albums)
- ❌ You saw "Path collision" warnings during `mlc plan`
- ❌ Tracks from different discs were being marked as duplicates

**Optional upgrade if:**
- ✅ You only have single-disc albums
- ✅ You haven't run `mlc plan` yet

### Upgrade Steps

1. **Download new binary** for your platform
2. **Replace old binary** (no database changes needed)
3. **Re-run clustering** with `--force-recluster` flag to rebuild clusters

```bash
# Download (macOS Apple Silicon)
curl -L https://github.com/Fauli/music-janitor/releases/download/v1.9.2/mlc-v1.9.2-darwin-arm64.tar.gz -o mlc.tar.gz

# Extract
tar -xzf mlc.tar.gz
cd mlc-v1.9.2-darwin-arm64

# Verify
./mlc --version
# Output: mlc version v1.9.2

# Re-cluster with fix
./mlc cluster --force-recluster

# Generate new plan
./mlc plan
```

### Important: Must Re-Cluster

Because the cluster key format has changed, you **must** re-cluster your library:

```bash
# Clear old clusters and rebuild with new disc-aware keys
./mlc cluster --force-recluster
```

This will:
- Clear existing cluster data
- Rebuild clusters using new disc-aware keys
- Correctly separate tracks from different discs

---

## 📊 Impact

### Before v1.9.2 (Broken)

```
Cluster Key: nat king cole|track no03|studio|180
  ├─ Disco 1/Track No03.mp3 ← KEPT
  ├─ Disco 2/Track No03.mp3 ← SKIPPED (wrong!)
  └─ Disco 3/Track No03.mp3 ← SKIPPED (wrong!)

Result: Only 1 track kept, 2 tracks lost
```

### After v1.9.2 (Fixed)

```
Cluster Key: nat king cole|track no03|studio|180|disc1
  └─ Disco 1/Track No03.mp3 ← KEPT

Cluster Key: nat king cole|track no03|studio|180|disc2
  └─ Disco 2/Track No03.mp3 ← KEPT

Cluster Key: nat king cole|track no03|studio|180|disc3
  └─ Disco 3/Track No03.mp3 ← KEPT

Result: All 3 tracks kept correctly
```

---

## 🔍 Technical Details

### Cluster Key Format

The cluster key format has been updated to include disc number as the 5th component:

| Component | Example | Purpose |
|-----------|---------|---------|
| Artist (normalized) | `nat king cole` | Group by artist |
| Title (normalized) | `track no03` | Group by title |
| Version type | `studio` | Separate remixes/live/acoustic |
| Duration bucket | `180` | Group similar durations (±1.5s) |
| **Disc number** | `disc1` | **NEW: Separate multi-disc tracks** |

### Example Cluster Keys

```
# Single disc album (disc number = 0)
the beatles|yesterday|studio|126|disc0

# Multi-disc album - Disc 1
nat king cole|track no03|studio|180|disc1

# Multi-disc album - Disc 2
nat king cole|track no03|studio|180|disc2

# Multi-disc album - Disc 3
nat king cole|track no03|studio|180|disc3
```

### Backward Compatibility

The fix is **backward compatible** with existing databases:
- No schema changes required
- `tag_disc` field was already in metadata table
- Default disc number is `0` for single-disc albums

However, you **must re-cluster** to get the new behavior.

---

## 📦 Installation

### Download Binary

```bash
# macOS (Apple Silicon)
curl -L https://github.com/Fauli/music-janitor/releases/download/v1.9.2/mlc-v1.9.2-darwin-arm64.tar.gz -o mlc.tar.gz
tar -xzf mlc.tar.gz
./mlc --version

# macOS (Intel)
curl -L https://github.com/Fauli/music-janitor/releases/download/v1.9.2/mlc-v1.9.2-darwin-amd64.tar.gz -o mlc.tar.gz

# Linux (x86_64)
curl -L https://github.com/Fauli/music-janitor/releases/download/v1.9.2/mlc-v1.9.2-linux-amd64.tar.gz -o mlc.tar.gz

# Linux (ARM64)
curl -L https://github.com/Fauli/music-janitor/releases/download/v1.9.2/mlc-v1.9.2-linux-arm64.tar.gz -o mlc.tar.gz
```

### Verify Checksums

```bash
# Download checksums
curl -L https://github.com/Fauli/music-janitor/releases/download/v1.9.2/SHA256SUMS -o SHA256SUMS

# Verify (macOS)
shasum -a 256 -c SHA256SUMS

# Verify (Linux)
sha256sum -c SHA256SUMS
```

---

## 🔗 Links

- [v1.9.1 Release Notes](https://github.com/Fauli/music-janitor/releases/tag/v1.9.1) - SQL NULL handling fix
- [v1.9.0 Release Notes](https://github.com/Fauli/music-janitor/releases/tag/v1.9.0) - Self-healing release
- [Issue Tracker](https://github.com/Fauli/music-janitor/issues)

---

**Diff**: https://github.com/Fauli/music-janitor/compare/v1.9.1...v1.9.2

**Status**: ✅ Critical Fix (recommended for all users with multi-disc albums)
