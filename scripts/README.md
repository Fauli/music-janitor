# Build Scripts

This directory contains scripts for building and releasing MLC.

## build-release.sh

Builds multi-platform release packages for MLC.

### Usage

```bash
# Build release for current git version
./scripts/build-release.sh

# Build release for specific version
./scripts/build-release.sh v1.7.0
```

### What it does

1. **Cleans** previous builds
2. **Builds** binaries for all platforms:
   - macOS (Intel & Apple Silicon)
   - Linux (amd64 & arm64)
3. **Packages** each binary into platform-specific archives:
   - `.tar.gz` for Unix-like systems
   - Includes: binary, README.md, RELEASE_NOTES.md
4. **Generates** SHA256 checksums
5. **Places** everything in `releases/` directory

### Output Structure

```
releases/
├── mlc-v1.7.0-darwin-amd64.tar.gz
├── mlc-v1.7.0-darwin-arm64.tar.gz
├── mlc-v1.7.0-linux-amd64.tar.gz
├── mlc-v1.7.0-linux-arm64.tar.gz
└── SHA256SUMS
```

### Verifying Checksums

```bash
cd releases
shasum -a 256 -c SHA256SUMS
```

### Testing a Release

```bash
# Extract
tar -xzf mlc-v1.7.0-darwin-arm64.tar.gz

# Run
./mlc-v1.7.0-darwin-arm64/mlc --version
```

### Creating a GitHub Release

```bash
# Using GitHub CLI
gh release create v1.7.0 releases/* \
  --title "v1.7.0 - Version-Aware Clustering" \
  --notes-file docs/RELEASE_NOTES_v1.7.0.md
```

## Makefile Integration

The script is integrated into the Makefile:

```bash
# Build release packages
make release-package

# Build binaries only (no packaging)
make release-all
```

## Requirements

- Go 1.21+
- `tar` (Unix-like systems)
- `zip` (Windows packages - currently not built)
- `shasum` or `sha256sum` (checksum generation)
- `git` (for version detection)

## Platforms

Currently building for:
- **macOS**: amd64 (Intel), arm64 (Apple Silicon)
- **Linux**: amd64, arm64

Windows builds are excluded due to syscall compatibility issues in the codebase.

## Troubleshooting

### Build fails for a platform

Check the error message. Common issues:
- Missing Go cross-compilation support
- Platform-specific code not properly tagged
- Syscall usage not compatible with target platform

### Checksums fail

Ensure `shasum` or `sha256sum` is installed:
```bash
# macOS/Linux
which shasum
which sha256sum
```

### Version is "dirty"

You have uncommitted changes. Commit them first:
```bash
git status
git add .
git commit -m "your changes"
```

## Customization

Edit the `PLATFORMS` array in the script to add/remove platforms:

```bash
PLATFORMS=(
    "darwin/amd64/darwin-amd64"
    "darwin/arm64/darwin-arm64"
    "linux/amd64/linux-amd64"
    "linux/arm64/linux-arm64"
    # Add more platforms:
    # "windows/amd64/windows-amd64"
    # "freebsd/amd64/freebsd-amd64"
)
```
