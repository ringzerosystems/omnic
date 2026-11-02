package ffmpeg

import (
	"context"
	"errors"
	"testing"
)

func TestLinuxDeviceManager(t *testing.T) {
	t.Parallel()

	pactlSourcesJSON := `[
		{"name": "alsa_output.pci.monitor", "description": "Monitor of Built-in Audio"},
		{"name": "alsa_input.pci.analog-stereo", "description": "Built-in Microphone"}
	]`
	pactlSinksJSON := `[
		{"name": "alsa_output.pci.analog-stereo", "description": "Built-in Audio Analog Stereo"}
	]`

	mockRunner := func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "pactl" {
			if len(args) >= 3 && args[2] == "sources" {
				return []byte(pactlSourcesJSON), nil
			}
			if len(args) >= 3 && args[2] == "sinks" {
				return []byte(pactlSinksJSON), nil
			}
			if len(args) >= 1 && args[0] == "get-default-source" {
				return []byte("alsa_input.pci.analog-stereo\n"), nil
			}
			if len(args) >= 1 && args[0] == "get-default-sink" {
				return []byte("alsa_output.pci.analog-stereo\n"), nil
			}
		}
		return nil, errors.New("command not found")
	}

	mgr := newLinuxDeviceManager(mockRunner)
	ctx := context.Background()

	inputs, err := mgr.ListInputDevices(ctx)
	if err != nil {
		t.Fatalf("ListInputDevices failed: %v", err)
	}
	if len(inputs) != 1 {
		t.Fatalf("expected 1 input device (excluding monitor), got %d", len(inputs))
	}
	if inputs[0].Name != "Built-in Microphone" {
		t.Errorf("got name %q, want Built-in Microphone", inputs[0].Name)
	}

	outputs, err := mgr.ListOutputDevices(ctx)
	if err != nil {
		t.Fatalf("ListOutputDevices failed: %v", err)
	}
	if len(outputs) != 1 {
		t.Fatalf("expected 1 output device, got %d", len(outputs))
	}

	defIn, err := mgr.GetDefaultInput(ctx)
	if err != nil || defIn == nil || defIn.ID != "alsa_input.pci.analog-stereo" {
		t.Fatalf("unexpected default input: %+v", defIn)
	}

	defOut, err := mgr.GetDefaultOutput(ctx)
	if err != nil || defOut == nil || defOut.ID != "alsa_output.pci.analog-stereo" {
		t.Fatalf("unexpected default output: %+v", defOut)
	}
}

func TestDarwinDeviceManager(t *testing.T) {
	t.Parallel()

	avfOutput := `[AVFoundation indev @ 0x7fa2b00] AVFoundation video devices:
[AVFoundation indev @ 0x7fa2b00] [0] FaceTime HD Camera
[AVFoundation indev @ 0x7fa2b00] AVFoundation audio devices:
[AVFoundation indev @ 0x7fa2b00] [0] MacBook Pro Microphone
[AVFoundation indev @ 0x7fa2b00] [1] External USB Mic`

	spAudioJSON := `{
		"SPAudioDataType": [
			{
				"_items": [
					{
						"_name": "MacBook Pro Speakers",
						"coreaudio_default_audio_output_device": "spaudio_yes"
					},
					{
						"_name": "External Headphones",
						"coreaudio_default_audio_output_device": "spaudio_no"
					}
				]
			}
		]
	}`

	mockRunner := func(_ context.Context, name string, _ ...string) ([]byte, error) {
		if name == "ffmpeg" {
			return []byte(avfOutput), nil
		}
		if name == "system_profiler" {
			return []byte(spAudioJSON), nil
		}
		return nil, errors.New("command not found")
	}

	mgr := newDarwinDeviceManager(mockRunner)
	ctx := context.Background()

	inputs, err := mgr.ListInputDevices(ctx)
	if err != nil {
		t.Fatalf("ListInputDevices failed: %v", err)
	}
	if len(inputs) != 2 {
		t.Fatalf("expected 2 darwin input devices, got %d", len(inputs))
	}
	if inputs[0].ID != "0" || inputs[0].Name != "MacBook Pro Microphone" {
		t.Errorf("unexpected input 0: %+v", inputs[0])
	}
	if inputs[1].ID != "1" || inputs[1].Name != "External USB Mic" {
		t.Errorf("unexpected input 1: %+v", inputs[1])
	}

	outputs, err := mgr.ListOutputDevices(ctx)
	if err != nil {
		t.Fatalf("ListOutputDevices failed: %v", err)
	}
	if len(outputs) != 2 {
		t.Fatalf("expected 2 darwin output devices, got %d", len(outputs))
	}
	if !outputs[0].IsDefault {
		t.Errorf("expected speakers to be default: %+v", outputs[0])
	}

	defIn, err := mgr.GetDefaultInput(ctx)
	if err != nil || defIn.ID != "0" {
		t.Errorf("unexpected default input: %+v", defIn)
	}

	defOut, err := mgr.GetDefaultOutput(ctx)
	if err != nil || defOut.Name != "MacBook Pro Speakers" {
		t.Errorf("unexpected default output: %+v", defOut)
	}
}

func TestStubDeviceManager(t *testing.T) {
	t.Parallel()

	mgr := newStubDeviceManager()
	ctx := context.Background()

	in, err := mgr.GetDefaultInput(ctx)
	if err != nil || in == nil {
		t.Fatalf("expected stub input, got %v", err)
	}
	out, err := mgr.GetDefaultOutput(ctx)
	if err != nil || out == nil {
		t.Fatalf("expected stub output, got %v", err)
	}
}

func TestPlayerAndRecorderUnits(t *testing.T) {
	t.Parallel()

	player := NewPlayer()
	if player.IsPlaying() {
		t.Errorf("expected player to not be playing")
	}
	if err := player.Stop(); err != nil {
		t.Errorf("stop idle player failed: %v", err)
	}
	if _, err := player.Play(context.Background(), ""); err == nil {
		t.Errorf("expected error on empty media path")
	}

	rec := NewRecorder(RecordConfig{
		SampleRate: 16000,
		Bitrate:    "24k",
	})
	if err := rec.Cancel(); err != nil {
		t.Errorf("cancel idle recorder failed: %v", err)
	}
	if _, err := rec.Stop(); err == nil {
		t.Errorf("expected error stopping idle recorder")
	}
}
