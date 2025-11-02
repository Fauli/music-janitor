package meta

import (
	"encoding/json"
	"testing"
)

func TestIntOrStringUnmarshal(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "integer value",
			input:    `{"value": 16}`,
			expected: 16,
		},
		{
			name:     "string integer",
			input:    `{"value": "24"}`,
			expected: 24,
		},
		{
			name:     "N/A string",
			input:    `{"value": "N/A"}`,
			expected: 0,
		},
		{
			name:     "empty string",
			input:    `{"value": ""}`,
			expected: 0,
		},
		{
			name:     "zero",
			input:    `{"value": 0}`,
			expected: 0,
		},
		{
			name:     "invalid string",
			input:    `{"value": "invalid"}`,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result struct {
				Value IntOrString `json:"value"`
			}

			err := json.Unmarshal([]byte(tt.input), &result)
			if err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}

			if result.Value.Value != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, result.Value.Value)
			}
		})
	}
}

func TestFFprobeStreamUnmarshal(t *testing.T) {
	// Test real-world ffprobe output with string bits_per_raw_sample
	jsonData := `{
		"index": 0,
		"codec_name": "pcm_s16le",
		"codec_type": "audio",
		"sample_rate": "44100",
		"channels": 2,
		"channel_layout": "stereo",
		"bits_per_sample": 16,
		"bits_per_raw_sample": "N/A",
		"duration": "180.5",
		"bit_rate": "1411200"
	}`

	var stream FFprobeStream
	err := json.Unmarshal([]byte(jsonData), &stream)
	if err != nil {
		t.Fatalf("Failed to unmarshal FFprobeStream: %v", err)
	}

	if stream.CodecName != "pcm_s16le" {
		t.Errorf("Expected codec_name 'pcm_s16le', got '%s'", stream.CodecName)
	}

	if stream.SampleRate != 44100 {
		t.Errorf("Expected sample_rate 44100, got %d", stream.SampleRate)
	}

	if stream.BitsPerSample.Value != 16 {
		t.Errorf("Expected bits_per_sample 16, got %d", stream.BitsPerSample.Value)
	}

	if stream.BitsPerRawSample.Value != 0 {
		t.Errorf("Expected bits_per_raw_sample 0 (from N/A), got %d", stream.BitsPerRawSample.Value)
	}
}

func TestFFprobeStreamUnmarshalAIFF(t *testing.T) {
	// Test AIFF files which often have string bits_per_raw_sample
	jsonData := `{
		"index": 0,
		"codec_name": "pcm_s16be",
		"codec_type": "audio",
		"sample_rate": "44100",
		"channels": 2,
		"bits_per_sample": "16",
		"bits_per_raw_sample": "16",
		"duration": "240.123"
	}`

	var stream FFprobeStream
	err := json.Unmarshal([]byte(jsonData), &stream)
	if err != nil {
		t.Fatalf("Failed to unmarshal AIFF FFprobeStream: %v", err)
	}

	if stream.BitsPerSample.Value != 16 {
		t.Errorf("Expected bits_per_sample 16, got %d", stream.BitsPerSample.Value)
	}

	if stream.BitsPerRawSample.Value != 16 {
		t.Errorf("Expected bits_per_raw_sample 16, got %d", stream.BitsPerRawSample.Value)
	}
}
