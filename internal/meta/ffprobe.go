package meta

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"

	"github.com/franz/music-janitor/internal/util"
)

// FFprobeInfo represents the output from ffprobe
type FFprobeInfo struct {
	Streams []FFprobeStream `json:"streams"`
	Format  *FFprobeFormat  `json:"format"`
}

// IntOrString can unmarshal both integers and strings from JSON
type IntOrString struct {
	Value int
}

// UnmarshalJSON implements custom unmarshaling for IntOrString
func (i *IntOrString) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as int first
	var intVal int
	if err := json.Unmarshal(data, &intVal); err == nil {
		i.Value = intVal
		return nil
	}

	// Try as string
	var strVal string
	if err := json.Unmarshal(data, &strVal); err != nil {
		return err
	}

	// Parse string to int (ignore errors, default to 0)
	if strVal == "" || strVal == "N/A" {
		i.Value = 0
		return nil
	}

	parsed, err := strconv.Atoi(strVal)
	if err != nil {
		i.Value = 0
		return nil
	}

	i.Value = parsed
	return nil
}

// FFprobeStream represents an audio stream
type FFprobeStream struct {
	Index              int         `json:"index"`
	CodecName          string      `json:"codec_name"`
	CodecType          string      `json:"codec_type"`
	SampleRate         int         `json:"sample_rate,string"`
	Channels           int         `json:"channels"`
	ChannelLayout      string      `json:"channel_layout"`
	BitsPerSample      IntOrString `json:"bits_per_sample"`
	BitsPerRawSample   IntOrString `json:"bits_per_raw_sample"`
	Duration           string      `json:"duration"`
	BitRate            string      `json:"bit_rate"`
}

// FFprobeFormat represents container format metadata
type FFprobeFormat struct {
	Filename       string            `json:"filename"`
	FormatName     string            `json:"format_name"`
	FormatLongName string            `json:"format_long_name"`
	Duration       string            `json:"duration"`
	Size           string            `json:"size"`
	BitRate        string            `json:"bit_rate"`
	Tags           map[string]string `json:"tags"`
}

// RunFFprobe executes ffprobe and parses the JSON output
func RunFFprobe(path string) (*FFprobeInfo, error) {
	// Check if ffprobe is available
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return nil, util.ErrNotFound
	}

	// Run ffprobe with JSON output
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	)

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("ffprobe failed: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("ffprobe execution failed: %w", err)
	}

	// Parse JSON
	var info FFprobeInfo
	if err := json.Unmarshal(output, &info); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe output: %w", err)
	}

	return &info, nil
}

// CheckFFprobeAvailable checks if ffprobe is available in PATH
func CheckFFprobeAvailable() bool {
	_, err := exec.LookPath("ffprobe")
	return err == nil
}
