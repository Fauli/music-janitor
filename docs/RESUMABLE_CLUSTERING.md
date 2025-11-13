# Resumable Clustering

## Overview

As of v1.6.0, MLC supports **resumable clustering**, allowing you to safely interrupt and resume the clustering phase of the `plan` command.

This is especially useful for large music collections where clustering can take many hours.

## How It Works

### Automatic Progress Tracking

When you run `mlc plan`, the clustering phase now:

1. **Saves progress every 1000 files** processed
2. **Tracks the last processed file ID** in the database
3. **Preserves existing clusters** in memory and database
4. **Automatically resumes** from where it left off if interrupted

### Resume Behavior

**If you interrupt clustering** (Ctrl+C, kill, crash):
```bash
# First run - gets interrupted after 50,000 files
mlc plan --db my-library.db --dest /destination

# Second run - automatically resumes from file 50,000
mlc plan --db my-library.db --dest /destination
```

You'll see a message like:
```
Resuming clustering from file ID 125432 (50000/192880 files processed)
Rebuilding cluster map from existing clusters...
Loaded 45234 existing clusters with 50000 files
```

### Force Re-clustering

If you want to **discard resume state** and start fresh:

```bash
mlc plan --db my-library.db --dest /destination --force-recluster
```

This will:
- Clear all existing clusters
- Clear clustering progress
- Start from file #1

## Use Cases

### Normal Operation

```bash
# Start clustering (or resume if interrupted)
mlc plan --db my-library.db --dest /music
```

### After Metadata Changes

If you've re-scanned files or updated metadata, force a complete re-cluster:

```bash
mlc rescan --db my-library.db --errors-only
mlc plan --db my-library.db --dest /music --force-recluster
```

### Checking Resume State

You can check if there's an in-progress clustering operation by looking at the database:

```bash
sqlite3 my-library.db "SELECT * FROM clustering_progress"
```

If the table has a row, clustering was interrupted and will resume.

## Technical Details

### Database Schema

The `clustering_progress` table tracks:
- `last_processed_file_id`: Last file that was successfully clustered
- `total_files`: Total number of files to cluster
- `files_processed`: How many files have been processed
- `clusters_created`: Number of clusters created so far
- `started_at`: When clustering started
- `updated_at`: Last progress update

### Performance

- **Progress saves every 1000 files**: Minimal overhead (~1 DB write per 1000 files)
- **On resume**: Rebuilds cluster map from database (fast, typically < 10 seconds even for 100k+ files)
- **Memory efficient**: Only active clusters kept in memory

### Safety

- **Progress saved on Ctrl+C**: The context cancellation handler saves state before exiting
- **Periodic saves**: Every 1000 files ensures minimal re-work on crash
- **Atomic operations**: Database transactions ensure consistency

## Migration

Existing databases will automatically upgrade to schema v3, which adds the `clustering_progress` table.

No action required - just upgrade MLC and run `plan` as usual.

## Limitations

### When Resume Works

✅ Interrupting during clustering (grouping files)
✅ Interrupting during cluster writing
✅ Ctrl+C / SIGINT / SIGTERM
✅ Process crash / system restart

### When Resume Doesn't Work

❌ After scoring phase starts (scoring is fast, doesn't need resume)
❌ After planning phase starts (planning is fast, doesn't need resume)
❌ If you use `--force-recluster` flag

## Examples

### Large Collection (192k files)

**Without resume** (old behavior):
- Clustering takes 15 hours
- If interrupted at hour 10, you lose 10 hours of work
- Must restart from scratch

**With resume** (new behavior):
- Clustering takes 15 hours
- If interrupted at hour 10, resume takes 5 hours
- Only re-processes remaining 5 hours of work

### Typical Workflow

```bash
# Start clustering
mlc plan --db my-library.db --dest /music

# ... hours later, realize you need to stop ...
# Press Ctrl+C

# Later, resume where you left off
mlc plan --db my-library.db --dest /music
# Automatically continues from last checkpoint

# When done, execute
mlc execute --db my-library.db --verify hash
```

## Troubleshooting

### Resume Not Working

If resume doesn't seem to work:

1. Check if progress exists:
   ```bash
   sqlite3 my-library.db "SELECT * FROM clustering_progress"
   ```

2. If no progress, clustering may have completed or been force-cleared

3. Check for `--force-recluster` flag (disables resume)

### Corrupted Resume State

If you suspect resume state is corrupted:

```bash
# Clear progress and start fresh
sqlite3 my-library.db "DELETE FROM clustering_progress"

# Or use the flag
mlc plan --db my-library.db --dest /music --force-recluster
```

### Memory Issues on Resume

If rebuilding the cluster map uses too much memory (unlikely unless you have millions of duplicates):

```bash
# Force recluster to start fresh
mlc plan --db my-library.db --dest /music --force-recluster
```

## FAQ

**Q: Does resume slow down clustering?**
A: No. Progress saves happen every 1000 files (~once per second), adding negligible overhead.

**Q: Can I resume after upgrading MLC?**
A: Yes. The resume state is stored in the database, independent of the MLC binary version.

**Q: Will resume work if I move the database file?**
A: Yes. Resume state is in the database, not dependent on file paths.

**Q: What if I cancel during the "writing clusters" phase?**
A: Resume works here too! It will rebuild the cluster map from the database and continue.

**Q: How do I check if clustering finished?**
A: If `clustering_progress` table is empty, clustering completed successfully.

## Version History

- **v1.6.0**: Initial release of resumable clustering
- Schema v3 introduced `clustering_progress` table
