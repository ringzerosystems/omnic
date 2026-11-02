package ffmpeg

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
)

type darwinDeviceManager struct {
	runner CommandRunner
}

func newDarwinDeviceManager(runner CommandRunner) *darwinDeviceManager {
	if runner == nil {
		runner = defaultCommandRunner
	}
	return &darwinDeviceManager{runner: runner}
}

var avfAudioRegex = regexp.MustCompile(`\[(\d+)\]\s+(.+)`)

func parseAVFoundationAudioDevices(out []byte) []AudioDevice {
	var devices []AudioDevice
	lines := strings.Split(string(out), "\n")
	inAudioSection := false

	for _, line := range lines {
		if strings.Contains(line, "AVFoundation audio devices:") {
			inAudioSection = true
			continue
		}
		if inAudioSection {
			if strings.Contains(line, "AVFoundation video devices:") {
				break
			}
			matches := avfAudioRegex.FindStringSubmatch(line)
			if len(matches) == 3 {
				idx := matches[1]
				name := strings.TrimSpace(matches[2])
				devices = append(devices, AudioDevice{
					ID:        idx,
					Name:      name,
					Type:      DeviceInput,
					Backend:   "avfoundation",
					IsDefault: len(devices) == 0,
				})
			}
		}
	}
	return devices
}

type spAudioItem struct {
	Name      string `json:"_name"`
	IsDefault string `json:"coreaudio_default_audio_output_device"`
}

type spAudioDataType struct {
	Items []spAudioItem `json:"_items"`
}

type spAudioRoot struct {
	DataType []spAudioDataType `json:"SPAudioDataType"`
}

func parseSystemProfilerAudio(out []byte) []AudioDevice {
	var root spAudioRoot
	if err := json.Unmarshal(out, &root); err != nil {
		return nil
	}
	var res []AudioDevice
	for _, dt := range root.DataType {
		for _, item := range dt.Items {
			if item.Name == "" {
				continue
			}
			isDef := item.IsDefault == "spaudio_yes" || item.IsDefault == "yes"
			res = append(res, AudioDevice{
				ID:        item.Name,
				Name:      item.Name,
				Type:      DeviceOutput,
				Backend:   "coreaudio",
				IsDefault: isDef,
			})
		}
	}
	return res
}

func (m *darwinDeviceManager) ListInputDevices(ctx context.Context) ([]AudioDevice, error) {
	out, err := m.runner(ctx, "ffmpeg", "-f", "avfoundation", "-list_devices", "true", "-i", "")
	devices := parseAVFoundationAudioDevices(out)
	if len(devices) > 0 {
		return devices, nil
	}
	if err == nil {
		return devices, nil
	}
	return []AudioDevice{{
		ID:        "0",
		Name:      "Default macOS Microphone",
		Type:      DeviceInput,
		Backend:   "avfoundation",
		IsDefault: true,
	}}, nil
}

func (m *darwinDeviceManager) ListOutputDevices(ctx context.Context) ([]AudioDevice, error) {
	out, err := m.runner(ctx, "system_profiler", "SPAudioDataType", "-json")
	if err == nil {
		devices := parseSystemProfilerAudio(out)
		if len(devices) > 0 {
			return devices, nil
		}
	}
	return []AudioDevice{{
		ID:        "default",
		Name:      "Default macOS Output",
		Type:      DeviceOutput,
		Backend:   "coreaudio",
		IsDefault: true,
	}}, nil
}

func (m *darwinDeviceManager) GetDefaultInput(ctx context.Context) (*AudioDevice, error) {
	devs, err := m.ListInputDevices(ctx)
	if err != nil || len(devs) == 0 {
		return &AudioDevice{
			ID:        "0",
			Name:      "Default macOS Microphone",
			Type:      DeviceInput,
			Backend:   "avfoundation",
			IsDefault: true,
		}, nil
	}
	for i := range devs {
		if devs[i].IsDefault {
			return &devs[i], nil
		}
	}
	return &devs[0], nil
}

func (m *darwinDeviceManager) GetDefaultOutput(ctx context.Context) (*AudioDevice, error) {
	devs, err := m.ListOutputDevices(ctx)
	if err != nil || len(devs) == 0 {
		return &AudioDevice{
			ID:        "default",
			Name:      "Default macOS Output",
			Type:      DeviceOutput,
			Backend:   "coreaudio",
			IsDefault: true,
		}, nil
	}
	for i := range devs {
		if devs[i].IsDefault {
			return &devs[i], nil
		}
	}
	return &devs[0], nil
}
