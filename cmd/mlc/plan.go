package main

import (
	"context"
	"fmt"
	"time"

	"github.com/franz/music-janitor/internal/cluster"
	"github.com/franz/music-janitor/internal/meta"
	"github.com/franz/music-janitor/internal/musicbrainz"
	"github.com/franz/music-janitor/internal/plan"
	"github.com/franz/music-janitor/internal/report"
	"github.com/franz/music-janitor/internal/score"
	"github.com/franz/music-janitor/internal/store"
	"github.com/franz/music-janitor/internal/util"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Cluster duplicates, score quality, and generate execution plan",
	Long: `Cluster duplicate files by analyzing artist, title, and duration.
Score each file's quality based on codec, bitrate, and tags.
Generate an execution plan for copying/moving files to destination.

This command performs three operations:
1. Clustering: Group duplicate files together
2. Scoring: Calculate quality scores and select winners
3. Planning: Determine actions for each file (copy/skip)

Use --dry-run to preview the plan without making changes.`,
	RunE: runPlan,
}

func init() {
	rootCmd.AddCommand(planCmd)

	// Plan-specific flags
	planCmd.Flags().Bool("dry-run", false, "Preview plan without saving to database")
	planCmd.Flags().Bool("force-recluster", false, "Force complete re-clustering (discards resume state)")
}

func runPlan(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Get configuration from viper (flags override config file)
	dest := viper.GetString("destination")
	if dest == "" {
		return fmt.Errorf("destination directory is required (use --dest/-d or set in config)")
	}

	mode := viper.GetString("mode")
	if mode == "" {
		mode = "copy"
	}

	// Validate mode
	validModes := map[string]bool{
		"copy":     true,
		"move":     true,
		"hardlink": true,
		"symlink":  true,
	}
	if !validModes[mode] {
		return fmt.Errorf("invalid mode: %s (must be one of: copy, move, hardlink, symlink)", mode)
	}

	dbPath := viper.GetString("db")
	verbose := viper.GetBool("verbose")
	quiet := viper.GetBool("quiet")
	dryRun := viper.GetBool("dry-run")
	forceRecluster, _ := cmd.Flags().GetBool("force-recluster")

	// Set log level
	util.SetVerbose(verbose)
	util.SetQuiet(quiet)

	// Auto-tune for NAS if destination is on network storage
	// Plan stage doesn't use concurrency, but we detect and log network info
	var nasMode *bool
	if viper.IsSet("nas_mode") {
		val := viper.GetBool("nas_mode")
		nasMode = &val
	}

	nasConfig, err := util.AutoTuneForPath("", dest, nasMode, 1)
	if err != nil {
		util.WarnLog("Auto-tuning failed: %v", err)
	}
	// Store nasConfig for potential future use (batch operations, etc.)
	_ = nasConfig

	util.InfoLog("Opening database: %s", dbPath)

	// Check if database is on network storage
	dbNetworkOptimized := false
	if dbInfo, err := util.DetectNetworkFilesystem(dbPath); err == nil && dbInfo.IsNetwork {
		dbNetworkOptimized = true
		util.InfoLog("Database on network storage (%s) - applying optimizations", dbInfo.Protocol)
	}

	// Open database with network optimizations if needed
	db, err := store.OpenWithOptions(dbPath, &store.OpenOptions{
		NetworkOptimized: dbNetworkOptimized || (nasConfig != nil && nasConfig.IsNASMode),
	})
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Create event logger with appropriate log level
	logLevel := report.LevelInfo // Default
	if quiet {
		logLevel = report.LevelWarning // Only warnings and errors
	} else if verbose {
		logLevel = report.LevelDebug // Everything
	}

	logger, err := report.NewEventLogger("artifacts", logLevel)
	if err != nil {
		util.WarnLog("Failed to create event logger: %v", err)
		logger = report.NullLogger()
	}
	defer logger.Close()

	if logger.Path() != "" {
		util.InfoLog("Event log: %s", logger.Path())
	}

	// MusicBrainz integration (optional)
	var mbCache *musicbrainz.Cache
	enableMusicBrainz := viper.GetBool("musicbrainz")
	preloadMusicBrainz := viper.GetBool("musicbrainz_preload")

	if enableMusicBrainz {
		util.InfoLog("=== MusicBrainz Integration ===")
		util.InfoLog("Initializing MusicBrainz client...")

		// Create MusicBrainz client
		mbClient := musicbrainz.NewClient()
		defer mbClient.Close()

		// Create cache
		mbCache = musicbrainz.NewCache(db.DB(), mbClient)

		// Ensure cache schema exists
		if err := mbCache.EnsureSchema(); err != nil {
			util.WarnLog("Failed to initialize MusicBrainz cache: %v", err)
			util.WarnLog("Continuing without MusicBrainz...")
		} else {
			// Set global normalizer for meta package
			meta.GlobalMBNormalizer = mbCache

			util.SuccessLog("MusicBrainz integration enabled")

			// Show cache stats
			entries, hits, _ := mbCache.GetStats()
			util.InfoLog("  Cache: %d artists, %d hits", entries, hits)

			// Optional preload
			if preloadMusicBrainz {
				util.InfoLog("")
				util.InfoLog("Preloading artists from MusicBrainz...")
				util.InfoLog("This will take a while (1 request/second rate limit)")

				// Get all unique artists from metadata
				artists, err := db.GetAllUniqueArtists()
				if err != nil {
					util.WarnLog("Failed to get artists: %v", err)
				} else if len(artists) > 0 {
					util.InfoLog("Found %d unique artists to preload", len(artists))
					if err := mbCache.PreloadArtists(ctx, artists); err != nil {
						util.WarnLog("Preload failed: %v", err)
					}
				}
			}
		}
		util.InfoLog("")
	}

	// Phase 1: Clustering
	util.InfoLog("=== Phase 1: Clustering ===")

	clusterer := cluster.New(&cluster.Config{
		Store:          db,
		Logger:         logger,
		ForceRecluster: forceRecluster,
	})

	startTime := time.Now()

	clusterResult, err := clusterer.Cluster(ctx)
	if err != nil {
		return fmt.Errorf("clustering failed: %w", err)
	}

	clusterDuration := time.Since(startTime)

	util.SuccessLog("Clustering complete in %v", clusterDuration.Round(time.Millisecond))
	util.InfoLog("  Clusters created: %d", clusterResult.ClustersCreated)
	util.InfoLog("  Singleton clusters: %d", clusterResult.SingletonClusters)
	util.InfoLog("  Duplicate clusters: %d", clusterResult.DuplicateClusters)
	if len(clusterResult.Errors) > 0 {
		util.WarnLog("  Errors: %d", len(clusterResult.Errors))
	}

	// Phase 2: Quality Scoring
	util.InfoLog("")
	util.InfoLog("=== Phase 2: Quality Scoring ===")

	scorer := score.New(&score.Config{
		Store:  db,
		Logger: logger,
	})

	scoreStart := time.Now()

	scoreResult, err := scorer.Score(ctx)
	if err != nil {
		return fmt.Errorf("scoring failed: %w", err)
	}

	scoreDuration := time.Since(scoreStart)

	util.SuccessLog("Scoring complete in %v", scoreDuration.Round(time.Millisecond))
	util.InfoLog("  Files scored: %d", scoreResult.FilesScored)
	util.InfoLog("  Winners selected: %d", scoreResult.WinnersSelected)
	if len(scoreResult.Errors) > 0 {
		util.WarnLog("  Errors: %d", len(scoreResult.Errors))
	}

	// Phase 3: Planning
	util.InfoLog("")
	util.InfoLog("=== Phase 3: Planning ===")
	util.InfoLog("Destination: %s", dest)
	util.InfoLog("Mode: %s", mode)
	if dryRun {
		util.InfoLog("Dry-run mode: no changes will be made")
	}

	planner := plan.New(&plan.Config{
		Store:  db,
		Mode:   mode,
		Logger: logger,
	})

	planStart := time.Now()

	planResult, err := planner.Plan(ctx, dest)
	if err != nil {
		return fmt.Errorf("planning failed: %w", err)
	}

	planDuration := time.Since(planStart)

	util.SuccessLog("Planning complete in %v", planDuration.Round(time.Millisecond))
	util.InfoLog("  Winners planned: %d", planResult.WinnersPlanned)
	util.InfoLog("  Duplicates skipped: %d", planResult.DuplicatesSkipped)
	util.InfoLog("  Singletons: %d", planResult.SingletonsPlanned)
	if len(planResult.Errors) > 0 {
		util.WarnLog("  Errors: %d", len(planResult.Errors))
	}

	// Summary
	util.InfoLog("")
	util.SuccessLog("=== Plan Summary ===")
	util.InfoLog("Total time: %v", (clusterDuration + scoreDuration + planDuration).Round(time.Millisecond))
	util.InfoLog("Database: %s", dbPath)

	// Show action counts
	copyPlans, _ := db.CountPlansByAction("copy")
	movePlans, _ := db.CountPlansByAction("move")
	hardlinkPlans, _ := db.CountPlansByAction("hardlink")
	symlinkPlans, _ := db.CountPlansByAction("symlink")
	skipPlans, _ := db.CountPlansByAction("skip")

	util.InfoLog("")
	util.InfoLog("Planned actions:")
	if copyPlans > 0 {
		util.InfoLog("  Copy: %d files", copyPlans)
	}
	if movePlans > 0 {
		util.InfoLog("  Move: %d files", movePlans)
	}
	if hardlinkPlans > 0 {
		util.InfoLog("  Hardlink: %d files", hardlinkPlans)
	}
	if symlinkPlans > 0 {
		util.InfoLog("  Symlink: %d files", symlinkPlans)
	}
	if skipPlans > 0 {
		util.InfoLog("  Skip (duplicates): %d files", skipPlans)
	}

	// Next step guidance
	util.InfoLog("")
	if copyPlans+movePlans+hardlinkPlans+symlinkPlans == 0 {
		util.WarnLog("⚠️  No files to copy/move!")
		if skipPlans > 0 {
			util.InfoLog("   All %d files are duplicates (will be skipped)", skipPlans)
			util.InfoLog("   Your library is already deduplicated!")
		}
		return nil
	}

	if dryRun {
		util.SuccessLog("✓ Dry-run complete!")
		util.InfoLog("")
		util.InfoLog("Review the plan above, then:")
		util.InfoLog("  Execute plan:  mlc execute --db %s", dbPath)
		util.InfoLog("")
		util.InfoLog("TIP: Check artifacts/events-*.jsonl for detailed plan")
	} else {
		util.SuccessLog("✓ Plan created!")
		util.InfoLog("")
		util.InfoLog("Next step:")
		util.InfoLog("  mlc execute --db %s --verify hash", dbPath)
		util.InfoLog("")
		util.InfoLog("Files will be %s'd from source to: %s", mode, dest)
	}

	return nil
}
