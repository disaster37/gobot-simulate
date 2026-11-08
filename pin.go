package simulate

import (
	"sync"
	"time"

	"github.com/disaster37/gobot-simulate/internal/state"
	"github.com/disaster37/gobot-simulate/internal/trace"
	"gobot.io/x/gobot/v2"
)

const (
	pinEventNone    = 0
	pinEventFalling = 1
	pinEventRising  = 2
	pinEventBoth    = 3

	pinBiasDefault  = 0
	pinBiasDisable  = 1
	pinBiasPullDown = 2
	pinBiasPullUp   = 3
)

type edgeHandler = func(lineOffset int, timestamp time.Duration, detectedEdge string, seqno uint32, lseqno uint32)

type digitalPinConfig struct {
	label        string
	output       bool
	activeLow    bool
	bias         int
	drive        int
	debounce     time.Duration
	edge         int
	handler      edgeHandler
	pollInterval time.Duration
	pollQuit     chan struct{}
}

type simDigitalPin struct {
	adaptor *Adaptor
	id      string

	mu        sync.Mutex
	cfg       digitalPinConfig
	exported  bool
	lastValue int
	seqno     uint32
	polling   bool
	startedAt time.Time
}

// DigitalPin implements gobot.DigitalPinnerProvider.
func (a *Adaptor) DigitalPin(id string) (gobot.DigitalPinner, error) {
	if err := state.ValidateIdentifier("pin", id); err != nil {
		return nil, err
	}
	a.pinMu.Lock()
	defer a.pinMu.Unlock()
	if pin, ok := a.pins[id]; ok {
		return pin, nil
	}
	pinState := a.board.EnsurePin(id, state.PinModeInput)
	pin := &simDigitalPin{
		adaptor:   a,
		id:        id,
		lastValue: pinState.DigitalValue,
		startedAt: time.Now(),
		cfg:       digitalPinConfig{},
	}
	a.pins[id] = pin
	return pin, nil
}

// Export implements gobot.DigitalPinner.
func (p *simDigitalPin) Export() error {
	p.mu.Lock()
	p.exported = true
	p.mu.Unlock()
	return nil
}

// Unexport implements gobot.DigitalPinner.
func (p *simDigitalPin) Unexport() error {
	p.mu.Lock()
	p.exported = false
	p.mu.Unlock()
	return nil
}

// Read implements gobot.DigitalPinner.
func (p *simDigitalPin) Read() (int, error) {
	value := p.adaptor.board.DigitalReadPin(p.id).DigitalValue
	p.mu.Lock()
	activeLow := p.cfg.activeLow
	p.mu.Unlock()
	if activeLow {
		value = 1 - value
	}
	return value, nil
}

// Write implements gobot.DigitalPinner.
func (p *simDigitalPin) Write(val int) error {
	p.mu.Lock()
	activeLow := p.cfg.activeLow
	output := p.cfg.output
	p.mu.Unlock()
	level := val
	if activeLow {
		level = 1 - level
	}
	if !output {
		p.mu.Lock()
		p.cfg.output = true
		p.mu.Unlock()
	}
	started := p.adaptor.sim.clk.Now()
	p.adaptor.board.DigitalWritePin(p.id, byte(level), true, state.PinModeOutput, state.SourceHardwareCode) //nolint:gosec // intentional width conversion
	p.adaptor.recordTrace(trace.KindPinWrite, p.id, "write", level, state.SourceHardwareCode, started, nil)
	p.fireEdge()
	p.adaptor.afterPinWrite(p.id, level)
	return nil
}

// ApplyOptions implements gobot.DigitalPinOptionApplier.
func (p *simDigitalPin) ApplyOptions(options ...func(gobot.DigitalPinOptioner) bool) error {
	for _, option := range options {
		if option == nil {
			continue
		}
		option(p)
	}
	return nil
}

// DirectionBehavior implements gobot.DigitalPinValuer.
func (p *simDigitalPin) DirectionBehavior() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cfg.output {
		return "out"
	}
	return "in"
}

// SetLabel implements gobot.DigitalPinOptioner.
func (p *simDigitalPin) SetLabel(label string) bool {
	p.adaptor.board.LabelPin(p.id, label)
	p.mu.Lock()
	changed := p.cfg.label != label
	p.cfg.label = label
	p.mu.Unlock()
	return changed
}

// SetDirectionOutput implements gobot.DigitalPinOptioner.
func (p *simDigitalPin) SetDirectionOutput(initialState int) bool {
	p.mu.Lock()
	changed := !p.cfg.output
	p.cfg.output = true
	p.mu.Unlock()
	level := initialState
	p.mu.Lock()
	if p.cfg.activeLow {
		level = 1 - level
	}
	p.mu.Unlock()
	p.adaptor.board.DigitalWritePin(p.id, byte(level), true, state.PinModeOutput, state.SourceSimulation) //nolint:gosec // intentional width conversion
	p.fireEdge()
	return changed
}

// SetDirectionInput implements gobot.DigitalPinOptioner.
func (p *simDigitalPin) SetDirectionInput() bool {
	p.mu.Lock()
	changed := p.cfg.output
	p.cfg.output = false
	p.mu.Unlock()
	_ = p.adaptor.board.BindPin(p.id, "", state.PinModeInput)
	return changed
}

// SetActiveLow implements gobot.DigitalPinOptioner.
func (p *simDigitalPin) SetActiveLow() bool {
	p.mu.Lock()
	changed := !p.cfg.activeLow
	p.cfg.activeLow = true
	p.mu.Unlock()
	return changed
}

// SetBias implements gobot.DigitalPinOptioner.
func (p *simDigitalPin) SetBias(bias int) bool {
	p.mu.Lock()
	changed := p.cfg.bias != bias
	p.cfg.bias = bias
	p.mu.Unlock()
	switch bias {
	case pinBiasPullUp:
		_ = p.adaptor.board.BindPin(p.id, "", state.PinModeInputPullup)
	case pinBiasPullDown:
		_ = p.adaptor.board.BindPin(p.id, "", state.PinModeInput)
	}
	return changed
}

// SetDrive implements gobot.DigitalPinOptioner.
func (p *simDigitalPin) SetDrive(drive int) bool {
	p.mu.Lock()
	changed := p.cfg.drive != drive
	p.cfg.drive = drive
	p.mu.Unlock()
	return changed
}

// SetDebounce implements gobot.DigitalPinOptioner.
func (p *simDigitalPin) SetDebounce(period time.Duration) bool {
	p.mu.Lock()
	changed := p.cfg.debounce != period
	p.cfg.debounce = period
	p.mu.Unlock()
	return changed
}

// SetEventHandlerForEdge implements gobot.DigitalPinOptioner.
func (p *simDigitalPin) SetEventHandlerForEdge(handler edgeHandler, edge int) bool {
	p.mu.Lock()
	changed := p.cfg.edge != edge
	p.cfg.edge = edge
	p.cfg.handler = handler
	needsPoll := p.cfg.pollInterval > 0 && !p.polling
	p.polling = needsPoll
	interval := p.cfg.pollInterval
	quit := p.cfg.pollQuit
	p.mu.Unlock()
	if needsPoll {
		p.startPolling(interval, quit)
	}
	return changed
}

// SetPollForEdgeDetection implements gobot.DigitalPinOptioner.
func (p *simDigitalPin) SetPollForEdgeDetection(pollInterval time.Duration, pollQuitChan chan struct{}) bool {
	p.mu.Lock()
	changed := p.cfg.pollInterval != pollInterval
	p.cfg.pollInterval = pollInterval
	p.cfg.pollQuit = pollQuitChan
	handler := p.cfg.handler
	needsPoll := pollInterval > 0 && handler != nil && !p.polling
	p.polling = needsPoll
	p.mu.Unlock()
	if pollInterval <= 0 {
		p.mu.Lock()
		p.polling = false
		p.mu.Unlock()
		return changed
	}
	if needsPoll {
		p.startPolling(pollInterval, pollQuitChan)
	}
	return changed
}

func (p *simDigitalPin) startPolling(interval time.Duration, quit chan struct{}) {
	if interval < 50*time.Microsecond {
		interval = 50 * time.Microsecond
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-quit:
				return
			case <-ticker.C:
				p.fireEdge()
			}
		}
	}()
}

func (p *simDigitalPin) fireEdge() {
	p.mu.Lock()
	handler := p.cfg.handler
	edge := p.cfg.edge
	last := p.lastValue
	p.mu.Unlock()
	if handler == nil || edge == pinEventNone {
		return
	}
	current := p.adaptor.board.DigitalReadPin(p.id).DigitalValue
	if current == last {
		return
	}
	p.mu.Lock()
	p.lastValue = current
	p.seqno++
	seq := p.seqno
	p.mu.Unlock()
	switch {
	case current == 1 && (edge == pinEventRising || edge == pinEventBoth):
		handler(0, time.Since(p.startedAt), "rising edge", seq, seq)
	case current == 0 && (edge == pinEventFalling || edge == pinEventBoth):
		handler(0, time.Since(p.startedAt), "falling edge", seq, seq)
	}
}

type simPWMPin struct {
	adaptor *Adaptor
	id      string

	mu       sync.Mutex
	exported bool
	enabled  bool
	polarity bool
	period   uint32
	duty     uint32
}

// PWMPin implements gobot.PWMPinnerProvider.
func (a *Adaptor) PWMPin(id string) (gobot.PWMPinner, error) {
	if err := state.ValidateIdentifier("pin", id); err != nil {
		return nil, err
	}
	a.pinMu.Lock()
	defer a.pinMu.Unlock()
	if pin, ok := a.pwm[id]; ok {
		return pin, nil
	}
	pin := &simPWMPin{adaptor: a, id: id, polarity: true, period: 20000000, enabled: true}
	a.pwm[id] = pin
	return pin, nil
}

// Export implements gobot.PWMPinner.
func (p *simPWMPin) Export() error {
	p.mu.Lock()
	p.exported = true
	p.mu.Unlock()
	return nil
}

// Unexport implements gobot.PWMPinner.
func (p *simPWMPin) Unexport() error {
	p.mu.Lock()
	p.exported = false
	p.mu.Unlock()
	return nil
}

// Enabled implements gobot.PWMPinner.
func (p *simPWMPin) Enabled() (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.enabled, nil
}

// SetEnabled implements gobot.PWMPinner.
func (p *simPWMPin) SetEnabled(val bool) error {
	p.mu.Lock()
	p.enabled = val
	p.mu.Unlock()
	if !val {
		p.adaptor.board.PwmWritePin(p.id, 0, state.SourceHardwareCode)
	}
	return nil
}

// Polarity implements gobot.PWMPinner.
func (p *simPWMPin) Polarity() (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.polarity, nil
}

// SetPolarity implements gobot.PWMPinner.
func (p *simPWMPin) SetPolarity(normal bool) error {
	p.mu.Lock()
	p.polarity = normal
	p.mu.Unlock()
	return nil
}

// Period implements gobot.PWMPinner.
func (p *simPWMPin) Period() (uint32, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.period, nil
}

// SetPeriod implements gobot.PWMPinner.
func (p *simPWMPin) SetPeriod(period uint32) error {
	p.mu.Lock()
	p.period = period
	p.mu.Unlock()
	return nil
}

// DutyCycle implements gobot.PWMPinner.
func (p *simPWMPin) DutyCycle() (uint32, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.duty, nil
}

// SetDutyCycle implements gobot.PWMPinner.
func (p *simPWMPin) SetDutyCycle(duty uint32) error {
	p.mu.Lock()
	p.duty = duty
	period := p.period
	enabled := p.enabled
	p.mu.Unlock()
	if !enabled {
		return nil
	}
	level := 0
	if period > 0 {
		level = int((uint64(duty) * 255) / uint64(period)) //nolint:gosec // intentional width conversion
	}
	if level > 255 {
		level = 255
	}
	p.adaptor.board.PwmWritePin(p.id, byte(level), state.SourceHardwareCode)
	return nil
}

// afterPinWrite emulates devices whose output depends on another pin. The
// HC-SR04 ultrasonic sensor emits an echo pulse on its echo pin when its
// trigger pin rises; the pulse width encodes the measured distance.
func (a *Adaptor) afterPinWrite(pin string, level int) {
	if a.sim != nil {
		a.sim.observeDisplayPin(a.board.ID(), pin, level)
	}
	if level == 0 {
		return
	}
	for _, device := range a.board.Devices() {
		if device.Kind != state.DeviceKindDistance || len(device.Pins) < 2 || device.Pins[0] != pin {
			continue
		}
		distance := 1.0
		if value, ok := device.State["distance"].(float64); ok && value > 0 {
			distance = value
		}
		go a.pulseEcho(device.Pins[1], distance)
	}
}

func (a *Adaptor) pulseEcho(pin string, distance float64) {
	pulse := time.Duration(2 * distance / 343 * float64(time.Second))
	if pulse <= 0 || pulse > 100*time.Millisecond {
		return
	}
	a.board.DigitalWritePin(pin, 1, false, state.PinModeInput, state.SourceSimulation)
	a.firePinEdge(pin)
	time.Sleep(pulse)
	a.board.DigitalWritePin(pin, 0, false, state.PinModeInput, state.SourceSimulation)
	a.firePinEdge(pin)
}

func (a *Adaptor) firePinEdge(pin string) {
	a.pinMu.Lock()
	digitalPin := a.pins[pin]
	a.pinMu.Unlock()
	if digitalPin != nil {
		digitalPin.fireEdge()
	}
}

// recordTrace appends one adaptor trace record.
func (a *Adaptor) recordTrace(kind trace.Kind, target, operation string, value interface{}, source state.EventSource, started time.Time, err error) {
	if a.sim == nil || a.sim.traces == nil {
		return
	}
	rec := trace.Record{
		BoardID:    a.board.ID(),
		Kind:       kind,
		Target:     target,
		Operation:  operation,
		Value:      value,
		Source:     source,
		DurationNs: a.sim.clk.Since(started).Nanoseconds(),
	}
	if err != nil {
		rec.Error = err.Error()
	}
	a.sim.traces.Append(rec)
}

var _ gobot.DigitalPinnerProvider = (*Adaptor)(nil)
var _ gobot.PWMPinnerProvider = (*Adaptor)(nil)
