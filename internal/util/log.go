package util

import (
	"fmt"
	"os"
	"time"
)

// LogLevel represents the severity of a log message
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

var (
	currentLogLevel = LevelInfo
	useColors       = true
)

// SetLogLevel sets the minimum log level to display
func SetLogLevel(level LogLevel) {
	currentLogLevel = level
}

// SetVerbose enables verbose (debug) logging
func SetVerbose(verbose bool) {
	if verbose {
		currentLogLevel = LevelDebug
	}
}

// SetQuiet enables quiet mode (errors only)
func SetQuiet(quiet bool) {
	if quiet {
		currentLogLevel = LevelError
	}
}

// SetColors enables or disables colored output
func SetColors(enabled bool) {
	useColors = enabled
}

func colorize(color string, text string) string {
	if !useColors {
		return text
	}
	reset := "\033[0m"
	return color + text + reset
}

// DebugLog logs debug messages
func DebugLog(format string, args ...interface{}) {
	if currentLogLevel <= LevelDebug {
		gray := "\033[90m"
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(os.Stderr, "%s [DEBUG] %s\n", colorize(gray, timestamp()), msg)
	}
}

// InfoLog logs informational messages
func InfoLog(format string, args ...interface{}) {
	if currentLogLevel <= LevelInfo {
		cyan := "\033[36m"
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(os.Stderr, "%s [INFO]  %s\n", colorize(cyan, timestamp()), msg)
	}
}

// WarnLog logs warning messages
func WarnLog(format string, args ...interface{}) {
	if currentLogLevel <= LevelWarn {
		yellow := "\033[33m"
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(os.Stderr, "%s [WARN]  %s\n", colorize(yellow, timestamp()), msg)
	}
}

// ErrorLog logs error messages
func ErrorLog(format string, args ...interface{}) {
	if currentLogLevel <= LevelError {
		red := "\033[31m"
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(os.Stderr, "%s [ERROR] %s\n", colorize(red, timestamp()), msg)
	}
}

// SuccessLog logs success messages (always shown unless quiet)
func SuccessLog(format string, args ...interface{}) {
	if currentLogLevel <= LevelInfo {
		green := "\033[32m"
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(os.Stderr, "%s [OK]    %s\n", colorize(green, timestamp()), msg)
	}
}

func timestamp() string {
	return time.Now().Format("15:04:05")
}
