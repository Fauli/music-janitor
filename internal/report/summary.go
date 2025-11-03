package report

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/franz/music-janitor/internal/store"
	"github.com/franz/music-janitor/internal/util"
)

// SummaryReport represents a complete summary report
type SummaryReport struct {
	GeneratedAt time.Time
	Duration    time.Duration

	// Scan statistics
	FilesScanned     int
	FilesValid       int
	FilesWithErrors  int

	// Cluster statistics
	ClustersCreated   int
	SingletonClusters int
	DuplicateClusters int

	// Planning statistics
	WinnersPlanned    int
	DuplicatesSkipped int

	// Execution statistics
	FilesExecuted  int
	FilesFailed    int
	BytesWritten   int64
	ExecutionTime  time.Duration

	// Details
	TopErrors      []ErrorSummary
	Conflicts      []ConflictInfo
	DuplicateSets  []DuplicateSet

	// Metadata
	SourcePath      string
	DestinationPath string
	Mode            string
	DatabasePath    string
	EventLogPath    string
}

// ErrorSummary represents an error with its count
type ErrorSummary struct {
	Error string
	Count int
}

// ConflictInfo represents a file conflict
type ConflictInfo struct {
	SrcPath  string
	DestPath string
	Reason   string
}

// DuplicateSet represents a cluster of duplicate files
type DuplicateSet struct {
	ClusterKey string
	Hint       string
	Winner     DuplicateFile
	Losers     []DuplicateFile
}

// DuplicateFile represents a file in a duplicate set
type DuplicateFile struct {
	Path         string
	Score        float64
	Codec        string
	Bitrate      int
	SampleRate   int
	Lossless     bool
	SizeBytes    int64
}

// GenerateSummaryReport creates a summary report from database and event logs
func GenerateSummaryReport(db *store.Store, eventLogPath string) (*SummaryReport, error) {
	report := &SummaryReport{
		GeneratedAt:  time.Now(),
		EventLogPath: eventLogPath,
		TopErrors:    make([]ErrorSummary, 0),
		Conflicts:    make([]ConflictInfo, 0),
		DuplicateSets: make([]DuplicateSet, 0),
	}

	// Gather scan statistics
	discovered, _ := db.CountFilesByStatus("discovered")
	metaOK, _ := db.CountFilesByStatus("meta_ok")
	errors, _ := db.CountFilesByStatus("error")
	executed, _ := db.CountFilesByStatus("executed")

	report.FilesScanned = discovered + metaOK + errors + executed
	report.FilesValid = metaOK + executed
	report.FilesWithErrors = errors

	// Gather cluster statistics
	clusters, _ := db.GetAllClusters()
	report.ClustersCreated = len(clusters)

	for _, cluster := range clusters {
		members, _ := db.GetClusterMembers(cluster.ClusterKey)
		if len(members) == 1 {
			report.SingletonClusters++
		} else {
			report.DuplicateClusters++
		}
	}

	// Gather planning statistics
	copyPlans, _ := db.CountPlansByAction("copy")
	movePlans, _ := db.CountPlansByAction("move")
	hardlinkPlans, _ := db.CountPlansByAction("hardlink")
	symlinkPlans, _ := db.CountPlansByAction("symlink")
	skipPlans, _ := db.CountPlansByAction("skip")

	report.WinnersPlanned = copyPlans + movePlans + hardlinkPlans + symlinkPlans
	report.DuplicatesSkipped = skipPlans

	// Gather execution statistics
	allExecs, _ := db.GetAllExecutions()
	for _, exec := range allExecs {
		if exec.VerifyOK {
			report.FilesExecuted++
			report.BytesWritten += exec.BytesWritten
		} else if exec.Error != "" {
			report.FilesFailed++
		}
	}

	// Gather duplicate sets (top 20 by member count)
	report.DuplicateSets = gatherDuplicateSets(db, 20)

	// Gather top errors (top 10)
	report.TopErrors = gatherTopErrors(db, 10)

	return report, nil
}

// gatherDuplicateSets retrieves duplicate clusters with details
func gatherDuplicateSets(db *store.Store, limit int) []DuplicateSet {
	clusters, _ := db.GetAllClusters()
	sets := make([]DuplicateSet, 0)

	// Build duplicate sets
	for _, cluster := range clusters {
		members, _ := db.GetClusterMembers(cluster.ClusterKey)
		if len(members) <= 1 {
			continue // Skip singletons
		}

		set := DuplicateSet{
			ClusterKey: cluster.ClusterKey,
			Hint:       cluster.Hint,
			Losers:     make([]DuplicateFile, 0),
		}

		// Get details for each member
		for _, member := range members {
			file, _ := db.GetFileByID(member.FileID)
			if file == nil {
				continue
			}

			metadata, _ := db.GetMetadata(member.FileID)

			dupFile := DuplicateFile{
				Path:      file.SrcPath,
				Score:     member.QualityScore,
				SizeBytes: file.SizeBytes,
			}

			if metadata != nil {
				dupFile.Codec = metadata.Codec
				dupFile.Bitrate = metadata.BitrateKbps
				dupFile.SampleRate = metadata.SampleRate
				dupFile.Lossless = metadata.Lossless
			}

			if member.Preferred {
				set.Winner = dupFile
			} else {
				set.Losers = append(set.Losers, dupFile)
			}
		}

		sets = append(sets, set)
	}

	// Sort by number of losers (most duplicates first)
	sort.Slice(sets, func(i, j int) bool {
		return len(sets[i].Losers) > len(sets[j].Losers)
	})

	// Limit results
	if len(sets) > limit {
		sets = sets[:limit]
	}

	return sets
}

// gatherTopErrors retrieves the most common errors
func gatherTopErrors(db *store.Store, limit int) []ErrorSummary {
	errorFiles, _ := db.GetFilesByStatus("error")

	errorCounts := make(map[string]int)
	for _, file := range errorFiles {
		if file.Error != "" {
			errorCounts[file.Error]++
		}
	}

	// Convert to slice
	errors := make([]ErrorSummary, 0, len(errorCounts))
	for err, count := range errorCounts {
		errors = append(errors, ErrorSummary{
			Error: err,
			Count: count,
		})
	}

	// Sort by count (descending)
	sort.Slice(errors, func(i, j int) bool {
		return errors[i].Count > errors[j].Count
	})

	// Limit results
	if len(errors) > limit {
		errors = errors[:limit]
	}

	return errors
}

// WriteMarkdownReport writes the summary report as Markdown
func WriteMarkdownReport(report *SummaryReport, outputPath string) error {
	// Create output directory
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Generate markdown content
	var md strings.Builder

	// Header
	md.WriteString("# Music Library Cleaner - Summary Report\n\n")
	md.WriteString(fmt.Sprintf("**Generated:** %s\n\n", report.GeneratedAt.Format("2006-01-02 15:04:05")))

	if report.DatabasePath != "" {
		md.WriteString(fmt.Sprintf("**Database:** `%s`\n\n", report.DatabasePath))
	}
	if report.EventLogPath != "" {
		md.WriteString(fmt.Sprintf("**Event Log:** `%s`\n\n", report.EventLogPath))
	}

	md.WriteString("---\n\n")

	// Overview
	md.WriteString("## 📊 Overview\n\n")
	md.WriteString("| Metric | Value |\n")
	md.WriteString("|--------|-------|\n")
	md.WriteString(fmt.Sprintf("| Files Scanned | %d |\n", report.FilesScanned))
	md.WriteString(fmt.Sprintf("| Files Valid | %d |\n", report.FilesValid))
	if report.FilesWithErrors > 0 {
		md.WriteString(fmt.Sprintf("| Files with Errors | %d |\n", report.FilesWithErrors))
	}
	md.WriteString("\n")

	// Clustering
	if report.ClustersCreated > 0 {
		md.WriteString("## 🔗 Clustering\n\n")
		md.WriteString("| Metric | Value |\n")
		md.WriteString("|--------|-------|\n")
		md.WriteString(fmt.Sprintf("| Total Clusters | %d |\n", report.ClustersCreated))
		md.WriteString(fmt.Sprintf("| Unique Files (Singletons) | %d |\n", report.SingletonClusters))
		md.WriteString(fmt.Sprintf("| Duplicate Groups | %d |\n", report.DuplicateClusters))
		md.WriteString("\n")
	}

	// Planning
	if report.WinnersPlanned > 0 || report.DuplicatesSkipped > 0 {
		md.WriteString("## 📋 Planning\n\n")
		md.WriteString("| Metric | Value |\n")
		md.WriteString("|--------|-------|\n")
		md.WriteString(fmt.Sprintf("| Winners Selected | %d |\n", report.WinnersPlanned))
		md.WriteString(fmt.Sprintf("| Duplicates Skipped | %d |\n", report.DuplicatesSkipped))

		if report.DestinationPath != "" {
			md.WriteString(fmt.Sprintf("| Destination | `%s` |\n", report.DestinationPath))
		}
		if report.Mode != "" {
			md.WriteString(fmt.Sprintf("| Mode | %s |\n", report.Mode))
		}
		md.WriteString("\n")
	}

	// Execution
	if report.FilesExecuted > 0 || report.FilesFailed > 0 {
		md.WriteString("## ⚡ Execution\n\n")
		md.WriteString("| Metric | Value |\n")
		md.WriteString("|--------|-------|\n")
		md.WriteString(fmt.Sprintf("| Files Executed | %d |\n", report.FilesExecuted))
		if report.FilesFailed > 0 {
			md.WriteString(fmt.Sprintf("| Files Failed | %d |\n", report.FilesFailed))
		}
		md.WriteString(fmt.Sprintf("| Bytes Written | %s |\n", util.FormatBytes(report.BytesWritten)))
		if report.ExecutionTime > 0 {
			md.WriteString(fmt.Sprintf("| Execution Time | %s |\n", report.ExecutionTime.Round(time.Second)))
		}
		md.WriteString("\n")
	}

	// Duplicate Sets
	if len(report.DuplicateSets) > 0 {
		md.WriteString("## 🔍 Duplicate Groups (Top 20)\n\n")
		md.WriteString("*Showing groups with the most duplicates*\n\n")

		for i, set := range report.DuplicateSets {
			md.WriteString(fmt.Sprintf("### %d. %s\n\n", i+1, set.Hint))
			if set.Hint == "" {
				md.WriteString(fmt.Sprintf("### %d. Cluster %s\n\n", i+1, set.ClusterKey[:8]))
			}

			md.WriteString(fmt.Sprintf("**Total copies:** %d duplicates + 1 winner\n\n", len(set.Losers)))

			// Winner
			md.WriteString("**✅ Winner (kept):**\n")
			md.WriteString(fmt.Sprintf("- **Score:** %.1f\n", set.Winner.Score))
			md.WriteString(fmt.Sprintf("- **Format:** %s", set.Winner.Codec))
			if set.Winner.Lossless {
				md.WriteString(" (lossless)")
			}
			md.WriteString("\n")
			if set.Winner.Bitrate > 0 {
				md.WriteString(fmt.Sprintf("- **Bitrate:** %d kbps\n", set.Winner.Bitrate))
			}
			if set.Winner.SampleRate > 0 {
				md.WriteString(fmt.Sprintf("- **Sample Rate:** %d Hz\n", set.Winner.SampleRate))
			}
			md.WriteString(fmt.Sprintf("- **Size:** %s\n", util.FormatBytes(set.Winner.SizeBytes)))
			md.WriteString(fmt.Sprintf("- **Path:** `%s`\n", truncatePath(set.Winner.Path, 80)))
			md.WriteString("\n")

			// Losers
			if len(set.Losers) > 0 {
				md.WriteString("**❌ Duplicates (skipped):**\n\n")

				for j, loser := range set.Losers {
					md.WriteString(fmt.Sprintf("%d. Score: %.1f | %s", j+1, loser.Score, loser.Codec))
					if loser.Lossless {
						md.WriteString(" (lossless)")
					}
					if loser.Bitrate > 0 {
						md.WriteString(fmt.Sprintf(" | %d kbps", loser.Bitrate))
					}
					md.WriteString(fmt.Sprintf(" | %s\n", util.FormatBytes(loser.SizeBytes)))
					md.WriteString(fmt.Sprintf("   - `%s`\n", truncatePath(loser.Path, 80)))
				}
				md.WriteString("\n")
			}
		}
	}

	// Errors
	if len(report.TopErrors) > 0 {
		md.WriteString("## ⚠️ Top Errors\n\n")
		md.WriteString("| Count | Error |\n")
		md.WriteString("|-------|-------|\n")
		for _, err := range report.TopErrors {
			md.WriteString(fmt.Sprintf("| %d | %s |\n", err.Count, err.Error))
		}
		md.WriteString("\n")
	}

	// Conflicts
	if len(report.Conflicts) > 0 {
		md.WriteString("## 🚨 Conflicts\n\n")
		md.WriteString("| Source | Destination | Reason |\n")
		md.WriteString("|--------|-------------|--------|\n")
		for _, conflict := range report.Conflicts {
			md.WriteString(fmt.Sprintf("| `%s` | `%s` | %s |\n",
				truncatePath(conflict.SrcPath, 40),
				truncatePath(conflict.DestPath, 40),
				conflict.Reason))
		}
		md.WriteString("\n")
	}

	// Footer
	md.WriteString("---\n\n")
	md.WriteString("*Generated by [MLC](https://github.com/franz/music-janitor) - Music Library Cleaner*\n")

	// Write to file
	if err := os.WriteFile(outputPath, []byte(md.String()), 0644); err != nil {
		return fmt.Errorf("failed to write report: %w", err)
	}

	return nil
}

// truncatePath truncates a file path to a maximum length
func truncatePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	// Truncate from the middle, keeping start and end
	start := maxLen/2 - 2
	end := len(path) - (maxLen/2 - 2)
	return path[:start] + "..." + path[end:]
}
