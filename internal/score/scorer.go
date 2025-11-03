package score

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/franz/music-janitor/internal/cluster"
	"github.com/franz/music-janitor/internal/report"
	"github.com/franz/music-janitor/internal/store"
	"github.com/franz/music-janitor/internal/util"
)

// Scorer calculates quality scores for files and selects winners
type Scorer struct {
	store  *store.Store
	logger *report.EventLogger
}

// Config holds scorer configuration
type Config struct {
	Store  *store.Store
	Logger *report.EventLogger
}

// New creates a new Scorer
func New(cfg *Config) *Scorer {
	return &Scorer{
		store:  cfg.Store,
		logger: cfg.Logger,
	}
}

// Result represents scoring results
type Result struct {
	FilesScored      int
	ClustersProcessed int
	WinnersSelected  int
	Errors           []error
}

// scoredMember represents a cluster member with its score and metadata
type scoredMember struct {
	member *store.ClusterMember
	file   *store.File
	meta   *store.Metadata
	score  float64
}

// Score calculates quality scores for all clustered files and selects winners
func (s *Scorer) Score(ctx context.Context) (*Result, error) {
	util.InfoLog("Starting quality scoring")

	// Get all clusters
	clusters, err := s.store.GetAllClusters()
	if err != nil {
		return nil, fmt.Errorf("failed to get clusters: %w", err)
	}

	if len(clusters) == 0 {
		util.InfoLog("No clusters to score")
		return &Result{}, nil
	}

	totalClusters := len(clusters)
	util.InfoLog("Found %d clusters to process", totalClusters)

	result := &Result{
		Errors: make([]error, 0),
	}

	// Counters for progress reporting
	var processed atomic.Int64
	var scored atomic.Int64
	var winners atomic.Int64

	// Start progress reporter
	progressCtx, cancelProgress := context.WithCancel(ctx)
	defer cancelProgress()

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-progressCtx.Done():
				return
			case <-ticker.C:
				p := processed.Load()
				if p > 0 {
					percentage := float64(p) / float64(totalClusters) * 100
					util.InfoLog("Scoring: %d/%d clusters (%.1f%%) - %d files scored, %d winners selected",
						p, totalClusters, percentage, scored.Load(), winners.Load())
				}
			}
		}
	}()

	// Process each cluster
	for _, cluster := range clusters {
		select {
		case <-ctx.Done():
			result.ClustersProcessed = int(processed.Load())
			result.FilesScored = int(scored.Load())
			result.WinnersSelected = int(winners.Load())
			return result, ctx.Err()
		default:
		}

		// Get cluster members
		members, err := s.store.GetClusterMembers(cluster.ClusterKey)
		if err != nil {
			util.ErrorLog("Failed to get members for cluster %s: %v", cluster.ClusterKey, err)
			result.Errors = append(result.Errors, err)
			continue
		}

		// Score each member
		var scoredMembers []scoredMember

		for _, member := range members {
			// Get file and metadata
			file, err := s.store.GetFileByID(member.FileID)
			if err != nil {
				util.ErrorLog("Failed to get file %d: %v", member.FileID, err)
				result.Errors = append(result.Errors, err)
				continue
			}

			metadata, err := s.store.GetMetadata(member.FileID)
			if err != nil || metadata == nil {
				util.ErrorLog("Failed to get metadata for file %d: %v", member.FileID, err)
				result.Errors = append(result.Errors, err)
				continue
			}

			// Calculate quality score
			score := CalculateQualityScore(metadata, file)

			// Update score in database
			if err := s.store.UpdateClusterMemberScore(cluster.ClusterKey, member.FileID, score); err != nil {
				util.ErrorLog("Failed to update score for file %d: %v", member.FileID, err)
				result.Errors = append(result.Errors, err)
				continue
			}

			scoredMembers = append(scoredMembers, scoredMember{
				member: member,
				file:   file,
				meta:   metadata,
				score:  score,
			})

			scored.Add(1)
		}

		// Select winner (highest score, with tie-breakers)
		if len(scoredMembers) > 0 {
			winner := selectWinner(scoredMembers)

			// Mark winner as preferred
			if err := s.store.UpdateClusterMemberPreferred(cluster.ClusterKey, winner.file.ID, true); err != nil {
				util.ErrorLog("Failed to mark winner for cluster %s: %v", cluster.ClusterKey, err)
				result.Errors = append(result.Errors, err)
			} else {
				winners.Add(1)

				// Log score events for all members
				if s.logger != nil {
					for _, sm := range scoredMembers {
						isWinner := sm.file.ID == winner.file.ID
						s.logger.LogScore(sm.file.FileKey, sm.file.SrcPath, cluster.ClusterKey, sm.score, isWinner)
					}
				}
			}
		}

		processed.Add(1)
	}

	cancelProgress()

	// Update final counts
	result.ClustersProcessed = int(processed.Load())
	result.FilesScored = int(scored.Load())
	result.WinnersSelected = int(winners.Load())

	util.SuccessLog("Scoring complete: %d clusters processed, %d files scored, %d winners selected",
		result.ClustersProcessed, result.FilesScored, result.WinnersSelected)

	return result, nil
}

// CalculateQualityScore calculates a quality score for a file
// Higher score = better quality
func CalculateQualityScore(m *store.Metadata, f *store.File) float64 {
	score := 0.0

	// 1. Codec tier scoring (largest weight)
	score += getCodecScore(m.Codec, m.Lossless, m.BitrateKbps)

	// 2. Bit depth & sample rate bonuses
	score += getBitDepthScore(m.BitDepth)
	score += getSampleRateScore(m.SampleRate)

	// 3. Lossless verification bonus
	if m.Lossless {
		score += 10.0
	}

	// 4. Tag completeness bonus
	score += getTagCompletenessScore(m)

	// 5. File size bonus (larger is better for lossless, up to a point)
	if m.Lossless && f.SizeBytes > 0 {
		// Small bonus for larger files (indicates less compression/higher quality)
		sizeMB := float64(f.SizeBytes) / (1024.0 * 1024.0)
		if sizeMB > 50 {
			score += 2.0
		} else if sizeMB > 20 {
			score += 1.0
		}
	}

	return score
}

// getCodecScore returns score based on codec tier
func getCodecScore(codec string, lossless bool, bitrateKbps int) float64 {
	codec = strings.ToLower(codec)

	// Lossless codecs (highest tier)
	if lossless {
		switch codec {
		case "flac":
			return 40.0
		case "alac":
			return 40.0
		case "ape":
			return 35.0
		case "wavpack", "wv":
			return 35.0
		case "tta":
			return 30.0
		default:
			if strings.HasPrefix(codec, "pcm_") {
				return 40.0 // WAV/AIFF PCM
			}
			return 30.0 // Unknown lossless
		}
	}

	// Lossy codecs
	switch codec {
	case "aac":
		// AAC VBR is high quality
		if bitrateKbps >= 256 {
			return 25.0
		} else if bitrateKbps >= 192 {
			return 22.0
		} else if bitrateKbps >= 128 {
			return 18.0
		}
		return 15.0

	case "mp3":
		// MP3 320 CBR / V0 VBR
		if bitrateKbps >= 320 {
			return 20.0
		} else if bitrateKbps >= 256 {
			return 18.0 // V0 VBR average
		} else if bitrateKbps >= 192 {
			return 15.0
		} else if bitrateKbps >= 128 {
			return 12.0
		}
		return 8.0

	case "opus":
		// Opus is very efficient
		if bitrateKbps >= 192 {
			return 24.0
		} else if bitrateKbps >= 128 {
			return 22.0
		} else if bitrateKbps >= 96 {
			return 18.0
		}
		return 15.0

	case "vorbis":
		// Ogg Vorbis
		if bitrateKbps >= 256 {
			return 22.0
		} else if bitrateKbps >= 192 {
			return 19.0
		} else if bitrateKbps >= 128 {
			return 16.0
		}
		return 12.0

	default:
		// Unknown codec
		if bitrateKbps >= 256 {
			return 15.0
		}
		return 10.0
	}
}

// getBitDepthScore returns bonus for higher bit depth
func getBitDepthScore(bitDepth int) float64 {
	switch {
	case bitDepth >= 24:
		return 5.0
	case bitDepth >= 20:
		return 3.0
	case bitDepth >= 16:
		return 0.0 // Baseline
	default:
		return -2.0 // Penalty for low bit depth
	}
}

// getSampleRateScore returns bonus for higher sample rate
func getSampleRateScore(sampleRate int) float64 {
	switch {
	case sampleRate >= 96000:
		return 5.0 // Hi-res (96kHz, 192kHz)
	case sampleRate >= 48000:
		return 2.0 // 48kHz
	case sampleRate >= 44100:
		return 0.0 // CD quality baseline
	case sampleRate >= 32000:
		return -1.0
	default:
		return -3.0 // Low quality
	}
}

// getTagCompletenessScore returns bonus for complete tags
func getTagCompletenessScore(m *store.Metadata) float64 {
	score := 0.0

	// Core tags present
	if m.TagArtist != "" {
		score += 1.0
	}
	if m.TagAlbum != "" {
		score += 1.0
	}
	if m.TagTitle != "" {
		score += 1.0
	}
	if m.TagTrack > 0 {
		score += 1.0
	}

	// Bonus for complete tagging
	if score >= 4.0 {
		score += 1.0
	}

	return score
}

// selectWinner chooses the best file from scored members
// Tie-breakers: highest score → largest file size → oldest mtime → lexical path
func selectWinner(members []scoredMember) scoredMember {
	if len(members) == 0 {
		return scoredMember{}
	}

	winner := members[0]

	for i := 1; i < len(members); i++ {
		candidate := members[i]

		// Compare scores
		if candidate.score > winner.score {
			winner = candidate
			continue
		} else if candidate.score < winner.score {
			continue
		}

		// Tie-breaker 1: File size (larger is better for same quality)
		if candidate.file.SizeBytes > winner.file.SizeBytes {
			winner = candidate
			continue
		} else if candidate.file.SizeBytes < winner.file.SizeBytes {
			continue
		}

		// Tie-breaker 2: Older file (mtime)
		if candidate.file.MtimeUnix < winner.file.MtimeUnix {
			winner = candidate
			continue
		} else if candidate.file.MtimeUnix > winner.file.MtimeUnix {
			continue
		}

		// Tie-breaker 3: Lexical path order (deterministic)
		if candidate.file.SrcPath < winner.file.SrcPath {
			winner = candidate
		}
	}

	return winner
}

// GetDurationProximityScore returns a score bonus for duration proximity
// Used when comparing files in the same cluster
func GetDurationProximityScore(duration1, duration2 int) float64 {
	delta := cluster.GetDurationDelta(duration1, duration2)

	// Delta in seconds
	deltaSec := float64(delta) / 1000.0

	switch {
	case deltaSec <= 1.5:
		return 6.0 // Very close match
	case deltaSec <= 3.0:
		return 3.0 // Good match
	case deltaSec <= 5.0:
		return 1.0 // Acceptable match
	default:
		return -2.0 // Penalty for large difference
	}
}

// GetFileExtension returns the file extension in lowercase
func GetFileExtension(path string) string {
	ext := filepath.Ext(path)
	return strings.ToLower(ext)
}
