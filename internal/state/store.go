package state

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/disaster37/gobot-simulate/internal/clock"
)

var (
	// ErrBoardNotFound is returned when a board does not exist.
	ErrBoardNotFound = errors.New("simulate: board not found")
	// ErrDeviceNotFound is returned when a device does not exist.
	ErrDeviceNotFound = errors.New("simulate: device not found")
	// ErrPinNotFound is returned when inspecting a pin that was never created.
	ErrPinNotFound = errors.New("simulate: pin not found")
	// ErrInvalidValue is returned for semantically invalid values.
	ErrInvalidValue = errors.New("simulate: invalid value")
	// ErrReadOnly is returned when a device or pin is not writable.
	ErrReadOnly = errors.New("simulate: device or pin is read-only")
	// ErrInvalidPinMode is returned for an unknown pin mode.
	ErrInvalidPinMode = errors.New("simulate: invalid pin mode")
	// ErrClientCap is returned when the maximum SSE client count is reached.
	ErrClientCap = errors.New("simulate: maximum SSE clients reached")
)

// Hooks holds test override callbacks copied out of a board before invocation.
type Hooks struct {
	Reconnect    func() error
	DigitalWrite func(pin string, val byte) error
	ServoWrite   func(pin string, val byte) error
	PwmWrite     func(pin string, val byte) error
	AnalogRead   func(pin string) (int, error)
	DigitalRead  func(pin string) (int, error)
	ValueRead    func(name string) (interface{}, error)
	ValuesRead   func() (map[string]interface{}, error)
	FunctionCall func(name, parameters string) (int, error)
}

// Board is a thread-safe handle to one simulated board.
type Board struct {
	mu       sync.RWMutex
	st       BoardState
	hooks    Hooks
	faultErr error
	clk      clock.Clock
	emit     func(Event)
	rename   func(id, name string)
	noise    atomic.Uint64
}

func newBoard(id, name, platform, port string, latency time.Duration, clk clock.Clock, emit func(Event), rename func(string, string)) *Board {
	now := clk.Now()
	values := make(map[string]ExtraValue, 1)
	values["test"] = ExtraValue{Name: "test", Value: 99, UpdatedAt: now}
	return &Board{
		st: BoardState{
			ID:             id,
			Name:           name,
			Platform:       platform,
			Port:           port,
			SimulatedLagMs: latency.Milliseconds(),
			Pins:           make(map[string]PinState),
			Devices:        make(map[string]DeviceState),
			Values:         values,
			Functions:      make(map[string]ExtraFunction),
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		clk:    clk,
		emit:   emit,
		rename: rename,
	}
}

// ID returns the immutable board identifier.
func (b *Board) ID() string { return b.st.ID }

// Snapshot returns a deep copy of the board state.
func (b *Board) Snapshot() BoardState {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return cloneBoard(b.st)
}

// SetIdentity updates the mutable display identity of the board.
func (b *Board) SetIdentity(name, platform, port string) error {
	b.mu.Lock()
	id := b.st.ID
	changed := false
	if name != "" && b.st.Name != name {
		b.st.Name = name
		changed = true
	}
	if platform != "" && b.st.Platform != platform {
		b.st.Platform = platform
		changed = true
	}
	if port != "" && b.st.Port != port {
		b.st.Port = port
		changed = true
	}
	if changed {
		b.st.UpdatedAt = b.clk.Now()
	}
	b.mu.Unlock()

	if changed && b.rename != nil && name != "" && ValidateID(name) == nil {
		b.rename(id, name)
	}
	if changed {
		b.publishBoard(SourceSimulation)
	}
	return nil
}

// SetLatency configures the artificial lag applied to I/O operations.
func (b *Board) SetLatency(d time.Duration) {
	b.mu.Lock()
	b.st.SimulatedLagMs = d.Milliseconds()
	b.st.UpdatedAt = b.clk.Now()
	b.mu.Unlock()
}

// Latency returns the configured artificial lag.
func (b *Board) Latency() time.Duration {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return time.Duration(b.st.SimulatedLagMs) * time.Millisecond
}

// SetConnected marks the board connected/disconnected and emits board.updated.
func (b *Board) SetConnected(connected bool) bool {
	b.mu.Lock()
	changed := b.st.Connected != connected || b.st.Online != connected
	b.st.Connected = connected
	b.st.Online = connected
	if changed {
		b.st.UpdatedAt = b.clk.Now()
	}
	b.mu.Unlock()
	if changed {
		b.publishBoard(SourceSimulation)
		if connected {
			st := b.Snapshot()
			b.emit(Event{Type: EventBoardStarted, Source: SourceSimulation, BoardID: st.ID, Board: &st})
		}
	}
	return changed
}

// Connected reports whether the board is connected.
func (b *Board) Connected() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.st.Connected
}

// IncrementReconnect increments the reconnect counter and emits board.updated.
func (b *Board) IncrementReconnect() {
	b.mu.Lock()
	b.st.ReconnectCount++
	b.st.Connected = true
	b.st.Online = true
	b.st.UpdatedAt = b.clk.Now()
	b.mu.Unlock()
	b.publishBoard(SourceSimulation)
}

// SetFault injects or clears a fault. The original error is retained so I/O
// methods can return it unchanged.
func (b *Board) SetFault(err error) bool {
	b.mu.Lock()
	before := b.st.Fault
	changed := (err == nil) != (b.faultErr == nil) || (err != nil && before.Message != err.Error())
	b.faultErr = err
	if err != nil {
		b.st.Fault = FaultState{Active: true, Message: err.Error(), UpdatedAt: b.clk.Now()}
	} else {
		b.st.Fault = FaultState{Active: false, UpdatedAt: b.clk.Now()}
	}
	fault := b.st.Fault
	b.st.UpdatedAt = b.clk.Now()
	b.mu.Unlock()

	if changed {
		b.emit(Event{
			Type:    EventFaultChanged,
			Source:  SourceSimulation,
			BoardID: b.st.ID,
			Fault:   &fault,
		})
	}
	return changed
}

// FaultError returns the currently injected error, if any.
func (b *Board) FaultError() error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.faultErr
}

// Hooks returns a copy of the board's override callbacks.
func (b *Board) Hooks() Hooks {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.hooks
}

// UpdateHooks mutates the override callbacks under the board lock.
func (b *Board) UpdateHooks(fn func(*Hooks)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	fn(&b.hooks)
}

// Pin returns a copy of a pin state.
func (b *Board) Pin(pin string) (PinState, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	p, ok := b.st.Pins[pin]
	return p, ok
}

// EnsurePin creates a pin if it does not exist and returns its state.
func (b *Board) EnsurePin(pin string, mode PinMode) PinState {
	b.mu.Lock()
	defer b.mu.Unlock()

	p, ok := b.st.Pins[pin]
	if !ok {
		p = PinState{Pin: pin, Mode: mode, UpdatedAt: b.clk.Now()}
		b.st.Pins[pin] = p
		b.st.UpdatedAt = b.clk.Now()
	}
	return p
}

// BindPin binds a label and mode to a pin.
func (b *Board) BindPin(pin, label string, mode PinMode) error {
	if !mode.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidPinMode, mode)
	}

	b.mu.Lock()
	old, existed := b.st.Pins[pin]
	p := old
	if !existed {
		p.Pin = pin
	}
	if label != "" {
		p.Label = label
	}
	p.Mode = mode
	if mode == PinModeInputPullup {
		p.Pullup = true
		if p.InvertInitial {
			p.DigitalValue = 0
			p.ByteValue = 0
		} else {
			p.DigitalValue = 1
			p.ByteValue = 1
		}
	}
	p.UpdatedAt = b.clk.Now()
	b.st.Pins[pin] = p
	b.st.UpdatedAt = p.UpdatedAt
	devices := b.updateDevicesLocked(p)
	b.mu.Unlock()

	b.publishPin(p, devices, old.DigitalValue, p.DigitalValue, SourceSimulation)
	return nil
}

// LabelPin associates a human-readable label with a pin.
func (b *Board) LabelPin(pin, label string) {
	if label == "" {
		return
	}
	b.mu.Lock()
	p, ok := b.st.Pins[pin]
	if !ok {
		p = PinState{Pin: pin, Mode: PinModeUnset, UpdatedAt: b.clk.Now()}
	}
	p.Label = label
	b.st.Pins[pin] = p
	b.st.UpdatedAt = b.clk.Now()
	b.mu.Unlock()
}

// SetInputPullup applies the legacy pull-up semantics to a list of pins.
func (b *Board) SetInputPullup(pins ...string) {
	for _, pin := range pins {
		b.mu.Lock()
		old, existed := b.st.Pins[pin]
		p := old
		if !existed {
			p.Pin = pin
		}
		p.Mode = PinModeInputPullup
		p.Pullup = true
		if p.InvertInitial {
			p.DigitalValue = 0
			p.ByteValue = 0
		} else {
			p.DigitalValue = 1
			p.ByteValue = 1
		}
		p.UpdatedAt = b.clk.Now()
		b.st.Pins[pin] = p
		b.st.UpdatedAt = p.UpdatedAt
		devices := b.updateDevicesLocked(p)
		b.mu.Unlock()

		b.publishPin(p, devices, old.DigitalValue, p.DigitalValue, SourceSimulation)
	}
}

// SetInvertInitialPinState marks a pin as having an inverted initial state.
func (b *Board) SetInvertInitialPinState(pin string) {
	b.mu.Lock()
	old, existed := b.st.Pins[pin]
	p := old
	if !existed {
		p = PinState{Pin: pin, Mode: PinModeUnset}
	}
	p.InvertInitial = true
	if p.Pullup {
		p.DigitalValue = 0
		p.ByteValue = 0
	}
	p.UpdatedAt = b.clk.Now()
	b.st.Pins[pin] = p
	b.st.UpdatedAt = p.UpdatedAt
	devices := b.updateDevicesLocked(p)
	b.mu.Unlock()

	b.publishPin(p, devices, old.DigitalValue, p.DigitalValue, SourceSimulation)
}

// DigitalReadPin returns the pin state, creating it as an input if needed.
// Device-level read faults (stuck, welded, noisy) are applied to the value a
// driver observes; the stored command value is left untouched.
func (b *Board) DigitalReadPin(pin string) PinState {
	b.mu.Lock()
	raw, ok := b.st.Pins[pin]
	p := raw
	if !ok {
		p = PinState{Pin: pin, Mode: PinModeInput, UpdatedAt: b.clk.Now()}
		b.st.Pins[pin] = p
		b.st.UpdatedAt = p.UpdatedAt
		raw = p
	}
	devices := b.devicesForPinLocked(pin)
	value, _ := effectiveDigital(raw, devices, b.noise.Add(1))
	b.mu.Unlock()

	p.DigitalValue = value
	p.ByteValue = byte(value) //nolint:gosec // intentional width conversion
	if !ok {
		b.publishPin(p, nil, nil, p.DigitalValue, SourceHardwareCode)
	}
	return p
}

// DigitalWritePin updates a pin's digital value and bound devices.
func (b *Board) DigitalWritePin(pin string, val byte, setMode bool, mode PinMode, source EventSource) PinState {
	b.mu.Lock()
	old, existed := b.st.Pins[pin]
	p := old
	if !existed {
		p.Pin = pin
	}
	if setMode {
		p.Mode = mode
	}
	b.noise.Add(1)
	commanded := byte(writeFaulted(pin, int(val), b.devicesForPinLocked(pin))) //nolint:gosec // intentional width conversion
	p.DigitalValue = int(commanded)
	p.ByteValue = commanded
	p.UpdatedAt = b.clk.Now()
	b.st.Pins[pin] = p
	b.st.UpdatedAt = p.UpdatedAt
	devices := b.updateDevicesLocked(p)
	b.mu.Unlock()

	b.publishPin(p, devices, old.DigitalValue, int(commanded), source)
	return p
}

// SetDigitalValue updates a digital value while preserving the pin mode.
// Callers at untrusted boundaries must validate 0..255; this low-level method
// mirrors the legacy gobot-fat mock, which stores the byte representation.
func (b *Board) SetDigitalValue(pin string, val int, source EventSource) PinState {
	//nolint:gosec // validated to 0..255 at the public and store boundaries
	return b.DigitalWritePin(pin, byte(val), false, PinModeInput, source)
}

// PwmWritePin updates a PWM pin.
func (b *Board) PwmWritePin(pin string, val byte, source EventSource) PinState {
	return b.byteWrite(pin, val, PinModePWM, source)
}

// ServoWritePin updates a servo pin.
func (b *Board) ServoWritePin(pin string, val byte, source EventSource) PinState {
	return b.byteWrite(pin, val, PinModeServo, source)
}

func (b *Board) byteWrite(pin string, val byte, mode PinMode, source EventSource) PinState {
	b.mu.Lock()
	old, existed := b.st.Pins[pin]
	p := old
	if !existed {
		p.Pin = pin
	}
	p.Mode = mode
	b.noise.Add(1)
	commanded := byte(writeFaulted(pin, int(val), b.devicesForPinLocked(pin))) //nolint:gosec // intentional width conversion
	p.ByteValue = commanded
	p.DigitalValue = int(commanded)
	p.UpdatedAt = b.clk.Now()
	b.st.Pins[pin] = p
	b.st.UpdatedAt = p.UpdatedAt
	devices := b.updateDevicesLocked(p)
	b.mu.Unlock()

	b.publishPin(p, devices, int(old.ByteValue), int(commanded), source)
	return p
}

// AnalogReadPin returns the pin's analog value, creating it if needed.
func (b *Board) AnalogReadPin(pin string) PinState {
	b.mu.Lock()
	p, ok := b.st.Pins[pin]
	if !ok {
		p = PinState{Pin: pin, Mode: PinModeAnalog, UpdatedAt: b.clk.Now()}
		b.st.Pins[pin] = p
		b.st.UpdatedAt = p.UpdatedAt
	}
	b.mu.Unlock()

	if !ok {
		b.publishPin(p, nil, nil, p.AnalogValue, SourceHardwareCode)
	}
	return p
}

// AnalogWritePin updates an analog pin.
func (b *Board) AnalogWritePin(pin string, val int, source EventSource) PinState {
	b.mu.Lock()
	old, existed := b.st.Pins[pin]
	p := old
	if !existed {
		p.Pin = pin
	}
	p.Mode = PinModeAnalog
	p.AnalogValue = val
	p.UpdatedAt = b.clk.Now()
	b.st.Pins[pin] = p
	b.st.UpdatedAt = p.UpdatedAt
	devices := b.updateDevicesLocked(p)
	b.mu.Unlock()

	b.publishPin(p, devices, old.AnalogValue, val, source)
	return p
}

// RegisterDevice upserts a device and returns the stored copy. A device
// loaded from the topology file wins over later discovery/manual updates: the
// declared fields are retained while omitted runtime metadata is filled in.
func (b *Board) RegisterDevice(d DeviceState) DeviceState {
	b.mu.Lock()
	now := b.clk.Now()
	d.BoardID = b.st.ID
	d.UpdatedAt = now
	if d.State == nil {
		d.State = make(map[string]interface{})
	}
	if d.Kind.Valid() {
		if d.Group == "" {
			d.Group = GroupForKind(d.Kind)
		}
		if d.Capabilities == (DeviceCapabilities{}) {
			d.Capabilities = DefaultCapabilities(d.Kind)
		}
	}
	if d.Wiring.IsZero() {
		d.Wiring = defaultWiring(d.Kind, d.Inverted)
		if IsInput(d.Kind) && len(d.Pins) > 0 {
			if pin, ok := b.st.Pins[d.Pins[0]]; ok && pin.Pullup {
				d.Wiring.Pull = PullUp
			}
		}
	}
	d.Wiring = d.Wiring.Normalized()
	if d.Pins == nil {
		d.Pins = []string{}
	}
	if existing, ok := b.st.Devices[d.ID]; ok && existing.Source == SourceTopology && d.Source != SourceTopology {
		d = mergeDiscovered(existing, d)
	}
	if d.Source == "" {
		d.Source = SourceRuntime
	}
	b.st.Devices[d.ID] = d
	if d.Label != "" {
		for _, pin := range d.Pins {
			p, ok := b.st.Pins[pin]
			if !ok {
				p = PinState{Pin: pin, Mode: PinModeUnset, UpdatedAt: now}
			}
			p.Label = d.Label
			b.st.Pins[pin] = p
		}
	}
	b.deriveDeviceLocked(&d, b.firstPinForDeviceLocked(d))
	b.st.Devices[d.ID] = d
	b.st.UpdatedAt = now
	stored := cloneDevice(d)
	b.mu.Unlock()

	b.publishDevice(stored, SourceSimulation)
	return stored
}

// Source markers used to implement topology precedence.
const (
	SourceTopology  = "topology"
	SourceRuntime   = "runtime"
	SourceDiscovery = "discovery"
)

func defaultWiring(kind DeviceKind, inverted bool) Wiring {
	w := Wiring{}.Normalized()
	if inverted {
		w.Drive = DriveActiveLow
	}
	switch kind {
	case DeviceKindButton, DeviceKindCaptor, DeviceKindPIR:
		w.Mechanism = MechanismMomentary
		if inverted {
			w.Pull = PullUp
		}
	default:
		w.Mechanism = MechanismNone
	}
	return w
}

func mergeDiscovered(existing, discovered DeviceState) DeviceState {
	merged := existing
	merged.UpdatedAt = discovered.UpdatedAt
	if merged.Driver == "" {
		merged.Driver = discovered.Driver
	}
	if merged.Name == "" {
		merged.Name = discovered.Name
	}
	if merged.Label == "" {
		merged.Label = discovered.Label
	}
	if len(merged.Pins) == 0 {
		merged.Pins = discovered.Pins
	}
	if merged.State == nil {
		merged.State = discovered.State
	}
	if merged.Capabilities == (DeviceCapabilities{}) {
		merged.Capabilities = discovered.Capabilities
	}
	return merged
}

func (b *Board) firstPinForDeviceLocked(d DeviceState) *PinState {
	if len(d.Pins) == 0 {
		return nil
	}
	if p, ok := b.st.Pins[d.Pins[0]]; ok {
		return &p
	}
	return nil
}

func (b *Board) devicesForPinLocked(pin string) []DeviceState {
	var out []DeviceState
	for _, d := range b.st.Devices {
		if containsString(d.Pins, pin) {
			out = append(out, d)
		}
	}
	return out
}

func (b *Board) deriveDeviceLocked(d *DeviceState, pin *PinState) {
	d.Wiring = d.Wiring.Normalized()
	level := 0
	if pin != nil {
		level = pin.DigitalValue
	} else if len(d.Pins) == 0 {
		if v, ok := d.State["pinValue"].(int); ok {
			level = v
		}
	}
	d.Energised = d.Wiring.Energised(level)
	d.LoadPowered = d.Wiring.LoadPowered(level)
	if d.HasFault(FaultOpenCoil) || d.HasFault(FaultDisconnected) {
		d.LoadPowered = false
	}
	d.ExpectedIdle = d.Wiring.ExpectedIdleLevel()
	d.Warnings = WiringWarnings(*d, pin)
}

// SetDeviceFault injects a device fault and returns the updated device.
func (b *Board) SetDeviceFault(deviceID string, fault DeviceFault) (DeviceState, error) {
	if !fault.Valid() {
		return DeviceState{}, fmt.Errorf("%w: %q", ErrInvalidFault, fault)
	}
	b.mu.Lock()
	d, ok := b.st.Devices[deviceID]
	if !ok {
		b.mu.Unlock()
		return DeviceState{}, fmt.Errorf("%w: %s", ErrDeviceNotFound, deviceID)
	}
	if !d.HasFault(fault) {
		d.Faults = append(d.Faults, fault)
	}
	d.UpdatedAt = b.clk.Now()
	b.deriveDeviceLocked(&d, b.firstPinForDeviceLocked(d))
	b.st.Devices[deviceID] = d
	b.st.UpdatedAt = d.UpdatedAt
	stored := cloneDevice(d)
	b.mu.Unlock()
	b.publishDevice(stored, SourceSimulation)
	return stored, nil
}

// ClearDeviceFault removes one device fault.
func (b *Board) ClearDeviceFault(deviceID string, fault DeviceFault) (DeviceState, error) {
	b.mu.Lock()
	d, ok := b.st.Devices[deviceID]
	if !ok {
		b.mu.Unlock()
		return DeviceState{}, fmt.Errorf("%w: %s", ErrDeviceNotFound, deviceID)
	}
	out := d.Faults[:0]
	for _, candidate := range d.Faults {
		if candidate != fault {
			out = append(out, candidate)
		}
	}
	d.Faults = out
	d.UpdatedAt = b.clk.Now()
	b.deriveDeviceLocked(&d, b.firstPinForDeviceLocked(d))
	b.st.Devices[deviceID] = d
	b.st.UpdatedAt = d.UpdatedAt
	stored := cloneDevice(d)
	b.mu.Unlock()
	b.publishDevice(stored, SourceSimulation)
	return stored, nil
}

// ClearDeviceFaults removes every device fault.
func (b *Board) ClearDeviceFaults(deviceID string) (DeviceState, error) {
	b.mu.Lock()
	d, ok := b.st.Devices[deviceID]
	if !ok {
		b.mu.Unlock()
		return DeviceState{}, fmt.Errorf("%w: %s", ErrDeviceNotFound, deviceID)
	}
	d.Faults = nil
	d.UpdatedAt = b.clk.Now()
	b.deriveDeviceLocked(&d, b.firstPinForDeviceLocked(d))
	b.st.Devices[deviceID] = d
	b.st.UpdatedAt = d.UpdatedAt
	stored := cloneDevice(d)
	b.mu.Unlock()
	b.publishDevice(stored, SourceSimulation)
	return stored, nil
}

// SetDeviceStateField updates one runtime field of a device's state map.
func (b *Board) SetDeviceStateField(deviceID, field string, value interface{}) (DeviceState, error) {
	b.mu.Lock()
	d, ok := b.st.Devices[deviceID]
	if !ok {
		b.mu.Unlock()
		return DeviceState{}, fmt.Errorf("%w: %s", ErrDeviceNotFound, deviceID)
	}
	if d.State == nil {
		d.State = make(map[string]interface{})
	}
	d.State[field] = value
	d.UpdatedAt = b.clk.Now()
	b.deriveDeviceLocked(&d, b.firstPinForDeviceLocked(d))
	b.st.Devices[deviceID] = d
	b.st.UpdatedAt = d.UpdatedAt
	stored := cloneDevice(d)
	b.mu.Unlock()
	b.publishDevice(stored, SourceSimulation)
	return stored, nil
}

// HasDevice reports whether a device ID is already registered.
func (b *Board) HasDevice(id string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, ok := b.st.Devices[id]
	return ok
}

// Device returns a copy of a device state.
func (b *Board) Device(id string) (DeviceState, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	d, ok := b.st.Devices[id]
	return cloneDevice(d), ok
}

// Devices returns device states sorted by ID.
func (b *Board) Devices() []DeviceState {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]DeviceState, 0, len(b.st.Devices))
	for _, d := range b.st.Devices {
		out = append(out, cloneDevice(d))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// SetValue stores an extra value and updates bound value devices.
func (b *Board) SetValue(name string, val interface{}, source EventSource) (ExtraValue, interface{}) {
	b.mu.Lock()
	old, existed := b.st.Values[name]
	ev := ExtraValue{Name: name, Value: val, UpdatedAt: b.clk.Now()}
	b.st.Values[name] = ev
	b.st.UpdatedAt = ev.UpdatedAt
	devices := b.updateValueDevicesLocked(name, val)
	b.mu.Unlock()

	var oldVal interface{}
	if existed {
		oldVal = old.Value
	}
	b.emit(Event{
		Type:      EventValueUpdated,
		Source:    source,
		BoardID:   b.st.ID,
		ValueName: name,
		OldValue:  oldVal,
		NewValue:  val,
	})
	for _, d := range devices {
		b.publishDevice(d, source)
	}
	return ev, oldVal
}

// Value returns a copy of an extra value.
func (b *Board) Value(name string) (ExtraValue, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	v, ok := b.st.Values[name]
	return v, ok
}

// Values returns a copy of all extra values.
func (b *Board) Values() map[string]ExtraValue {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make(map[string]ExtraValue, len(b.st.Values))
	for k, v := range b.st.Values {
		out[k] = v
	}
	return out
}

// SetFunctionCode configures a function's simulated return code.
func (b *Board) SetFunctionCode(name string, code int, source EventSource) ExtraFunction {
	b.mu.Lock()
	fn, ok := b.st.Functions[name]
	if !ok {
		fn.Name = name
	}
	fn.ReturnCode = code
	b.st.Functions[name] = fn
	b.st.UpdatedAt = b.clk.Now()
	devices := b.updateFunctionDevicesLocked(fn)
	b.mu.Unlock()

	for _, d := range devices {
		b.publishDevice(d, source)
	}
	return fn
}

// Function returns a copy of a function definition.
func (b *Board) Function(name string) (ExtraFunction, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	fn, ok := b.st.Functions[name]
	return fn, ok
}

// CallFunction records a function invocation and returns its configured code.
func (b *Board) CallFunction(name, parameters string, source EventSource) (ExtraFunction, int) {
	b.mu.Lock()
	fn, ok := b.st.Functions[name]
	if !ok {
		fn.Name = name
	}
	now := b.clk.Now()
	fn.LastParameters = parameters
	fn.CallCount++
	fn.LastCalledAt = now
	b.st.Functions[name] = fn
	b.st.UpdatedAt = now
	devices := b.updateFunctionDevicesLocked(fn)
	b.mu.Unlock()

	b.emit(Event{
		Type:      EventFunctionCalled,
		Source:    source,
		BoardID:   b.st.ID,
		ValueName: name,
		NewValue:  fn.ReturnCode,
		OldValue:  parameters,
	})
	for _, d := range devices {
		b.publishDevice(d, source)
	}
	return fn, fn.ReturnCode
}

func (b *Board) updateDevicesLocked(pin PinState) []DeviceState {
	now := b.clk.Now()
	var out []DeviceState
	for id, d := range b.st.Devices {
		if !containsString(d.Pins, pin.Pin) {
			continue
		}
		if d.State == nil {
			d.State = make(map[string]interface{})
		}
		switch d.Kind {
		case DeviceKindRelay, DeviceKindLED, DeviceKindRGBLED:
			d.Active = pin.DigitalValue == 1
			if d.Inverted {
				d.Active = pin.DigitalValue == 0
			}
			d.State["pinValue"] = pin.DigitalValue
			d.State["active"] = d.Active
		case DeviceKindButton, DeviceKindCaptor, DeviceKindPIR:
			d.Active = pin.DigitalValue != d.DefaultState
			d.State["pinValue"] = pin.DigitalValue
			d.State["defaultState"] = d.DefaultState
		case DeviceKindPWM, DeviceKindDirectPin:
			d.State["byteValue"] = int(pin.ByteValue)
			d.State["pinValue"] = pin.DigitalValue
		case DeviceKindServo, DeviceKindMotor, DeviceKindStepper:
			d.State["angle"] = int(pin.ByteValue)
			d.State["pinValue"] = pin.DigitalValue
		case DeviceKindAnalog, DeviceKindSensor, DeviceKindAnalogActuator,
			DeviceKindTemperature, DeviceKindDistance, DeviceKindIMU, DeviceKindADC,
			DeviceKindExpander, DeviceKindDisplay, DeviceKindRFID, DeviceKindBuzzer:
			d.State["analogValue"] = pin.AnalogValue
			d.State["value"] = float64(pin.AnalogValue)
		case DeviceKindValue, DeviceKindFunction:
			continue
		}
		d.UpdatedAt = now
		b.deriveDeviceLocked(&d, &pin)
		b.st.Devices[id] = d
		out = append(out, cloneDevice(d))
	}
	return out
}

func (b *Board) updateValueDevicesLocked(name string, val interface{}) []DeviceState {
	now := b.clk.Now()
	var out []DeviceState
	for id, d := range b.st.Devices {
		if d.Kind != DeviceKindValue || d.ValueName != name {
			continue
		}
		if d.State == nil {
			d.State = make(map[string]interface{})
		}
		d.State["value"] = val
		d.Active = isTruthy(val)
		d.UpdatedAt = now
		b.st.Devices[id] = d
		out = append(out, cloneDevice(d))
	}
	return out
}

func (b *Board) updateFunctionDevicesLocked(fn ExtraFunction) []DeviceState {
	now := b.clk.Now()
	var out []DeviceState
	for id, d := range b.st.Devices {
		if d.Kind != DeviceKindFunction || d.FunctionName != fn.Name {
			continue
		}
		if d.State == nil {
			d.State = make(map[string]interface{})
		}
		d.State["callCount"] = fn.CallCount
		d.State["returnCode"] = fn.ReturnCode
		d.State["lastParameters"] = fn.LastParameters
		d.UpdatedAt = now
		b.st.Devices[id] = d
		out = append(out, cloneDevice(d))
	}
	return out
}

func (b *Board) publishBoard(source EventSource) {
	st := b.Snapshot()
	b.emit(Event{Type: EventBoardUpdated, Source: source, BoardID: st.ID, Board: &st})
}

func (b *Board) publishPin(pin PinState, devices []DeviceState, oldVal, newVal interface{}, source EventSource) {
	p := pin
	b.emit(Event{
		Type:     EventPinUpdated,
		Source:   source,
		BoardID:  b.st.ID,
		Pin:      pin.Pin,
		OldValue: oldVal,
		NewValue: newVal,
		PinState: &p,
	})
	for _, d := range devices {
		b.publishDevice(d, source)
	}
}

func (b *Board) publishDevice(d DeviceState, source EventSource) {
	dev := d
	b.emit(Event{
		Type:     EventDeviceUpdated,
		Source:   source,
		BoardID:  d.BoardID,
		DeviceID: d.ID,
		Device:   &dev,
	})
}

// DefaultCapabilities returns the interaction capabilities for a device kind.
func DefaultCapabilities(kind DeviceKind) DeviceCapabilities {
	switch kind {
	case DeviceKindRelay, DeviceKindLED, DeviceKindRGBLED:
		return DeviceCapabilities{Readable: true, Writable: true, Toggleable: true, Commandable: true}
	case DeviceKindButton, DeviceKindCaptor, DeviceKindPIR:
		return DeviceCapabilities{Readable: true, Writable: true, Pressable: true, Toggleable: true}
	case DeviceKindSensor, DeviceKindAnalog, DeviceKindTemperature, DeviceKindDistance,
		DeviceKindIMU, DeviceKindADC, DeviceKindExpander, DeviceKindRFID:
		return DeviceCapabilities{Readable: true, Writable: true, MinValue: floatPtr(0), MaxValue: floatPtr(1023)}
	case DeviceKindPWM, DeviceKindDirectPin, DeviceKindBuzzer, DeviceKindAnalogActuator:
		return DeviceCapabilities{Readable: true, Writable: true, Commandable: true, MinValue: floatPtr(0), MaxValue: floatPtr(255)}
	case DeviceKindServo:
		return DeviceCapabilities{Readable: true, Writable: true, Commandable: true, MinValue: floatPtr(0), MaxValue: floatPtr(180), Unit: "deg"}
	case DeviceKindMotor:
		return DeviceCapabilities{Readable: true, Writable: true, Commandable: true, MinValue: floatPtr(-255), MaxValue: floatPtr(255)}
	case DeviceKindStepper:
		return DeviceCapabilities{Readable: true, Writable: true, Commandable: true}
	case DeviceKindDisplay:
		return DeviceCapabilities{Readable: true, Writable: true, Commandable: true}
	case DeviceKindValue:
		return DeviceCapabilities{Readable: true, Writable: true}
	case DeviceKindFunction:
		return DeviceCapabilities{Callable: true}
	default:
		return DeviceCapabilities{}
	}
}

func floatPtr(v float64) *float64 { return &v }

func containsString(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func isTruthy(v interface{}) bool {
	switch value := v.(type) {
	case bool:
		return value
	case string:
		return value != ""
	case float64:
		return value != 0
	case float32:
		return value != 0
	case int:
		return value != 0
	case int64:
		return value != 0
	case nil:
		return false
	default:
		return true
	}
}

func cloneBoard(st BoardState) BoardState {
	out := st
	out.Pins = make(map[string]PinState, len(st.Pins))
	for k, v := range st.Pins {
		out.Pins[k] = v
	}
	out.Devices = make(map[string]DeviceState, len(st.Devices))
	for k, v := range st.Devices {
		out.Devices[k] = cloneDevice(v)
	}
	out.Values = make(map[string]ExtraValue, len(st.Values))
	for k, v := range st.Values {
		out.Values[k] = v
	}
	out.Functions = make(map[string]ExtraFunction, len(st.Functions))
	for k, v := range st.Functions {
		out.Functions[k] = v
	}
	return out
}

func cloneDevice(d DeviceState) DeviceState {
	out := d
	if d.Pins != nil {
		out.Pins = append([]string(nil), d.Pins...)
	}
	if d.State != nil {
		out.State = make(map[string]interface{}, len(d.State))
		for k, v := range d.State {
			out.State[k] = v
		}
	}
	if d.Labels != nil {
		out.Labels = make(map[string]string, len(d.Labels))
		for k, v := range d.Labels {
			out.Labels[k] = v
		}
	}
	if d.Warnings != nil {
		out.Warnings = append([]string(nil), d.Warnings...)
	}
	if d.Faults != nil {
		out.Faults = append([]DeviceFault(nil), d.Faults...)
	}
	return out
}

// Store manages all simulated boards and the global event sequence.
type Store struct {
	mu        sync.RWMutex
	boards    map[string]*Board
	aliases   map[string]string
	seq       atomic.Uint64
	emitMu    sync.Mutex
	ring      *RingBuffer
	hub       *Hub
	clk       clock.Clock
	startedAt time.Time
	version   string
}

// NewStore creates a store with the given event history size.
func NewStore(version string, eventBuffer int, clk clock.Clock) *Store {
	s := &Store{
		boards:    make(map[string]*Board),
		aliases:   make(map[string]string),
		ring:      NewRingBuffer(eventBuffer),
		hub:       NewHub(),
		clk:       clk,
		startedAt: clk.Now(),
		version:   version,
	}
	return s
}

// RegisterBoard creates or returns a board by ID.
func (s *Store) RegisterBoard(id, name, platform, port string, latency time.Duration) (*Board, error) {
	if err := ValidateIdentifier("board", id); err != nil {
		return nil, err
	}

	s.mu.Lock()
	if existing, ok := s.boards[id]; ok {
		s.mu.Unlock()
		_ = existing.SetIdentity(name, platform, port)
		return existing, nil
	}
	b := newBoard(id, name, platform, port, latency, s.clk, s.emit, s.addAlias)
	s.boards[id] = b
	s.addAliasLocked(id, id)
	if ValidateID(name) == nil {
		s.addAliasLocked(name, id)
	}
	s.mu.Unlock()

	b.publishBoard(SourceSimulation)
	return b, nil
}

// BoardByID returns the board handle or ErrBoardNotFound.
func (s *Store) BoardByID(id string) (*Board, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if b, ok := s.boards[id]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrBoardNotFound, id)
}

// Resolve returns a board by ID, name, or case-insensitive alias.
func (s *Store) Resolve(idOrName string) (*Board, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if b, ok := s.boards[idOrName]; ok {
		return b, nil
	}
	if id, ok := s.aliases[idOrName]; ok {
		if b, ok := s.boards[id]; ok {
			return b, nil
		}
	}
	lower := strings.ToLower(idOrName)
	if id, ok := s.aliases[lower]; ok {
		if b, ok := s.boards[id]; ok {
			return b, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrBoardNotFound, idOrName)
}

func (s *Store) addAlias(id, name string) {
	s.mu.Lock()
	s.addAliasLocked(name, id)
	s.mu.Unlock()
}

func (s *Store) addAliasLocked(alias, id string) {
	if alias == "" {
		return
	}
	if existing, ok := s.aliases[alias]; ok && existing != id {
		return
	}
	s.aliases[alias] = id
	s.aliases[strings.ToLower(alias)] = id
}

func (s *Store) emit(evt Event) {
	s.emitMu.Lock()
	defer s.emitMu.Unlock()

	evt.Seq = s.seq.Add(1)
	evt.Timestamp = s.clk.Now()
	s.ring.Append(evt)
	s.hub.Publish(evt)
}

// Snapshot returns a copy of all boards and metadata.
func (s *Store) Snapshot() Snapshot {
	s.mu.RLock()
	boards := make([]*Board, 0, len(s.boards))
	for _, b := range s.boards {
		boards = append(boards, b)
	}
	s.mu.RUnlock()

	states := make([]BoardState, 0, len(boards))
	for _, b := range boards {
		states = append(states, b.Snapshot())
	}
	sort.Slice(states, func(i, j int) bool { return states[i].ID < states[j].ID })

	now := s.clk.Now()
	return Snapshot{
		Meta: ServerMetadata{
			Version:       s.version,
			StartedAt:     s.startedAt,
			UptimeSeconds: int64(now.Sub(s.startedAt).Seconds()),
			CurrentSeq:    s.seq.Load(),
			DroppedEvents: s.hub.Dropped(),
		},
		Boards: states,
	}
}

// Boards returns board states sorted by ID.
func (s *Store) Boards() []BoardState { return s.Snapshot().Boards }

// BoardState returns one board state.
func (s *Store) BoardState(id string) (BoardState, error) {
	b, err := s.Resolve(id)
	if err != nil {
		return BoardState{}, err
	}
	return b.Snapshot(), nil
}

// Devices returns devices for a board.
func (s *Store) Devices(boardID string) ([]DeviceState, error) {
	b, err := s.Resolve(boardID)
	if err != nil {
		return nil, err
	}
	return b.Devices(), nil
}

// Device returns one device.
func (s *Store) Device(boardID, deviceID string) (DeviceState, error) {
	b, err := s.Resolve(boardID)
	if err != nil {
		return DeviceState{}, err
	}
	d, ok := b.Device(deviceID)
	if !ok {
		return DeviceState{}, fmt.Errorf("%w: %s", ErrDeviceNotFound, deviceID)
	}
	return d, nil
}

// Pin returns one pin.
func (s *Store) Pin(boardID, pin string) (PinState, error) {
	b, err := s.Resolve(boardID)
	if err != nil {
		return PinState{}, err
	}
	p, ok := b.Pin(pin)
	if !ok {
		return PinState{}, fmt.Errorf("%w: %s", ErrPinNotFound, pin)
	}
	return p, nil
}

// Values returns extra values for a board.
func (s *Store) Values(boardID string) (map[string]ExtraValue, error) {
	b, err := s.Resolve(boardID)
	if err != nil {
		return nil, err
	}
	return b.Values(), nil
}

// SetDigital sets a pin digital value preserving its mode. Values must be in
// 0..255; 0 and 1 keep their classic digital meaning, while larger byte values
// are accepted for PWM-like writes.
func (s *Store) SetDigital(boardID, pin string, val int, source EventSource) error {
	if val < 0 || val > 255 {
		return fmt.Errorf("%w: digital value %d must be between 0 and 255", ErrInvalidValue, val)
	}
	b, err := s.Resolve(boardID)
	if err != nil {
		return err
	}
	b.SetDigitalValue(pin, val, source)
	return nil
}

// PressButton presses a button or captor.
func (s *Store) PressButton(boardID, deviceID string) error {
	b, err := s.Resolve(boardID)
	if err != nil {
		return err
	}
	d, ok := b.Device(deviceID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrDeviceNotFound, deviceID)
	}
	if d.Kind != DeviceKindButton && d.Kind != DeviceKindCaptor {
		return fmt.Errorf("%w: device %s is not pressable", ErrReadOnly, deviceID)
	}
	if len(d.Pins) == 0 {
		return fmt.Errorf("%w: device %s has no pin", ErrInvalidValue, deviceID)
	}
	b.SetDigitalValue(d.Pins[0], 1-d.DefaultState, SourceUI)
	return nil
}

// ReleaseButton releases a button or captor.
func (s *Store) ReleaseButton(boardID, deviceID string) error {
	b, err := s.Resolve(boardID)
	if err != nil {
		return err
	}
	d, ok := b.Device(deviceID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrDeviceNotFound, deviceID)
	}
	if d.Kind != DeviceKindButton && d.Kind != DeviceKindCaptor {
		return fmt.Errorf("%w: device %s is not pressable", ErrReadOnly, deviceID)
	}
	if len(d.Pins) == 0 {
		return fmt.Errorf("%w: device %s has no pin", ErrInvalidValue, deviceID)
	}
	b.SetDigitalValue(d.Pins[0], d.DefaultState, SourceUI)
	return nil
}

// ToggleDevice toggles a relay, LED, button, or captor.
func (s *Store) ToggleDevice(boardID, deviceID string) error {
	b, err := s.Resolve(boardID)
	if err != nil {
		return err
	}
	d, ok := b.Device(deviceID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrDeviceNotFound, deviceID)
	}
	switch d.Kind {
	case DeviceKindRelay, DeviceKindLED:
		if len(d.Pins) == 0 {
			return fmt.Errorf("%w: device %s has no pin", ErrInvalidValue, deviceID)
		}
		val := 1
		if d.Inverted {
			val = 0
		}
		if d.Active {
			val = 1 - val
		}
		b.SetDigitalValue(d.Pins[0], val, SourceUI)
	case DeviceKindButton, DeviceKindCaptor:
		if len(d.Pins) == 0 {
			return fmt.Errorf("%w: device %s has no pin", ErrInvalidValue, deviceID)
		}
		if d.Active {
			return s.ReleaseButton(boardID, deviceID)
		}
		return s.PressButton(boardID, deviceID)
	default:
		return fmt.Errorf("%w: device %s is not toggleable", ErrReadOnly, deviceID)
	}
	return nil
}

// SetPWM writes a PWM value to a pin.
func (s *Store) SetPWM(boardID, pin string, val byte, source EventSource) error {
	b, err := s.Resolve(boardID)
	if err != nil {
		return err
	}
	b.PwmWritePin(pin, val, source)
	return nil
}

// SetServo writes a servo angle to a pin.
func (s *Store) SetServo(boardID, pin string, angle byte, source EventSource) error {
	if angle > 180 {
		return fmt.Errorf("%w: servo angle %d must be between 0 and 180", ErrInvalidValue, angle)
	}
	b, err := s.Resolve(boardID)
	if err != nil {
		return err
	}
	b.ServoWritePin(pin, angle, source)
	return nil
}

// SetAnalog writes an analog value to a pin.
func (s *Store) SetAnalog(boardID, pin string, val int, source EventSource) error {
	b, err := s.Resolve(boardID)
	if err != nil {
		return err
	}
	b.AnalogWritePin(pin, val, source)
	return nil
}

// SetValue stores an extra value.
func (s *Store) SetValue(boardID, name string, val interface{}, source EventSource) error {
	b, err := s.Resolve(boardID)
	if err != nil {
		return err
	}
	b.SetValue(name, val, source)
	return nil
}

// CallFunction invokes an extra function.
func (s *Store) CallFunction(boardID, name, params string, source EventSource) (int, error) {
	b, err := s.Resolve(boardID)
	if err != nil {
		return 0, err
	}
	_, result := b.CallFunction(name, params, source)
	return result, nil
}

// SetFunctionCode configures a function's mocked return code.
func (s *Store) SetFunctionCode(boardID, name string, code int, source EventSource) error {
	b, err := s.Resolve(boardID)
	if err != nil {
		return err
	}
	b.SetFunctionCode(name, code, source)
	return nil
}

// SetFault injects a board fault.
func (s *Store) SetFault(boardID string, faultErr error) error {
	b, err := s.Resolve(boardID)
	if err != nil {
		return err
	}
	b.SetFault(faultErr)
	return nil
}

// ClearFault clears a board fault.
func (s *Store) ClearFault(boardID string) error { return s.SetFault(boardID, nil) }

// Subscribe returns a new event channel.
func (s *Store) Subscribe() <-chan Event { return s.hub.Subscribe() }

// SubscribeWithReplay subscribes and returns buffered events after lastSeq.
func (s *Store) SubscribeWithReplay(lastSeq uint64) (<-chan Event, []Event, bool) {
	ch := s.hub.Subscribe()
	if lastSeq == 0 {
		return ch, nil, false
	}
	missed, ok := s.ring.Since(lastSeq)
	return ch, missed, ok
}

// Unsubscribe removes an event channel.
func (s *Store) Unsubscribe(ch <-chan Event) { s.hub.Unsubscribe(ch) }

// DroppedEvents returns the number of dropped subscriber events.
func (s *Store) DroppedEvents() uint64 { return s.hub.Dropped() }

// CurrentSeq returns the latest event sequence number.
func (s *Store) CurrentSeq() uint64 { return s.seq.Load() }
