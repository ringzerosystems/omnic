package ffmpeg

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Device holds information about a legacy PulseAudio device.
type Device struct {
	Name         string `json:"name"`
	FriendlyName string `json:"description"`
}

// DeviceType classifies audio inputs and outputs.
type DeviceType string

const (
	DeviceInput  DeviceType = "input"
	DeviceOutput DeviceType = "output"
)

// AudioDevice represents a hardware or software audio device.
type AudioDevice struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Type      DeviceType `json:"type"`
	Backend   string     `json:"backend"`
	IsDefault bool       `json:"is_default"`
}

// DeviceManager discovers system audio devices.
type DeviceManager interface {
	GetDefaultInput(ctx context.Context) (*AudioDevice, error)
	GetDefaultOutput(ctx context.Context) (*AudioDevice, error)
	ListInputDevices(ctx context.Context) ([]AudioDevice, error)
	ListOutputDevices(ctx context.Context) ([]AudioDevice, error)
}

// CommandRunner executes a system command and returns standard output or error.
type CommandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

func defaultCommandRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	//nolint:gosec // arguments are constructed internally
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// NewDeviceManager returns an OS-appropriate DeviceManager.
func NewDeviceManager() DeviceManager {
	return NewDeviceManagerWithRunner(defaultCommandRunner)
}

// NewDeviceManagerWithRunner creates a DeviceManager with a custom command runner.
func NewDeviceManagerWithRunner(runner CommandRunner) DeviceManager {
	if runtime.GOOS == "darwin" {
		return newDarwinDeviceManager(runner)
	}
	if runtime.GOOS == "linux" {
		return newLinuxDeviceManager(runner)
	}
	return newStubDeviceManager()
}

// DiscoverDevices returns the default mic and sink (monitor) devices for backward compatibility.
func DiscoverDevices() (mic string, sink string) {
	ctx := context.Background()
	mgr := NewDeviceManager()
	in, err := mgr.GetDefaultInput(ctx)
	if err == nil && in != nil {
		mic = in.ID
	}
	if out, err := mgr.GetDefaultOutput(ctx); err == nil && out != nil {
		sink = out.ID
		if !strings.HasSuffix(sink, ".monitor") && in != nil && in.Backend == "pulse" {
			sink += ".monitor"
		}
	}
	if mic == "" {
		mic = "default"
	}
	if sink == "" {
		sink = "default"
	}
	return mic, sink
}

// ListSources returns available sources for backward compatibility.
func ListSources() (mics []Device, sinks []Device, err error) {
	out, err := exec.Command("pactl", "--format=json", "list", "sources").Output()
	if err != nil {
		return nil, nil, fmt.Errorf("listing sources: %w", err)
	}

	var devices []Device
	if err := json.Unmarshal(out, &devices); err != nil {
		return nil, nil, fmt.Errorf("unmarshaling sources: %w", err)
	}

	for i := range devices {
		if devices[i].FriendlyName == "" {
			devices[i].FriendlyName = devices[i].Name
		}
		if strings.Contains(devices[i].Name, ".monitor") {
			sinks = append(sinks, devices[i])
		} else {
			mics = append(mics, devices[i])
		}
	}
	return mics, sinks, nil
}
