package simulate

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/disaster37/gobot-simulate/internal/bus"
	"github.com/disaster37/gobot-simulate/internal/state"
	"github.com/disaster37/gobot-simulate/internal/trace"
	"gobot.io/x/gobot/v2"
)

// AttachDevices stores driver handles for a board so commands are dispatched
// through the registered driver rather than by mutating pin state directly.
func (s *Simulator) AttachDevices(boardID string, devices ...gobot.Device) error {
	board, err := s.store.Resolve(boardID)
	if err != nil {
		return err
	}
	s.driversMu.Lock()
	defer s.driversMu.Unlock()
	if s.drivers[board.ID()] == nil {
		s.drivers[board.ID()] = make(map[string]gobot.Device)
	}
	for _, device := range devices {
		if device == nil {
			continue
		}
		id := state.NormalizeIdentifier(device.Name())
		s.drivers[board.ID()][id] = device
	}
	return nil
}

func (s *Simulator) attachDriver(boardID, deviceID string, device gobot.Device) {
	if device == nil {
		return
	}
	s.driversMu.Lock()
	defer s.driversMu.Unlock()
	if s.drivers[boardID] == nil {
		s.drivers[boardID] = make(map[string]gobot.Device)
	}
	s.drivers[boardID][deviceID] = device
}

func (s *Simulator) driver(boardID, deviceID string) gobot.Device {
	s.driversMu.RLock()
	defer s.driversMu.RUnlock()
	if board, ok := s.drivers[boardID]; ok {
		return board[deviceID]
	}
	return nil
}

// CommandDevice dispatches a driver-aware command for a device. It never
// holds a board lock while invoking the driver.
func (s *Simulator) CommandDevice(boardID, deviceID, action string, value *int) error {
	if err := state.ValidateIdentifier("device", deviceID); err != nil {
		return err
	}
	if _, err := s.store.Device(boardID, deviceID); err != nil {
		return err
	}
	driver := s.driver(boardID, deviceID)
	if driver == nil {
		return fmt.Errorf("%w: device %s has no attached driver", ErrReadOnly, deviceID)
	}
	started := s.clk.Now()
	err := dispatchCommand(driver, action, value)
	rec := trace.Record{
		BoardID:    boardID,
		Kind:       trace.KindDriverEvent,
		Target:     deviceID,
		Operation:  action,
		Value:      value,
		Source:     state.SourceUI,
		DurationNs: s.clk.Since(started).Nanoseconds(),
	}
	if err != nil {
		rec.Error = err.Error()
	}
	s.traces.Append(rec)
	return err
}

func dispatchCommand(driver gobot.Device, action string, value *int) error {
	level := 0
	if value != nil {
		level = *value
	}
	switch strings.ToLower(action) {
	case "on":
		if d, ok := driver.(interface{ On() error }); ok {
			return d.On()
		}
	case "off":
		if d, ok := driver.(interface{ Off() error }); ok {
			return d.Off()
		}
	case "toggle":
		if d, ok := driver.(interface{ Toggle() error }); ok {
			return d.Toggle()
		}
	case "brightness":
		if d, ok := driver.(interface{ Brightness(byte) error }); ok {
			return d.Brightness(byte(level))
		}
	case "angle", "move":
		if d, ok := driver.(interface{ Move(int) error }); ok {
			return d.Move(level)
		}
		if d, ok := driver.(interface{ Move(uint8) error }); ok {
			return d.Move(uint8(level))
		}
	case "step", "steps":
		if d, ok := driver.(interface{ Move(int) error }); ok {
			return d.Move(level)
		}
	case "speed":
		if d, ok := driver.(interface{ SetSpeed(byte) error }); ok {
			return d.SetSpeed(byte(level))
		}
		if d, ok := driver.(interface{ SetSpeed(uint) error }); ok {
			return d.SetSpeed(uint(level))
		}
	case "setrgb":
		if d, ok := driver.(interface{ SetRGB(byte, byte, byte) error }); ok {
			return d.SetRGB(byte(level>>16), byte(level>>8), byte(level))
		}
	case "measure", "distance":
		if d, ok := driver.(interface{ MeasureDistance() (float64, error) }); ok {
			_, err := d.MeasureDistance()
			return err
		}
	}
	if commander, ok := driver.(gobot.Commander); ok {
		if command := commander.Command(action); command != nil {
			result := command(map[string]interface{}{
				"value": level,
				"level": level,
				"angle": level,
				"speed": level,
				"steps": level,
			})
			if err, ok := result.(error); ok {
				return err
			}
			return nil
		}
		for name, command := range commander.Commands() {
			if !strings.EqualFold(name, action) {
				continue
			}
			result := command(map[string]interface{}{
				"value": level,
				"level": level,
				"angle": level,
				"speed": level,
				"steps": level,
			})
			if err, ok := result.(error); ok {
				return err
			}
			return nil
		}
	}
	return fmt.Errorf("%w: %s does not support action %q", ErrReadOnly, driver.Name(), action)
}

// SetDeviceFault injects a device-level fault.
func (s *Simulator) SetDeviceFault(boardID, deviceID, fault string) (DeviceState, error) {
	board, err := s.store.Resolve(boardID)
	if err != nil {
		return DeviceState{}, err
	}
	parsed, err := state.ParseDeviceFault(fault)
	if err != nil {
		return DeviceState{}, err
	}
	updated, err := board.SetDeviceFault(deviceID, parsed)
	if err != nil {
		return DeviceState{}, err
	}
	s.traces.Append(trace.Record{
		BoardID:   board.ID(),
		Kind:      trace.KindFault,
		Target:    deviceID,
		Operation: "set",
		Value:     string(parsed),
		Source:    state.SourceUI,
	})
	return updated, nil
}

// ClearDeviceFault removes one device-level fault.
func (s *Simulator) ClearDeviceFault(boardID, deviceID, fault string) (DeviceState, error) {
	board, err := s.store.Resolve(boardID)
	if err != nil {
		return DeviceState{}, err
	}
	parsed, err := state.ParseDeviceFault(fault)
	if err != nil {
		return DeviceState{}, err
	}
	updated, err := board.ClearDeviceFault(deviceID, parsed)
	if err != nil {
		return DeviceState{}, err
	}
	s.traces.Append(trace.Record{
		BoardID:   board.ID(),
		Kind:      trace.KindFault,
		Target:    deviceID,
		Operation: "clear",
		Value:     string(parsed),
		Source:    state.SourceUI,
	})
	return updated, nil
}

// ClearDeviceFaults removes every device-level fault.
func (s *Simulator) ClearDeviceFaults(boardID, deviceID string) (DeviceState, error) {
	board, err := s.store.Resolve(boardID)
	if err != nil {
		return DeviceState{}, err
	}
	updated, err := board.ClearDeviceFaults(deviceID)
	if err != nil {
		return DeviceState{}, err
	}
	s.traces.Append(trace.Record{
		BoardID:   board.ID(),
		Kind:      trace.KindFault,
		Target:    deviceID,
		Operation: "clear-all",
		Source:    state.SourceUI,
	})
	return updated, nil
}

// Step records a business-level timeline annotation.
func (s *Simulator) Step(boardID, name, status string, detail map[string]interface{}) error {
	board, err := s.store.Resolve(boardID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: step name must not be empty", ErrInvalidValue)
	}
	s.traces.Append(trace.Record{
		BoardID:   board.ID(),
		Kind:      trace.KindStep,
		Target:    name,
		Operation: status,
		Value:     status,
		Source:    state.SourceSimulation,
		Detail:    detail,
	})
	return nil
}

// Trace returns bounded trace records.
func (s *Simulator) Trace(boardID string, since uint64, kinds []string, target string) ([]trace.Record, error) {
	board, err := s.store.Resolve(boardID)
	if err != nil {
		return nil, err
	}
	parsed := make([]trace.Kind, 0, len(kinds))
	for _, raw := range kinds {
		kind := trace.Kind(strings.TrimSpace(raw))
		if !kind.Valid() {
			return nil, fmt.Errorf("%w: unknown trace kind %q", ErrInvalidValue, raw)
		}
		parsed = append(parsed, kind)
	}
	return s.traces.Query(board.ID(), since, parsed, target), nil
}

// Waveform returns downsampled step series.
func (s *Simulator) Waveform(boardID string, pins []string, from, to time.Time, maxPoints int) ([]trace.Series, error) {
	board, err := s.store.Resolve(boardID)
	if err != nil {
		return nil, err
	}
	return s.traces.Waveform(board.ID(), pins, from, to, maxPoints), nil
}

// SubscribeTrace returns a trace record channel for SSE streaming.
func (s *Simulator) SubscribeTrace() <-chan trace.Record { return s.traces.Subscribe() }

// UnsubscribeTrace removes a trace channel.
func (s *Simulator) UnsubscribeTrace(ch <-chan trace.Record) { s.traces.Unsubscribe(ch) }

// Buses returns the bus inventory for a board.
func (s *Simulator) Buses(boardID string) ([]map[string]interface{}, error) {
	adaptor, err := s.adaptorFor(boardID)
	if err != nil {
		return nil, err
	}
	return adaptor.buses.Buses(), nil
}

// Chips returns the chip inventory for a board.
func (s *Simulator) Chips(boardID string) ([]bus.ChipEntry, error) {
	adaptor, err := s.adaptorFor(boardID)
	if err != nil {
		return nil, err
	}
	return adaptor.buses.Chips(), nil
}

// Chip returns one chip entry for a board.
func (s *Simulator) Chip(boardID, chipID string) (bus.ChipEntry, error) {
	adaptor, err := s.adaptorFor(boardID)
	if err != nil {
		return bus.ChipEntry{}, err
	}
	entry, err := adaptor.buses.Chip(chipID)
	if err != nil {
		return bus.ChipEntry{}, err
	}
	return *entry, nil
}

// FindChip searches all boards for a chip ID.
func (s *Simulator) FindChip(chipID string) (bus.ChipEntry, error) {
	s.adaptorsMu.Lock()
	adaptors := make([]*Adaptor, 0, len(s.adaptors))
	for _, adaptor := range s.adaptors {
		adaptors = append(adaptors, adaptor)
	}
	s.adaptorsMu.Unlock()
	for _, adaptor := range adaptors {
		entry, err := adaptor.buses.Chip(chipID)
		if err == nil {
			return *entry, nil
		}
	}
	return bus.ChipEntry{}, fmt.Errorf("%w: %s", bus.ErrChipNotFound, chipID)
}

// ChipValues returns descriptor + live values (and display snapshot).
func (s *Simulator) ChipValues(chipID string) (map[string]interface{}, error) {
	entry, err := s.FindChip(chipID)
	if err != nil {
		return nil, err
	}
	adaptor, err := s.adaptorFor(entry.BoardID)
	if err != nil {
		return nil, err
	}
	model, err := adaptor.buses.ChipModel(chipID)
	if err != nil {
		return nil, err
	}
	out := bus.ChipModelJSON(model)
	out["chip"] = entry
	return out, nil
}

// SetChipField updates one simulated chip quantity.
func (s *Simulator) SetChipField(boardID, chipID, field string, value interface{}) (bus.ChipEntry, error) {
	adaptor, err := s.adaptorFor(boardID)
	if err != nil {
		return bus.ChipEntry{}, err
	}
	model, err := adaptor.buses.ChipModel(chipID)
	if err != nil {
		return bus.ChipEntry{}, err
	}
	if setErr := model.Set(field, value); setErr != nil {
		return bus.ChipEntry{}, setErr
	}
	entry, err := adaptor.buses.Chip(chipID)
	if err != nil {
		return bus.ChipEntry{}, err
	}
	return *entry, nil
}

// RawChip executes a raw chip transaction from the bus console.
func (s *Simulator) RawChip(boardID, chipID string, txn bus.Transaction) (bus.Response, error) {
	adaptor, err := s.adaptorFor(boardID)
	if err != nil {
		return bus.Response{}, err
	}
	model, err := adaptor.buses.ChipModel(chipID)
	if err != nil {
		return bus.Response{}, err
	}
	if len(txn.Write) > 32 || txn.ReadLen > 256 {
		return bus.Response{}, fmt.Errorf("%w: raw transaction too large", ErrInvalidValue)
	}
	return model.Transfer(txn)
}

// SetBusFault injects a bus-level fault.
func (s *Simulator) SetBusFault(boardID, busID, fault string) error {
	adaptor, err := s.adaptorFor(boardID)
	if err != nil {
		return err
	}
	return adaptor.buses.SetBusFault(busID, fault)
}

// ClearBusFault clears a bus-level fault.
func (s *Simulator) ClearBusFault(boardID, busID, fault string) error {
	adaptor, err := s.adaptorFor(boardID)
	if err != nil {
		return err
	}
	return adaptor.buses.ClearBusFault(busID, fault)
}

func (s *Simulator) adaptorFor(boardID string) (*Adaptor, error) {
	board, err := s.store.Resolve(boardID)
	if err != nil {
		return nil, err
	}
	s.adaptorsMu.Lock()
	defer s.adaptorsMu.Unlock()
	if adaptor, ok := s.adaptors[board.ID()]; ok {
		return adaptor, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrBoardNotFound, boardID)
}

// DigitalRead reads a pin through the board model, applying device faults.
func (s *Simulator) DigitalRead(boardID, pin string) (int, error) {
	board, err := s.store.Resolve(boardID)
	if err != nil {
		return 0, err
	}
	return board.DigitalReadPin(pin).DigitalValue, nil
}

// AnalogRead reads an analog pin through the board model.
func (s *Simulator) AnalogRead(boardID, pin string) (int, error) {
	board, err := s.store.Resolve(boardID)
	if err != nil {
		return 0, err
	}
	return board.AnalogReadPin(pin).AnalogValue, nil
}

// BindPinMode records an aREST/particle pin mode.
func (s *Simulator) BindPinMode(boardID, pin, mode string) error {
	parsed := state.PinMode(strings.ToLower(strings.ReplaceAll(mode, "_", "-")))
	switch mode {
	case "i":
		parsed = state.PinModeInput
	case "I":
		parsed = state.PinModeInputPullup
	case "o":
		parsed = state.PinModeOutput
	}
	if parsed == "input-pull-up" {
		parsed = state.PinModeInputPullup
	}
	if !parsed.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidPinMode, mode)
	}
	return s.BindPin(boardID, pin, "", parsed)
}

// BoardLatency returns the configured artificial lag.
func (s *Simulator) BoardLatency(boardID string) time.Duration {
	board, err := s.store.Resolve(boardID)
	if err != nil {
		return 0
	}
	return board.Latency()
}

// BoardFault returns the active board-wide fault error.
func (s *Simulator) BoardFault(boardID string) error {
	board, err := s.store.Resolve(boardID)
	if err != nil {
		return err
	}
	return board.FaultError()
}

// ScenarioSetPin sets a digitised pin value.
func (s *Simulator) ScenarioSetPin(_ context.Context, boardID, pin string, value int) error {
	return s.SetDigital(boardID, pin, value, SourceSimulation)
}

// ScenarioSetValue sets an extra value.
func (s *Simulator) ScenarioSetValue(_ context.Context, boardID, name string, value interface{}) error {
	return s.SetValue(boardID, name, value, SourceSimulation)
}

// ScenarioPress presses a button.
func (s *Simulator) ScenarioPress(_ context.Context, boardID, deviceID string) error {
	return s.PressButton(boardID, deviceID)
}

// ScenarioRelease releases a button.
func (s *Simulator) ScenarioRelease(_ context.Context, boardID, deviceID string) error {
	return s.ReleaseButton(boardID, deviceID)
}

// ScenarioCommand dispatches a driver-aware command.
func (s *Simulator) ScenarioCommand(_ context.Context, boardID, deviceID, action string, value *int) error {
	return s.CommandDevice(boardID, deviceID, action, value)
}

// ScenarioSetDeviceFault injects a device fault.
func (s *Simulator) ScenarioSetDeviceFault(_ context.Context, boardID, deviceID, fault string) error {
	_, err := s.SetDeviceFault(boardID, deviceID, fault)
	return err
}

// ScenarioClearDeviceFault clears device faults.
func (s *Simulator) ScenarioClearDeviceFault(_ context.Context, boardID, deviceID string) error {
	_, err := s.ClearDeviceFaults(boardID, deviceID)
	return err
}

// ScenarioEmitStep records a step marker.
func (s *Simulator) ScenarioEmitStep(_ context.Context, boardID, name, status string, detail map[string]interface{}) error {
	return s.Step(boardID, name, status, detail)
}
