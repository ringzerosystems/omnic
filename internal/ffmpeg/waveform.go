package ffmpeg

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os/exec"
)

// DefaultWaveformBars is the default number of bars for Telegram voice note waveforms.
const DefaultWaveformBars = 64

// NormalizePCMToWaveform computes a 5-bit normalized waveform (0-31 integer amplitude envelope)
// across N equal bars from raw 16-bit PCM audio samples.
func NormalizePCMToWaveform(samples []int16, bars int) []int {
	if bars <= 0 {
		bars = DefaultWaveformBars
	}
	result := make([]int, bars)
	if len(samples) == 0 {
		return result
	}
	numSamples := len(samples)

	rmsValues := make([]float64, bars)
	var maxRMS float64
	samplesPerBar := float64(numSamples) / float64(bars)

	for i := range bars {
		start := int(float64(i) * samplesPerBar)
		end := min(int(float64(i+1)*samplesPerBar), numSamples)
		if start >= end {
			start = max(0, end-1)
		}

		var sumSq float64
		count := 0
		for j := start; j < end; j++ {
			val := float64(samples[j])
			sumSq += val * val
			count++
		}

		if count > 0 {
			rms := math.Sqrt(sumSq / float64(count))
			rmsValues[i] = rms
			if rms > maxRMS {
				maxRMS = rms
			}
		}
	}

	if maxRMS <= 0 {
		return result
	}

	for i, rms := range rmsValues {
		normalized := math.Floor((rms / maxRMS) * 31.0)
		if normalized < 0 {
			normalized = 0
		} else if normalized > 31 {
			normalized = 31
		}
		result[i] = int(normalized)
	}

	return result
}

// ExtractWaveformFromBytes parses little-endian signed 16-bit PCM bytes and computes the normalized waveform.
func ExtractWaveformFromBytes(pcmBytes []byte, bars int) []int {
	numSamples := len(pcmBytes) / 2
	if numSamples == 0 {
		if bars <= 0 {
			bars = DefaultWaveformBars
		}
		return make([]int, bars)
	}

	samples := make([]int16, numSamples)
	for i := range numSamples {
		//nolint:gosec // intentional 16-bit signed PCM conversion from raw bytes
		samples[i] = int16(binary.LittleEndian.Uint16(pcmBytes[i*2 : i*2+2]))
	}
	return NormalizePCMToWaveform(samples, bars)
}

// ExtractWaveform uses FFmpeg to decode an audio file into mono 16-bit 16kHz PCM and extracts its waveform envelope.
func ExtractWaveform(ctx context.Context, filePath string, bars int) ([]int, error) {
	if bars <= 0 {
		bars = DefaultWaveformBars
	}

	var stdout bytes.Buffer
	//nolint:gosec // filePath is sanitized by caller
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-i", filePath,
		"-f", "s16le",
		"-ac", "1",
		"-ar", "16000",
		"pipe:1",
	)
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("decoding audio with ffmpeg: %w", err)
	}

	return ExtractWaveformFromBytes(stdout.Bytes(), bars), nil
}
