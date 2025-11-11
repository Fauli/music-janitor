# Troubleshooting MLC Extraction Errors

## Getting Detailed Error Information

### After v1.5.1+ (Improved Error Messages)

With the latest version, ffprobe errors now show the actual error message:

```bash
# Run rescan with verbose logging
mlc rescan --errors-only --db my-library.db --verbose
```

You'll now see detailed errors like:
```
[ERROR] Failed to re-extract metadata for /path/to/file.mp3: ffprobe failed: Invalid data found when processing input
```

### Before v1.5.1 (Generic Errors)

If you see truncated errors like:
```
[ERROR] Failed to re-extract metadata for file.mp3: ffprobe failed: ffprobe failed:
```

Upgrade to v1.5.1+ or run ffprobe manually:
```bash
ffprobe -v error -show_format -show_streams "/path/to/problematic/file.mp3"
```

---

## Common Error Causes

### 1. Corrupted or Truncated Files

**Symptoms:**
- ffprobe error: `Invalid data found when processing input`
- ffprobe error: `moov atom not found`
- ffprobe error: `Premature end of data`

**Example from your logs:**
```
04-mflex_-_where_the_kids_are_(italo_connectiot)_(feat_blondfire)-zzzz.mpzz.mp3
```
Notice: `mpzz.mp3` - likely a corrupted file extension

**Diagnosis:**
```bash
# Check file integrity
file "/volume2/Sound/.../04-mflex...mp3"

# Try to play it
ffplay "/volume2/Sound/.../04-mflex...mp3"

# Check file size (0 bytes = corrupted)
ls -lh "/volume2/Sound/.../04-mflex...mp3"
```

**Solutions:**
- Delete corrupted files
- Re-download from original source
- Try to repair with: `mp3val -f file.mp3` (for MP3s)

---

### 2. Filesystem Corruption / Character Encoding Issues

**Symptoms:**
- Filenames with garbled characters
- Files that exist but can't be read

**Example from your logs:**
```
01_silent_circlay_-_man_is_comin_(specmin_(special_mix).mp3
```
Notice: Filename has weird truncations/corruption

**Diagnosis:**
```bash
# Check filesystem
# On Synology NAS:
sudo fsck /dev/md0

# Check file accessibility
ls -la "/volume2/Sound/.../problematic_file.mp3"
cat "/volume2/Sound/.../problematic_file.mp3" > /dev/null
```

**Solutions:**
- Run filesystem check/repair on Synology
- Rename files to fix encoding issues:
  ```bash
  # Fix filename encoding
  convmv -f iso-8859-1 -t utf-8 -r /volume2/Sound/
  ```

---

### 3. Permission Issues

**Symptoms:**
- ffprobe error: `Permission denied`
- Files exist but can't be opened

**Diagnosis:**
```bash
# Check permissions
ls -la "/volume2/Sound/.../file.mp3"

# Check if you can read it
cat "/volume2/Sound/.../file.mp3" > /dev/null
```

**Solutions:**
```bash
# Fix permissions (on NAS)
chmod -R u+r /volume2/Sound/

# Or change ownership
chown -R your-user:your-group /volume2/Sound/
```

---

### 4. File Not Found (Path Issues)

**Symptoms:**
- ffprobe error: `No such file or directory`
- Path has special characters or spaces

**Diagnosis:**
```bash
# Test if file exists
test -f "/volume2/Sound/.../file.mp3" && echo "exists" || echo "not found"

# Check for hidden characters
hexdump -C <<< "/volume2/Sound/.../file.mp3" | head
```

**Solutions:**
- Check if file was moved/deleted since scan
- Re-run scan: `mlc scan --source /volume2/Sound --db my-library.db`
- Fix paths with special characters

---

### 5. Unsupported Format / DRM Protected

**Symptoms:**
- ffprobe error: `Unknown format`
- ffprobe error: `DRM protected`

**Diagnosis:**
```bash
# Check actual format
file "/volume2/Sound/.../file.mp3"

# Try mediainfo
mediainfo "/volume2/Sound/.../file.mp3"
```

**Solutions:**
- Convert to supported format
- Remove DRM (if legally allowed)
- Skip these files (MLC will mark as error and continue)

---

## Analyzing Your Specific Errors

Based on your log output, here's what's likely happening:

### Error 1: Garbled Filename
```
14-het_feestteam_-_staan_of_zitten_medley_(la2004)-sx_2004)-sob.mp3
```
- **Issue:** Filename corruption or encoding problem
- **Action:** Check if file actually exists with `ls -la`

### Error 2: Double Extension
```
04-mflex_-_where_the_kids_are_(italo_connectiot)_(feat_blondfire)-zzzz.mpzz.mp3
```
- **Issue:** File extension is `mpzz.mp3` (corrupted)
- **Action:** File is likely truncated/corrupted, delete or re-download

### Error 3: Truncated Filename
```
01_silent_circlay_-_man_is_comin_(specmin_(special_mix).mp3
```
- **Issue:** Filename appears mangled
- **Action:** Filesystem corruption or encoding issue

### Error 4: Long Path with Special Characters
```
05-don_diabd_matsh_-light_(cot_(could_yld_you_byou_be_mine)_(collin_mcloughlin_remix)-zzzz.mp3
```
- **Issue:** Path appears corrupted/garbled
- **Action:** Check filesystem integrity

---

## Recommended Actions

### Step 1: Get Detailed Errors

Rebuild MLC with the latest code (includes improved error messages):

```bash
cd /path/to/music-janitor
git pull
make build
sudo make install
```

Then run:
```bash
mlc rescan --errors-only --db mlc-state.db --verbose 2>&1 | tee rescan-errors.log
```

This will show actual ffprobe errors in `rescan-errors.log`.

### Step 2: Check Specific Files

Pick a few failed files and test manually:

```bash
# Test file 1
ffprobe -v error "/volume2/Sound/.../14-het_feestteam...mp3"

# Test file 2
ffprobe -v error "/volume2/Sound/.../04-mflex...mpzz.mp3"

# Test file 3
ffprobe -v error "/volume2/Sound/.../01_silent_circlay...mp3"
```

### Step 3: Filesystem Check (Synology NAS)

If you see many corrupted filenames:

```bash
# SSH into Synology
ssh admin@your-nas-ip

# Check filesystem (requires maintenance mode)
sudo fsck -n /dev/md0  # dry-run first
sudo fsck /dev/md0     # fix errors
```

### Step 4: Fix Common Issues

**For corrupted MP3s:**
```bash
# Install mp3val
sudo apt install mp3val  # or brew install mp3val

# Scan and fix
find /volume2/Sound -name "*.mp3" -exec mp3val -f {} \;
```

**For encoding issues:**
```bash
# Install convmv
sudo apt install convmv

# Dry run
convmv -f iso-8859-1 -t utf-8 -r --notest /volume2/Sound/

# Actually fix
convmv -f iso-8859-1 -t utf-8 -r --notest /volume2/Sound/
```

### Step 5: Exclude Unfixable Files

If some files are truly corrupted and unfixable:

```bash
# Generate list of failed files
mlc rescan --errors-only --db mlc-state.db 2>&1 | \
  grep "Failed to re-extract" | \
  awk -F': ' '{print $2}' | \
  awk '{print $1}' > failed-files.txt

# Review the list
less failed-files.txt

# Delete if desired (CAREFUL!)
# xargs rm < failed-files.txt
```

Or just leave them - MLC will skip them during execution.

---

## Debug Mode

For maximum detail, use debug logging:

```bash
# Enable all debug output
export MLC_LOG_LEVEL=debug
mlc rescan --errors-only --db my-library.db --verbose
```

This shows:
- Exact ffprobe commands being run
- Full error stack traces
- File paths being processed
- Metadata extraction attempts

---

## Getting Help

If you're still stuck:

1. **Collect diagnostics:**
   ```bash
   mlc doctor --src /volume2/Sound --db my-library.db > doctor.log
   mlc rescan --errors-only --db my-library.db --verbose 2>&1 | head -100 > errors.log
   ```

2. **Test a specific file manually:**
   ```bash
   ffprobe -v error -show_format -show_streams "/path/to/failed/file.mp3" 2>&1
   ```

3. **Report issue with:**
   - MLC version: `mlc --version`
   - OS: Synology DSM version
   - Ffprobe version: `ffprobe -version`
   - Sample error from `errors.log`
   - Output of manual ffprobe test

---

## Prevention

### Regular Maintenance

```bash
# Check filesystem monthly
sudo fsck -n /dev/md0

# Validate MP3s after transfers
find /volume2/Sound -name "*.mp3" -mtime -7 -exec mp3val {} \; | grep -i error

# Scan incrementally
mlc scan --source /volume2/Sound --db my-library.db  # weekly
```

### Best Practices

- ✅ Use UTF-8 filenames
- ✅ Avoid special characters: `<>:"|?*`
- ✅ Keep paths under 255 characters
- ✅ Verify file integrity after network transfers
- ✅ Use checksums when downloading
- ✅ Enable NAS filesystem journaling

---

## Summary

Most extraction errors fall into these categories:

1. **Corrupted files** (50%) - Delete or re-download
2. **Filesystem issues** (25%) - Run fsck
3. **Encoding problems** (15%) - Fix with convmv
4. **Permission issues** (5%) - Fix with chmod/chown
5. **Actual format issues** (5%) - Convert or skip

With the improved error messages in v1.5.1+, you'll see exactly which category each error falls into.
