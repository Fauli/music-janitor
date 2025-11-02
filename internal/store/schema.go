package store

// Schema v1 - Initial database schema
// Based on PLAN.md section 4
const schemaV1 = `
-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_version (
  version INTEGER PRIMARY KEY,
  applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Files discovered in source
CREATE TABLE IF NOT EXISTS files (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  file_key TEXT UNIQUE NOT NULL,
  src_path TEXT NOT NULL,
  size_bytes INTEGER,
  mtime_unix INTEGER,
  sha1 TEXT,
  status TEXT NOT NULL DEFAULT 'discovered',
  error TEXT,
  first_seen_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  last_update_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_files_status ON files(status);
CREATE INDEX IF NOT EXISTS idx_files_file_key ON files(file_key);

-- Extracted metadata (one row per file)
CREATE TABLE IF NOT EXISTS metadata (
  file_id INTEGER PRIMARY KEY REFERENCES files(id) ON DELETE CASCADE,
  format TEXT,
  codec TEXT,
  container TEXT,
  duration_ms INTEGER,
  sample_rate INTEGER,
  bit_depth INTEGER,
  channels INTEGER,
  bitrate_kbps INTEGER,
  lossless INTEGER DEFAULT 0,
  tag_artist TEXT,
  tag_album TEXT,
  tag_title TEXT,
  tag_albumartist TEXT,
  tag_date TEXT,
  tag_disc INTEGER,
  tag_disc_total INTEGER,
  tag_track INTEGER,
  tag_track_total INTEGER,
  tag_compilation INTEGER DEFAULT 0,
  musicbrainz_recording_id TEXT,
  musicbrainz_release_id TEXT,
  raw_tags_json TEXT
);

CREATE INDEX IF NOT EXISTS idx_metadata_artist ON metadata(tag_artist);
CREATE INDEX IF NOT EXISTS idx_metadata_album ON metadata(tag_album);
CREATE INDEX IF NOT EXISTS idx_metadata_title ON metadata(tag_title);

-- Groups of files believed to be the same recording (dedup cluster)
CREATE TABLE IF NOT EXISTS clusters (
  cluster_key TEXT PRIMARY KEY,
  hint TEXT
);

CREATE TABLE IF NOT EXISTS cluster_members (
  cluster_key TEXT REFERENCES clusters(cluster_key) ON DELETE CASCADE,
  file_id INTEGER REFERENCES files(id) ON DELETE CASCADE,
  quality_score REAL,
  preferred INTEGER DEFAULT 0,
  PRIMARY KEY (cluster_key, file_id)
);

CREATE INDEX IF NOT EXISTS idx_cluster_members_file_id ON cluster_members(file_id);
CREATE INDEX IF NOT EXISTS idx_cluster_members_preferred ON cluster_members(cluster_key, preferred);

-- Planned destination mapping per winning file
CREATE TABLE IF NOT EXISTS plans (
  file_id INTEGER PRIMARY KEY REFERENCES files(id) ON DELETE CASCADE,
  action TEXT NOT NULL,
  dest_path TEXT,
  reason TEXT
);

CREATE INDEX IF NOT EXISTS idx_plans_dest_path ON plans(dest_path);
CREATE INDEX IF NOT EXISTS idx_plans_action ON plans(action);

-- Execution results
CREATE TABLE IF NOT EXISTS executions (
  file_id INTEGER PRIMARY KEY REFERENCES files(id) ON DELETE CASCADE,
  started_at DATETIME,
  completed_at DATETIME,
  bytes_written INTEGER,
  verify_ok INTEGER DEFAULT 0,
  error TEXT
);

CREATE INDEX IF NOT EXISTS idx_executions_verify_ok ON executions(verify_ok);
`
