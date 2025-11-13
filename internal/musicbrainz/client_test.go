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

func TestDeduplicateArtistNames(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "no duplicates",
			input:    []string{"Artist A", "Artist B", "Artist C"},
			expected: []string{"Artist A", "Artist B", "Artist C"},
		},
		{
			name:     "exact duplicates",
			input:    []string{"The Beatles", "Pink Floyd", "The Beatles", "Radiohead"},
			expected: []string{"The Beatles", "Pink Floyd", "Radiohead"},
		},
		{
			name:     "case insensitive duplicates",
			input:    []string{"The Beatles", "the beatles", "THE BEATLES", "Pink Floyd"},
			expected: []string{"The Beatles", "Pink Floyd"}, // Keeps first occurrence casing
		},
		{
			name:     "whitespace variations",
			input:    []string{"  Artist  ", "Artist", " Artist ", "Other"},
			expected: []string{"  Artist  ", "Other"}, // Keeps first occurrence
		},
		{
			name:     "empty strings filtered",
			input:    []string{"Artist A", "", "   ", "Artist B", ""},
			expected: []string{"Artist A", "Artist B"},
		},
		{
			name:     "all duplicates",
			input:    []string{"Same", "same", "SAME", "Same"},
			expected: []string{"Same"},
		},
		{
			name:     "empty input",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "single item",
			input:    []string{"Only One"},
			expected: []string{"Only One"},
		},
		{
			name:     "mixed case with different artists",
			input:    []string{"Beatles", "beatles", "Stones", "STONES", "Who"},
			expected: []string{"Beatles", "Stones", "Who"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deduplicateArtistNames(tt.input)

			// Check length
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d unique names, got %d", len(tt.expected), len(result))
				t.Logf("Input:    %v", tt.input)
				t.Logf("Expected: %v", tt.expected)
				t.Logf("Got:      %v", result)
				return
			}

			// Check contents
			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("At index %d: expected %q, got %q", i, expected, result[i])
				}
			}
		})
	}
}

func TestDeduplicateArtistNames_PreservesFirstOccurrence(t *testing.T) {
	// Test that the function preserves the casing and spacing of the first occurrence
	input := []string{
		"The Beatles",       // Original
		"the beatles",       // Different case
		"  The Beatles  ",   // Different spacing
		"THE BEATLES",       // All caps
		"Pink Floyd",        // Different artist
		"pink floyd",        // Different case
	}

	result := deduplicateArtistNames(input)

	// Should have exactly 2 unique artists
	if len(result) != 2 {
		t.Fatalf("Expected 2 unique artists, got %d: %v", len(result), result)
	}

	// First occurrence should be "The Beatles" (original casing)
	if result[0] != "The Beatles" {
		t.Errorf("Expected first entry to be 'The Beatles', got %q", result[0])
	}

	// Second occurrence should be "Pink Floyd" (original casing)
	if result[1] != "Pink Floyd" {
		t.Errorf("Expected second entry to be 'Pink Floyd', got %q", result[1])
	}
}
