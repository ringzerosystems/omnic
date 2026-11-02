package ffmpeg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"time"
)

// RecordConfig defines audio capture parameters.
type RecordConfig struct {
	DeviceID    string        `json:"device_id"`
	Bitrate     string        `json:"bitrate"`
	MaxDuration time.Duration `json:"max_duration"`
	SampleRate  int           `json:"sample_rate"`
}

// Recorder captures microphone input using FFmpeg and encodes OGG Opus audio.
type Recorder struct {
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	cancel     context.CancelFunc
	outputFile string
	startTime  time.Time
	cfg        RecordConfig
	mu         sync.Mutex
	running    bool
}

// NewRecorder constructs a new audio Recorder with the provided configuration.
func NewRecorder(cfg RecordConfig) *Recorder {
	if cfg.SampleRate <= 0 {
		cfg.SampleRate = 16000
	}
	if cfg.Bitrate == "" {
		cfg.Bitrate = "24k"
	}
	return &Recorder{cfg: cfg}
}

func buildRecordArgs(cfg RecordConfig, outputFile string) []string {
	args := []string{"-hide_banner", "-loglevel", "error"}
	if runtime.GOOS == "darwin" {
		dev := cfg.DeviceID
		if dev == "" {
			dev = "0"
		}
		args = append(args, "-f", "avfoundation", "-i", ":"+dev)
	} else {
		dev := cfg.DeviceID
		if dev == "" || dev == "default" {
			args = append(args, "-f", "pulse", "-i", "default")
		} else {
			args = append(args, "-f", "pulse", "-i", dev)
		}
	}

	return append(args,
		"-c:a", "libopus",
		"-b:a", cfg.Bitrate,
		"-ar", strconv.Itoa(cfg.SampleRate),
		"-ac", "1",
		"-f", "ogg",
		"-y",
		outputFile,
	)
}

// Start begins audio recording to the specified output file path.
func (r *Recorder) Start(ctx context.Context, outputFile string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return errors.New("recorder already running")
	}

	recordCtx, cancel := context.WithCancel(ctx)
	if r.cfg.MaxDuration > 0 {
		recordCtx, cancel = context.WithTimeout(ctx, r.cfg.MaxDuration)
	}

	args := buildRecordArgs(r.cfg, outputFile)
	//nolint:gosec // arguments are sanitized internally
	cmd := exec.CommandContext(recordCtx, "ffmpeg", args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("opening ffmpeg stdin: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("starting ffmpeg: %w", err)
	}

	r.cmd = cmd
	r.stdin = stdin
	r.cancel = cancel
	r.outputFile = outputFile
	r.startTime = time.Now()
	r.running = true
	return nil
}

// Stop gracefully terminates the recording and returns elapsed duration.
func (r *Recorder) Stop() (time.Duration, error) {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return 0, errors.New("recorder not running")
	}
	r.running = false
	cmd := r.cmd
	stdin := r.stdin
	startTime := r.startTime
	cancel := r.cancel
	r.mu.Unlock()

	if stdin != nil {
		_, _ = stdin.Write([]byte("q\n"))
		_ = stdin.Close()
	}

	waitErr := cmd.Wait()
	if cancel != nil {
		cancel()
	}

	duration := time.Since(startTime)
	if waitErr != nil && !errors.Is(waitErr, context.Canceled) {
		return duration, fmt.Errorf("ffmpeg recording error: %w", waitErr)
	}
	return duration, nil
}

// Cancel terminates the recording immediately and cleans up scratch files.
func (r *Recorder) Cancel() error {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return nil
	}
	r.running = false
	cmd := r.cmd
	cancel := r.cancel
	outputFile := r.outputFile
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	if outputFile != "" {
		_ = os.Remove(outputFile)
	}
	return nil
}
