# Release Notes — v1.4.0

**Release Date:** 2025-11-05
**Focus:** MusicBrainz Artist Name Normalization

---

## 🎵 Major Feature: MusicBrainz Integration

MLC now integrates with the **MusicBrainz** music database for professional-grade artist name normalization! This automatically resolves artist aliases and variations, dramatically improving deduplication accuracy.

### What's New

**Automatic Artist Alias Resolution:**
- "The Beatles" = "Beatles" = "the beatles" → All clustered together ✅
- "The Weeknd" = "Weeknd" → Same artist ✅
- Handles international artists with multiple spellings
- Professional music database with 2M+ artists

**Smart Caching:**
- Database-backed cache (never queries same artist twice)
- Permanent storage (cache persists forever)
- Works offline after initial preload
- Hit counter for analytics

**Performance Optimized:**
- Rate limiting (respects MusicBrainz 1 req/sec limit)
- Preload mode for batch operations
- Graceful fallback to local rules if API unavailable
- Configurable via CLI or config file

### New CLI Flags

```bash
--musicbrainz              # Enable MusicBrainz lookups
--musicbrainz-preload      # Batch preload all artists (recommended)
```

### Usage Examples

**Basic (on-demand lookups):**
```bash
mlc plan --dest /clean --musicbrainz
```

**Recommended (preload for large libraries):**
```bash
# First time - preload all artists
mlc plan --dest /clean --musicbrainz --musicbrainz-preload

# Subsequent runs use cache (instant)
mlc plan --dest /clean --musicbrainz
```

**Config file:**
```yaml
# configs/my-library.yaml
musicbrainz: true
musicbrainz_preload: false  # Optional
```

### How It Works

**Before (v1.3.0):**
```
"The Beatles" → cluster1
"Beatles"     → cluster2  ❌ Different clusters
"the beatles" → cluster3  ❌ Different clusters
```

**After (v1.4.0 with MusicBrainz):**
```
"The Beatles" → MusicBrainz → "The Beatles" (canonical) → cluster1
"Beatles"     → MusicBrainz → "The Beatles" (canonical) → cluster1  ✅
"the beatles" → cache hit   → "The Beatles" (canonical) → cluster1  ✅
```

### Technical Details

**New Components:**
- `internal/musicbrainz/client.go` - HTTP client with rate limiting
- `internal/musicbrainz/cache.go` - Database-backed cache
- `musicbrainz_cache` table - Stores canonical names + aliases
- `Store.GetAllUniqueArtists()` - Helper for preloading

**Database Schema:**
```sql
CREATE TABLE musicbrainz_cache (
    search_name TEXT PRIMARY KEY,
    canonical_name TEXT NOT NULL,
    mbid TEXT,
    aliases TEXT,
    score INTEGER,
    cached_at DATETIME,
    hit_count INTEGER
);
```

---

## 📊 Benefits

**Improved Deduplication:**
- Catches artist name variations that were previously missed
- Reduces false negatives (same artist, different names)
- Professional-grade music metadata normalization

**Better Organization:**
- Consistent artist folder names
- Reduces "Beatles" vs "The Beatles" folder duplicates
- Cleaner destination library structure

**Time Savings:**
- No manual alias map maintenance
- Automatic resolution of 1000s of artist variations
- One-time preload (~8 min for 500 artists)

---

## 📦 What's Included

### Core Implementation
- ✅ MusicBrainz API client (HTTP client, rate limiting, error handling)
- ✅ Database caching (permanent cache, hit tracking)
- ✅ Integration with clustering pipeline (transparent normalization)
- ✅ Preload support (batch operations for large libraries)
- ✅ CLI flags (`--musicbrainz`, `--musicbrainz-preload`)
- ✅ Config file support (`musicbrainz: true`)

### Quality Assurance
- ✅ Unit tests (rate limiting)
- ✅ Integration tests (real API calls, skippable with `-short`)
- ✅ Graceful fallback (continues without MusicBrainz if API fails)
- ✅ Comprehensive error handling

### Documentation
- ✅ README section with examples
- ✅ Troubleshooting guide
- ✅ Performance notes
- ✅ Privacy considerations

---

## 🔄 Upgrading from v1.3.0

**No breaking changes!** MusicBrainz is opt-in.

**To enable:**
```bash
# Option 1: CLI flag
mlc plan --dest /clean --musicbrainz

# Option 2: Config file
echo "musicbrainz: true" >> configs/my-library.yaml
```

**Database migration:**
- `musicbrainz_cache` table created automatically
- No action required - schema migration is automatic

**Recommended workflow:**
```bash
# 1. Rescan if needed (no MusicBrainz yet)
mlc rescan --db library.db

# 2. Plan with MusicBrainz (preload recommended)
mlc plan --dest /clean --db library.db --musicbrainz --musicbrainz-preload

# 3. Execute as normal
mlc execute --db library.db
```

---

## 📝 Configuration Reference

### CLI Flags
| Flag | Default | Description |
|------|---------|-------------|
| `--musicbrainz` | `false` | Enable MusicBrainz artist normalization |
| `--musicbrainz-preload` | `false` | Preload all artists before clustering |

### Config File
```yaml
musicbrainz: true              # Enable MusicBrainz
musicbrainz_preload: false     # Preload mode (optional)
```

### Environment Variables
```bash
export MLC_MUSICBRAINZ=true
export MLC_MUSICBRAINZ_PRELOAD=true
```

---

## 🐛 Known Issues

None reported at release time.

**Potential considerations:**
- First preload run requires internet (1 req/sec API limit)
- API rate limit errors (503) → Wait 60 seconds and retry
- Low-confidence matches (<90%) use original name (safe fallback)

---

## 📈 Performance Notes

**Preload Performance:**
- 100 artists: ~2 minutes
- 500 artists: ~8 minutes
- 1000 artists: ~17 minutes

**After Preload:**
- Clustering: Instant (database lookup only)
- No network requests (100% cached)
- Works completely offline

**Recommendation:**
- Libraries <100 unique artists: Use `--musicbrainz` (on-demand)
- Libraries >100 unique artists: Use `--musicbrainz-preload` (batch)

---

## 🎯 Use Cases

**Perfect for:**
- Libraries with artist name variations
- International music collections (multiple language spellings)
- Compilations with inconsistent artist tags
- Large libraries requiring professional-grade deduplication

**Skip if:**
- Artist tags are already consistent
- No internet connectivity
- Small library (<100 files)
- Want fastest possible clustering

---

## 🙏 Acknowledgments

- **MusicBrainz Foundation** - For providing the excellent music database API
- **Community** - For requesting this feature

---

## 🔗 Links

- **Documentation:** [README.md](README.md#musicbrainz-integration-artist-name-normalization)
- **MusicBrainz API:** https://musicbrainz.org/doc/MusicBrainz_API
- **Issue Tracker:** https://github.com/franz/music-janitor/issues
- **Previous Release:** [v1.3.0](RELEASE_NOTES_v1.3.0.md)

---

## ⬇️ Download

### macOS (Apple Silicon)
```bash
curl -L https://github.com/franz/music-janitor/releases/download/v1.4.0/mlc-darwin-arm64 -o mlc
chmod +x mlc
```

### macOS (Intel)
```bash
curl -L https://github.com/franz/music-janitor/releases/download/v1.4.0/mlc-darwin-amd64 -o mlc
chmod +x mlc
```

### Linux (x64)
```bash
curl -L https://github.com/franz/music-janitor/releases/download/v1.4.0/mlc-linux-amd64 -o mlc
chmod +x mlc
```

### Build from source
```bash
git clone https://github.com/franz/music-janitor.git
cd music-janitor
git checkout v1.4.0
make build
```

---

**Full Changelog:** https://github.com/franz/music-janitor/compare/v1.3.0...v1.4.0
