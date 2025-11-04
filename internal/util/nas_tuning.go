package util

import (
	"fmt"
)

// NASConfig holds NAS-optimized settings
type NASConfig struct {
	Concurrency   int
	BufferSize    int
	RetryAttempts int
	TimeoutSec    int
	IsNASMode     bool
	DetectedInfo  *NetworkInfo
}

// AutoTuneForPath detects if paths are on network storage and returns optimized settings
// If nasMode is explicitly set (true/false via pointer), it overrides auto-detection
func AutoTuneForPath(srcPath, destPath string, nasMode *bool, baseConcurrency int) (*NASConfig, error) {
	cfg := &NASConfig{
		Concurrency:   baseConcurrency,
		BufferSize:    128 * 1024, // Default 128KB
		RetryAttempts: 0,
		TimeoutSec:    10,
		IsNASMode:     false,
	}

	// Check if NAS mode is explicitly set via flag/config
	if nasMode != nil {
		cfg.IsNASMode = *nasMode
		if cfg.IsNASMode {
			applyNASOptimizations(cfg)
			InfoLog("NAS mode: explicitly enabled via config/flag")
		} else {
			InfoLog("NAS mode: explicitly disabled via config/flag")
		}
		return cfg, nil
	}

	// Auto-detect network filesystems
	var isNetwork bool
	var detectedInfo *NetworkInfo

	// Check source path
	if srcPath != "" {
		srcInfo, err := DetectNetworkFilesystem(srcPath)
		if err != nil {
			WarnLog("Failed to detect filesystem for source (%s): %v", srcPath, err)
		} else if srcInfo.IsNetwork {
			isNetwork = true
			detectedInfo = srcInfo
			InfoLog("Network filesystem detected: source is on %s (%s)",
				srcInfo.Protocol, srcInfo.MountPath)
		}
	}

	// Check destination path (if provided)
	if !isNetwork && destPath != "" {
		destInfo, err := DetectNetworkFilesystem(destPath)
		if err != nil {
			WarnLog("Failed to detect filesystem for destination (%s): %v", destPath, err)
		} else if destInfo.IsNetwork {
			isNetwork = true
			detectedInfo = destInfo
			InfoLog("Network filesystem detected: destination is on %s (%s)",
				destInfo.Protocol, destInfo.MountPath)
		}
	}

	// Apply NAS optimizations if network detected
	if isNetwork {
		cfg.IsNASMode = true
		cfg.DetectedInfo = detectedInfo
		applyNASOptimizations(cfg)

		InfoLog("")
		InfoLog("=== NAS Optimization Enabled ===")
		InfoLog("Detected %s mount at: %s", detectedInfo.Protocol, detectedInfo.MountPath)
		InfoLog("Auto-tuned settings:")
		InfoLog("  Concurrency: %d → %d workers", baseConcurrency, cfg.Concurrency)
		InfoLog("  Buffer size: 128KB → %dKB", cfg.BufferSize/1024)
		InfoLog("  Retry attempts: 0 → %d", cfg.RetryAttempts)
		InfoLog("  Timeout: %ds per operation", cfg.TimeoutSec)
		InfoLog("")
		InfoLog("TIP: Use --nas-mode=false to disable auto-tuning")
		InfoLog("")
	} else {
		InfoLog("Local filesystem detected - using standard settings")
	}

	return cfg, nil
}

// applyNASOptimizations applies NAS-specific optimizations to config
func applyNASOptimizations(cfg *NASConfig) {
	// Reduce concurrency to avoid overwhelming network connections
	// NAS devices often have limited concurrent connection capacity
	if cfg.Concurrency > 4 {
		cfg.Concurrency = 4
	} else if cfg.Concurrency == 0 {
		cfg.Concurrency = 2 // Minimum for NAS
	}

	// Increase buffer size for network transfers
	// Larger buffers reduce round-trips over network
	cfg.BufferSize = 256 * 1024 // 256KB for network

	// Enable retries for transient network failures
	cfg.RetryAttempts = 3

	// Increase timeout for network operations
	cfg.TimeoutSec = 30
}

// FormatNASSettings returns a human-readable string of NAS settings
func FormatNASSettings(cfg *NASConfig) string {
	if !cfg.IsNASMode {
		return "NAS mode: disabled (local filesystem)"
	}

	protocol := "unknown"
	mountPath := "unknown"
	if cfg.DetectedInfo != nil {
		protocol = cfg.DetectedInfo.Protocol
		mountPath = cfg.DetectedInfo.MountPath
	}

	return fmt.Sprintf(`NAS mode: enabled
  Protocol: %s
  Mount: %s
  Concurrency: %d workers
  Buffer: %dKB
  Retries: %d
  Timeout: %ds`,
		protocol, mountPath,
		cfg.Concurrency, cfg.BufferSize/1024,
		cfg.RetryAttempts, cfg.TimeoutSec)
}
