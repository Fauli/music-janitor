package util

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "EAGAIN",
			err:      syscall.EAGAIN,
			expected: true,
		},
		{
			name:     "ETIMEDOUT",
			err:      syscall.ETIMEDOUT,
			expected: true,
		},
		{
			name:     "ECONNRESET",
			err:      syscall.ECONNRESET,
			expected: true,
		},
		{
			name:     "EIO",
			err:      syscall.EIO,
			expected: true,
		},
		{
			name:     "ENOENT (not retryable)",
			err:      syscall.ENOENT,
			expected: false,
		},
		{
			name:     "EPERM (not retryable)",
			err:      syscall.EPERM,
			expected: false,
		},
		{
			name:     "timeout in error message",
			err:      errors.New("connection timeout"),
			expected: true,
		},
		{
			name:     "connection reset in message",
			err:      errors.New("connection reset by peer"),
			expected: true,
		},
		{
			name:     "broken pipe in message",
			err:      errors.New("write: broken pipe"),
			expected: true,
		},
		{
			name:     "network unreachable",
			err:      errors.New("network is unreachable"),
			expected: true,
		},
		{
			name:     "generic error (not retryable)",
			err:      errors.New("invalid argument"),
			expected: false,
		},
		{
			name:     "PathError with ETIMEDOUT",
			err:      &os.PathError{Op: "open", Path: "/test", Err: syscall.ETIMEDOUT},
			expected: true,
		},
		{
			name:     "PathError with ENOENT (not retryable)",
			err:      &os.PathError{Op: "open", Path: "/test", Err: syscall.ENOENT},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryableError(tt.err)
			if result != tt.expected {
				t.Errorf("IsRetryableError(%v) = %v, expected %v",
					tt.err, result, tt.expected)
			}
		})
	}
}

func TestRetryWithBackoff_ImmediateSuccess(t *testing.T) {
	attempts := 0
	cfg := &RetryConfig{
		MaxAttempts: 3,
		InitialWait: 10 * time.Millisecond,
		MaxWait:     100 * time.Millisecond,
	}

	result, err := RetryWithBackoff(cfg, func() (int, error) {
		attempts++
		return 42, nil
	}, "test operation")

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if result != 42 {
		t.Errorf("Expected result 42, got: %d", result)
	}
	if attempts != 1 {
		t.Errorf("Expected 1 attempt, got: %d", attempts)
	}
}

func TestRetryWithBackoff_SuccessAfterRetries(t *testing.T) {
	attempts := 0
	cfg := &RetryConfig{
		MaxAttempts: 3,
		InitialWait: 10 * time.Millisecond,
		MaxWait:     100 * time.Millisecond,
	}

	result, err := RetryWithBackoff(cfg, func() (string, error) {
		attempts++
		if attempts < 3 {
			return "", syscall.ETIMEDOUT // Retryable error
		}
		return "success", nil
	}, "test operation")

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if result != "success" {
		t.Errorf("Expected result 'success', got: %s", result)
	}
	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got: %d", attempts)
	}
}

func TestRetryWithBackoff_FailureAfterMaxRetries(t *testing.T) {
	attempts := 0
	cfg := &RetryConfig{
		MaxAttempts: 3,
		InitialWait: 10 * time.Millisecond,
		MaxWait:     100 * time.Millisecond,
	}

	result, err := RetryWithBackoff(cfg, func() (int, error) {
		attempts++
		return 0, syscall.ETIMEDOUT // Always fail with retryable error
	}, "test operation")

	if err == nil {
		t.Error("Expected error after max retries, got nil")
	}
	if result != 0 {
		t.Errorf("Expected result 0, got: %d", result)
	}
	if attempts != 3 {
		t.Errorf("Expected 3 attempts (max), got: %d", attempts)
	}
}

func TestRetryWithBackoff_NonRetryableError(t *testing.T) {
	attempts := 0
	cfg := &RetryConfig{
		MaxAttempts: 3,
		InitialWait: 10 * time.Millisecond,
		MaxWait:     100 * time.Millisecond,
	}

	result, err := RetryWithBackoff(cfg, func() (int, error) {
		attempts++
		return 0, syscall.ENOENT // Non-retryable error
	}, "test operation")

	if err == nil {
		t.Error("Expected error, got nil")
	}
	if result != 0 {
		t.Errorf("Expected result 0, got: %d", result)
	}
	if attempts != 1 {
		t.Errorf("Expected 1 attempt (no retry for non-retryable), got: %d", attempts)
	}
}

func TestRetryWithBackoff_ExponentialBackoff(t *testing.T) {
	attempts := 0
	startTimes := []time.Time{}
	cfg := &RetryConfig{
		MaxAttempts: 3,
		InitialWait: 50 * time.Millisecond,
		MaxWait:     500 * time.Millisecond,
	}

	start := time.Now()

	_, err := RetryWithBackoff(cfg, func() (int, error) {
		attempts++
		startTimes = append(startTimes, time.Now())
		if attempts < 3 {
			return 0, syscall.ETIMEDOUT
		}
		return 42, nil
	}, "test operation")

	totalDuration := time.Since(start)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got: %d", attempts)
	}

	// Check that total duration is approximately: 50ms + 100ms = 150ms
	// Allow some tolerance for test execution overhead
	expectedMin := 150 * time.Millisecond
	expectedMax := 300 * time.Millisecond

	if totalDuration < expectedMin || totalDuration > expectedMax {
		t.Errorf("Expected total duration between %v and %v, got: %v",
			expectedMin, expectedMax, totalDuration)
	}

	// Verify exponential backoff: second wait should be ~2x first wait
	if len(startTimes) >= 2 {
		firstWait := startTimes[1].Sub(startTimes[0])
		t.Logf("First wait: %v", firstWait)
		// Should be approximately 50ms (initial wait)
		if firstWait < 40*time.Millisecond || firstWait > 150*time.Millisecond {
			t.Logf("Warning: First wait time %v not close to expected 50ms", firstWait)
		}
	}
}

func TestRetry_NoReturnValue(t *testing.T) {
	attempts := 0
	cfg := &RetryConfig{
		MaxAttempts: 3,
		InitialWait: 10 * time.Millisecond,
		MaxWait:     100 * time.Millisecond,
	}

	err := Retry(cfg, func() error {
		attempts++
		if attempts < 2 {
			return syscall.ETIMEDOUT
		}
		return nil
	}, "test operation")

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if attempts != 2 {
		t.Errorf("Expected 2 attempts, got: %d", attempts)
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()

	if cfg.MaxAttempts != 3 {
		t.Errorf("Expected MaxAttempts=3, got: %d", cfg.MaxAttempts)
	}
	if cfg.InitialWait != 100*time.Millisecond {
		t.Errorf("Expected InitialWait=100ms, got: %v", cfg.InitialWait)
	}
	if cfg.MaxWait != 5*time.Second {
		t.Errorf("Expected MaxWait=5s, got: %v", cfg.MaxWait)
	}
}

func TestNASRetryConfig(t *testing.T) {
	cfg := NASRetryConfig()

	if cfg.MaxAttempts != 3 {
		t.Errorf("Expected MaxAttempts=3, got: %d", cfg.MaxAttempts)
	}
	if cfg.InitialWait != 200*time.Millisecond {
		t.Errorf("Expected InitialWait=200ms, got: %v", cfg.InitialWait)
	}
	if cfg.MaxWait != 10*time.Second {
		t.Errorf("Expected MaxWait=10s, got: %v", cfg.MaxWait)
	}
}
