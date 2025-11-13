# Release Notes - v1.7.0

## Version-Aware Clustering

**Release Date**: TBD
**Type**: Minor Release (Breaking Change)

---

## Summary

This release introduces **version-aware clustering** to correctly separate different artistic works (remixes, live performances, acoustic versions) while still grouping duplicate copies of the same recording.

### Breaking Change

⚠️ **Cluster key format has changed**, requiring users to run `mlc plan --force-recluster` after upgrading to v1.7.0.

---

## What's New

### Version-Aware Clustering

**Problem Solved**: Previously, MLC would cluster a studio recording with its remix, live version, or acoustic rendition as "duplicates" because all version suffixes were removed during normalization. This caused incorrect groupings where different artistic works would be treated as duplicates, and one would be discarded.

**Example of old behavior** (v1.6.0 and earlier):
```
Files:
  - "Song Title.mp3"              (studio, 320kbps)
  - "Song Title (Remix).mp3"      (remix, 256kbps)
  - "Song Title (Live).mp3"       (live, 192kbps)

Result: All 3 files clustered together → Remix and Live discarded ❌
```

**New behavior** (v1.7.0):
```
Files:
  - "Song Title.mp3"              (studio, 320kbps)
  - "Song Title (Remix).mp3"      (remix, 256kbps)
  - "Song Title (Live).mp3"       (live, 192kbps)

Result: 3 separate clusters → All files kept ✓
```

### Cluster Key Format Change

**Old format** (v1.6.0):
```
artist_norm|title_base|duration_bucket
```

**New format** (v1.7.0):
```
artist_norm|title_base|version_type|duration_bucket
```

**Examples**:
```
"the beatles|hey jude|studio|423"
"daft punk|get lucky|remix|248"
"nirvana|smells like teen spirit|live|300"
"eric clapton|layla|acoustic|247"
```

### Version Type Detection

MLC now automatically detects version types from song titles with the following precedence:

1. **`live`** - Live performances, concerts, sessions
2. **`acoustic`** - Acoustic/unplugged versions
3. **`remix`** - Remixed versions, edits, extended versions
4. **`demo`** - Demo recordings, alternates, outtakes
5. **`instrumental`** - Instrumental/karaoke versions
6. **`studio`** - Original studio recordings (default)

**Precedence Example**:
- `"Song Title (Live Acoustic)"` → `live` (live takes precedence)
- `"Song Title (Acoustic Remix)"` → `acoustic` (acoustic takes precedence)

### Remastered Versions Still Cluster Correctly

Remastered versions of studio recordings are still correctly identified as duplicates:

```
Files:
  - "Song Title.mp3"              → version_type: studio
  - "Song Title (2011 Remaster).mp3" → version_type: studio

Result: Both cluster together ✓
```

---

## Migration Guide

### After Upgrading to v1.7.0

1. **Force re-clustering** (required):
   ```bash
   mlc plan --db library.db --dest /music --force-recluster
   ```

2. **Why is this needed?**
   - Existing clusters use the old key format without version type
   - Files that should be in separate clusters are currently grouped together
   - Re-clustering fixes these incorrect groupings

3. **What happens during re-clustering?**
   - All clusters are regenerated with new version-aware keys
   - Remixes, live versions, acoustic versions get their own clusters
   - Quality scoring runs again to select best version within each cluster
   - Destination paths are recalculated

4. **Expected behavior changes**:
   - More clusters will be created (remixes/live/acoustic separated)
   - Fewer files marked as duplicates
   - Different files may be selected as "winners" within clusters

---

## Technical Details

### Implementation

**Files modified**:
- `internal/meta/normalize.go` - Added `DetectVersionType()` function
- `internal/cluster/cluster.go` - Updated `GenerateClusterKey()` to include version type
- `internal/meta/normalize_test.go` - Added 40+ test cases for version detection
- `internal/cluster/cluster_test.go` - Updated tests for new cluster key format
- `docs/PROCESS_DETAILS.md` - Updated clustering documentation

**New functions**:
- `DetectVersionType(title string) string` - Detects version type from title
- Updated `removeVersionSuffixes()` to remove ALL version indicators

### Version Detection Keywords

**Live**:
- `live`, `concert`, `session`, `unplugged live`

**Acoustic**:
- `acoustic`, `unplugged`

**Remix**:
- `remix`, `mix`, `edit`, `dub`, `bootleg`, `mashup`, `radio`, `club`, `extended`

**Demo**:
- `demo`, `rough`, `alternate`, `outtake`, `unreleased`

**Instrumental**:
- `instrumental`, `karaoke`, `backing track`

**Studio** (default):
- Everything else, including: `remaster`, `deluxe`, `bonus`, `anniversary`

### Edge Cases Handled

1. **Multiple version indicators**: `"Song (Live Acoustic)"` → `live` (precedence)
2. **Remaster exclusion**: `"Song (2011 Remaster)"` → `studio` (not `remix`)
3. **Edition exclusion**: `"Song (Deluxe Edition)"` → `studio` (not `remix`)
4. **Filename fallback**: Version detection also works on filenames without tags

---

## Testing

### Test Coverage

- **40+ version detection test cases** covering all version types and edge cases
- **Cluster key generation tests** with version type validation
- **Precedence tests** for multiple version indicators
- **Edge case tests** for remaster/edition exclusions

### Validated Scenarios

✓ Studio recording vs. remix separation
✓ Studio recording vs. live separation
✓ Studio recording vs. acoustic separation
✓ Remastered versions cluster with original
✓ Deluxe editions cluster with original
✓ Multiple version indicators (precedence)
✓ Filename-based version detection
✓ Unicode and special characters

---

## Known Issues

None reported yet. Please report issues at: https://github.com/franz/music-janitor/issues

---

## Compatibility

- **Requires**: Go 1.21+, SQLite 3.35+
- **Platforms**: Linux (amd64, arm64), macOS (amd64, arm64)
- **Breaking**: Cluster key format change requires `--force-recluster`

---

## Future Enhancements

See `TODO.md` for upcoming features:

- **v1.7.1**: Stale cluster detection (automatic detection when re-clustering needed)
- **v1.8.0**: Enhanced MusicBrainz integration
- **v2.0.0**: Acoustic fingerprinting support

---

## Credits

Developed with assistance from Claude Code (Anthropic).

Report issues: https://github.com/franz/music-janitor/issues
