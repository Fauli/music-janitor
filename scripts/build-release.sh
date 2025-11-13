#!/bin/bash
#
# build-release.sh - Build multi-platform release binaries
#
# This script builds MLC binaries for multiple platforms and packages them
# into release archives with checksums.
#
# Usage:
#   ./scripts/build-release.sh [version]
#
# If version is not provided, it will be extracted from git tags.

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
BINARY_NAME="mlc"
RELEASE_DIR="releases"
BUILD_DIR="build/release"
DIST_DIR="dist"

# Platform configurations
# Format: "GOOS/GOARCH/filename-suffix"
PLATFORMS=(
    "darwin/amd64/darwin-amd64"
    "darwin/arm64/darwin-arm64"
    "linux/amd64/linux-amd64"
    "linux/arm64/linux-arm64"
)

# Determine version
if [ -n "$1" ]; then
    VERSION="$1"
else
    # Try to get version from git
    VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
fi

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  MLC Release Builder${NC}"
echo -e "${BLUE}========================================${NC}"
echo -e "${GREEN}Version:${NC} $VERSION"
echo ""

# Clean previous builds
echo -e "${YELLOW}Cleaning previous builds...${NC}"
rm -rf "$RELEASE_DIR" "$BUILD_DIR" "$DIST_DIR"
mkdir -p "$RELEASE_DIR" "$BUILD_DIR" "$DIST_DIR"

# Build for each platform
echo -e "${YELLOW}Building binaries...${NC}"
for platform in "${PLATFORMS[@]}"; do
    IFS='/' read -r -a parts <<< "$platform"
    GOOS="${parts[0]}"
    GOARCH="${parts[1]}"
    SUFFIX="${parts[2]}"

    OUTPUT_NAME="${BINARY_NAME}-${SUFFIX}"
    if [ "$GOOS" = "windows" ]; then
        OUTPUT_NAME="${OUTPUT_NAME}.exe"
    fi

    echo -e "  ${GREEN}→${NC} Building ${GOOS}/${GOARCH}..."

    GOOS=$GOOS GOARCH=$GOARCH go build \
        -ldflags "-X main.Version=$VERSION" \
        -trimpath \
        -o "$BUILD_DIR/$OUTPUT_NAME" \
        ./cmd/mlc

    if [ $? -eq 0 ]; then
        echo -e "    ${GREEN}✓${NC} $OUTPUT_NAME"
    else
        echo -e "    ${RED}✗${NC} Failed to build $OUTPUT_NAME"
        exit 1
    fi
done

echo ""

# Create release packages
echo -e "${YELLOW}Creating release packages...${NC}"

for platform in "${PLATFORMS[@]}"; do
    IFS='/' read -r -a parts <<< "$platform"
    GOOS="${parts[0]}"
    GOARCH="${parts[1]}"
    SUFFIX="${parts[2]}"

    BINARY="${BINARY_NAME}-${SUFFIX}"
    if [ "$GOOS" = "windows" ]; then
        BINARY="${BINARY}.exe"
    fi

    ARCHIVE_NAME="${BINARY_NAME}-${VERSION}-${SUFFIX}"
    ARCHIVE_DIR="$DIST_DIR/$ARCHIVE_NAME"

    echo -e "  ${GREEN}→${NC} Packaging ${SUFFIX}..."

    # Create package directory
    mkdir -p "$ARCHIVE_DIR"

    # Copy binary
    cp "$BUILD_DIR/$BINARY" "$ARCHIVE_DIR/$BINARY_NAME"
    if [ "$GOOS" = "windows" ]; then
        mv "$ARCHIVE_DIR/$BINARY_NAME" "$ARCHIVE_DIR/${BINARY_NAME}.exe"
    fi

    # Copy documentation
    cp README.md "$ARCHIVE_DIR/" 2>/dev/null || echo "README.md not found (skipped)"
    cp LICENSE "$ARCHIVE_DIR/" 2>/dev/null || echo "LICENSE not found (skipped)"

    # Copy release notes if they exist
    if [ -f "docs/RELEASE_NOTES_${VERSION}.md" ]; then
        cp "docs/RELEASE_NOTES_${VERSION}.md" "$ARCHIVE_DIR/RELEASE_NOTES.md"
    elif [ -f "docs/RELEASE_NOTES_v${VERSION}.md" ]; then
        cp "docs/RELEASE_NOTES_v${VERSION}.md" "$ARCHIVE_DIR/RELEASE_NOTES.md"
    fi

    # Create archive based on platform
    cd "$DIST_DIR"
    if [ "$GOOS" = "windows" ]; then
        # Create ZIP for Windows
        zip -r -q "${ARCHIVE_NAME}.zip" "$ARCHIVE_NAME"
        echo -e "    ${GREEN}✓${NC} ${ARCHIVE_NAME}.zip"
    else
        # Create tar.gz for Unix-like systems
        tar -czf "${ARCHIVE_NAME}.tar.gz" "$ARCHIVE_NAME"
        echo -e "    ${GREEN}✓${NC} ${ARCHIVE_NAME}.tar.gz"
    fi
    cd - > /dev/null

    # Clean up package directory
    rm -rf "$ARCHIVE_DIR"
done

echo ""

# Generate checksums
echo -e "${YELLOW}Generating checksums...${NC}"
cd "$DIST_DIR"

# SHA256 checksums
sha256sum *.tar.gz *.zip 2>/dev/null > SHA256SUMS || \
    shasum -a 256 *.tar.gz *.zip 2>/dev/null > SHA256SUMS || \
    echo -e "${RED}Warning: Could not generate SHA256 checksums${NC}"

if [ -f SHA256SUMS ]; then
    echo -e "  ${GREEN}✓${NC} SHA256SUMS created"
fi

cd - > /dev/null

# Move everything to releases directory
mv "$DIST_DIR"/* "$RELEASE_DIR/"
rmdir "$DIST_DIR"

echo ""

# Display release contents
echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  Release Files${NC}"
echo -e "${BLUE}========================================${NC}"
ls -lh "$RELEASE_DIR"

echo ""

# Display checksums
if [ -f "$RELEASE_DIR/SHA256SUMS" ]; then
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}  SHA256 Checksums${NC}"
    echo -e "${BLUE}========================================${NC}"
    cat "$RELEASE_DIR/SHA256SUMS"
    echo ""
fi

# Summary
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  Build Complete!${NC}"
echo -e "${GREEN}========================================${NC}"
echo -e "Version:  ${GREEN}$VERSION${NC}"
echo -e "Location: ${GREEN}$RELEASE_DIR/${NC}"
echo ""
echo -e "To test a binary:"
echo -e "  ${BLUE}tar -xzf $RELEASE_DIR/${BINARY_NAME}-${VERSION}-darwin-arm64.tar.gz${NC}"
echo -e "  ${BLUE}./${BINARY_NAME} --version${NC}"
echo ""
echo -e "To create a GitHub release:"
echo -e "  ${BLUE}gh release create $VERSION $RELEASE_DIR/* --title \"$VERSION\" --notes-file docs/RELEASE_NOTES_${VERSION}.md${NC}"
echo ""
