package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/franz/music-janitor/internal/store"
	"github.com/franz/music-janitor/internal/util"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run diagnostic checks on the environment and configuration",
	Long: `Run diagnostic checks to ensure mlc can operate correctly.

This command checks:
- Required tools (ffprobe)
- Optional tools (fpcalc for fingerprinting)
- Disk space availability
- File permissions (read source, write destination)
- Database accessibility and integrity
- SQLite version compatibility

Use this command to troubleshoot issues before running mlc operations.`,
	RunE: runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)

	// Doctor-specific flags
	doctorCmd.Flags().String("src", "", "Source directory to check (optional)")
	doctorCmd.Flags().String("dest", "", "Destination directory to check (optional)")
}

type checkResult struct {
	name    string
	message string
	error   bool
	warning bool
}

func runDoctor(cmd *cobra.Command, args []string) error {
	util.InfoLog("=== MLC Doctor - System Diagnostics ===")
	util.InfoLog("")

	results := []checkResult{}

	// 1. Check ffprobe
	results = append(results, checkFFprobe())

	// 2. Check fpcalc (optional)
	results = append(results, checkFpcalc())

	// 3. Check SQLite
	results = append(results, checkSQLite())

	// 4. Check database file
	dbPath := viper.GetString("db")
	results = append(results, checkDatabase(dbPath))

	// 5. Check source directory
	srcPath, _ := cmd.Flags().GetString("src")
	if srcPath == "" {
		srcPath = viper.GetString("source")
	}
	if srcPath != "" {
		results = append(results, checkSourceDirectory(srcPath))
	}

	// 6. Check destination directory
	destPath, _ := cmd.Flags().GetString("dest")
	if destPath == "" {
		destPath = viper.GetString("dest")
	}
	if destPath != "" {
		results = append(results, checkDestinationDirectory(destPath))
	}

	// 7. Check disk space
	if srcPath != "" {
		results = append(results, checkDiskSpace(srcPath, "source"))
	}
	if destPath != "" && destPath != srcPath {
		results = append(results, checkDiskSpace(destPath, "destination"))
	}

	// Print results
	util.InfoLog("")
	util.InfoLog("=== Diagnostic Results ===")
	util.InfoLog("")

	hasErrors := false
	hasWarnings := false

	for _, r := range results {
		symbol := "✓"
		if r.error {
			symbol = "✗"
			hasErrors = true
		} else if r.warning {
			symbol = "⚠"
			hasWarnings = true
		}

		line := fmt.Sprintf("[%s] %s", symbol, r.name)
		if r.message != "" {
			line += fmt.Sprintf(": %s", r.message)
		}

		if r.error {
			util.ErrorLog("%s", line)
		} else if r.warning {
			util.WarnLog("%s", line)
		} else {
			util.SuccessLog("%s", line)
		}
	}

	// Summary
	util.InfoLog("")
	if hasErrors {
		util.ErrorLog("❌ Some critical checks failed. Please resolve errors before running mlc.")
		return fmt.Errorf("system diagnostics failed")
	} else if hasWarnings {
		util.WarnLog("⚠️  Some checks produced warnings. Review them before proceeding.")
	} else {
		util.SuccessLog("✅ All checks passed! System is ready for mlc operations.")
	}

	return nil
}

// checkFFprobe verifies ffprobe is available and gets version
func checkFFprobe() checkResult {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffprobe", "-version")
	output, err := cmd.CombinedOutput()

	if err != nil {
		return checkResult{
			name:    "ffprobe",
			error:   true,
			message: "not found or not executable (required for metadata extraction)",
		}
	}

	// Parse version from first line
	lines := strings.Split(string(output), "\n")
	version := "unknown"
	if len(lines) > 0 {
		parts := strings.Fields(lines[0])
		if len(parts) >= 3 {
			version = parts[2]
		}
	}

	return checkResult{
		name:    "ffprobe",
		message: fmt.Sprintf("version %s", version),
	}
}

// checkFpcalc verifies fpcalc is available (optional)
func checkFpcalc() checkResult {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "fpcalc", "-version")
	output, err := cmd.CombinedOutput()

	if err != nil {
		return checkResult{
			name:    "fpcalc (optional)",
			warning: true,
			message: "not found (required only for fingerprinting feature)",
		}
	}

	// Parse version
	lines := strings.Split(string(output), "\n")
	version := "unknown"
	if len(lines) > 0 {
		parts := strings.Fields(lines[0])
		if len(parts) >= 2 {
			version = parts[1]
		}
	}

	return checkResult{
		name:    "fpcalc (optional)",
		message: fmt.Sprintf("version %s", version),
	}
}

// checkSQLite verifies SQLite version
func checkSQLite() checkResult {
	// We're using modernc.org/sqlite which doesn't require external sqlite
	// Just verify we can get the version
	version := store.SQLiteVersion()
	if version == "" {
		return checkResult{
			name:    "SQLite",
			error:   true,
			message: "unable to determine version",
		}
	}

	return checkResult{
		name:    "SQLite",
		message: fmt.Sprintf("version %s (built-in)", version),
	}
}

// checkDatabase verifies database file accessibility
func checkDatabase(dbPath string) checkResult {
	if dbPath == "" {
		return checkResult{
			name:    "Database",
			warning: true,
			message: "no database path specified (use --db flag or config)",
		}
	}

	// Check if database exists
	info, err := os.Stat(dbPath)
	if err != nil {
		if os.IsNotExist(err) {
			return checkResult{
				name:    "Database",
				message: fmt.Sprintf("%s (will be created on first run)", dbPath),
			}
		}
		return checkResult{
			name:    "Database",
			error:   true,
			message: fmt.Sprintf("cannot access %s: %v", dbPath, err),
		}
	}

	// Check if it's a regular file
	if !info.Mode().IsRegular() {
		return checkResult{
			name:    "Database",
			error:   true,
			message: fmt.Sprintf("%s is not a regular file", dbPath),
		}
	}

	// Try to open it
	db, err := store.Open(dbPath)
	if err != nil {
		return checkResult{
			name:    "Database",
			error:   true,
			message: fmt.Sprintf("cannot open %s: %v", dbPath, err),
		}
	}
	defer db.Close()

	// Check integrity
	if err := db.CheckIntegrity(); err != nil {
		return checkResult{
			name:    "Database",
			error:   true,
			message: fmt.Sprintf("integrity check failed: %v", err),
		}
	}

	// Get some stats
	fileCount, _ := db.CountFilesByStatus("meta_ok")
	size := util.FormatBytes(info.Size())

	return checkResult{
		name:    "Database",
		message: fmt.Sprintf("%s (%s, %d files)", dbPath, size, fileCount),
	}
}

// checkSourceDirectory verifies source directory is readable
func checkSourceDirectory(path string) checkResult {
	info, err := os.Stat(path)
	if err != nil {
		return checkResult{
			name:    "Source directory",
			error:   true,
			message: fmt.Sprintf("cannot access %s: %v", path, err),
		}
	}

	if !info.IsDir() {
		return checkResult{
			name:    "Source directory",
			error:   true,
			message: fmt.Sprintf("%s is not a directory", path),
		}
	}

	// Check read permission by trying to list directory
	entries, err := os.ReadDir(path)
	if err != nil {
		return checkResult{
			name:    "Source directory",
			error:   true,
			message: fmt.Sprintf("cannot read %s: %v", path, err),
		}
	}

	return checkResult{
		name:    "Source directory",
		message: fmt.Sprintf("%s (%d entries)", path, len(entries)),
	}
}

// checkDestinationDirectory verifies destination directory is writable
func checkDestinationDirectory(path string) checkResult {
	// Check if exists
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Try to create it
			if err := os.MkdirAll(path, 0755); err != nil {
				return checkResult{
					name:    "Destination directory",
					error:   true,
					message: fmt.Sprintf("cannot create %s: %v", path, err),
				}
			}
			return checkResult{
				name:    "Destination directory",
				message: fmt.Sprintf("%s (created)", path),
			}
		}
		return checkResult{
			name:    "Destination directory",
			error:   true,
			message: fmt.Sprintf("cannot access %s: %v", path, err),
		}
	}

	if !info.IsDir() {
		return checkResult{
			name:    "Destination directory",
			error:   true,
			message: fmt.Sprintf("%s is not a directory", path),
		}
	}

	// Check write permission by creating a temp file
	testFile := filepath.Join(path, ".mlc_write_test")
	f, err := os.Create(testFile)
	if err != nil {
		return checkResult{
			name:    "Destination directory",
			error:   true,
			message: fmt.Sprintf("cannot write to %s: %v", path, err),
		}
	}
	f.Close()
	os.Remove(testFile)

	return checkResult{
		name:    "Destination directory",
		message: fmt.Sprintf("%s (writable)", path),
	}
}

// checkDiskSpace verifies available disk space
func checkDiskSpace(path string, label string) checkResult {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return checkResult{
			name:    fmt.Sprintf("Disk space (%s)", label),
			warning: true,
			message: fmt.Sprintf("cannot determine disk space: %v", err),
		}
	}

	// Available bytes = available blocks * block size
	availBytes := stat.Bavail * uint64(stat.Bsize)
	totalBytes := stat.Blocks * uint64(stat.Bsize)
	usedBytes := totalBytes - (stat.Bfree * uint64(stat.Bsize))

	availGB := float64(availBytes) / (1024 * 1024 * 1024)
	usedPercent := float64(usedBytes) / float64(totalBytes) * 100

	// Warn if less than 10GB available or >90% used
	warning := false
	warningMsg := ""
	if availGB < 10 {
		warning = true
		warningMsg = " (low space!)"
	} else if usedPercent > 90 {
		warning = true
		warningMsg = " (>90% used)"
	}

	return checkResult{
		name:    fmt.Sprintf("Disk space (%s)", label),
		warning: warning,
		message: fmt.Sprintf("%.1f GB available%s", availGB, warningMsg),
	}
}
