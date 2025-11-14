package util

import "github.com/spf13/viper"

// GetAutoHealing returns whether auto-healing is enabled
// Auto-healing can be disabled with --no-auto-healing flag
func GetAutoHealing() bool {
	// If no-auto-healing is set, return false (auto-healing disabled)
	// Otherwise return true (auto-healing enabled by default)
	return !viper.GetBool("no-auto-healing")
}
