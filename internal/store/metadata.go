package store

import (
	"database/sql"
	"fmt"
)

// InsertMetadata inserts or replaces metadata for a file
func (s *Store) InsertMetadata(m *Metadata) error {
	_, err := s.db.Exec(`
		INSERT INTO metadata (
			file_id, format, codec, container,
			duration_ms, sample_rate, bit_depth, channels, bitrate_kbps, lossless,
			tag_artist, tag_album, tag_title, tag_albumartist, tag_date,
			tag_disc, tag_disc_total, tag_track, tag_track_total, tag_compilation,
			musicbrainz_recording_id, musicbrainz_release_id, raw_tags_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(file_id) DO UPDATE SET
			format = excluded.format,
			codec = excluded.codec,
			container = excluded.container,
			duration_ms = excluded.duration_ms,
			sample_rate = excluded.sample_rate,
			bit_depth = excluded.bit_depth,
			channels = excluded.channels,
			bitrate_kbps = excluded.bitrate_kbps,
			lossless = excluded.lossless,
			tag_artist = excluded.tag_artist,
			tag_album = excluded.tag_album,
			tag_title = excluded.tag_title,
			tag_albumartist = excluded.tag_albumartist,
			tag_date = excluded.tag_date,
			tag_disc = excluded.tag_disc,
			tag_disc_total = excluded.tag_disc_total,
			tag_track = excluded.tag_track,
			tag_track_total = excluded.tag_track_total,
			tag_compilation = excluded.tag_compilation,
			musicbrainz_recording_id = excluded.musicbrainz_recording_id,
			musicbrainz_release_id = excluded.musicbrainz_release_id,
			raw_tags_json = excluded.raw_tags_json
	`,
		m.FileID, m.Format, m.Codec, m.Container,
		m.DurationMs, m.SampleRate, m.BitDepth, m.Channels, m.BitrateKbps, m.Lossless,
		m.TagArtist, m.TagAlbum, m.TagTitle, m.TagAlbumArtist, m.TagDate,
		m.TagDisc, m.TagDiscTotal, m.TagTrack, m.TagTrackTotal, m.TagCompilation,
		m.MusicBrainzRecordingID, m.MusicBrainzReleaseID, m.RawTagsJSON,
	)

	if err != nil {
		return fmt.Errorf("failed to insert metadata: %w", err)
	}

	return nil
}

// GetMetadata retrieves metadata for a file
func (s *Store) GetMetadata(fileID int64) (*Metadata, error) {
	m := &Metadata{}
	err := s.db.QueryRow(`
		SELECT file_id, COALESCE(format, ''), COALESCE(codec, ''), COALESCE(container, ''),
		       COALESCE(duration_ms, 0), COALESCE(sample_rate, 0), COALESCE(bit_depth, 0),
		       COALESCE(channels, 0), COALESCE(bitrate_kbps, 0), COALESCE(lossless, 0),
		       COALESCE(tag_artist, ''), COALESCE(tag_album, ''),
		       COALESCE(tag_title, ''), COALESCE(tag_albumartist, ''),
		       COALESCE(tag_date, ''), COALESCE(tag_disc, 0), COALESCE(tag_disc_total, 0),
		       COALESCE(tag_track, 0), COALESCE(tag_track_total, 0), COALESCE(tag_compilation, 0),
		       COALESCE(musicbrainz_recording_id, ''),
		       COALESCE(musicbrainz_release_id, ''),
		       COALESCE(raw_tags_json, '')
		FROM metadata WHERE file_id = ?
	`, fileID).Scan(
		&m.FileID, &m.Format, &m.Codec, &m.Container,
		&m.DurationMs, &m.SampleRate, &m.BitDepth, &m.Channels, &m.BitrateKbps, &m.Lossless,
		&m.TagArtist, &m.TagAlbum, &m.TagTitle, &m.TagAlbumArtist, &m.TagDate,
		&m.TagDisc, &m.TagDiscTotal, &m.TagTrack, &m.TagTrackTotal, &m.TagCompilation,
		&m.MusicBrainzRecordingID, &m.MusicBrainzReleaseID, &m.RawTagsJSON,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata: %w", err)
	}

	return m, nil
}

// GetFilesWithMetadata retrieves files with their metadata
func (s *Store) GetFilesWithMetadata() ([]struct {
	File     *File
	Metadata *Metadata
}, error) {
	rows, err := s.db.Query(`
		SELECT
			f.id, f.file_key, f.src_path, f.size_bytes, f.mtime_unix,
			COALESCE(f.sha1, ''), f.status, COALESCE(f.error, ''),
			f.first_seen_at, f.last_update_at,
			m.file_id, COALESCE(m.format, ''), COALESCE(m.codec, ''), COALESCE(m.container, ''),
			COALESCE(m.duration_ms, 0), COALESCE(m.sample_rate, 0), COALESCE(m.bit_depth, 0),
			COALESCE(m.channels, 0), COALESCE(m.bitrate_kbps, 0), COALESCE(m.lossless, 0),
			COALESCE(m.tag_artist, ''), COALESCE(m.tag_album, ''),
			COALESCE(m.tag_title, ''), COALESCE(m.tag_albumartist, ''),
			COALESCE(m.tag_date, ''), COALESCE(m.tag_disc, 0), COALESCE(m.tag_disc_total, 0),
			COALESCE(m.tag_track, 0), COALESCE(m.tag_track_total, 0), COALESCE(m.tag_compilation, 0),
			COALESCE(m.musicbrainz_recording_id, ''),
			COALESCE(m.musicbrainz_release_id, ''),
			COALESCE(m.raw_tags_json, '')
		FROM files f
		INNER JOIN metadata m ON f.id = m.file_id
		WHERE f.status = 'meta_ok'
		ORDER BY f.id
	`)

	if err != nil {
		return nil, fmt.Errorf("failed to query files with metadata: %w", err)
	}
	defer rows.Close()

	var results []struct {
		File     *File
		Metadata *Metadata
	}

	for rows.Next() {
		f := &File{}
		m := &Metadata{}

		err := rows.Scan(
			&f.ID, &f.FileKey, &f.SrcPath, &f.SizeBytes, &f.MtimeUnix,
			&f.SHA1, &f.Status, &f.Error,
			&f.FirstSeenAt, &f.LastUpdate,
			&m.FileID, &m.Format, &m.Codec, &m.Container,
			&m.DurationMs, &m.SampleRate, &m.BitDepth, &m.Channels, &m.BitrateKbps, &m.Lossless,
			&m.TagArtist, &m.TagAlbum, &m.TagTitle, &m.TagAlbumArtist, &m.TagDate,
			&m.TagDisc, &m.TagDiscTotal, &m.TagTrack, &m.TagTrackTotal, &m.TagCompilation,
			&m.MusicBrainzRecordingID, &m.MusicBrainzReleaseID, &m.RawTagsJSON,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		results = append(results, struct {
			File     *File
			Metadata *Metadata
		}{File: f, Metadata: m})
	}

	return results, rows.Err()
}

// GetAllMetadata returns all metadata records as a map indexed by file_id
func (s *Store) GetAllMetadata() (map[int64]*Metadata, error) {
	rows, err := s.db.Query(`
		SELECT file_id, COALESCE(format, ''), COALESCE(codec, ''), COALESCE(container, ''),
		       COALESCE(duration_ms, 0), COALESCE(sample_rate, 0), COALESCE(bit_depth, 0),
		       COALESCE(channels, 0), COALESCE(bitrate_kbps, 0), COALESCE(lossless, 0),
		       COALESCE(tag_artist, ''), COALESCE(tag_album, ''),
		       COALESCE(tag_title, ''), COALESCE(tag_track, 0), COALESCE(tag_disc, 0),
		       COALESCE(tag_date, ''), COALESCE(tag_albumartist, '')
		FROM metadata
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query all metadata: %w", err)
	}
	defer rows.Close()

	result := make(map[int64]*Metadata)
	for rows.Next() {
		m := &Metadata{}
		var losslessInt int
		err := rows.Scan(
			&m.FileID, &m.Format, &m.Codec, &m.Container,
			&m.DurationMs, &m.SampleRate, &m.BitDepth, &m.Channels, &m.BitrateKbps, &losslessInt,
			&m.TagArtist, &m.TagAlbum,
			&m.TagTitle, &m.TagTrack, &m.TagDisc, &m.TagDate,
			&m.TagAlbumArtist,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan metadata: %w", err)
		}

		m.Lossless = losslessInt == 1
		result[m.FileID] = m
	}

	return result, rows.Err()
}

// InsertMetadataBatch inserts multiple metadata records in a single transaction
func (s *Store) InsertMetadataBatch(metadataList []*Metadata) error {
	if len(metadataList) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO metadata (
			file_id, format, codec, container, duration_ms, sample_rate, bit_depth,
			channels, bitrate_kbps, lossless,
			tag_artist, tag_album, tag_title, tag_track, tag_disc, tag_date,
			tag_albumartist, tag_compilation
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, m := range metadataList {
		losslessInt := 0
		if m.Lossless {
			losslessInt = 1
		}
		compilationInt := 0
		if m.TagCompilation {
			compilationInt = 1
		}

		_, err := stmt.Exec(
			m.FileID, m.Format, m.Codec, m.Container,
			m.DurationMs, m.SampleRate, m.BitDepth, m.Channels,
			m.BitrateKbps, losslessInt,
			m.TagArtist, m.TagAlbum, m.TagTitle, m.TagTrack, m.TagDisc, m.TagDate,
			m.TagAlbumArtist, compilationInt,
		)
		if err != nil {
			return fmt.Errorf("failed to insert metadata for file %d: %w", m.FileID, err)
		}
	}

	return tx.Commit()
}

// GetAllUniqueArtists returns all unique artist names from the metadata table
// Returns both tag_artist and tag_albumartist values (deduplicated)
func (s *Store) GetAllUniqueArtists() ([]string, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT artist_name FROM (
			SELECT tag_artist AS artist_name FROM metadata WHERE tag_artist != ''
			UNION
			SELECT tag_albumartist AS artist_name FROM metadata WHERE tag_albumartist != ''
		)
		ORDER BY artist_name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query unique artists: %w", err)
	}
	defer rows.Close()

	var artists []string
	for rows.Next() {
		var artist string
		if err := rows.Scan(&artist); err != nil {
			return nil, fmt.Errorf("failed to scan artist: %w", err)
		}
		artists = append(artists, artist)
	}

	return artists, rows.Err()
}

// GetMetadataByFileID retrieves metadata for a specific file ID
// Returns nil if no metadata found (not an error)
func (s *Store) GetMetadataByFileID(fileID int64) (*Metadata, error) {
	m := &Metadata{}
	err := s.db.QueryRow(`
		SELECT file_id, COALESCE(format, ''), COALESCE(codec, ''), COALESCE(container, ''),
		       COALESCE(duration_ms, 0), COALESCE(sample_rate, 0), COALESCE(bit_depth, 0),
		       COALESCE(channels, 0), COALESCE(bitrate_kbps, 0), COALESCE(lossless, 0),
		       COALESCE(tag_artist, ''), COALESCE(tag_album, ''), COALESCE(tag_title, ''),
		       COALESCE(tag_albumartist, ''), COALESCE(tag_date, ''),
		       COALESCE(tag_disc, 0), COALESCE(tag_disc_total, 0), COALESCE(tag_track, 0),
		       COALESCE(tag_track_total, 0), COALESCE(tag_compilation, 0),
		       COALESCE(musicbrainz_recording_id, ''), COALESCE(musicbrainz_release_id, ''),
		       COALESCE(raw_tags_json, '')
		FROM metadata
		WHERE file_id = ?
	`, fileID).Scan(
		&m.FileID, &m.Format, &m.Codec, &m.Container,
		&m.DurationMs, &m.SampleRate, &m.BitDepth, &m.Channels, &m.BitrateKbps, &m.Lossless,
		&m.TagArtist, &m.TagAlbum, &m.TagTitle,
		&m.TagAlbumArtist, &m.TagDate,
		&m.TagDisc, &m.TagDiscTotal, &m.TagTrack, &m.TagTrackTotal, &m.TagCompilation,
		&m.MusicBrainzRecordingID, &m.MusicBrainzReleaseID,
		&m.RawTagsJSON,
	)

	if err == sql.ErrNoRows {
		return nil, nil // No metadata found, not an error
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get metadata for file %d: %w", fileID, err)
	}

	return m, nil
}
