package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// EventType represents the type of event
type EventType string

const (
	EventScan      EventType = "scan"
	EventMeta      EventType = "meta"
	EventCluster   EventType = "cluster"
	EventScore     EventType = "score"
	EventPlan      EventType = "plan"
	EventExecute   EventType = "execute"
	EventSkip      EventType = "skip"
	EventDuplicate EventType = "duplicate"
	EventConflict  EventType = "conflict"
	EventError     EventType = "error"
)

// EventLevel represents the severity level
type EventLevel string

const (
	LevelDebug   EventLevel = "debug"
	LevelInfo    EventLevel = "info"
	LevelWarning EventLevel = "warning"
	LevelError   EventLevel = "error"
)

// levelPriority maps event levels to numeric priorities for comparison
var levelPriority = map[EventLevel]int{
	LevelDebug:   0,
	LevelInfo:    1,
	LevelWarning: 2,
	LevelError:   3,
}

// Event represents a single event in the pipeline
type Event struct {
	Timestamp    time.Time         `json:"ts"`
	Level        EventLevel        `json:"level"`
	Event        EventType         `json:"event"`
	FileKey      string            `json:"file_key,omitempty"`
	SrcPath      string            `json:"src_path,omitempty"`
	DestPath     string            `json:"dest_path,omitempty"`
	ClusterKey   string            `json:"cluster_key,omitempty"`
	QualityScore float64           `json:"quality_score,omitempty"`
	Action       string            `json:"action,omitempty"`
	Reason       string            `json:"reason,omitempty"`
	BytesWritten int64             `json:"bytes_written,omitempty"`
	Duration     int64             `json:"duration_ms,omitempty"` // in milliseconds
	Error        string            `json:"error,omitempty"`
	Extra        map[string]string `json:"extra,omitempty"`
}

// EventLogger writes events to a JSONL file
type EventLogger struct {
	file     *os.File
	encoder  *json.Encoder
	mu       sync.Mutex
	path     string
	minLevel EventLevel
}

// NewEventLogger creates a new event logger with a minimum log level
// minLevel determines which events are written (e.g., LevelInfo skips LevelDebug)
func NewEventLogger(outputDir string, minLevel EventLevel) (*EventLogger, error) {
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Generate filename with timestamp
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("events-%s.jsonl", timestamp)
	path := filepath.Join(outputDir, filename)

	// Open file for writing
	file, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("failed to create event log: %w", err)
	}

	return &EventLogger{
		file:     file,
		encoder:  json.NewEncoder(file),
		path:     path,
		minLevel: minLevel,
	}, nil
}

// Log writes an event to the JSONL file
func (l *EventLogger) Log(event *Event) error {
	if l == nil || l.file == nil {
		return nil // Silently ignore if logger not initialized
	}

	// Filter by minimum level
	if levelPriority[event.Level] < levelPriority[l.minLevel] {
		return nil // Skip events below minimum level
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	if err := l.encoder.Encode(event); err != nil {
		return fmt.Errorf("failed to encode event: %w", err)
	}

	return nil
}

// LogScan logs a file scan event
func (l *EventLogger) LogScan(fileKey, srcPath string, sizeBytes int64) error {
	return l.Log(&Event{
		Level:   LevelInfo,
		Event:   EventScan,
		FileKey: fileKey,
		SrcPath: srcPath,
		Extra: map[string]string{
			"size_bytes": fmt.Sprintf("%d", sizeBytes),
		},
	})
}

// LogMeta logs a metadata extraction event
func (l *EventLogger) LogMeta(fileKey, srcPath, codec string, lossless bool, err error) error {
	level := LevelInfo
	errMsg := ""
	if err != nil {
		level = LevelError
		errMsg = err.Error()
	}

	return l.Log(&Event{
		Level:   level,
		Event:   EventMeta,
		FileKey: fileKey,
		SrcPath: srcPath,
		Error:   errMsg,
		Extra: map[string]string{
			"codec":    codec,
			"lossless": fmt.Sprintf("%t", lossless),
		},
	})
}

// LogCluster logs a clustering event
func (l *EventLogger) LogCluster(fileKey, srcPath, clusterKey string, memberCount int) error {
	return l.Log(&Event{
		Level:      LevelInfo,
		Event:      EventCluster,
		FileKey:    fileKey,
		SrcPath:    srcPath,
		ClusterKey: clusterKey,
		Extra: map[string]string{
			"member_count": fmt.Sprintf("%d", memberCount),
		},
	})
}

// LogScore logs a quality scoring event
func (l *EventLogger) LogScore(fileKey, srcPath, clusterKey string, qualityScore float64, preferred bool) error {
	level := LevelDebug
	if preferred {
		level = LevelInfo
	}

	return l.Log(&Event{
		Level:        level,
		Event:        EventScore,
		FileKey:      fileKey,
		SrcPath:      srcPath,
		ClusterKey:   clusterKey,
		QualityScore: qualityScore,
		Extra: map[string]string{
			"preferred": fmt.Sprintf("%t", preferred),
		},
	})
}

// LogPlan logs a planning event
func (l *EventLogger) LogPlan(fileKey, srcPath, destPath, action, reason string) error {
	event := EventPlan
	if action == "skip" {
		event = EventSkip
	}

	return l.Log(&Event{
		Level:    LevelInfo,
		Event:    event,
		FileKey:  fileKey,
		SrcPath:  srcPath,
		DestPath: destPath,
		Action:   action,
		Reason:   reason,
	})
}

// LogExecute logs an execution event
func (l *EventLogger) LogExecute(fileKey, srcPath, destPath, action string, bytesWritten int64, duration time.Duration, err error) error {
	level := LevelInfo
	errMsg := ""
	if err != nil {
		level = LevelError
		errMsg = err.Error()
	}

	return l.Log(&Event{
		Level:        level,
		Event:        EventExecute,
		FileKey:      fileKey,
		SrcPath:      srcPath,
		DestPath:     destPath,
		Action:       action,
		BytesWritten: bytesWritten,
		Duration:     duration.Milliseconds(),
		Error:        errMsg,
	})
}

// LogDuplicate logs a duplicate detection event
func (l *EventLogger) LogDuplicate(clusterKey string, winnerPath string, loserPaths []string, qualityScore float64) error {
	return l.Log(&Event{
		Level:        LevelWarning,
		Event:        EventDuplicate,
		ClusterKey:   clusterKey,
		SrcPath:      winnerPath,
		QualityScore: qualityScore,
		Extra: map[string]string{
			"loser_count": fmt.Sprintf("%d", len(loserPaths)),
		},
	})
}

// LogConflict logs a file conflict event
func (l *EventLogger) LogConflict(srcPath, destPath, reason string) error {
	return l.Log(&Event{
		Level:    LevelWarning,
		Event:    EventConflict,
		SrcPath:  srcPath,
		DestPath: destPath,
		Reason:   reason,
	})
}

// LogError logs an error event
func (l *EventLogger) LogError(event EventType, srcPath string, err error) error {
	return l.Log(&Event{
		Level:   LevelError,
		Event:   event,
		SrcPath: srcPath,
		Error:   err.Error(),
	})
}

// Close closes the event log file
func (l *EventLogger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	return l.file.Close()
}

// Path returns the path to the event log file
func (l *EventLogger) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

// NullLogger returns a no-op event logger
func NullLogger() *EventLogger {
	return nil
}
