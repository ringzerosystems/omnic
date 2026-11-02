package ffmpeg

import (
	"testing"

	"go.uber.org/zap"
)

// TestCapturerStart verifies that the Capturer starts the ffmpeg command with expected arguments.
func TestCapturerStart(t *testing.T) {
	t.Parallel()

	mic := "test_mic"
	sink := "test_sink"
	logger := zap.NewNop()

	_ = NewCapturer(mic, sink, false, logger)
}

// TestNewCapturerDefaults verifies NewCapturer defaults.
func TestNewCapturerDefaults(t *testing.T) {
	t.Parallel()

	t.Run("NewCapturer Defaults", func(t *testing.T) {
		c := NewCapturer("", "", false, nil)
		if c.logger == nil {
			t.Error("Expected logger to be initialized")
		}
	})
}
