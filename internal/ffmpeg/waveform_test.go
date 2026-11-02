package ffmpeg_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/ringzerosystems/omnic/internal/ffmpeg"
)

func TestNormalizePCMToWaveform_Empty(t *testing.T) {
	wf := ffmpeg.NormalizePCMToWaveform(nil, 64)
	if len(wf) != 64 {
		t.Fatalf("expected 64 bars, got %d", len(wf))
	}
	for i, v := range wf {
		if v != 0 {
			t.Errorf("bar %d: expected 0, got %d", i, v)
		}
	}
}

func TestNormalizePCMToWaveform_Silence(t *testing.T) {
	samples := make([]int16, 1000)
	wf := ffmpeg.NormalizePCMToWaveform(samples, 32)
	if len(wf) != 32 {
		t.Fatalf("expected 32 bars, got %d", len(wf))
	}
	for i, v := range wf {
		if v != 0 {
			t.Errorf("bar %d: expected 0, got %d", i, v)
		}
	}
}

func TestNormalizePCMToWaveform_Signal(t *testing.T) {
	// Generate a signal with ramp-up amplitude
	numSamples := 6400
	samples := make([]int16, numSamples)
	for i := range numSamples {
		amp := float64(i) / float64(numSamples) * 30000.0
		freq := 440.0
		timeSec := float64(i) / 16000.0
		samples[i] = int16(amp * math.Sin(2*math.Pi*freq*timeSec))
	}

	wf := ffmpeg.NormalizePCMToWaveform(samples, 64)
	if len(wf) != 64 {
		t.Fatalf("expected 64 bars, got %d", len(wf))
	}

	// Last bar should be peak (31)
	hasMax := false
	for _, v := range wf {
		if v < 0 || v > 31 {
			t.Errorf("value %d out of 5-bit range [0, 31]", v)
		}
		if v == 31 {
			hasMax = true
		}
	}
	if !hasMax {
		t.Errorf("expected at least one bar with peak amplitude 31")
	}

	// First bar should be substantially lower than last bar
	if wf[0] >= wf[len(wf)-1] {
		t.Errorf("expected ramp up: wf[0]=%d, wf[last]=%d", wf[0], wf[len(wf)-1])
	}
}

func TestExtractWaveformFromBytes(t *testing.T) {
	pcmBytes := make([]byte, 128)
	wf := ffmpeg.ExtractWaveformFromBytes(pcmBytes, 0)
	if len(wf) != ffmpeg.DefaultWaveformBars {
		t.Fatalf("expected default %d bars, got %d", ffmpeg.DefaultWaveformBars, len(wf))
	}
}

func TestExtractWaveform_NonExistentFile(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := ffmpeg.ExtractWaveform(ctx, "/path/does/not/exist.ogg", 64)
	if err == nil {
		t.Fatalf("expected error for non-existent file, got nil")
	}
}
