# Configuration Guide

MLC supports flexible configuration with multiple layers of precedence.

## Configuration Precedence

Settings are loaded in the following order (later sources override earlier ones):

1. **Config file** (`configs/example.yaml` or via `--config`)
2. **Environment variables** (prefixed with `MLC_`)
3. **Command-line flags** (highest priority)

This allows you to set defaults in a config file and override specific values as needed.

## Configuration Methods

### 1. Config File (YAML)

Create a config file (e.g., `my-library.yaml`):

```yaml
source: "/Volumes/MessyMusic"
destination: "/Volumes/MusicClean"
concurrency: 8
mode: copy
hashing: sha1
dry_run: false
```

Then use it:

```bash
mlc scan --config my-library.yaml
```

By default, mlc looks for `configs/example.yaml` in the current directory.

### 2. Environment Variables

Set environment variables with the `MLC_` prefix:

```bash
export MLC_SOURCE="/Volumes/MessyMusic"
export MLC_DESTINATION="/Volumes/MusicClean"
export MLC_CONCURRENCY=16
export MLC_MODE=copy

mlc scan
```

Environment variable names map to config file keys in uppercase with underscores.

### 3. Command-Line Flags

Override any setting with command-line flags:

```bash
mlc scan \
  --source /Volumes/MessyMusic \
  --dest /Volumes/MusicClean \
  --concurrency 16 \
  --mode copy \
  --dry-run
```

Short flags are available for common options:
- `-s` / `--source`
- `-d` / `--dest`
- `-c` / `--concurrency`
- `-v` / `--verbose`
- `-q` / `--quiet`

## Available Options

### Core Paths

| Flag | Env Var | Config | Description |
|------|---------|--------|-------------|
| `-s, --source` | `MLC_SOURCE` | `source` | Source directory to scan |
| `-d, --dest` | `MLC_DESTINATION` | `destination` | Destination directory for clean library |
| `--db` | `MLC_DB` | `db` | State database file path |

### Execution Options

| Flag | Env Var | Config | Description |
|------|---------|--------|-------------|
| `--mode` | `MLC_MODE` | `mode` | Execution mode: `copy`, `move`, `hardlink`, `symlink` |
| `-c, --concurrency` | `MLC_CONCURRENCY` | `concurrency` | Number of parallel workers |
| `--layout` | `MLC_LAYOUT` | `layout` | Destination layout: `default`, `alt1`, `alt2` |
| `--dry-run` | `MLC_DRY_RUN` | `dry_run` | Plan without executing (dry-run mode) |

### Quality & Verification

| Flag | Env Var | Config | Description |
|------|---------|--------|-------------|
| `--hashing` | `MLC_HASHING` | `hashing` | Hash algorithm: `sha1`, `xxh3`, `none` |
| `--verify` | `MLC_VERIFY` | `verify` | Verification mode: `size`, `hash`, `full` |
| `--fingerprinting` | `MLC_FINGERPRINTING` | `fingerprinting` | Enable acoustic fingerprinting |

### Duplicate Handling

| Flag | Env Var | Config | Description |
|------|---------|--------|-------------|
| `--duplicates` | `MLC_DUPLICATE_POLICY` | `duplicate_policy` | Duplicate policy: `keep`, `quarantine`, `delete` |
| `--prefer-existing` | `MLC_PREFER_EXISTING` | `prefer_existing` | Prefer existing files on conflict |

### Output Control

| Flag | Env Var | Config | Description |
|------|---------|--------|-------------|
| `-v, --verbose` | `MLC_VERBOSE` | `verbose` | Verbose output (debug logs) |
| `-q, --quiet` | `MLC_QUIET` | `quiet` | Quiet mode (errors only) |

### Performance & Network Options

| Flag | Env Var | Config | Description |
|------|---------|--------|-------------|
| `--nas-mode` | `MLC_NAS_MODE` | `nas_mode` | Enable/disable NAS optimizations (default: auto-detect) |

**NAS Mode Details:**

MLC automatically detects network filesystems (NFS, SMB/CIFS, AFP) and applies optimizations:
- **Auto-detection** (default): Detects network storage and applies tuning automatically
- **`--nas-mode=true`**: Force enable NAS optimizations even if not detected
- **`--nas-mode=false`**: Force disable NAS optimizations even if detected

When NAS mode is active, MLC applies:
- Lower concurrency (4 instead of 8) to reduce network congestion
- Larger I/O buffers (256KB instead of 128KB) for better network throughput
- Retry logic with exponential backoff for transient network failures
- SQLite optimizations (reduced fsync, memory temp store, larger cache)

**Auto-detection messages:**
```
Source on network storage (SMB) - applying optimizations
Concurrency: 4 (NAS-optimized)
Buffer size: 256 KB (NAS-optimized)
```

## Examples

### Example 1: Config File + Flag Override

**my-library.yaml:**
```yaml
source: "/Volumes/MessyMusic"
destination: "/Volumes/MusicClean"
concurrency: 8
mode: copy
```

**Command:**
```bash
# Use config but override concurrency
mlc scan --config my-library.yaml -c 16
```

**Result:** Uses source and destination from config, but runs with 16 workers instead of 8.

### Example 2: Environment Variables

```bash
# Set defaults via environment
export MLC_SOURCE="/Volumes/MessyMusic"
export MLC_CONCURRENCY=12

# Run with env defaults
mlc scan

# Override specific values
mlc scan -c 4  # Uses 4 workers instead of 12
```

### Example 3: Pure Command-Line

```bash
# No config file needed
mlc scan \
  -s /Volumes/MessyMusic \
  --db my-library.db \
  -c 8 \
  -v
```

### Example 4: Dry-Run with Override

```yaml
# prod.yaml
source: "/Volumes/MessyMusic"
destination: "/Volumes/MusicClean"
mode: move  # Dangerous!
```

```bash
# Test safely first
mlc scan --config prod.yaml --mode copy --dry-run

# Review the plan, then execute
mlc execute --config prod.yaml --mode copy
```

### Example 5: NAS Configuration

For libraries on network storage (NAS), use these settings:

**nas-library.yaml:**
```yaml
# Store database locally for best performance
db: "~/mlc-projects/nas-library.db"

# Source and destination on network storage
source: "/Volumes/NAS/MessyMusic"
destination: "/Volumes/NAS/CleanMusic"

# NAS mode auto-detects by default
# Uncomment to force enable/disable:
# nas_mode: true

# Use hash verification for network transfers
verify: hash
mode: copy

# Concurrency will be auto-tuned (default 4 for NAS)
# Override if you know your network can handle more:
# concurrency: 6
```

**Usage:**
```bash
# Scan with auto-detection
mlc scan --config nas-library.yaml -v

# Output shows:
# "Source on network storage (SMB) - applying optimizations"
# "Database on local storage - optimal configuration"

# Plan and execute
mlc plan --config nas-library.yaml
mlc execute --config nas-library.yaml

# If auto-detection is wrong, override:
mlc scan --config nas-library.yaml --nas-mode=false
```

**Key points for NAS:**
- Keep database on local SSD/disk (not on NAS)
- Auto-tuning reduces concurrency to avoid network congestion
- Larger buffers (256KB) improve throughput
- Automatic retry on transient network failures
- Use hash verification for data integrity

## Best Practices

### 1. **Use config files for persistent settings**
   - Store your library-specific settings in a YAML file
   - Commit sanitized configs to version control (remove sensitive paths)

### 2. **Use environment variables for user-specific settings**
   - Good for CI/CD pipelines
   - User-specific paths without modifying config files

### 3. **Use flags for one-off changes**
   - Testing different concurrency levels
   - Dry-run mode before executing
   - Overriding mode temporarily

### 4. **Safety pattern**
   ```bash
   # Always dry-run first
   mlc plan --config my-library.yaml --dry-run

   # Review the plan in artifacts/plans/

   # Execute with explicit mode
   mlc execute --config my-library.yaml --mode copy
   ```

### 5. **NAS/Network Storage Best Practices**
   - **Always keep database local**: Store `.db` file on local SSD, not on NAS
   - **Trust auto-detection**: Let MLC detect network storage automatically
   - **Use hash verification**: Set `verify: hash` for network transfers
   - **Monitor with verbose**: Use `-v` flag to see optimization messages
   - **Test concurrency**: NAS defaults to 4 workers; increase if your network can handle it
   - **Check event logs**: Review `artifacts/events-*.jsonl` for retry statistics

   ```yaml
   # Optimal NAS configuration
   db: "~/mlc-projects/nas.db"           # Local database
   source: "/Volumes/NAS/Music"           # Network source
   destination: "/Volumes/NAS/Clean"      # Network destination
   verify: hash                            # Integrity checking
   nas_mode: auto                          # Let MLC detect (default)
   ```

## Configuration Validation

MLC validates configuration at runtime:
- **Source directory** must exist before scanning
- **Destination directory** will be created if it doesn't exist
- **Mode** must be one of: `copy`, `move`, `hardlink`, `symlink`
- **Concurrency** must be > 0 (defaults to 8)
- **Hashing** must be one of: `sha1`, `xxh3`, `none`

Invalid values will produce clear error messages with suggestions.

## Advanced: Multiple Library Configs

You can manage multiple music libraries with separate configs:

```bash
# Family library (safe defaults)
mlc scan --config configs/family.yaml

# Personal high-res library (lossless only)
mlc scan --config configs/hires.yaml

# DJ library (specific layout)
mlc scan --config configs/dj.yaml
```

Each config can have different:
- Source/destination paths
- Quality thresholds
- Layout rules
- Duplicate policies
