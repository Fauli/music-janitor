package main

import (
	"context"
	"fmt"
	"time"

	"github.com/franz/music-janitor/internal/cluster"
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
	planCmd.Flags().Bool("dry-run", false, "Show plan without executing")
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

	// Set log level
	util.SetVerbose(verbose)
	util.SetQuiet(quiet)

	util.InfoLog("Opening database: %s", dbPath)

	// Open database
	db, err := store.Open(dbPath)
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

	// Phase 1: Clustering
	util.InfoLog("=== Phase 1: Clustering ===")

	clusterer := cluster.New(&cluster.Config{
		Store:  db,
		Logger: logger,
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

	util.InfoLog("")
	if dryRun {
		util.InfoLog("Dry-run complete. Review the plan above.")
		util.InfoLog("To execute: mlc execute")
	} else {
		util.InfoLog("Plan ready for execution.")
		util.InfoLog("Next step: mlc execute")
	}

	return nil
}
