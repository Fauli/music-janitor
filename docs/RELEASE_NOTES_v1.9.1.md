# MLC v1.9.1 - Database Compatibility Fix

**Release Date**: 2024-11-14
**Type**: Patch Release (Critical Bug Fix)

---

## 🐛 Critical Fix

This is a **critical bug fix** for users upgrading from v1.8.x to v1.9.0. If you experienced SQL scan errors when running `mlc plan`, this release fixes that issue.

### Issue Fixed

When running `mlc plan` with a database created before v1.9.0, users encountered this error:

```
[ERROR] Failed to get metadata for file X: failed to get metadata:
sql: Scan error on column index 16, name "tag_disc_total":
converting NULL to int is unsupported
```

### Root Cause

The database schema allowed NULL values for integer metadata fields (`tag_disc`, `tag_disc_total`, `tag_track`, `tag_track_total`), but the Go code was trying to scan them into `int` types which don't support NULL values.

### Fix Applied

Added `COALESCE` to all metadata query functions to convert NULL values to 0 for integer fields:

```sql
-- Before (caused error)
SELECT tag_disc, tag_disc_total FROM metadata

-- After (fixed)
SELECT COALESCE(tag_disc, 0), COALESCE(tag_disc_total, 0) FROM metadata
```

---

## 📝 Changes

### Fixed
- **SQL scan error** when reading metadata with NULL integer fields
- Added `COALESCE` for all integer fields in metadata queries:
  - `duration_ms`, `sample_rate`, `bit_depth`, `channels`, `bitrate_kbps`
  - `tag_disc`, `tag_disc_total`, `tag_track`, `tag_track_total`
  - `lossless`, `tag_compilation`
- **Database compatibility** with databases created before v1.9.0

### Functions Updated
- `GetMetadata()` - Main metadata retrieval function
- `GetFilesWithMetadata()` - Clustering function
- `GetAllMetadata()` - Bulk metadata retrieval
- `GetMetadataByFileID()` - Sibling file consensus function

---

## 🚀 Upgrade from v1.9.0

**If you encountered the SQL scan error, you MUST upgrade to v1.9.1.**

### Upgrade Steps

1. **Download new binary** for your platform
2. **Replace old binary** (no database changes needed)
3. **Run plan again** - it should work now

```bash
# Download (macOS Apple Silicon)
curl -L https://github.com/Fauli/music-janitor/releases/download/v1.9.1/mlc-v1.9.1-darwin-arm64.tar.gz -o mlc.tar.gz

# Extract
tar -xzf mlc.tar.gz
cd mlc-v1.9.1-darwin-arm64

# Verify
./mlc --version
# Output: mlc version v1.9.1

# Run plan (should work now)
./mlc plan
```

---

## ✅ Who Should Upgrade?

**Upgrade immediately if:**
- ❌ You upgraded from v1.8.x to v1.9.0
- ❌ You encountered SQL scan errors during `mlc plan`
- ❌ Your plan command failed with "converting NULL to int" errors

**Optional upgrade if:**
- ✅ You're running v1.9.0 without issues (fresh install)
- ✅ You haven't experienced any errors

---

## 🧪 Testing

All existing tests pass with the fix:
```bash
go test ./internal/store -v
# PASS
```

The fix ensures backward compatibility with databases containing NULL values.

---

## 📦 Installation

### Download Binary

```bash
# macOS (Apple Silicon)
curl -L https://github.com/Fauli/music-janitor/releases/download/v1.9.1/mlc-v1.9.1-darwin-arm64.tar.gz -o mlc.tar.gz
tar -xzf mlc.tar.gz
./mlc --version

# macOS (Intel)
curl -L https://github.com/Fauli/music-janitor/releases/download/v1.9.1/mlc-v1.9.1-darwin-amd64.tar.gz -o mlc.tar.gz

# Linux (x86_64)
curl -L https://github.com/Fauli/music-janitor/releases/download/v1.9.1/mlc-v1.9.1-linux-amd64.tar.gz -o mlc.tar.gz

# Linux (ARM64)
curl -L https://github.com/Fauli/music-janitor/releases/download/v1.9.1/mlc-v1.9.1-linux-arm64.tar.gz -o mlc.tar.gz
```

### Verify Checksums

```bash
# Download checksums
curl -L https://github.com/Fauli/music-janitor/releases/download/v1.9.1/SHA256SUMS -o SHA256SUMS

# Verify (macOS)
shasum -a 256 -c SHA256SUMS

# Verify (Linux)
sha256sum -c SHA256SUMS
```

---

## 🔗 Links

- [v1.9.0 Release Notes](https://github.com/Fauli/music-janitor/releases/tag/v1.9.0) - Original self-healing release
- [Issue Tracker](https://github.com/Fauli/music-janitor/issues)

---

**Diff**: https://github.com/Fauli/music-janitor/compare/v1.9.0...v1.9.1

**Status**: ✅ Critical Fix (recommended for all v1.9.0 users)
