package plan

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/franz/music-janitor/internal/report"
	"github.com/franz/music-janitor/internal/store"
	"github.com/franz/music-janitor/internal/util"
)

// Planner creates execution plans for clustered files
type Planner struct {
	store  *store.Store
	mode   string // copy, move, hardlink, symlink
	logger *report.EventLogger
}

// Config holds planner configuration
type Config struct {
	Store  *store.Store
	Mode   string // copy, move, hardlink, symlink
	Logger *report.EventLogger
}

// New creates a new Planner
func New(cfg *Config) *Planner {
	if cfg.Mode == "" {
		cfg.Mode = "copy" // Default to safe copy mode
	}

	return &Planner{
		store:  cfg.Store,
		mode:   cfg.Mode,
		logger: cfg.Logger,
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

		// Check if this is a true compilation (compilation flag + multiple artists)
		isCompilation := false
		if winnerMeta.TagCompilation {
			isCompilation = p.isRealCompilation(winner.FileID, winnerMeta.TagAlbum)
		}

		// Generate destination path
		destPath := GenerateDestPath(destRoot, winnerMeta, winnerFile.SrcPath, isCompilation)

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

		// Log plan event for winner
		if p.logger != nil {
			p.logger.LogPlan(winnerFile.FileKey, winnerFile.SrcPath, destPath, p.mode, winnerPlan.Reason)
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

			// Log plan event for loser
			if p.logger != nil {
				loserFile, err := p.store.GetFileByID(loser.FileID)
				if err == nil {
					p.logger.LogPlan(loserFile.FileKey, loserFile.SrcPath, "", "skip", loserPlan.Reason)
				}
			}

			duplicatesSkipped.Add(1)
		}

		processed.Add(1)
	}

	cancelProgress()

	// Update final counts (before collision resolution)
	result.WinnersPlanned = int(winnersPlanned.Load())
	result.DuplicatesSkipped = int(duplicatesSkipped.Load())
	result.SingletonsPlanned = int(singletonsPlanned.Load())

	util.InfoLog("Initial planning: %d winners, %d duplicates skipped",
		result.WinnersPlanned, result.DuplicatesSkipped)

	// Resolve path collisions - pick best quality file for each dest_path
	util.InfoLog("Resolving destination path collisions...")
	collisionsResolved, err := p.resolvePathCollisions()
	if err != nil {
		util.WarnLog("Failed to resolve path collisions: %v", err)
	} else if collisionsResolved > 0 {
		util.WarnLog("Resolved %d path collisions (kept highest quality files)", collisionsResolved)
		// Update counts
		result.DuplicatesSkipped += collisionsResolved
		result.WinnersPlanned -= collisionsResolved
	}

	util.SuccessLog("Planning complete: %d winners, %d duplicates skipped (%d singletons)",
		result.WinnersPlanned, result.DuplicatesSkipped, result.SingletonsPlanned)

	return result, nil
}

// resolvePathCollisions detects when multiple files would be copied to the same dest_path
// and resolves conflicts by keeping only the highest quality file
func (p *Planner) resolvePathCollisions() (int, error) {
	// Get all plans that aren't skipped
	allPlans, err := p.store.GetAllPlans()
	if err != nil {
		return 0, fmt.Errorf("failed to get plans: %w", err)
	}

	// Group plans by destination path
	pathMap := make(map[string][]*store.Plan)
	for _, plan := range allPlans {
		if plan.Action != "skip" && plan.DestPath != "" {
			pathMap[plan.DestPath] = append(pathMap[plan.DestPath], plan)
		}
	}

	collisionsResolved := 0

	// Process each collision group
	for destPath, plans := range pathMap {
		if len(plans) <= 1 {
			continue // No collision
		}

		// We have a collision - multiple files want the same dest_path
		util.WarnLog("Path collision detected: %d files -> %s", len(plans), destPath)

		// Get quality scores for each file
		type scoredPlan struct {
			plan  *store.Plan
			score float64
		}
		scored := make([]scoredPlan, 0, len(plans))

		for _, plan := range plans {
			// Try to get quality score from cluster_members
			// Find any cluster containing this file
			var qualityScore float64
			clusters, _ := p.store.GetAllClusters()
			for _, cluster := range clusters {
				members, _ := p.store.GetClusterMembers(cluster.ClusterKey)
				for _, member := range members {
					if member.FileID == plan.FileID {
						qualityScore = member.QualityScore
						break
					}
				}
				if qualityScore > 0 {
					break
				}
			}

			scored = append(scored, scoredPlan{
				plan:  plan,
				score: qualityScore,
			})
		}

		// Sort by quality score (descending)
		// Using simple bubble sort since collision groups are typically small
		for i := 0; i < len(scored); i++ {
			for j := i + 1; j < len(scored); j++ {
				if scored[j].score > scored[i].score {
					scored[i], scored[j] = scored[j], scored[i]
				}
			}
		}

		// Keep the highest quality file, skip the rest
		winner := scored[0]
		util.InfoLog("  Keeping: file %d (score: %.1f)", winner.plan.FileID, winner.score)

		for _, loser := range scored[1:] {
			util.InfoLog("  Skipping: file %d (score: %.1f)", loser.plan.FileID, loser.score)

			// Update plan to skip
			updatedPlan := &store.Plan{
				FileID:   loser.plan.FileID,
				Action:   "skip",
				DestPath: "",
				Reason:   fmt.Sprintf("path collision (score: %.1f, winner: %d at %s)", loser.score, winner.plan.FileID, destPath),
			}

			if err := p.store.InsertPlan(updatedPlan); err != nil {
				util.WarnLog("Failed to update plan for collision loser %d: %v", loser.plan.FileID, err)
			} else {
				collisionsResolved++
			}
		}
	}

	return collisionsResolved, nil
}

// isRealCompilation checks if a file belongs to a compilation album with multiple artists
// Returns true only if TagCompilation=true AND album has tracks from different artists
func (p *Planner) isRealCompilation(fileID int64, albumName string) bool {
	// Get all files with the same album
	files, err := p.store.GetAllFiles()
	if err != nil {
		return false
	}

	// Collect unique track artists for this album
	// For compilations, we care about track artists, not album artist
	artistsInAlbum := make(map[string]bool)
	for _, file := range files {
		metadata, err := p.store.GetMetadata(file.ID)
		if err != nil || metadata == nil {
			continue
		}

		// Check if same album
		if metadata.TagAlbum == albumName && albumName != "" {
			// Use track artist (not album artist) to detect multiple artists
			artist := metadata.TagArtist
			if artist != "" {
				artistsInAlbum[strings.ToLower(artist)] = true
			}
		}
	}

	// True compilation has 3+ different artists
	return len(artistsInAlbum) >= 3
}

// GenerateDestPath creates a destination path for a file
// Format: {AlbumArtist or Artist}/{Album}/{Track} - {Title}.{ext}
// For compilations: Various Artists/{Album}/{Track} - {Artist} - {Title}.{ext}
func GenerateDestPath(destRoot string, m *store.Metadata, srcPath string, isCompilation bool) string {
	// Determine track artist (for filename in compilations)
	trackArtist := m.TagArtist
	if trackArtist == "" {
		trackArtist = "Unknown Artist"
	}

	// Determine folder artist
	var folderArtist string
	if isCompilation {
		folderArtist = "Various Artists"
	} else {
		folderArtist = m.TagAlbumArtist
		if folderArtist == "" {
			folderArtist = m.TagArtist
		}
		if folderArtist == "" {
			folderArtist = "Unknown Artist"
		}
	}
	folderArtist = SanitizePathComponent(folderArtist)

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

	// For compilations, include artist in filename
	if isCompilation {
		filename += SanitizePathComponent(trackArtist) + " - "
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
		return filepath.Join(destRoot, folderArtist, album, discFolder, filename)
	}

	return filepath.Join(destRoot, folderArtist, album, filename)
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
