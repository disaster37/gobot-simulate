package simulate

import (
	"github.com/disaster37/gobot-simulate/internal/display"
	"github.com/disaster37/gobot-simulate/internal/state"
)

type displayBinding struct {
	deviceID string
	decoder  display.Decoder
	pins     []string
}

func (s *Simulator) attachDisplay(boardID string, device state.DeviceState) {
	if device.Kind != state.DeviceKindDisplay || device.DisplayModel == "" {
		return
	}
	decoder := display.New(device.DisplayModel, device.Pins)
	if decoder == nil {
		return
	}
	s.displayMu.Lock()
	if s.displays[boardID] == nil {
		s.displays[boardID] = make(map[string]displayBinding)
	}
	s.displays[boardID][device.ID] = displayBinding{
		deviceID: device.ID,
		decoder:  decoder,
		pins:     append([]string(nil), device.Pins...),
	}
	s.displayMu.Unlock()
	if board, err := s.store.Resolve(boardID); err == nil {
		_, _ = board.SetDeviceStateField(device.ID, "display", decoder.Snapshot())
	}
}

func (s *Simulator) observeDisplayPin(boardID, pin string, level int) {
	s.displayMu.RLock()
	bindings := make([]displayBinding, 0, len(s.displays[boardID]))
	for _, binding := range s.displays[boardID] {
		if containsPin(binding.pins, pin) {
			bindings = append(bindings, binding)
		}
	}
	s.displayMu.RUnlock()
	if len(bindings) == 0 {
		return
	}
	board, err := s.store.Resolve(boardID)
	if err != nil {
		return
	}
	for _, binding := range bindings {
		binding.decoder.Pin(pin, level)
		_, _ = board.SetDeviceStateField(binding.deviceID, "display", binding.decoder.Snapshot())
	}
}

func containsPin(pins []string, pin string) bool {
	for _, candidate := range pins {
		if candidate == pin {
			return true
		}
	}
	return false
}
