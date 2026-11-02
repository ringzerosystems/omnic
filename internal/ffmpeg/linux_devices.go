package ffmpeg

import (
	"context"
	"encoding/json"
	"strings"
)

type linuxDeviceManager struct {
	runner CommandRunner
}

func newLinuxDeviceManager(runner CommandRunner) *linuxDeviceManager {
	if runner == nil {
		runner = defaultCommandRunner
	}
	return &linuxDeviceManager{runner: runner}
}

type pactlNode struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (m *linuxDeviceManager) parsePulseSources(out []byte, defaultID string) []AudioDevice {
	var nodes []pactlNode
	if err := json.Unmarshal(out, &nodes); err != nil {
		return nil
	}
	var res []AudioDevice
	for _, n := range nodes {
		if strings.Contains(n.Name, ".monitor") {
			continue
		}
		desc := n.Description
		if desc == "" {
			desc = n.Name
		}
		res = append(res, AudioDevice{
			ID:        n.Name,
			Name:      desc,
			Type:      DeviceInput,
			Backend:   "pulse",
			IsDefault: n.Name == defaultID,
		})
	}
	return res
}

func (m *linuxDeviceManager) parsePulseSinks(out []byte, defaultID string) []AudioDevice {
	var nodes []pactlNode
	if err := json.Unmarshal(out, &nodes); err != nil {
		return nil
	}
	var res []AudioDevice
	for _, n := range nodes {
		desc := n.Description
		if desc == "" {
			desc = n.Name
		}
		res = append(res, AudioDevice{
			ID:        n.Name,
			Name:      desc,
			Type:      DeviceOutput,
			Backend:   "pulse",
			IsDefault: n.Name == defaultID,
		})
	}
	return res
}

func parseAlsaCards(out []byte, devType DeviceType) []AudioDevice {
	var res []AudioDevice
	lines := strings.SplitSeq(string(out), "\n")
	for line := range lines {
		if strings.HasPrefix(line, "card ") {
			res = append(res, AudioDevice{
				ID:        "default",
				Name:      strings.TrimSpace(line),
				Type:      devType,
				Backend:   "alsa",
				IsDefault: len(res) == 0,
			})
		}
	}
	return res
}

func (m *linuxDeviceManager) ListInputDevices(ctx context.Context) ([]AudioDevice, error) {
	defIn, _ := m.GetDefaultInput(ctx)
	defID := ""
	if defIn != nil {
		defID = defIn.ID
	}
	out, err := m.runner(ctx, "pactl", "--format=json", "list", "sources")
	if err == nil {
		devices := m.parsePulseSources(out, defID)
		if len(devices) > 0 {
			return devices, nil
		}
	}
	if alsaOut, aErr := m.runner(ctx, "arecord", "-l"); aErr == nil {
		devices := parseAlsaCards(alsaOut, DeviceInput)
		if len(devices) > 0 {
			return devices, nil
		}
	}
	return []AudioDevice{{
		ID:        "default",
		Name:      "Default Linux Input",
		Type:      DeviceInput,
		Backend:   "pulse",
		IsDefault: true,
	}}, nil
}

func (m *linuxDeviceManager) ListOutputDevices(ctx context.Context) ([]AudioDevice, error) {
	defOut, _ := m.GetDefaultOutput(ctx)
	defID := ""
	if defOut != nil {
		defID = defOut.ID
	}
	out, err := m.runner(ctx, "pactl", "--format=json", "list", "sinks")
	if err == nil {
		devices := m.parsePulseSinks(out, defID)
		if len(devices) > 0 {
			return devices, nil
		}
	}
	if alsaOut, aErr := m.runner(ctx, "aplay", "-l"); aErr == nil {
		devices := parseAlsaCards(alsaOut, DeviceOutput)
		if len(devices) > 0 {
			return devices, nil
		}
	}
	return []AudioDevice{{
		ID:        "default",
		Name:      "Default Linux Output",
		Type:      DeviceOutput,
		Backend:   "pulse",
		IsDefault: true,
	}}, nil
}

func (m *linuxDeviceManager) GetDefaultInput(ctx context.Context) (*AudioDevice, error) {
	out, err := m.runner(ctx, "pactl", "get-default-source")
	if err == nil {
		name := strings.TrimSpace(string(out))
		if name != "" {
			return &AudioDevice{
				ID:        name,
				Name:      name,
				Type:      DeviceInput,
				Backend:   "pulse",
				IsDefault: true,
			}, nil
		}
	}
	return &AudioDevice{
		ID:        "default",
		Name:      "Default Linux Input",
		Type:      DeviceInput,
		Backend:   "pulse",
		IsDefault: true,
	}, nil
}

func (m *linuxDeviceManager) GetDefaultOutput(ctx context.Context) (*AudioDevice, error) {
	out, err := m.runner(ctx, "pactl", "get-default-sink")
	if err == nil {
		name := strings.TrimSpace(string(out))
		if name != "" {
			return &AudioDevice{
				ID:        name,
				Name:      name,
				Type:      DeviceOutput,
				Backend:   "pulse",
				IsDefault: true,
			}, nil
		}
	}
	return &AudioDevice{
		ID:        "default",
		Name:      "Default Linux Output",
		Type:      DeviceOutput,
		Backend:   "pulse",
		IsDefault: true,
	}, nil
}
