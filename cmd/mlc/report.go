package main

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/franz/music-janitor/internal/report"
	"github.com/franz/music-janitor/internal/store"
	"github.com/franz/music-janitor/internal/util"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate a summary report from the database and event logs",
	Long: `Generate a comprehensive summary report in Markdown format.

The report includes:
- File scan statistics
- Clustering and duplicate detection results
- Planning decisions
- Execution results
- Top errors and warnings
- Detailed duplicate group information

The report is saved to artifacts/reports/<timestamp>/summary.md`,
	RunE: runReport,
}

func init() {
	rootCmd.AddCommand(reportCmd)

	// Report-specific flags
	reportCmd.Flags().String("out", "", "Output directory for report (default: artifacts/reports/<timestamp>)")
	reportCmd.Flags().String("event-log", "", "Path to event log file (optional)")
}

func runReport(cmd *cobra.Command, args []string) error {
	// Get configuration
	dbPath := viper.GetString("db")
	verbose := viper.GetBool("verbose")
	quiet := viper.GetBool("quiet")

	// Set log level
	util.SetVerbose(verbose)
	util.SetQuiet(quiet)

	util.InfoLog("=== Generating Summary Report ===")
	util.InfoLog("Database: %s", dbPath)

	// Open database
	db, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Get event log path if specified
	eventLogPath, _ := cmd.Flags().GetString("event-log")

	// Generate report
	util.InfoLog("Analyzing data...")
	summaryReport, err := report.GenerateSummaryReport(db, eventLogPath)
	if err != nil {
		return fmt.Errorf("failed to generate report: %w", err)
	}

	summaryReport.DatabasePath = dbPath

	// Determine output path
	outputDir, _ := cmd.Flags().GetString("out")
	if outputDir == "" {
		timestamp := time.Now().Format("20060102-150405")
		outputDir = filepath.Join("artifacts", "reports", timestamp)
	}

	outputPath := filepath.Join(outputDir, "summary.md")

	// Write markdown report
	util.InfoLog("Writing report to: %s", outputPath)
	if err := report.WriteMarkdownReport(summaryReport, outputPath); err != nil {
		return fmt.Errorf("failed to write report: %w", err)
	}

	// Summary
	util.SuccessLog("Report generated successfully!")
	util.InfoLog("")
	util.InfoLog("Report saved to: %s", outputPath)
	util.InfoLog("")
	util.InfoLog("Summary:")
	util.InfoLog("  Files scanned: %d", summaryReport.FilesScanned)
	util.InfoLog("  Valid files: %d", summaryReport.FilesValid)
	if summaryReport.FilesWithErrors > 0 {
		util.WarnLog("  Errors: %d", summaryReport.FilesWithErrors)
	}
	if summaryReport.DuplicateClusters > 0 {
		util.InfoLog("  Duplicate groups: %d", summaryReport.DuplicateClusters)
		util.InfoLog("  Duplicates skipped: %d", summaryReport.DuplicatesSkipped)
	}
	if summaryReport.FilesExecuted > 0 {
		util.InfoLog("  Files executed: %d", summaryReport.FilesExecuted)
		util.InfoLog("  Bytes written: %s", util.FormatBytes(summaryReport.BytesWritten))
	}

	return nil
}
