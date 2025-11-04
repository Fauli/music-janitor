package util

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDetectFilesystemCaseSensitivity(t *testing.T) {
	// Create a temp directory for testing
	tempDir, err := os.MkdirTemp("", "mlc-fs-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	caseSensitive, err := DetectFilesystemCaseSensitivity(tempDir)
	if err != nil {
		t.Fatalf("DetectFilesystemCaseSensitivity failed: %v", err)
	}

	t.Logf("Detected filesystem case sensitivity: %v (OS: %s)", caseSensitive, runtime.GOOS)

	// Verify the detection by actually testing
	testFile1 := filepath.Join(tempDir, "TestCase.txt")
	testFile2 := filepath.Join(tempDir, "testcase.txt")

	// Create first file
	f, err := os.Create(testFile1)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	f.Close()

	// Check if second file (different case) exists
	_, err = os.Stat(testFile2)
	fileExists := (err == nil)

	if caseSensitive {
		// On case-sensitive FS, different case = different file
		if fileExists {
			t.Error("Case-sensitive FS detected, but files with different cases collide")
		}
	} else {
		// On case-insensitive FS, different case = same file
		if !fileExists {
			t.Error("Case-insensitive FS detected, but files with different cases don't collide")
		}
	}
}

func TestNormalizePath(t *testing.T) {
	testCases := []struct {
		name          string
		path          string
		caseSensitive bool
		expected      string
	}{
		{
			name:          "case-sensitive: no change",
			path:          "/Users/Test/Music",
			caseSensitive: true,
			expected:      "/Users/Test/Music",
		},
		{
			name:          "case-insensitive: lowercase",
			path:          "/Users/Test/Music",
			caseSensitive: false,
			expected:      "/users/test/music",
		},
		{
			name:          "case-insensitive: mixed case",
			path:          "/The Beatles/Abbey Road",
			caseSensitive: false,
			expected:      "/the beatles/abbey road",
		},
		{
			name:          "case-sensitive: preserve case",
			path:          "/The Beatles/Abbey Road",
			caseSensitive: true,
			expected:      "/The Beatles/Abbey Road",
		},
		{
			name:          "case-insensitive: with spaces",
			path:          "/Artist Name/Album Name/01 - Track.mp3",
			caseSensitive: false,
			expected:      "/artist name/album name/01 - track.mp3",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := NormalizePath(tc.path, tc.caseSensitive)
			if result != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestPathsEqual(t *testing.T) {
	testCases := []struct {
		name          string
		path1         string
		path2         string
		caseSensitive bool
		expected      bool
	}{
		{
			name:          "case-sensitive: exact match",
			path1:         "/Users/Test/Music",
			path2:         "/Users/Test/Music",
			caseSensitive: true,
			expected:      true,
		},
		{
			name:          "case-sensitive: different case",
			path1:         "/Users/Test/Music",
			path2:         "/users/test/music",
			caseSensitive: true,
			expected:      false,
		},
		{
			name:          "case-insensitive: different case",
			path1:         "/Users/Test/Music",
			path2:         "/users/test/music",
			caseSensitive: false,
			expected:      true,
		},
		{
			name:          "case-insensitive: exact match",
			path1:         "/Users/Test/Music",
			path2:         "/Users/Test/Music",
			caseSensitive: false,
			expected:      true,
		},
		{
			name:          "case-insensitive: artist names",
			path1:         "/The Beatles/Abbey Road",
			path2:         "/the beatles/abbey road",
			caseSensitive: false,
			expected:      true,
		},
		{
			name:          "case-sensitive: artist names",
			path1:         "/The Beatles/Abbey Road",
			path2:         "/the beatles/abbey road",
			caseSensitive: true,
			expected:      false,
		},
		{
			name:          "case-insensitive: completely different paths",
			path1:         "/Artist1/Album1",
			path2:         "/Artist2/Album2",
			caseSensitive: false,
			expected:      false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := PathsEqual(tc.path1, tc.path2, tc.caseSensitive)
			if result != tc.expected {
				t.Errorf("PathsEqual(%q, %q, caseSensitive=%v): expected %v, got %v",
					tc.path1, tc.path2, tc.caseSensitive, tc.expected, result)
			}
		})
	}
}

func TestNormalizePathCleanup(t *testing.T) {
	// Test that filepath.Clean is applied in both cases
	testCases := []struct {
		name          string
		path          string
		caseSensitive bool
	}{
		{
			name:          "case-sensitive: removes trailing slash",
			path:          "/path/to/dir/",
			caseSensitive: true,
		},
		{
			name:          "case-insensitive: removes trailing slash",
			path:          "/path/to/dir/",
			caseSensitive: false,
		},
		{
			name:          "case-sensitive: resolves ..",
			path:          "/path/to/../other",
			caseSensitive: true,
		},
		{
			name:          "case-insensitive: resolves ..",
			path:          "/path/to/../other",
			caseSensitive: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := NormalizePath(tc.path, tc.caseSensitive)
			cleaned := filepath.Clean(tc.path)
			if tc.caseSensitive {
				if result != cleaned {
					t.Errorf("Case-sensitive path should be cleaned: expected %q, got %q", cleaned, result)
				}
			} else {
				// Case-insensitive should also be cleaned AND lowercased
				expected := NormalizePath(cleaned, false)
				if result != expected {
					t.Errorf("Case-insensitive path should be cleaned and lowercased: expected %q, got %q", expected, result)
				}
			}
		})
	}
}
