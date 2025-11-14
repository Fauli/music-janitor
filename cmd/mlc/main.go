package main

import (
	"fmt"
	"os"

	"github.com/franz/music-janitor/internal/util"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// Version is set at build time
	Version = "dev"

	cfgFile string

	rootCmd = &cobra.Command{
		Use:   "mlc",
		Short: "Music Library Cleaner - deduplicate and organize your music collection",
		Long: `mlc (Music Library Cleaner) is a deterministic, resumable music library cleaner.
It scans a messy archive of audio files and produces a clean, deduplicated,
normalized destination library with audit logs and safe copy operations.`,
		Version: Version,
	}
)

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags - Core paths
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./configs/example.yaml)")
	rootCmd.PersistentFlags().StringP("source", "s", "", "source directory to scan")
	rootCmd.PersistentFlags().StringP("dest", "d", "", "destination directory for clean library")
	rootCmd.PersistentFlags().String("db", "mlc-state.db", "state database file")

	// Global flags - Output control
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "quiet output (errors only)")
	rootCmd.PersistentFlags().Bool("dry-run", false, "plan without executing (dry-run mode)")
	rootCmd.PersistentFlags().Bool("no-auto-healing", false, "disable automatic self-healing (warnings only)")

	// Global flags - Execution options
	rootCmd.PersistentFlags().String("mode", "", "execution mode: copy, move, hardlink, symlink (default: copy)")
	rootCmd.PersistentFlags().IntP("concurrency", "c", 0, "number of parallel workers (default: 8)")
	rootCmd.PersistentFlags().String("layout", "", "destination layout: default, alt1, alt2")
	rootCmd.PersistentFlags().Bool("nas-mode", false, "enable/disable NAS optimizations (default: auto-detect)")

	// Global flags - Quality & verification
	rootCmd.PersistentFlags().String("hashing", "", "hash algorithm: sha1, xxh3, none (default: sha1)")
	rootCmd.PersistentFlags().String("verify", "", "verification mode: size, hash, full (default: hash)")
	rootCmd.PersistentFlags().Bool("fingerprinting", false, "enable acoustic fingerprinting (requires fpcalc)")
	rootCmd.PersistentFlags().Bool("write-tags", true, "write enriched metadata tags to destination files (default: true)")

	// Global flags - MusicBrainz integration
	rootCmd.PersistentFlags().Bool("musicbrainz", false, "enable MusicBrainz artist name normalization (requires internet)")
	rootCmd.PersistentFlags().Bool("musicbrainz-preload", false, "preload all artists from MusicBrainz before clustering (slower but more accurate)")

	// Global flags - Duplicate handling
	rootCmd.PersistentFlags().String("duplicates", "", "duplicate policy: keep, quarantine, delete (default: keep)")
	rootCmd.PersistentFlags().Bool("prefer-existing", false, "prefer existing files in destination on conflict")

	// Bind flags to viper (command-line flags override config file)
	viper.BindPFlag("source", rootCmd.PersistentFlags().Lookup("source"))
	viper.BindPFlag("destination", rootCmd.PersistentFlags().Lookup("dest"))
	viper.BindPFlag("db", rootCmd.PersistentFlags().Lookup("db"))
	viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
	viper.BindPFlag("quiet", rootCmd.PersistentFlags().Lookup("quiet"))
	viper.BindPFlag("dry_run", rootCmd.PersistentFlags().Lookup("dry-run"))
	viper.BindPFlag("no-auto-healing", rootCmd.PersistentFlags().Lookup("no-auto-healing"))
	viper.BindPFlag("mode", rootCmd.PersistentFlags().Lookup("mode"))
	viper.BindPFlag("concurrency", rootCmd.PersistentFlags().Lookup("concurrency"))
	viper.BindPFlag("layout", rootCmd.PersistentFlags().Lookup("layout"))
	viper.BindPFlag("nas_mode", rootCmd.PersistentFlags().Lookup("nas-mode"))
	viper.BindPFlag("hashing", rootCmd.PersistentFlags().Lookup("hashing"))
	viper.BindPFlag("verify", rootCmd.PersistentFlags().Lookup("verify"))
	viper.BindPFlag("fingerprinting", rootCmd.PersistentFlags().Lookup("fingerprinting"))
	viper.BindPFlag("write-tags", rootCmd.PersistentFlags().Lookup("write-tags"))
	viper.BindPFlag("duplicate_policy", rootCmd.PersistentFlags().Lookup("duplicates"))
	viper.BindPFlag("prefer_existing", rootCmd.PersistentFlags().Lookup("prefer-existing"))
	viper.BindPFlag("musicbrainz", rootCmd.PersistentFlags().Lookup("musicbrainz"))
	viper.BindPFlag("musicbrainz_preload", rootCmd.PersistentFlags().Lookup("musicbrainz-preload"))
}

func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag
		viper.SetConfigFile(cfgFile)
	} else {
		// Search for config in common locations
		viper.AddConfigPath("./configs")
		viper.AddConfigPath(".")
		viper.SetConfigName("example")
		viper.SetConfigType("yaml")
	}

	// Read in environment variables that match
	viper.SetEnvPrefix("MLC")
	viper.AutomaticEnv()

	// If a config file is found, read it in
	if err := viper.ReadInConfig(); err == nil && !viper.GetBool("quiet") {
		util.InfoLog("Using config file: %s", viper.ConfigFileUsed())
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
