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

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./configs/example.yaml)")
	rootCmd.PersistentFlags().String("db", "mlc-state.db", "state database file")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "quiet output (errors only)")

	// Bind flags to viper
	viper.BindPFlag("db", rootCmd.PersistentFlags().Lookup("db"))
	viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
	viper.BindPFlag("quiet", rootCmd.PersistentFlags().Lookup("quiet"))
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
