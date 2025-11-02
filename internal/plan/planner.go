package plan

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/franz/music-janitor/internal/store"
	"github.com/franz/music-janitor/internal/util"
)

// Planner creates execution plans for clustered files
type Planner struct {
	store *store.Store
	mode  string // copy, move, hardlink, symlink
}

// Config holds planner configuration
type Config struct {
	Store *store.Store
	Mode  string // copy, move, hardlink, symlink
}

// New creates a new Planner
func New(cfg *Config) *Planner {
	if cfg.Mode == "" {
		cfg.Mode = "copy" // Default to safe copy mode
	}

	return &Planner{
		store: cfg.Store,
		mode:  cfg.Mode,
	}
}

// Result represents planning results
type Result struct {
	WinnersPlanned int
	DuplicatesSkipped int
	SingletonsPlanned int
	Errors []error
}

// Plan creates execution plans for all clustered files
func (p *Planner) Plan(ctx context.Context, destRoot string) (*Result, error) {
	util.InfoLog("Starting planning")
	util.InfoLog("Destination: %s", destRoot)
	util.InfoLog("Mode: %s", p.mode)

	// Get all clusters
	clusters, err := p.store.GetAllClusters()
	if err != nil {
		return nil, fmt.Errorf("failed to get clusters: %w", err)
	}

	if len(clusters) == 0 {
		util.InfoLog("No clusters to plan")
		return &Result{}, nil
	}

	totalClusters := len(clusters)
	util.InfoLog("Found %d clusters to plan", totalClusters)

	result := &Result{
		Errors: make([]error, 0),
	}

	// Clear existing plans (idempotent operation)
	if err := p.store.ClearPlans(); err != nil {
		return nil, fmt.Errorf("failed to clear plans: %w", err)
	}

	// Counters for progress reporting
	var processed atomic.Int64
	var winnersPlanned atomic.Int64
	var duplicatesSkipped atomic.Int64
	var singletonsPlanned atomic.Int64

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
					util.InfoLog("Planning: %d/%d clusters (%.1f%%) - %d winners, %d duplicates skipped",
						p, totalClusters, percentage, winnersPlanned.Load(), duplicatesSkipped.Load())
				}
			}
		}
	}()

	// Process each cluster
	for _, cluster := range clusters {
		select {
		case <-ctx.Done():
			result.WinnersPlanned = int(winnersPlanned.Load())
			result.DuplicatesSkipped = int(duplicatesSkipped.Load())
			result.SingletonsPlanned = int(singletonsPlanned.Load())
			return result, ctx.Err()
		default:
		}

		// Get cluster members
		members, err := p.store.GetClusterMembers(cluster.ClusterKey)
		if err != nil {
			util.ErrorLog("Failed to get members for cluster %s: %v", cluster.ClusterKey, err)
			result.Errors = append(result.Errors, err)
			continue
		}

		if len(members) == 0 {
			continue
		}

		// Find winner (should be marked as preferred from scoring phase)
		var winner *store.ClusterMember
		var losers []*store.ClusterMember

		for _, member := range members {
			if member.Preferred {
				winner = member
			} else {
				losers = append(losers, member)
			}
		}

		// If no winner marked (shouldn't happen), use first member
		if winner == nil {
			winner = members[0]
			if len(members) > 1 {
				losers = members[1:]
			}
		}

		// Get winner file and metadata for destination path generation
		winnerFile, err := p.store.GetFileByID(winner.FileID)
		if err != nil {
			util.ErrorLog("Failed to get winner file %d: %v", winner.FileID, err)
			result.Errors = append(result.Errors, err)
			continue
		}

		winnerMeta, err := p.store.GetMetadata(winner.FileID)
		if err != nil || winnerMeta == nil {
			util.ErrorLog("Failed to get winner metadata %d: %v", winner.FileID, err)
			result.Errors = append(result.Errors, err)
			continue
		}

		// Generate destination path
		destPath := GenerateDestPath(destRoot, winnerMeta, winnerFile.SrcPath)

		// Create plan for winner
		winnerPlan := &store.Plan{
			FileID:   winner.FileID,
			Action:   p.mode, // copy, move, etc.
			DestPath: destPath,
			Reason:   fmt.Sprintf("winner (score: %.1f)", winner.QualityScore),
		}

		if err := p.store.InsertPlan(winnerPlan); err != nil {
			util.ErrorLog("Failed to insert plan for winner %d: %v", winner.FileID, err)
			result.Errors = append(result.Errors, err)
			continue
		}

		winnersPlanned.Add(1)

		if len(members) == 1 {
			singletonsPlanned.Add(1)
		}

		// Create plans for losers (skip)
		for _, loser := range losers {
			loserPlan := &store.Plan{
				FileID:   loser.FileID,
				Action:   "skip",
				DestPath: "",
				Reason:   fmt.Sprintf("duplicate (score: %.1f, winner: %d)", loser.QualityScore, winner.FileID),
			}

			if err := p.store.InsertPlan(loserPlan); err != nil {
				util.ErrorLog("Failed to insert plan for loser %d: %v", loser.FileID, err)
				result.Errors = append(result.Errors, err)
				continue
			}

			duplicatesSkipped.Add(1)
		}

		processed.Add(1)
	}

	cancelProgress()

	// Update final counts
	result.WinnersPlanned = int(winnersPlanned.Load())
	result.DuplicatesSkipped = int(duplicatesSkipped.Load())
	result.SingletonsPlanned = int(singletonsPlanned.Load())

	util.SuccessLog("Planning complete: %d winners, %d duplicates skipped (%d singletons)",
		result.WinnersPlanned, result.DuplicatesSkipped, result.SingletonsPlanned)

	return result, nil
}

// GenerateDestPath creates a destination path for a file
// Format: {AlbumArtist or Artist}/{Album}/{Track} - {Title}.{ext}
func GenerateDestPath(destRoot string, m *store.Metadata, srcPath string) string {
	// Determine artist
	artist := m.TagAlbumArtist
	if artist == "" {
		artist = m.TagArtist
	}
	if artist == "" {
		artist = "Unknown Artist"
	}
	artist = SanitizePathComponent(artist)

	// Determine album
	album := m.TagAlbum
	if album == "" {
		album = "_Singles"
	}

	// Add year prefix if available
	if m.TagDate != "" {
		// Extract year (first 4 digits)
		year := extractYear(m.TagDate)
		if year != "" {
			album = year + " - " + album
		}
	}
	album = SanitizePathComponent(album)

	// Build filename
	filename := ""

	// Track number prefix
	if m.TagTrack > 0 {
		if m.TagTrackTotal >= 100 {
			filename = fmt.Sprintf("%03d", m.TagTrack)
		} else {
			filename = fmt.Sprintf("%02d", m.TagTrack)
		}
		filename += " - "
	}

	// Title
	title := m.TagTitle
	if title == "" {
		// Fall back to source filename without extension
		base := filepath.Base(srcPath)
		title = strings.TrimSuffix(base, filepath.Ext(base))
	}
	filename += SanitizePathComponent(title)

	// Extension from source
	ext := strings.ToLower(filepath.Ext(srcPath))
	filename += ext

	// Handle multi-disc albums
	var discFolder string
	if m.TagDiscTotal > 1 && m.TagDisc > 0 {
		discFolder = fmt.Sprintf("Disc %02d", m.TagDisc)
	}

	// Assemble full path
	if discFolder != "" {
		return filepath.Join(destRoot, artist, album, discFolder, filename)
	}

	return filepath.Join(destRoot, artist, album, filename)
}

// SanitizePathComponent removes illegal filesystem characters
func SanitizePathComponent(s string) string {
	// Replace illegal characters with underscores
	illegal := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	for _, char := range illegal {
		s = strings.ReplaceAll(s, char, "_")
	}

	// Collapse multiple underscores
	for strings.Contains(s, "__") {
		s = strings.ReplaceAll(s, "__", "_")
	}

	// Trim spaces and dots (Windows issues)
	s = strings.TrimSpace(s)
	s = strings.Trim(s, ".")

	// Limit length to 200 characters (filesystem limits)
	if len(s) > 200 {
		s = s[:200]
	}

	return s
}

// extractYear extracts a 4-digit year from a date string
func extractYear(date string) string {
	// Try to find 4 consecutive digits
	for i := 0; i <= len(date)-4; i++ {
		potential := date[i : i+4]
		// Check if all chars are digits
		allDigits := true
		for _, ch := range potential {
			if ch < '0' || ch > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			// Validate year range (1900-2099)
			year := potential
			if year >= "1900" && year <= "2099" {
				return year
			}
		}
	}
	return ""
}
