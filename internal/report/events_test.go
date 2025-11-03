package report

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestNewEventLogger(t *testing.T) {
	tmpDir := t.TempDir()

	logger, err := NewEventLogger(tmpDir, LevelDebug)
	if err != nil {
		t.Fatalf("NewEventLogger failed: %v", err)
	}
	defer logger.Close()

	if logger.path == "" {
		t.Error("EventLogger path is empty")
	}

	// Verify file exists
	if _, err := os.Stat(logger.path); os.IsNotExist(err) {
		t.Errorf("Event log file was not created at %s", logger.path)
	}

	// Verify filename format
	filename := filepath.Base(logger.path)
	if len(filename) < len("events-20060102-150405.jsonl") {
		t.Errorf("Event log filename format incorrect: %s", filename)
	}
}

func TestEventLogger_Log(t *testing.T) {
	tmpDir := t.TempDir()
	logger, err := NewEventLogger(tmpDir, LevelDebug)
	if err != nil {
		t.Fatalf("NewEventLogger failed: %v", err)
	}
	defer logger.Close()

	event := &Event{
		Timestamp: time.Now(),
		Level:     LevelInfo,
		Event:     EventScan,
		FileKey:   "test-key",
		SrcPath:   "/test/path.mp3",
	}

	if err := logger.Log(event); err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Verify event was written
	logger.Close()
	content, err := os.ReadFile(logger.path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if len(content) == 0 {
		t.Error("Log file is empty")
	}

	// Verify JSONL format
	var decoded Event
	if err := json.Unmarshal(content, &decoded); err != nil {
		t.Fatalf("Failed to decode JSONL: %v", err)
	}

	if decoded.FileKey != "test-key" {
		t.Errorf("Expected file_key 'test-key', got '%s'", decoded.FileKey)
	}
	if decoded.SrcPath != "/test/path.mp3" {
		t.Errorf("Expected src_path '/test/path.mp3', got '%s'", decoded.SrcPath)
	}
}

func TestEventLogger_MultipleEvents(t *testing.T) {
	tmpDir := t.TempDir()
	logger, err := NewEventLogger(tmpDir, LevelDebug)
	if err != nil {
		t.Fatalf("NewEventLogger failed: %v", err)
	}
	defer logger.Close()

	events := []*Event{
		{Level: LevelInfo, Event: EventScan, FileKey: "key1", SrcPath: "/path1.mp3"},
		{Level: LevelInfo, Event: EventMeta, FileKey: "key2", SrcPath: "/path2.flac"},
		{Level: LevelWarning, Event: EventDuplicate, ClusterKey: "cluster1"},
		{Level: LevelError, Event: EventError, SrcPath: "/path3.m4a", Error: "test error"},
	}

	for _, event := range events {
		if err := logger.Log(event); err != nil {
			t.Fatalf("Log failed: %v", err)
		}
	}

	logger.Close()

	// Read and verify all events
	file, err := os.Open(logger.path)
	if err != nil {
		t.Fatalf("Failed to open log file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineCount := 0
	for scanner.Scan() {
		lineCount++
		var decoded Event
		if err := json.Unmarshal(scanner.Bytes(), &decoded); err != nil {
			t.Fatalf("Failed to decode line %d: %v", lineCount, err)
		}

		// Verify timestamp was set
		if decoded.Timestamp.IsZero() {
			t.Errorf("Line %d: timestamp not set", lineCount)
		}
	}

	if lineCount != len(events) {
		t.Errorf("Expected %d events, got %d", len(events), lineCount)
	}
}

func TestEventLogger_ConcurrentWrites(t *testing.T) {
	tmpDir := t.TempDir()
	logger, err := NewEventLogger(tmpDir, LevelDebug)
	if err != nil {
		t.Fatalf("NewEventLogger failed: %v", err)
	}
	defer logger.Close()

	const numGoroutines = 10
	const eventsPerGoroutine = 20

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				event := &Event{
					Level:   LevelInfo,
					Event:   EventScan,
					FileKey: "concurrent-test",
					Extra: map[string]string{
						"goroutine": string(rune(id)),
						"sequence":  string(rune(j)),
					},
				}
				if err := logger.Log(event); err != nil {
					t.Errorf("Concurrent log failed: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()
	logger.Close()

	// Verify all events were written
	file, err := os.Open(logger.path)
	if err != nil {
		t.Fatalf("Failed to open log file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineCount := 0
	for scanner.Scan() {
		lineCount++
		var decoded Event
		if err := json.Unmarshal(scanner.Bytes(), &decoded); err != nil {
			t.Fatalf("Failed to decode line %d: %v", lineCount, err)
		}
	}

	expected := numGoroutines * eventsPerGoroutine
	if lineCount != expected {
		t.Errorf("Expected %d events, got %d", expected, lineCount)
	}
}

func TestEventLogger_LogScan(t *testing.T) {
	tmpDir := t.TempDir()
	logger, err := NewEventLogger(tmpDir, LevelDebug)
	if err != nil {
		t.Fatalf("NewEventLogger failed: %v", err)
	}
	defer logger.Close()

	err = logger.LogScan("file123", "/music/test.mp3", 12345678)
	if err != nil {
		t.Fatalf("LogScan failed: %v", err)
	}

	logger.Close()

	// Verify event
	content, _ := os.ReadFile(logger.path)
	var event Event
	json.Unmarshal(content, &event)

	if event.Event != EventScan {
		t.Errorf("Expected event type 'scan', got '%s'", event.Event)
	}
	if event.FileKey != "file123" {
		t.Errorf("Expected file_key 'file123', got '%s'", event.FileKey)
	}
	if event.Extra["size_bytes"] != "12345678" {
		t.Errorf("Expected size_bytes '12345678', got '%s'", event.Extra["size_bytes"])
	}
}

func TestEventLogger_LogMeta(t *testing.T) {
	tmpDir := t.TempDir()
	logger, err := NewEventLogger(tmpDir, LevelDebug)
	if err != nil {
		t.Fatalf("NewEventLogger failed: %v", err)
	}
	defer logger.Close()

	// Test success case
	err = logger.LogMeta("file123", "/music/test.flac", "flac", true, nil)
	if err != nil {
		t.Fatalf("LogMeta failed: %v", err)
	}

	logger.Close()

	// Verify event
	content, _ := os.ReadFile(logger.path)
	var event Event
	json.Unmarshal(content, &event)

	if event.Level != LevelInfo {
		t.Errorf("Expected level 'info', got '%s'", event.Level)
	}
	if event.Extra["codec"] != "flac" {
		t.Errorf("Expected codec 'flac', got '%s'", event.Extra["codec"])
	}
	if event.Extra["lossless"] != "true" {
		t.Errorf("Expected lossless 'true', got '%s'", event.Extra["lossless"])
	}
}

func TestEventLogger_LogMetaError(t *testing.T) {
	tmpDir := t.TempDir()
	logger, err := NewEventLogger(tmpDir, LevelDebug)
	if err != nil {
		t.Fatalf("NewEventLogger failed: %v", err)
	}
	defer logger.Close()

	// Test error case
	testErr := os.ErrNotExist
	err = logger.LogMeta("file123", "/music/test.flac", "", false, testErr)
	if err != nil {
		t.Fatalf("LogMeta failed: %v", err)
	}

	logger.Close()

	// Verify event
	content, _ := os.ReadFile(logger.path)
	var event Event
	json.Unmarshal(content, &event)

	if event.Level != LevelError {
		t.Errorf("Expected level 'error', got '%s'", event.Level)
	}
	if event.Error == "" {
		t.Error("Expected error message, got empty string")
	}
}

func TestEventLogger_LogExecute(t *testing.T) {
	tmpDir := t.TempDir()
	logger, err := NewEventLogger(tmpDir, LevelDebug)
	if err != nil {
		t.Fatalf("NewEventLogger failed: %v", err)
	}
	defer logger.Close()

	duration := 250 * time.Millisecond
	err = logger.LogExecute("file123", "/src/test.mp3", "/dest/test.mp3", "copy", 12345678, duration, nil)
	if err != nil {
		t.Fatalf("LogExecute failed: %v", err)
	}

	logger.Close()

	// Verify event
	content, _ := os.ReadFile(logger.path)
	var event Event
	json.Unmarshal(content, &event)

	if event.Event != EventExecute {
		t.Errorf("Expected event type 'execute', got '%s'", event.Event)
	}
	if event.Action != "copy" {
		t.Errorf("Expected action 'copy', got '%s'", event.Action)
	}
	if event.BytesWritten != 12345678 {
		t.Errorf("Expected bytes_written 12345678, got %d", event.BytesWritten)
	}
	if event.Duration != duration.Milliseconds() {
		t.Errorf("Expected duration %d ms, got %d ms", duration.Milliseconds(), event.Duration)
	}
}

func TestEventLogger_LogDuplicate(t *testing.T) {
	tmpDir := t.TempDir()
	logger, err := NewEventLogger(tmpDir, LevelDebug)
	if err != nil {
		t.Fatalf("NewEventLogger failed: %v", err)
	}
	defer logger.Close()

	losers := []string{"/music/dup1.mp3", "/music/dup2.mp3", "/music/dup3.mp3"}
	err = logger.LogDuplicate("cluster-key-123", "/music/winner.flac", losers, 85.5)
	if err != nil {
		t.Fatalf("LogDuplicate failed: %v", err)
	}

	logger.Close()

	// Verify event
	content, _ := os.ReadFile(logger.path)
	var event Event
	json.Unmarshal(content, &event)

	if event.Event != EventDuplicate {
		t.Errorf("Expected event type 'duplicate', got '%s'", event.Event)
	}
	if event.Level != LevelWarning {
		t.Errorf("Expected level 'warning', got '%s'", event.Level)
	}
	if event.ClusterKey != "cluster-key-123" {
		t.Errorf("Expected cluster_key 'cluster-key-123', got '%s'", event.ClusterKey)
	}
	if event.QualityScore != 85.5 {
		t.Errorf("Expected quality_score 85.5, got %f", event.QualityScore)
	}
	if event.Extra["loser_count"] != "3" {
		t.Errorf("Expected loser_count '3', got '%s'", event.Extra["loser_count"])
	}
}

func TestEventLogger_NullLogger(t *testing.T) {
	logger := NullLogger()

	// Should not panic
	err := logger.Log(&Event{Level: LevelInfo, Event: EventScan})
	if err != nil {
		t.Errorf("NullLogger.Log should not return error, got: %v", err)
	}

	err = logger.LogScan("key", "/path", 123)
	if err != nil {
		t.Errorf("NullLogger.LogScan should not return error, got: %v", err)
	}

	err = logger.Close()
	if err != nil {
		t.Errorf("NullLogger.Close should not return error, got: %v", err)
	}

	path := logger.Path()
	if path != "" {
		t.Errorf("NullLogger.Path should return empty string, got: %s", path)
	}
}

func TestEventLogger_AutoTimestamp(t *testing.T) {
	tmpDir := t.TempDir()
	logger, err := NewEventLogger(tmpDir, LevelDebug)
	if err != nil {
		t.Fatalf("NewEventLogger failed: %v", err)
	}
	defer logger.Close()

	// Log event without setting timestamp
	event := &Event{
		Level: LevelInfo,
		Event: EventScan,
	}

	if err := logger.Log(event); err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	logger.Close()

	// Verify timestamp was auto-set
	content, _ := os.ReadFile(logger.path)
	var decoded Event
	json.Unmarshal(content, &decoded)

	if decoded.Timestamp.IsZero() {
		t.Error("Expected timestamp to be auto-set, but it's zero")
	}

	// Timestamp should be recent
	if time.Since(decoded.Timestamp) > 5*time.Second {
		t.Errorf("Timestamp is too old: %v", decoded.Timestamp)
	}
}

func TestEventLogger_JSONLFormat(t *testing.T) {
	tmpDir := t.TempDir()
	logger, err := NewEventLogger(tmpDir, LevelDebug)
	if err != nil {
		t.Fatalf("NewEventLogger failed: %v", err)
	}
	defer logger.Close()

	// Log multiple events
	events := []Event{
		{Level: LevelInfo, Event: EventScan, FileKey: "key1"},
		{Level: LevelWarning, Event: EventDuplicate, ClusterKey: "cluster1"},
		{Level: LevelError, Event: EventError, Error: "test error"},
	}

	for _, e := range events {
		if err := logger.Log(&e); err != nil {
			t.Fatalf("Log failed: %v", err)
		}
	}

	logger.Close()

	// Verify JSONL format (one JSON object per line)
	file, err := os.Open(logger.path)
	if err != nil {
		t.Fatalf("Failed to open log file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Each line should be valid JSON
		var decoded Event
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Fatalf("Line %d is not valid JSON: %v\nLine: %s", lineNum, err, line)
		}

		// Verify required fields
		if decoded.Level == "" {
			t.Errorf("Line %d: missing level", lineNum)
		}
		if decoded.Event == "" {
			t.Errorf("Line %d: missing event type", lineNum)
		}
		if decoded.Timestamp.IsZero() {
			t.Errorf("Line %d: missing timestamp", lineNum)
		}
	}

	if lineNum != len(events) {
		t.Errorf("Expected %d lines, got %d", len(events), lineNum)
	}
}

func TestEventLogger_LogLevelFiltering(t *testing.T) {
	testCases := []struct {
		name          string
		minLevel      EventLevel
		events        []Event
		expectedCount int
	}{
		{
			name:     "LevelDebug logs all",
			minLevel: LevelDebug,
			events: []Event{
				{Level: LevelDebug, Event: EventScan},
				{Level: LevelInfo, Event: EventMeta},
				{Level: LevelWarning, Event: EventDuplicate},
				{Level: LevelError, Event: EventError},
			},
			expectedCount: 4,
		},
		{
			name:     "LevelInfo skips debug",
			minLevel: LevelInfo,
			events: []Event{
				{Level: LevelDebug, Event: EventScan},
				{Level: LevelInfo, Event: EventMeta},
				{Level: LevelWarning, Event: EventDuplicate},
				{Level: LevelError, Event: EventError},
			},
			expectedCount: 3,
		},
		{
			name:     "LevelWarning skips debug and info",
			minLevel: LevelWarning,
			events: []Event{
				{Level: LevelDebug, Event: EventScan},
				{Level: LevelInfo, Event: EventMeta},
				{Level: LevelWarning, Event: EventDuplicate},
				{Level: LevelError, Event: EventError},
			},
			expectedCount: 2,
		},
		{
			name:     "LevelError only logs errors",
			minLevel: LevelError,
			events: []Event{
				{Level: LevelDebug, Event: EventScan},
				{Level: LevelInfo, Event: EventMeta},
				{Level: LevelWarning, Event: EventDuplicate},
				{Level: LevelError, Event: EventError},
			},
			expectedCount: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			logger, err := NewEventLogger(tmpDir, tc.minLevel)
			if err != nil {
				t.Fatalf("NewEventLogger failed: %v", err)
			}
			defer logger.Close()

			// Log all events
			for _, e := range tc.events {
				if err := logger.Log(&e); err != nil {
					t.Fatalf("Log failed: %v", err)
				}
			}

			logger.Close()

			// Count lines in log file
			file, err := os.Open(logger.path)
			if err != nil {
				t.Fatalf("Failed to open log file: %v", err)
			}
			defer file.Close()

			scanner := bufio.NewScanner(file)
			lineCount := 0
			for scanner.Scan() {
				lineCount++
			}

			if lineCount != tc.expectedCount {
				t.Errorf("Expected %d events logged, got %d", tc.expectedCount, lineCount)
			}
		})
	}
}
