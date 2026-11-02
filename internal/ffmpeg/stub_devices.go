package ffmpeg

import "context"

type stubDeviceManager struct{}

func newStubDeviceManager() *stubDeviceManager {
	return &stubDeviceManager{}
}

func (m *stubDeviceManager) ListInputDevices(_ context.Context) ([]AudioDevice, error) {
	return []AudioDevice{{
		ID:        "default",
		Name:      "Default Microphone",
		Type:      DeviceInput,
		Backend:   "stub",
		IsDefault: true,
	}}, nil
}

func (m *stubDeviceManager) ListOutputDevices(_ context.Context) ([]AudioDevice, error) {
	return []AudioDevice{{
		ID:        "default",
		Name:      "Default Speaker",
		Type:      DeviceOutput,
		Backend:   "stub",
		IsDefault: true,
	}}, nil
}

func (m *stubDeviceManager) GetDefaultInput(ctx context.Context) (*AudioDevice, error) {
	devs, _ := m.ListInputDevices(ctx)
	return &devs[0], nil
}

func (m *stubDeviceManager) GetDefaultOutput(ctx context.Context) (*AudioDevice, error) {
	devs, _ := m.ListOutputDevices(ctx)
	return &devs[0], nil
}
