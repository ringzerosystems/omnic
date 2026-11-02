package ffmpeg

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

func TestFileStreamer(t *testing.T) {
	t.Parallel()

	t.Run("NewFileStreamer", func(t *testing.T) {
		fs := NewFileStreamer("test.wav", zap.NewNop())
		if fs.Filename != "test.wav" {
			t.Errorf("expected Filename test.wav, got %s", fs.Filename)
		}
	})

	t.Run("Start missing file", func(t *testing.T) {
		fs := NewFileStreamer("nonexistent.wav", zap.NewNop())
		_, err := fs.Start(context.Background())
		if err == nil {
			t.Error("expected error for nonexistent file, got nil")
		}
	})
}
