package plan

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/franz/music-janitor/internal/meta"
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

	// Step 1: Pre-load all data into memory
	util.InfoLog("Loading files and metadata into memory...")
	filesMap, err := p.store.GetAllFilesMap()
	if err != nil {
		return nil, fmt.Errorf("failed to load files: %w", err)
	}
	util.InfoLog("Loaded %d files", len(filesMap))

	metadataMap, err := p.store.GetAllMetadata()
	if err != nil {
		return nil, fmt.Errorf("failed to load metadata: %w", err)
	}
	util.InfoLog("Loaded %d metadata records", len(metadataMap))

	// Get all clusters
	clusters, err := p.store.GetAllClusters()
	if err != nil {
		return nil, fmt.Errorf("failed to get clusters: %w", err)
	}

	if len(clusters) == 0 {
		util.InfoLog("No clusters to plan")
		return &Result{}, nil
	}

	util.InfoLog("Loading cluster members into memory...")
	membersMap, err := p.store.GetAllClusterMembers()
	if err != nil {
		return nil, fmt.Errorf("failed to load cluster members: %w", err)
	}
	util.InfoLog("Loaded %d cluster memberships", len(membersMap))

	// Build quality score map for O(1) lookup during collision resolution
	qualityScoreMap := make(map[int64]float64)
	for _, members := range membersMap {
		for _, member := range members {
			qualityScoreMap[member.FileID] = member.QualityScore
		}
	}
	util.InfoLog("Built quality score map for %d files", len(qualityScoreMap))

	totalClusters := len(clusters)
	util.InfoLog("Found %d clusters to plan", totalClusters)

	result := &Result{
		Errors: make([]error, 0),
	}

	// Clear existing plans (idempotent operation)
	if err := p.store.ClearPlans(); err != nil {
		return nil, fmt.Errorf("failed to clear plans: %w", err)
	}

	// Prepare batch plans
	var allPlans []*store.Plan

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

	// Step 2: Process each cluster (in memory)
	util.InfoLog("Generating plans for all clusters...")
	for _, cluster := range clusters {
		select {
		case <-ctx.Done():
			result.WinnersPlanned = int(winnersPlanned.Load())
			result.DuplicatesSkipped = int(duplicatesSkipped.Load())
			result.SingletonsPlanned = int(singletonsPlanned.Load())
			return result, ctx.Err()
		default:
		}

		// Get cluster members from pre-loaded map
		members, ok := membersMap[cluster.ClusterKey]
		if !ok || len(members) == 0 {
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

		// Get winner file and metadata from pre-loaded maps
		winnerFile, fileExists := filesMap[winner.FileID]
		if !fileExists {
			util.ErrorLog("Winner file %d not found in pre-loaded data", winner.FileID)
			continue
		}

		winnerMeta, metaExists := metadataMap[winner.FileID]
		if !metaExists {
			util.ErrorLog("Winner metadata %d not found in pre-loaded data", winner.FileID)
			continue
		}

		// Check if this is a true compilation (compilation flag + multiple artists)
		isCompilation := false
		if winnerMeta.TagCompilation {
			isCompilation = p.isRealCompilationFast(winner.FileID, winnerMeta.TagAlbum, membersMap, metadataMap)
		}

		// Generate destination path
		destPath := GenerateDestPath(destRoot, winnerMeta, winnerFile.SrcPath, isCompilation)

		// Queue plan for winner
		winnerPlan := &store.Plan{
			FileID:   winner.FileID,
			Action:   p.mode, // copy, move, etc.
			DestPath: destPath,
			Reason:   fmt.Sprintf("winner (score: %.1f)", winner.QualityScore),
		}
		allPlans = append(allPlans, winnerPlan)

		// Log plan event for winner
		if p.logger != nil {
			p.logger.LogPlan(winnerFile.FileKey, winnerFile.SrcPath, destPath, p.mode, winnerPlan.Reason)
		}

		winnersPlanned.Add(1)

		if len(members) == 1 {
			singletonsPlanned.Add(1)
		}

		// Queue plans for losers (skip)
		for _, loser := range losers {
			loserPlan := &store.Plan{
				FileID:   loser.FileID,
				Action:   "skip",
				DestPath: "",
				Reason:   fmt.Sprintf("duplicate (score: %.1f, winner: %d)", loser.QualityScore, winner.FileID),
			}
			allPlans = append(allPlans, loserPlan)

			// Log plan event for loser
			if p.logger != nil {
				if loserFile, ok := filesMap[loser.FileID]; ok {
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

	// Step 3: Batch insert all plans
	util.InfoLog("Writing %d plans to database...", len(allPlans))
	batchSize := 5000
	for i := 0; i < len(allPlans); i += batchSize {
		end := i + batchSize
		if end > len(allPlans) {
			end = len(allPlans)
		}
		batch := allPlans[i:end]
		if err := p.store.InsertPlanBatch(batch); err != nil {
			util.ErrorLog("Failed to insert plan batch: %v", err)
			result.Errors = append(result.Errors, err)
		}
		util.InfoLog("Inserted %d/%d plans (%.1f%%)", end, len(allPlans),
			float64(end)/float64(len(allPlans))*100)
	}

	// Resolve path collisions - pick best quality file for each dest_path
	util.InfoLog("Resolving destination path collisions...")
	collisionsResolved, err := p.resolvePathCollisions(qualityScoreMap, filesMap)
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
// Handles both case-sensitive and case-insensitive filesystems
func (p *Planner) resolvePathCollisions(qualityScoreMap map[int64]float64, filesMap map[int64]*store.File) (int, error) {
	// Get all plans that aren't skipped
	allPlans, err := p.store.GetAllPlans()
	if err != nil {
		return 0, fmt.Errorf("failed to get plans: %w", err)
	}

	if len(allPlans) == 0 {
		return 0, nil
	}

	// Detect filesystem case sensitivity for the destination
	// Use the first plan's dest_path parent directory for testing
	var destRoot string
	for _, plan := range allPlans {
		if plan.Action != "skip" && plan.DestPath != "" {
			destRoot = filepath.Dir(plan.DestPath)
			break
		}
	}

	if destRoot == "" {
		return 0, nil // No non-skipped plans with dest paths
	}

	// Detect if destination filesystem is case-sensitive
	caseSensitive, err := util.DetectFilesystemCaseSensitivity(destRoot)
	if err != nil {
		util.WarnLog("Failed to detect filesystem case sensitivity, assuming case-sensitive: %v", err)
		caseSensitive = true // Safe default
	}

	if !caseSensitive {
		util.InfoLog("Detected case-insensitive filesystem - using case-insensitive path collision detection")
	}

	// Group plans by destination path (normalized for case-insensitive filesystems)
	pathMap := make(map[string][]*store.Plan)
	originalPaths := make(map[string]string) // normalized -> original path mapping

	for _, plan := range allPlans {
		if plan.Action != "skip" && plan.DestPath != "" {
			normalizedPath := util.NormalizePath(plan.DestPath, caseSensitive)
			pathMap[normalizedPath] = append(pathMap[normalizedPath], plan)
			// Remember the first original path we see for each normalized path
			if _, exists := originalPaths[normalizedPath]; !exists {
				originalPaths[normalizedPath] = plan.DestPath
			}
		}
	}

	collisionsResolved := 0

	// Process each collision group
	for normalizedPath, plans := range pathMap {
		if len(plans) <= 1 {
			continue // No collision
		}

		// We have a collision - multiple files want the same dest_path
		displayPath := originalPaths[normalizedPath]
		if !caseSensitive {
			util.WarnLog("Case-insensitive path collision detected: %d files -> %s", len(plans), displayPath)
		} else {
			util.WarnLog("Path collision detected: %d files -> %s", len(plans), displayPath)
		}

		// Get quality scores for each file
		type scoredPlan struct {
			plan  *store.Plan
			score float64
		}
		scored := make([]scoredPlan, 0, len(plans))

		for _, plan := range plans {
			// Get quality score from pre-loaded map (O(1) lookup)
			qualityScore := qualityScoreMap[plan.FileID]

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
		winnerFile := filesMap[winner.plan.FileID]
		winnerPath := "unknown"
		if winnerFile != nil {
			winnerPath = winnerFile.SrcPath
		}
		util.InfoLog("  Keeping: %s (score: %.1f)", winnerPath, winner.score)

		for _, loser := range scored[1:] {
			loserFile := filesMap[loser.plan.FileID]
			loserPath := "unknown"
			if loserFile != nil {
				loserPath = loserFile.SrcPath
			}
			util.InfoLog("  Skipping: %s (score: %.1f)", loserPath, loser.score)

			// Update plan to skip
			updatedPlan := &store.Plan{
				FileID:   loser.plan.FileID,
				Action:   "skip",
				DestPath: "",
				Reason:   fmt.Sprintf("path collision (score: %.1f, winner: %d at %s)", loser.score, winner.plan.FileID, displayPath),
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

// isRealCompilationFast checks if album is truly a compilation using pre-loaded maps
func (p *Planner) isRealCompilationFast(fileID int64, albumName string, membersMap map[string][]*store.ClusterMember, metadataMap map[int64]*store.Metadata) bool {
	if albumName == "" {
		return false
	}

	// Collect unique track artists for this album
	// For compilations, we care about track artists, not album artist
	artistsInAlbum := make(map[string]bool)

	// Iterate through all metadata to find files with same album
	for _, metadata := range metadataMap {
		if metadata.TagAlbum == albumName {
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
		// Apply canonical capitalization for consistency
		folderArtist = meta.CanonicalizeArtistName(folderArtist)
	}
	folderArtist = SanitizePathComponent(folderArtist)

	// Determine album
	album := m.TagAlbum
	// Clean album name (remove URLs, catalog numbers, etc.)
	album = meta.CleanAlbumName(album)
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
		trackArtist = meta.CanonicalizeArtistName(trackArtist)
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
// Enhanced to handle special cases from real-world messy libraries:
// - Ampersands at start (&me) are preserved
// - Exclamation marks (!!!) converted to underscore
// - Hash/@ symbols converted to underscore
// - Leading underscores removed (except for _Singles)
func SanitizePathComponent(s string) string {
	if s == "" {
		return ""
	}

	// Preserve special folder names
	if s == "_Singles" {
		return s
	}

	// Replace illegal filesystem characters with underscores
	illegal := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	for _, char := range illegal {
		s = strings.ReplaceAll(s, char, "_")
	}

	// Replace problematic characters that cause sorting/compatibility issues
	// But preserve ampersands (for artists like "&ME")
	s = strings.ReplaceAll(s, "!", "_")  // !!! → _
	s = strings.ReplaceAll(s, "#", "_")  // #root.access → _root.access
	s = strings.ReplaceAll(s, "@", "_")  // @djxiz → _djxiz

	// Collapse multiple underscores
	for strings.Contains(s, "__") {
		s = strings.ReplaceAll(s, "__", "_")
	}

	// Trim leading underscores (except for special folders like _Singles already handled)
	s = strings.TrimLeft(s, "_")

	// Trim spaces and dots (Windows issues)
	s = strings.TrimSpace(s)
	s = strings.Trim(s, ".")

	// Remove trailing underscores
	s = strings.TrimRight(s, "_")

	// Handle empty result after sanitization
	if s == "" {
		return "Unknown"
	}

	// Limit length to 200 characters (filesystem limits)
	if len(s) > 200 {
		s = s[:200]
		// Re-trim in case we cut in middle of word
		s = strings.TrimRight(s, " _.")
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
