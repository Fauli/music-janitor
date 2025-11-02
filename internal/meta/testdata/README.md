# Test Audio Fixtures

This directory contains a script to generate test audio files for metadata extraction testing.

## Prerequisites

- **ffmpeg** must be installed:
  ```bash
  # macOS
  brew install ffmpeg

  # Linux (Debian/Ubuntu)
  sudo apt-get install ffmpeg

  # Linux (Fedora/RHEL)
  sudo dnf install ffmpeg
  ```

## Generating Fixtures

Run the generator script:

```bash
cd internal/meta/testdata
chmod +x generate-fixtures.sh
./generate-fixtures.sh
```

This will create approximately 25 test audio files in various formats:

### Generated Files

**MP3 Files:**
- `test-mp3-320.mp3` — MP3 CBR 320kbps (no tags)
- `test-mp3-v0.mp3` — MP3 VBR V0 (no tags)
- `test-mp3-tagged.mp3` — MP3 192kbps with ID3v2 tags

**FLAC Files:**
- `test-flac-16-44.flac` — FLAC 16-bit 44.1kHz (no tags)
- `test-flac-24-96.flac` — FLAC 24-bit 96kHz (no tags)
- `test-flac-tagged.flac` — FLAC 16-bit with Vorbis comments

**M4A/AAC Files:**
- `test-aac-256.m4a` — AAC 256kbps (no tags)
- `test-aac-tagged.m4a` — AAC 192kbps with MP4 tags

**OGG Vorbis Files:**
- `test-vorbis-q6.ogg` — Vorbis quality 6 (no tags)
- `test-vorbis-tagged.ogg` — Vorbis quality 5 with comments

**Opus Files:**
- `test-opus-128.opus` — Opus 128kbps (no tags)
- `test-opus-tagged.opus` — Opus 96kbps with tags

**WAV Files:**
- `test-wav-16.wav` — WAV PCM 16-bit 44.1kHz
- `test-wav-24.wav` — WAV PCM 24-bit 48kHz

**AIFF Files:**
- `test-aiff-16.aiff` — AIFF PCM 16-bit 44.1kHz (big-endian)
- `test-aiff-24.aiff` — AIFF PCM 24-bit 48kHz
- `test-aiff-tagged.aiff` — AIFF 16-bit with ID3 tags

**Edge Cases:**
- `test-short.mp3` — Very short file (0.1 seconds)
- `test-no-tags.mp3` — MP3 with all metadata stripped
- `test-unicode.mp3` — Unicode characters in tags (Café, 北京, emoji)
- `test-stereo.flac` — Stereo (2 channels)
- `test-mono.flac` — Mono (1 channel)
- `test-corrupt.mp3` — Empty file (should fail)
- `test-truncated.flac` — Truncated file (should fail)

### Tagged Files Include

All files with "tagged" in the filename include these metadata fields:
- **Title:** "Test Song"
- **Artist:** "Test Artist"
- **Album:** "Test Album"
- **Album Artist:** "Test Album Artist"
- **Date:** "2023"
- **Track:** "1/10"
- **Disc:** "1/2"
- **Genre:** "Test Genre"

## Running Integration Tests

After generating fixtures, run the integration tests:

```bash
# From repository root
cd /Users/franz/git/music-janitor

# Run integration tests
go test -v ./internal/meta -run TestMetadataExtractionIntegration

# Run all meta tests including integration
go test -v ./internal/meta
```

## File Sizes

Generated files are small (typically 20-50 KB each) since they contain only 2 seconds of a simple 440 Hz sine wave. Total size for all fixtures: ~1-2 MB.

## What Tests Validate

The integration tests verify:

1. **Format Detection:** Correct identification of MP3, FLAC, M4A, OGG, Opus, WAV, AIFF
2. **Codec Detection:** Proper codec identification for each format
3. **Lossless Detection:** Correctly identifies lossless vs lossy formats
4. **Tag Extraction:** Reads ID3v2, Vorbis Comments, MP4 tags
5. **Audio Properties:** Sample rate, bit depth, channels, duration, bitrate
6. **Unicode Handling:** Non-ASCII characters in tags
7. **Edge Cases:** Short files, missing tags, corrupted files
8. **AIFF Bit Depth Bug:** Validates the IntOrString fix for AIFF files
9. **Filename Enrichment:** Metadata inference from filenames and paths
10. **Resumability:** Skips already-processed files

## Cleanup

Generated audio files are gitignored and can be safely deleted:

```bash
rm -f *.mp3 *.flac *.m4a *.ogg *.opus *.wav *.aiff
```

Then regenerate when needed with `./generate-fixtures.sh`.
