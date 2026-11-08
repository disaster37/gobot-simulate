package simulate

import (
	"fmt"
	"sync"
	"time"

	"github.com/disaster37/gobot-simulate/internal/bus"
	"github.com/disaster37/gobot-simulate/internal/state"
	"github.com/disaster37/gobot-simulate/internal/trace"
	"gobot.io/x/gobot/v2"
	"gobot.io/x/gobot/v2/drivers/aio"
	"gobot.io/x/gobot/v2/drivers/gpio"
)

// ExtraReader matches github.com/disaster37/gobot-arest/v2/drivers/extra.ExtraReader
// structurally without importing gobot-arest.
type ExtraReader interface {
	ValueRead(name string) (val interface{}, err error)
	ValuesRead() (vals map[string]interface{}, err error)
	FunctionCall(name string, parameters string) (val int, err error)
}

// Reconnecter matches the Reconnect() error method on TankAdaptor and TFPAdaptor.
type Reconnecter interface {
	Reconnect() error
}

var (
	_ gobot.Adaptor      = (*Adaptor)(nil)
	_ gobot.Connection   = (*Adaptor)(nil)
	_ gobot.Porter       = (*Adaptor)(nil)
	_ gpio.DigitalReader = (*Adaptor)(nil)
	_ gpio.DigitalWriter = (*Adaptor)(nil)
	_ gpio.PwmWriter     = (*Adaptor)(nil)
	_ gpio.ServoWriter   = (*Adaptor)(nil)
	_ aio.AnalogReader   = (*Adaptor)(nil)
	_ aio.AnalogWriter   = (*Adaptor)(nil)
	_ ExtraReader        = (*Adaptor)(nil)
	_ Reconnecter        = (*Adaptor)(nil)
)

// MockPlateform is a drop-in alias for Adaptor to preserve source compatibility
// with github.com/disaster37/gobot-fat/mock.MockPlateform.
type MockPlateform = Adaptor

// AdaptorConfig holds adaptor construction values.
type AdaptorConfig struct {
	ID          string
	Name        string
	Port        string
	Platform    string
	InputPullup []string
	Inverted    []string
	Values      map[string]interface{}
	Functions   map[string]int
	Latency     time.Duration
}

// Adaptor is a thread-safe simulated Gobot adaptor backed by a state.Board.
type Adaptor struct {
	sim   *Simulator
	board *state.Board
	buses *bus.Manager
	id    string
	mu    sync.RWMutex
	name  string
	port  string
	pinMu sync.Mutex
	pins  map[string]*simDigitalPin
	pwm   map[string]*simPWMPin
}

// WithAdaptorID sets the adaptor/board identifier.
func WithAdaptorID(id string) AdaptorOption { return func(c *AdaptorConfig) { c.ID = id } }

// WithAdaptorName sets the adaptor display name.
func WithAdaptorName(name string) AdaptorOption { return func(c *AdaptorConfig) { c.Name = name } }

// WithAdaptorPort sets the adaptor port string.
func WithAdaptorPort(port string) AdaptorOption { return func(c *AdaptorConfig) { c.Port = port } }

// WithPlatform sets the board platform label.
func WithPlatform(platform string) AdaptorOption {
	return func(c *AdaptorConfig) { c.Platform = platform }
}

// WithInputPullup enables pull-up semantics for the given pins.
func WithInputPullup(pins ...string) AdaptorOption {
	return func(c *AdaptorConfig) { c.InputPullup = append(c.InputPullup, pins...) }
}

// WithInvertedInitialPins marks pins with an inverted initial state.
func WithInvertedInitialPins(pins ...string) AdaptorOption {
	return func(c *AdaptorConfig) { c.Inverted = append(c.Inverted, pins...) }
}

// WithInitialValue seeds an aREST/extra value.
func WithInitialValue(name string, val interface{}) AdaptorOption {
	return func(c *AdaptorConfig) {
		if c.Values == nil {
			c.Values = make(map[string]interface{})
		}
		c.Values[name] = val
	}
}

// WithInitialFunctionCode seeds a function return code.
func WithInitialFunctionCode(name string, code int) AdaptorOption {
	return func(c *AdaptorConfig) {
		if c.Functions == nil {
			c.Functions = make(map[string]int)
		}
		c.Functions[name] = code
	}
}

// WithSimulatedLatency sets per-board simulated I/O latency.
func WithSimulatedLatency(d time.Duration) AdaptorOption {
	return func(c *AdaptorConfig) { c.Latency = d }
}

func applyAdaptorConfig(a *Adaptor, cfg AdaptorConfig) {
	if cfg.Name != "" {
		a.mu.Lock()
		a.name = cfg.Name
		a.port = cfg.Port
		a.mu.Unlock()
		_ = a.board.SetIdentity(cfg.Name, cfg.Platform, cfg.Port)
	}
	if cfg.Latency != 0 {
		a.board.SetLatency(cfg.Latency)
	}
	for _, pin := range dedupeStrings(cfg.Inverted) {
		a.board.SetInvertInitialPinState(pin)
	}
	for _, pin := range dedupeStrings(cfg.InputPullup) {
		a.board.SetInputPullup(pin)
	}
	for name, value := range cfg.Values {
		if state.ValidateID(name) == nil {
			a.board.SetValue(name, value, state.SourceSimulation)
		}
	}
	for name, code := range cfg.Functions {
		if state.ValidateID(name) == nil {
			a.board.SetFunctionCode(name, code, state.SourceSimulation)
		}
	}
}

// BoardID returns the underlying board identifier.
func (a *Adaptor) BoardID() string { return a.id }

// Name returns the adaptor name.
func (a *Adaptor) Name() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.name
}

// SetName updates the adaptor name and board display name.
func (a *Adaptor) SetName(name string) {
	if name == "" {
		return
	}
	a.mu.Lock()
	a.name = name
	a.mu.Unlock()
	_ = a.board.SetIdentity(name, "", "")
}

// Port returns the configured port string.
func (a *Adaptor) Port() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.port
}

// Connect marks the board connected.
func (a *Adaptor) Connect() error {
	if err := a.board.FaultError(); err != nil {
		return err
	}
	a.board.SetConnected(true)
	return nil
}

// Finalize marks the board disconnected.
func (a *Adaptor) Finalize() error {
	if err := a.board.FaultError(); err != nil {
		return err
	}
	a.board.SetConnected(false)
	return nil
}

// Reconnect invokes a test hook when present, otherwise the fault or reconnect counter.
func (a *Adaptor) Reconnect() error {
	if hook := a.board.Hooks().Reconnect; hook != nil {
		return hook()
	}
	if err := a.board.FaultError(); err != nil {
		return err
	}
	a.board.IncrementReconnect()
	return nil
}

// DigitalRead reads a pin, creating it as an input on first use.
func (a *Adaptor) DigitalRead(pin string) (int, error) {
	a.applyLatency()
	if hook := a.board.Hooks().DigitalRead; hook != nil {
		return hook(pin)
	}
	if err := a.board.FaultError(); err != nil {
		return 0, err
	}
	started := a.sim.clk.Now()
	value := a.board.DigitalReadPin(pin).DigitalValue
	a.recordTrace(trace.KindPinRead, pin, "read", value, state.SourceHardwareCode, started, nil)
	return value, nil
}

// DigitalWrite writes a digital value and auto-creates unknown pins.
func (a *Adaptor) DigitalWrite(pin string, val byte) error {
	a.applyLatency()
	if hook := a.board.Hooks().DigitalWrite; hook != nil {
		return hook(pin, val)
	}
	if err := a.board.FaultError(); err != nil {
		return err
	}
	started := a.sim.clk.Now()
	a.board.DigitalWritePin(pin, val, true, state.PinModeOutput, state.SourceHardwareCode)
	a.recordTrace(trace.KindPinWrite, pin, "write", int(val), state.SourceHardwareCode, started, nil)
	a.afterPinWrite(pin, int(val))
	return nil
}

// PwmWrite writes a PWM value.
func (a *Adaptor) PwmWrite(pin string, val byte) error {
	a.applyLatency()
	if hook := a.board.Hooks().PwmWrite; hook != nil {
		return hook(pin, val)
	}
	if err := a.board.FaultError(); err != nil {
		return err
	}
	started := a.sim.clk.Now()
	a.board.PwmWritePin(pin, val, state.SourceHardwareCode)
	a.recordTrace(trace.KindPWM, pin, "pwm", int(val), state.SourceHardwareCode, started, nil)
	return nil
}

// ServoWrite writes a servo angle.
func (a *Adaptor) ServoWrite(pin string, val byte) error {
	a.applyLatency()
	if hook := a.board.Hooks().ServoWrite; hook != nil {
		return hook(pin, val)
	}
	if err := a.board.FaultError(); err != nil {
		return err
	}
	started := a.sim.clk.Now()
	a.board.ServoWritePin(pin, val, state.SourceHardwareCode)
	a.recordTrace(trace.KindServo, pin, "servo", int(val), state.SourceHardwareCode, started, nil)
	return nil
}

// AnalogRead reads an analog pin value.
func (a *Adaptor) AnalogRead(pin string) (int, error) {
	a.applyLatency()
	if hook := a.board.Hooks().AnalogRead; hook != nil {
		return hook(pin)
	}
	if err := a.board.FaultError(); err != nil {
		return 0, err
	}
	started := a.sim.clk.Now()
	value := a.board.AnalogReadPin(pin).AnalogValue
	a.recordTrace(trace.KindAnalog, pin, "read", value, state.SourceHardwareCode, started, nil)
	return value, nil
}

// AnalogWrite writes an analog pin value.
func (a *Adaptor) AnalogWrite(pin string, val int) error {
	a.applyLatency()
	if err := a.board.FaultError(); err != nil {
		return err
	}
	started := a.sim.clk.Now()
	a.board.AnalogWritePin(pin, val, state.SourceHardwareCode)
	a.recordTrace(trace.KindAnalog, pin, "write", val, state.SourceHardwareCode, started, nil)
	return nil
}

// ValueRead returns one extra value (nil, nil when unknown).
func (a *Adaptor) ValueRead(name string) (interface{}, error) {
	if hook := a.board.Hooks().ValueRead; hook != nil {
		return hook(name)
	}
	if err := a.board.FaultError(); err != nil {
		return nil, err
	}
	started := a.sim.clk.Now()
	value, ok := a.board.Value(name)
	if !ok {
		a.recordTrace(trace.KindValueRead, name, "read", nil, state.SourceHardwareCode, started, nil)
		return nil, nil
	}
	a.recordTrace(trace.KindValueRead, name, "read", value.Value, state.SourceHardwareCode, started, nil)
	return value.Value, nil
}

// ValuesRead returns extra values seeded with {"test": 99} for MockPlateform
// compatibility. The aREST HTTP surface intentionally does not expose it.
func (a *Adaptor) ValuesRead() (map[string]interface{}, error) {
	if hook := a.board.Hooks().ValuesRead; hook != nil {
		return hook()
	}
	if err := a.board.FaultError(); err != nil {
		return nil, err
	}
	started := a.sim.clk.Now()
	out := map[string]interface{}{"test": 99}
	for name, value := range a.board.Values() {
		out[name] = value.Value
	}
	a.recordTrace(trace.KindValueRead, "*", "values", out, state.SourceHardwareCode, started, nil)
	return out, nil
}

// FunctionCall records and executes an extra function.
func (a *Adaptor) FunctionCall(name, parameters string) (int, error) {
	if hook := a.board.Hooks().FunctionCall; hook != nil {
		return hook(name, parameters)
	}
	if err := a.board.FaultError(); err != nil {
		return 0, err
	}
	started := a.sim.clk.Now()
	_, result := a.board.CallFunction(name, parameters, state.SourceHardwareCode)
	a.recordTrace(trace.KindFunctionCall, name, parameters, result, state.SourceHardwareCode, started, nil)
	return result, nil
}

// SetInvertInitialPinState marks a pin as inverted before pull-up setup.
func (a *Adaptor) SetInvertInitialPinState(pin string) {
	a.board.SetInvertInitialPinState(pin)
}

// SetInputPullup applies pull-up semantics to the given pins.
func (a *Adaptor) SetInputPullup(listPins ...string) {
	a.board.SetInputPullup(listPins...)
}

// GetDigitalPinState returns a pin's digital value (0 when unknown).
func (a *Adaptor) GetDigitalPinState(pin string) int {
	p, ok := a.board.Pin(pin)
	if !ok {
		return 0
	}
	return p.DigitalValue
}

// SetDigitalPinState sets a pin digital value and emits update events.
// It preserves the legacy no-error signature; values outside 0..255 are
// ignored. Use SetDigitalPinStateErr to observe ErrInvalidValue.
func (a *Adaptor) SetDigitalPinState(pin string, value int) {
	_ = a.SetDigitalPinStateErr(pin, value)
}

// SetDigitalPinStateErr validates and sets a pin digital value. Values must be
// in 0..255: 0 and 1 keep their classic digital meaning, while larger byte
// values are accepted for PWM-like writes.
func (a *Adaptor) SetDigitalPinStateErr(pin string, value int) error {
	if value < 0 || value > 255 {
		return fmt.Errorf("%w: digital value %d must be between 0 and 255", state.ErrInvalidValue, value)
	}
	a.board.SetDigitalValue(pin, value, state.SourceSimulation)
	return nil
}

// GetValueReadState returns a stored extra value.
func (a *Adaptor) GetValueReadState(name string) interface{} {
	value, ok := a.board.Value(name)
	if !ok {
		return nil
	}
	return value.Value
}

// SetValueReadState stores an extra value.
func (a *Adaptor) SetValueReadState(name string, value interface{}) {
	if state.ValidateID(name) != nil {
		return
	}
	started := a.sim.clk.Now()
	a.board.SetValue(name, value, state.SourceSimulation)
	a.recordTrace(trace.KindValueWrite, name, "write", value, state.SourceSimulation, started, nil)
}

// GetCallFunctionState returns the configured function return code.
func (a *Adaptor) GetCallFunctionState(name string) int {
	fn, ok := a.board.Function(name)
	if !ok {
		return 0
	}
	return fn.ReturnCode
}

// SetCallFunctionState sets the configured function return code.
func (a *Adaptor) SetCallFunctionState(name string, value int) {
	if state.ValidateID(name) != nil {
		return
	}
	a.board.SetFunctionCode(name, value, state.SourceSimulation)
}

// SetError injects or clears a board-wide fault.
func (a *Adaptor) SetError(err error) {
	started := a.sim.clk.Now()
	a.board.SetFault(err)
	a.recordTrace(trace.KindFault, "board", "fault", nil, state.SourceSimulation, started, err)
}

// TestReconnect overrides Reconnect.
func (a *Adaptor) TestReconnect(f func() error) {
	a.board.UpdateHooks(func(h *state.Hooks) { h.Reconnect = f })
}

// TestAdaptorDigitalWrite overrides DigitalWrite.
func (a *Adaptor) TestAdaptorDigitalWrite(f func(pin string, val byte) error) {
	a.board.UpdateHooks(func(h *state.Hooks) { h.DigitalWrite = f })
}

// TestAdaptorServoWrite overrides ServoWrite.
func (a *Adaptor) TestAdaptorServoWrite(f func(pin string, val byte) error) {
	a.board.UpdateHooks(func(h *state.Hooks) { h.ServoWrite = f })
}

// TestAdaptorPwmWrite overrides PwmWrite.
func (a *Adaptor) TestAdaptorPwmWrite(f func(pin string, val byte) error) {
	a.board.UpdateHooks(func(h *state.Hooks) { h.PwmWrite = f })
}

// TestAdaptorAnalogRead overrides AnalogRead.
func (a *Adaptor) TestAdaptorAnalogRead(f func(pin string) (int, error)) {
	a.board.UpdateHooks(func(h *state.Hooks) { h.AnalogRead = f })
}

// TestAdaptorDigitalRead overrides DigitalRead.
func (a *Adaptor) TestAdaptorDigitalRead(f func(pin string) (int, error)) {
	a.board.UpdateHooks(func(h *state.Hooks) { h.DigitalRead = f })
}

// TestAdaptorReadValue is a legacy alias for TestAdaptorDigitalRead.
func (a *Adaptor) TestAdaptorReadValue(f func(pin string) (int, error)) {
	a.board.UpdateHooks(func(h *state.Hooks) { h.DigitalRead = f })
}

// TestAdaptorValueRead overrides ValueRead.
func (a *Adaptor) TestAdaptorValueRead(f func(name string) (interface{}, error)) {
	a.board.UpdateHooks(func(h *state.Hooks) { h.ValueRead = f })
}

// TestAdaptorValuesRead overrides ValuesRead.
func (a *Adaptor) TestAdaptorValuesRead(f func() (map[string]interface{}, error)) {
	a.board.UpdateHooks(func(h *state.Hooks) { h.ValuesRead = f })
}

// TestAdaptorFunctionCall overrides FunctionCall.
func (a *Adaptor) TestAdaptorFunctionCall(f func(name, parameters string) (int, error)) {
	a.board.UpdateHooks(func(h *state.Hooks) { h.FunctionCall = f })
}

func (a *Adaptor) applyLatency() {
	if d := a.board.Latency(); d > 0 {
		time.Sleep(d)
	}
}
