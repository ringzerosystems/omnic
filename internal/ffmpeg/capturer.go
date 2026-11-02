package ffmpeg

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// Capturer orchestrates the ffmpeg process to capture Mic and Sink audio.
type Capturer struct {
	logger       *zap.Logger
	Mic          string
	Sink         string
	RecordToFile bool
}

// NewCapturer creates a new FFmpeg Capturer.
func NewCapturer(mic, sink string, recordToFile bool, logger *zap.Logger) *Capturer {
	if mic == "" || sink == "" {
		dMic, dSink := DiscoverDevices()
		if mic == "" {
			mic = dMic
		}
		if sink == "" {
			sink = dSink
		}
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Capturer{
		Mic:          mic,
		Sink:         sink,
		RecordToFile: recordToFile,
		logger:       logger,
	}
}

// Start initiates the FFmpeg capture process and returns a reader for the raw PCM bytes.
func (c *Capturer) Start(ctx context.Context) (io.ReadCloser, error) {
	args := []string{
		"-hide_banner", "-loglevel", "warning",
		"-f", "pulse", "-name", "Speaker_Capture", "-i", c.Sink,
		"-f", "pulse", "-name", "Mic_Capture", "-i", c.Mic,
		"-filter_complex", "[0:a]aresample=matrix_encoding=dplii,aformat=channel_layouts=mono[sink_mono];[1:a]aresample=matrix_encoding=dplii,aformat=channel_layouts=mono[mic_mono];[sink_mono][mic_mono]amerge=inputs=2[aout]",
		"-map", "[aout]",
		"-ac", "2",
		"-ar", "16000",
		"-c:a", "pcm_s16le",
	}

	if c.RecordToFile {
		timestamp := time.Now().Format("20060102_150405")
		filename := fmt.Sprintf("%s_output.wav", timestamp)
		c.logger.Info("Recording session to file", zap.String("filename", filename))
		args = append(args,
			"-f", "tee",
			fmt.Sprintf("[f=s16le]pipe:1|[f=wav]%s", filename),
		)
	} else {
		args = append(args, "-f", "s16le", "pipe:1")
	}

	//nolint:gosec // G204: subprocess arguments are constructed internally
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ffmpeg capture start: %w", err)
	}

	g, gCtx := errgroup.WithContext(ctx)

	// Log stderr
	g.Go(func() error {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			c.logger.Debug("ffmpeg capture output", zap.String("msg", scanner.Text()))
		}
		return scanner.Err()
	})

	// Monitor command completion
	g.Go(func() error {
		err := cmd.Wait()
		if err != nil && gCtx.Err() == nil {
			return fmt.Errorf("ffmpeg wait: %w", err)
		}
		return nil
	})

	return &CaptureReader{
		ReadCloser: stdout,
		g:          g,
	}, nil
}

// CaptureReader wraps an io.ReadCloser and an errgroup.Group to ensure
// that the background goroutines are cleaned up when the reader is closed.
type CaptureReader struct {
	io.ReadCloser
	g *errgroup.Group
}

// Close closes the underlying reader and waits for background goroutines.
func (r *CaptureReader) Close() error {
	err := r.ReadCloser.Close()
	if waitErr := r.g.Wait(); waitErr != nil && err == nil {
		err = waitErr
	}
	return err
}
