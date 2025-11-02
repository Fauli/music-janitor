#!/bin/bash
# Generate test audio fixtures for metadata extraction tests
# Requires: ffmpeg

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT_DIR="$SCRIPT_DIR"

echo "=== Generating Test Audio Fixtures ==="
echo "Output directory: $OUTPUT_DIR"
echo ""

# Check for ffmpeg
if ! command -v ffmpeg &> /dev/null; then
    echo "Error: ffmpeg is required but not installed"
    echo "Install: brew install ffmpeg (macOS) or apt-get install ffmpeg (Linux)"
    exit 1
fi

# Generate a 2-second sine wave (440 Hz A note)
generate_audio() {
    local format=$1
    local filename=$2
    local codec=$3
    local extra_opts=$4

    echo "Generating: $filename"

    ffmpeg -f lavfi -i "sine=frequency=440:duration=2" \
        -c:a "$codec" $extra_opts \
        -y "$OUTPUT_DIR/$filename" 2>&1 | grep -v "^ffmpeg version" || true
}

# Generate audio with metadata tags
generate_with_tags() {
    local format=$1
    local filename=$2
    local codec=$3
    local extra_opts=$4

    echo "Generating with tags: $filename"

    ffmpeg -f lavfi -i "sine=frequency=440:duration=2" \
        -c:a "$codec" $extra_opts \
        -metadata title="Test Song" \
        -metadata artist="Test Artist" \
        -metadata album="Test Album" \
        -metadata album_artist="Test Album Artist" \
        -metadata date="2023" \
        -metadata track="1/10" \
        -metadata disc="1/2" \
        -metadata genre="Test Genre" \
        -y "$OUTPUT_DIR/$filename" 2>&1 | grep -v "^ffmpeg version" || true
}

echo "1. Generating MP3 files..."
# MP3 CBR 320kbps
generate_audio mp3 "test-mp3-320.mp3" libmp3lame "-b:a 320k"
# MP3 VBR V0
generate_audio mp3 "test-mp3-v0.mp3" libmp3lame "-q:a 0"
# MP3 with tags
generate_with_tags mp3 "test-mp3-tagged.mp3" libmp3lame "-b:a 192k"

echo ""
echo "2. Generating FLAC files..."
# FLAC 16-bit 44.1kHz
generate_audio flac "test-flac-16-44.flac" flac "-sample_fmt s16 -ar 44100"
# FLAC 24-bit 96kHz
generate_audio flac "test-flac-24-96.flac" flac "-sample_fmt s32 -ar 96000"
# FLAC with tags
generate_with_tags flac "test-flac-tagged.flac" flac "-sample_fmt s16 -ar 44100"

echo ""
echo "3. Generating M4A/AAC files..."
# M4A AAC
generate_audio m4a "test-aac-256.m4a" aac "-b:a 256k"
# M4A with tags
generate_with_tags m4a "test-aac-tagged.m4a" aac "-b:a 192k"

echo ""
echo "4. Generating OGG Vorbis files..."
# OGG Vorbis
generate_audio ogg "test-vorbis-q6.ogg" libvorbis "-q:a 6"
# OGG with tags
generate_with_tags ogg "test-vorbis-tagged.ogg" libvorbis "-q:a 5"

echo ""
echo "5. Generating Opus files..."
# Opus
generate_audio opus "test-opus-128.opus" libopus "-b:a 128k"
# Opus with tags
generate_with_tags opus "test-opus-tagged.opus" libopus "-b:a 96k"

echo ""
echo "6. Generating WAV files..."
# WAV PCM 16-bit
generate_audio wav "test-wav-16.wav" pcm_s16le "-ar 44100"
# WAV PCM 24-bit
generate_audio wav "test-wav-24.wav" pcm_s24le "-ar 48000"

echo ""
echo "7. Generating AIFF files..."
# AIFF PCM 16-bit
generate_audio aiff "test-aiff-16.aiff" pcm_s16be "-ar 44100"
# AIFF PCM 24-bit
generate_audio aiff "test-aiff-24.aiff" pcm_s24be "-ar 48000"
# AIFF with tags (ID3 tags in AIFF)
generate_with_tags aiff "test-aiff-tagged.aiff" pcm_s16be "-ar 44100"

echo ""
echo "8. Generating edge case files..."

# Very short file (0.1 seconds)
echo "Generating: test-short.mp3 (very short)"
ffmpeg -f lavfi -i "sine=frequency=440:duration=0.1" \
    -c:a libmp3lame -b:a 192k \
    -y "$OUTPUT_DIR/test-short.mp3" 2>&1 | grep -v "^ffmpeg version" || true

# File with no tags
echo "Generating: test-no-tags.mp3"
ffmpeg -f lavfi -i "sine=frequency=440:duration=2" \
    -c:a libmp3lame -b:a 192k \
    -map_metadata -1 \
    -y "$OUTPUT_DIR/test-no-tags.mp3" 2>&1 | grep -v "^ffmpeg version" || true

# File with unicode in tags
echo "Generating: test-unicode.mp3"
ffmpeg -f lavfi -i "sine=frequency=440:duration=2" \
    -c:a libmp3lame -b:a 192k \
    -metadata title="Café über 北京 🎵" \
    -metadata artist="Björk & Señor López" \
    -metadata album="Αλφαβήτα (Album)" \
    -y "$OUTPUT_DIR/test-unicode.mp3" 2>&1 | grep -v "^ffmpeg version" || true

# Stereo and mono files
echo "Generating: test-stereo.flac"
ffmpeg -f lavfi -i "sine=frequency=440:duration=2" \
    -c:a flac -ac 2 \
    -y "$OUTPUT_DIR/test-stereo.flac" 2>&1 | grep -v "^ffmpeg version" || true

echo "Generating: test-mono.flac"
ffmpeg -f lavfi -i "sine=frequency=440:duration=2" \
    -c:a flac -ac 1 \
    -y "$OUTPUT_DIR/test-mono.flac" 2>&1 | grep -v "^ffmpeg version" || true

echo ""
echo "9. Creating corrupted/edge case files..."

# Empty file (should fail to parse)
echo "Creating: test-corrupt.mp3 (empty file)"
touch "$OUTPUT_DIR/test-corrupt.mp3"

# Truncated file
echo "Creating: test-truncated.flac (truncated)"
dd if="$OUTPUT_DIR/test-flac-16-44.flac" of="$OUTPUT_DIR/test-truncated.flac" bs=1024 count=2 2>/dev/null

echo ""
echo "=== Summary ==="
echo "Generated test fixtures in: $OUTPUT_DIR"
ls -lh "$OUTPUT_DIR"/*.{mp3,flac,m4a,ogg,opus,wav,aiff} 2>/dev/null | awk '{print $9, $5}'
echo ""
echo "Test files include:"
echo "  - MP3: CBR 320k, VBR V0, with/without tags"
echo "  - FLAC: 16/44.1, 24/96, with/without tags"
echo "  - M4A/AAC: 256k, 192k with tags"
echo "  - OGG Vorbis: q6, q5 with tags"
echo "  - Opus: 128k, 96k with tags"
echo "  - WAV: 16-bit, 24-bit PCM"
echo "  - AIFF: 16-bit, 24-bit PCM, with tags"
echo "  - Edge cases: short, no tags, unicode, stereo/mono, corrupted"
