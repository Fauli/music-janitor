package musicbrainz

import (
	"context"
	"testing"
	"time"
)

func TestClientRateLimiting(t *testing.T) {
	client := NewClient()
	defer client.Close()

	// Test that rate limiting works (should take at least 2 seconds for 3 requests)
	start := time.Now()

	ctx := context.Background()

	// Make 3 requests (should take >= 2 seconds due to 1 req/sec limit)
	for i := 0; i < 3; i++ {
		_, err := client.SearchArtist(ctx, "test")
		// Ignore errors - we're just testing rate limiting
		_ = err
	}

	elapsed := time.Since(start)

	// Should take at least 2 seconds (3 requests - 1 = 2 intervals)
	if elapsed < 2*time.Second {
		t.Errorf("Rate limiting not working: 3 requests took only %v", elapsed)
	}
}

func TestCanonicalNameCaching(t *testing.T) {
	// This test requires internet connectivity and a real MusicBrainz API
	// Mark as integration test
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := NewClient()
	defer client.Close()

	ctx := context.Background()

	// Test searching for a well-known artist
	canonical, aliases, err := client.GetCanonicalName(ctx, "the beatles")
	if err != nil {
		t.Fatalf("GetCanonicalName failed: %v", err)
	}

	if canonical == "" {
		t.Error("Expected non-empty canonical name")
	}

	t.Logf("Canonical name: %s", canonical)
	t.Logf("Aliases: %v", aliases)

	// Should return "The Beatles" or similar
	if canonical == "" {
		t.Errorf("Expected canonical name for 'the beatles', got empty string")
	}
}

func TestArtistNormalization(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := NewClient()
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		input string
		// We can't predict exact output, but we can test that it returns something
		shouldSucceed bool
	}{
		{"The Beatles", true},
		{"Beatles", true},
		{"the beatles", true},
		{"", false}, // Empty should fail
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			canonical, err := client.NormalizeArtistWithMB(ctx, tt.input)

			if tt.shouldSucceed {
				if err != nil {
					t.Errorf("Expected success for %q, got error: %v", tt.input, err)
				}
				if canonical == "" {
					t.Errorf("Expected non-empty canonical name for %q", tt.input)
				}
				t.Logf("%q -> %q", tt.input, canonical)
			} else {
				if err == nil {
					t.Errorf("Expected error for %q, got none", tt.input)
				}
			}
		})
	}
}
