package ffmpeg

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// FileStreamer streams a WAV file in real-time.
type FileStreamer struct {
	logger   *zap.Logger
	Filename string
}

// NewFileStreamer creates a new FileStreamer.
func NewFileStreamer(filename string, logger *zap.Logger) *FileStreamer {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &FileStreamer{
		Filename: filename,
		logger:   logger,
	}
}

// Start initiates the FFmpeg process to stream the file in real-time.
func (s *FileStreamer) Start(ctx context.Context) (io.ReadCloser, error) {
	if _, err := os.Stat(s.Filename); os.IsNotExist(err) {
		return nil, fmt.Errorf("file not found: %s", s.Filename)
	}

	args := []string{
		"-hide_banner", "-loglevel", "warning",
		"-re", // Read input at native frame rate
		"-i", s.Filename,
		"-ac", "2",
		"-ar", "16000",
		"-f", "s16le", "pipe:1",
	}

	s.logger.Info("Starting file stream", zap.String("filename", s.Filename))

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
		return nil, fmt.Errorf("ffmpeg stream start: %w", err)
	}

	g, gCtx := errgroup.WithContext(ctx)

	// Log stderr
	g.Go(func() error {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			s.logger.Debug("ffmpeg stream output", zap.String("msg", scanner.Text()))
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
