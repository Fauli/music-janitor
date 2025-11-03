package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/franz/music-janitor/internal/execute"
	"github.com/franz/music-janitor/internal/report"
	"github.com/franz/music-janitor/internal/store"
	"github.com/franz/music-janitor/internal/util"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var executeCmd = &cobra.Command{
	Use:   "execute",
	Short: "Execute the plan (copy/move files to destination)",
	Long: `Execute the planned actions to copy/move files to the destination.

This command:
1. Reads the execution plan from the database
2. Copies/moves files to their destination paths
3. Verifies file integrity after copy
4. Updates execution status in database
5. Supports resumability (skips already-executed files)

Safety features:
- Atomic copy (write to .part, then rename)
- Verification (size or hash checking)
- Resumable (can be interrupted and resumed)
- Move mode requires verification before deleting source

Use --dry-run with 'mlc plan' to preview before executing.`,
	RunE: runExecute,
}

func init() {
	rootCmd.AddCommand(executeCmd)
}

func runExecute(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Get configuration
	dbPath := viper.GetString("db")
	concurrency := viper.GetInt("concurrency")
	if concurrency <= 0 {
		concurrency = 4
	}

	verifyMode := viper.GetString("verify")
	if verifyMode == "" {
		verifyMode = "size"
	}

	verbose := viper.GetBool("verbose")
	quiet := viper.GetBool("quiet")

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

	// Check if we have plans
	allPlans, err := db.GetAllPlans()
	if err != nil {
		return fmt.Errorf("failed to get plans: %w", err)
	}

	if len(allPlans) == 0 {
		util.WarnLog("No plans found. Run 'mlc plan --dest <path>' first.")
		return nil
	}

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

	// Create executor
	util.InfoLog("=== Execution ===")
	util.InfoLog("Concurrency: %d workers", concurrency)
	util.InfoLog("Verification: %s", verifyMode)

	executor := execute.New(&execute.Config{
		Store:       db,
		Concurrency: concurrency,
		VerifyMode:  verifyMode,
		DryRun:      false,
		Logger:      logger,
	})

	startTime := time.Now()

	result, err := executor.Execute(ctx)
	if err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	duration := time.Since(startTime)

	// Summary
	util.InfoLog("")
	util.SuccessLog("=== Execution Summary ===")
	util.InfoLog("Total time: %v", duration.Round(time.Millisecond))
	util.InfoLog("Files processed: %d", result.Processed)
	util.InfoLog("  Succeeded: %d", result.Succeeded)
	util.InfoLog("  Skipped: %d", result.Skipped)
	if result.Failed > 0 {
		util.WarnLog("  Failed: %d", result.Failed)
	}
	util.InfoLog("Bytes written: %s", util.FormatBytes(result.BytesWritten))

	if result.Failed > 0 && len(result.Errors) > 0 {
		util.InfoLog("")
		util.WarnLog("Errors encountered:")
		for i, err := range result.Errors {
			if i >= 10 {
				util.WarnLog("... and %d more errors", len(result.Errors)-10)
				break
			}
			util.WarnLog("  - %v", err)
		}
	}

	// Show database stats
	successCount, _ := db.CountSuccessfulExecutions()
	totalBytes, _ := db.GetTotalBytesWritten()

	util.InfoLog("")
	util.InfoLog("Database totals:")
	util.InfoLog("  Successfully executed: %d files", successCount)
	util.InfoLog("  Total written: %s", util.FormatBytes(totalBytes))

	if result.Failed > 0 {
		util.InfoLog("")
		util.InfoLog("To retry failed files: mlc execute")
	}

	// Auto-generate summary report
	util.InfoLog("")
	util.InfoLog("Generating summary report...")

	summaryReport, err := report.GenerateSummaryReport(db, logger.Path())
	if err != nil {
		util.WarnLog("Failed to generate summary report: %v", err)
	} else {
		summaryReport.DatabasePath = dbPath
		summaryReport.ExecutionTime = duration

		timestamp := time.Now().Format("20060102-150405")
		reportDir := filepath.Join("artifacts", "reports", timestamp)
		reportPath := filepath.Join(reportDir, "summary.md")

		if err := report.WriteMarkdownReport(summaryReport, reportPath); err != nil {
			util.WarnLog("Failed to write summary report: %v", err)
		} else {
			util.SuccessLog("Summary report saved to: %s", reportPath)
		}
	}

	return nil
}
