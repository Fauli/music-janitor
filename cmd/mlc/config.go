package main

import (
	"github.com/spf13/viper"
)

// GetConfigString retrieves a string config value with proper precedence:
// 1. Command-line flag (if set)
// 2. Environment variable (MLC_*)
// 3. Config file
// 4. Default value
func GetConfigString(key string, defaultValue string) string {
	val := viper.GetString(key)
	if val == "" {
		return defaultValue
	}
	return val
}

// GetConfigInt retrieves an int config value with proper precedence
func GetConfigInt(key string, defaultValue int) int {
	val := viper.GetInt(key)
	if val == 0 {
		return defaultValue
	}
	return val
}

// GetConfigBool retrieves a bool config value
func GetConfigBool(key string) bool {
	return viper.GetBool(key)
}

// GetConfigStringSlice retrieves a string slice config value
func GetConfigStringSlice(key string) []string {
	return viper.GetStringSlice(key)
}
