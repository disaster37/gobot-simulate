package simulate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/disaster37/gobot-simulate/internal/bus"
	"github.com/disaster37/gobot-simulate/internal/clock"
	"github.com/disaster37/gobot-simulate/internal/scenario"
	"github.com/disaster37/gobot-simulate/internal/state"
	"github.com/disaster37/gobot-simulate/internal/topology"
	"github.com/disaster37/gobot-simulate/internal/trace"
	"github.com/disaster37/gobot-simulate/server"
	simweb "github.com/disaster37/gobot-simulate/webui"
	"gobot.io/x/gobot/v2"
)

// Version is the simulator release version. It is intended to be replaced at
// build time with -ldflags "-X github.com/disaster37/gobot-simulate.Version=...".
var Version = "0.1.0-dev"

var (
	ErrBoardNotFound  = state.ErrBoardNotFound
	ErrDeviceNotFound = state.ErrDeviceNotFound
	ErrPinNotFound    = state.ErrPinNotFound
	ErrInvalidValue   = state.ErrInvalidValue
	ErrReadOnly       = state.ErrReadOnly
	ErrInvalidPinMode = state.ErrInvalidPinMode
	ErrInvalidID      = state.ErrInvalidID
	ErrClientCap      = state.ErrClientCap
)

// Re-export state types for public callers.
type (
	PinMode            = state.PinMode
	DeviceKind         = state.DeviceKind
	EventSource        = state.EventSource
	EventType          = state.EventType
	PinState           = state.PinState
	DeviceCapabilities = state.DeviceCapabilities
	DeviceState        = state.DeviceState
	ExtraValue         = state.ExtraValue
	ExtraFunction      = state.ExtraFunction
	FaultState         = state.FaultState
	BoardState         = state.BoardState
	Event              = state.Event
	ServerMetadata     = state.ServerMetadata
	Snapshot           = state.Snapshot
	Wiring             = state.Wiring
	DeviceGroup        = state.DeviceGroup
	DeviceFault        = state.DeviceFault
	Drive              = state.Drive
	Contact            = state.Contact
	Mechanism          = state.Mechanism
	Pull               = state.Pull
)

// Pin modes.
const (
	PinModeUnset       = state.PinModeUnset
	PinModeInput       = state.PinModeInput
	PinModeOutput      = state.PinModeOutput
	PinModeInputPullup = state.PinModeInputPullup
	PinModePWM         = state.PinModePWM
	PinModeServo       = state.PinModeServo
	PinModeAnalog      = state.PinModeAnalog
)

// Device kinds.
const (
	DeviceKindRelay          = state.DeviceKindRelay
	DeviceKindLED            = state.DeviceKindLED
	DeviceKindRGBLED         = state.DeviceKindRGBLED
	DeviceKindButton         = state.DeviceKindButton
	DeviceKindCaptor         = state.DeviceKindCaptor
	DeviceKindPIR            = state.DeviceKindPIR
	DeviceKindSensor         = state.DeviceKindSensor
	DeviceKindPWM            = state.DeviceKindPWM
	DeviceKindServo          = state.DeviceKindServo
	DeviceKindAnalog         = state.DeviceKindAnalog
	DeviceKindAnalogActuator = state.DeviceKindAnalogActuator
	DeviceKindMotor          = state.DeviceKindMotor
	DeviceKindStepper        = state.DeviceKindStepper
	DeviceKindBuzzer         = state.DeviceKindBuzzer
	DeviceKindDirectPin      = state.DeviceKindDirectPin
	DeviceKindDistance       = state.DeviceKindDistance
	DeviceKindTemperature    = state.DeviceKindTemperature
	DeviceKindDisplay        = state.DeviceKindDisplay
	DeviceKindExpander       = state.DeviceKindExpander
	DeviceKindIMU            = state.DeviceKindIMU
	DeviceKindADC            = state.DeviceKindADC
	DeviceKindRFID           = state.DeviceKindRFID
	DeviceKindValue          = state.DeviceKindValue
	DeviceKindFunction       = state.DeviceKindFunction
)

// Device groups and wiring values.
const (
	GroupInputs    = state.GroupInputs
	GroupOutputs   = state.GroupOutputs
	GroupActuators = state.GroupActuators
	GroupSensors   = state.GroupSensors
	GroupDisplays  = state.GroupDisplays
	GroupExpanders = state.GroupExpanders
	GroupPins      = state.GroupPins
	GroupExtras    = state.GroupExtras

	DriveActiveHigh = state.DriveActiveHigh
	DriveActiveLow  = state.DriveActiveLow

	ContactNO = state.ContactNO
	ContactNC = state.ContactNC

	MechanismNone      = state.MechanismNone
	MechanismMomentary = state.MechanismMomentary
	MechanismLatching  = state.MechanismLatching
	MechanismSelector  = state.MechanismSelector

	PullNone     = state.PullNone
	PullUp       = state.PullUp
	PullDown     = state.PullDown
	PullExternal = state.PullExternal
)

// Event sources.
const (
	SourceHardwareCode = state.SourceHardwareCode
	SourceUI           = state.SourceUI
	SourceSimulation   = state.SourceSimulation
)

// Event types.
const (
	EventSnapshot      = state.EventSnapshot
	EventBoardUpdated  = state.EventBoardUpdated
	EventDeviceUpdated = state.EventDeviceUpdated
	EventPinUpdated    = state.EventPinUpdated
	EventValueUpdated  = state.EventValueUpdated
	EventFaultChanged  = state.EventFaultChanged
	EventHeartbeat     = state.EventHeartbeat
)

// Option configures a Simulator instance.
type Option func(*Simulator)

// AdaptorOption configures an Adaptor instance.
type AdaptorOption func(*AdaptorConfig)

// DeviceSpec defines device registration parameters when registering manually or via hints.
type DeviceSpec struct {
	ID           string
	Name         string
	Label        string
	Kind         DeviceKind
	DisplayModel string
	Pin          string
	Pins         []string
	ValueName    string
	FunctionName string
	Inverted     bool
	DefaultState *int
	Labels       map[string]string
	Capabilities *DeviceCapabilities
	Wiring       state.Wiring
	State        map[string]interface{}
	Source       string
}

// Simulator owns a state store, event hub, and optional Hertz server.
type Simulator struct {
	cfg           Config
	store         *state.Store
	clk           clock.Clock
	traces        *trace.Store
	version       string
	startedAt     time.Time
	adaptorsMu    sync.Mutex
	adaptors      map[string]*Adaptor
	driversMu     sync.RWMutex
	drivers       map[string]map[string]gobot.Device
	topologyMu    sync.RWMutex
	topology      *topology.Loaded
	scenarioMu    sync.RWMutex
	scenarios     *scenario.Engine
	displayMu     sync.RWMutex
	displays      map[string]map[string]displayBinding
	loadErr       error
	runtimeMu     sync.Mutex
	runtimeCtx    context.Context
	runtimeCancel context.CancelFunc
	runtimeOnce   sync.Once
	running       bool
	srvMu         sync.RWMutex
	srv           *server.Server
}

// New creates a simulator with optional configuration.
func New(opts ...Option) *Simulator {
	s := &Simulator{
		cfg:       DefaultConfig(),
		clk:       clock.RealClock{},
		version:   Version,
		adaptors:  make(map[string]*Adaptor),
		drivers:   make(map[string]map[string]gobot.Device),
		displays:  make(map[string]map[string]displayBinding),
		startedAt: time.Now(),
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.clk == nil {
		s.clk = clock.RealClock{}
	}
	s.cfg = normalizeConfig(s.cfg)
	s.startedAt = s.clk.Now()
	s.store = state.NewStore(s.version, s.cfg.EventBuffer, s.clk)
	s.traces = trace.New(s.cfg.TraceBuffer, s.clk)
	if s.cfg.Topology != "" {
		s.loadErr = s.LoadTopologyFile(s.cfg.Topology)
	}
	return s
}

// NewAdaptor creates a standalone adaptor with its own simulator.
func NewAdaptor(opts ...AdaptorOption) *Adaptor {
	return New().Adaptor("test", opts...)
}

// NewMockPlateform preserves the gobot-fat mock constructor.
func NewMockPlateform() *MockPlateform {
	return NewAdaptor(WithAdaptorID("test"), WithAdaptorName("test"))
}

// IntPtr is a helper for DeviceSpec.DefaultState.
func IntPtr(v int) *int { return &v }

// WithConfig replaces the simulator configuration.
func WithConfig(cfg Config) Option { return func(s *Simulator) { s.cfg = cfg } }

// WithAddr sets the listen address.
func WithAddr(addr string) Option { return func(s *Simulator) { s.cfg.Addr = addr } }

// WithPort sets the listen port (0 selects an ephemeral port).
func WithPort(port int) Option { return func(s *Simulator) { s.cfg.Port = port } }

// WithUIEnabled controls static web UI serving.
func WithUIEnabled(enabled bool) Option { return func(s *Simulator) { s.cfg.UIEnabled = enabled } }

// WithCORSOrigins sets the CORS allowlist.
func WithCORSOrigins(origins ...string) Option {
	return func(s *Simulator) { s.cfg.CORSOrigins = dedupeStrings(origins) }
}

// WithMaxClients sets the maximum concurrent SSE clients.
func WithMaxClients(max int) Option { return func(s *Simulator) { s.cfg.MaxClients = max } }

// WithEventBuffer sets the replay ring buffer size.
func WithEventBuffer(size int) Option { return func(s *Simulator) { s.cfg.EventBuffer = size } }

// WithLatency sets the default simulated I/O latency.
func WithLatency(d time.Duration) Option { return func(s *Simulator) { s.cfg.Latency = d } }

// WithClock injects a deterministic clock (useful for scenario tests).
func WithClock(clk clock.Clock) Option { return func(s *Simulator) { s.clk = clk } }

// WithAuthToken enables bearer-token authentication.
func WithAuthToken(token string) Option { return func(s *Simulator) { s.cfg.AuthToken = token } }

// WithTraceBuffer sets the bounded I/O trace ring size.
func WithTraceBuffer(size int) Option { return func(s *Simulator) { s.cfg.TraceBuffer = size } }

// WithTopology sets a topology file loaded during New.
func WithTopology(path string) Option { return func(s *Simulator) { s.cfg.Topology = path } }

// WithARESTEnabled controls the aREST HTTP emulation surface.
func WithARESTEnabled(enabled bool) Option { return func(s *Simulator) { s.cfg.AREST = enabled } }

// Adaptor creates or returns the adaptor for idOrName.
func (s *Simulator) Adaptor(idOrName string, opts ...AdaptorOption) *Adaptor {
	cfg := AdaptorConfig{ID: idOrName, Name: idOrName}
	for _, opt := range opts {
		opt(&cfg)
	}
	cfg.ID = sanitizeID(cfg.ID, idOrName)
	if cfg.Name == "" {
		cfg.Name = cfg.ID
	}
	if cfg.Platform == "" {
		cfg.Platform = "simulated"
	}
	if cfg.Latency == 0 {
		cfg.Latency = s.cfg.Latency
	}

	s.adaptorsMu.Lock()
	if existing, ok := s.adaptors[cfg.ID]; ok {
		s.adaptorsMu.Unlock()
		applyAdaptorConfig(existing, cfg)
		return existing
	}

	board, err := s.store.Resolve(cfg.ID)
	if err != nil {
		board, err = s.store.RegisterBoard(cfg.ID, cfg.Name, cfg.Platform, cfg.Port, cfg.Latency)
	}
	if err != nil {
		// sanitizeID guarantees a valid identifier, so this is unreachable in
		// practice; fall back to a deterministic valid board rather than
		// returning a nil adaptor.
		cfg.ID = "simulator"
		board, _ = s.store.RegisterBoard(cfg.ID, cfg.Name, cfg.Platform, cfg.Port, cfg.Latency)
	}
	a := &Adaptor{
		sim:   s,
		board: board,
		buses: bus.NewManager(board.ID(), s.clk, s.traces),
		id:    cfg.ID,
		name:  cfg.Name,
		port:  cfg.Port,
		pins:  make(map[string]*simDigitalPin),
		pwm:   make(map[string]*simPWMPin),
	}
	s.adaptors[cfg.ID] = a
	s.adaptorsMu.Unlock()

	applyAdaptorConfig(a, cfg)
	return a
}

// RegisterBoard registers an existing adaptor's board with the simulator.
func (s *Simulator) RegisterBoard(a *Adaptor) error {
	if a == nil || a.board == nil {
		return errors.New("simulate: nil adaptor")
	}
	s.adaptorsMu.Lock()
	s.adaptors[a.id] = a
	s.adaptorsMu.Unlock()
	return nil
}

// RegisterDevice registers a device on a board.
func (s *Simulator) RegisterDevice(boardID string, spec DeviceSpec) (DeviceState, error) {
	board, err := s.store.Resolve(boardID)
	if err != nil {
		return DeviceState{}, err
	}
	id := spec.ID
	if id == "" {
		id = spec.Name
	}
	if id == "" {
		id = "device"
	}
	if err := state.ValidateIdentifier("device", id); err != nil {
		return DeviceState{}, err
	}
	for _, pin := range devicePins(spec) {
		if err := state.ValidateIdentifier("pin", pin); err != nil {
			return DeviceState{}, err
		}
	}
	kind := spec.Kind
	if !kind.Valid() {
		kind = DeviceKindValue
	}
	name := spec.Name
	if name == "" {
		name = id
	}
	label := spec.Label
	if label == "" {
		label = name
	}
	defaultState := 0
	if spec.DefaultState != nil {
		defaultState = *spec.DefaultState
	}
	caps := state.DefaultCapabilities(kind)
	if spec.Capabilities != nil {
		caps = *spec.Capabilities
	}
	d := DeviceState{
		ID:           id,
		BoardID:      board.ID(),
		Name:         name,
		Label:        label,
		Kind:         kind,
		DisplayModel: spec.DisplayModel,
		Pins:         devicePins(spec),
		ValueName:    spec.ValueName,
		FunctionName: spec.FunctionName,
		Inverted:     spec.Inverted,
		DefaultState: defaultState,
		Capabilities: caps,
		Labels:       spec.Labels,
		Wiring:       spec.Wiring,
		State:        spec.State,
		Source:       spec.Source,
	}
	if d.State == nil {
		d.State = make(map[string]interface{})
	}
	stored := board.RegisterDevice(d)
	s.attachDisplay(board.ID(), stored)
	return stored, nil
}

// BindPin binds a pin label and mode.
func (s *Simulator) BindPin(boardID, pin, label string, mode PinMode) error {
	if err := state.ValidateIdentifier("pin", pin); err != nil {
		return err
	}
	board, err := s.store.Resolve(boardID)
	if err != nil {
		return err
	}
	return board.BindPin(pin, label, mode)
}

// Snapshot returns the complete simulator state.
func (s *Simulator) Snapshot() Snapshot {
	snap := s.store.Snapshot()
	if srv := s.currentServer(); srv != nil {
		snap.Meta.ConnectedClients = srv.ClientCount()
	}
	return snap
}

// Boards returns all board states.
func (s *Simulator) Boards() []BoardState { return s.store.Boards() }

// Board returns one board state.
func (s *Simulator) Board(boardID string) (BoardState, error) { return s.store.BoardState(boardID) }

// Devices returns all devices for a board.
func (s *Simulator) Devices(boardID string) ([]DeviceState, error) { return s.store.Devices(boardID) }

// Device returns one device.
func (s *Simulator) Device(boardID, deviceID string) (DeviceState, error) {
	return s.store.Device(boardID, deviceID)
}

// Pin returns one pin.
func (s *Simulator) Pin(boardID, pin string) (PinState, error) { return s.store.Pin(boardID, pin) }

// Values returns extra values for a board.
func (s *Simulator) Values(boardID string) (map[string]ExtraValue, error) {
	return s.store.Values(boardID)
}

// SetDigital sets a digital pin value. Values must be in 0..255: 0 and 1 keep
// their classic digital meaning, while larger byte values are accepted for
// compatibility with PWM-like writes.
func (s *Simulator) SetDigital(boardID, pin string, val int, source EventSource) error {
	if err := state.ValidateIdentifier("pin", pin); err != nil {
		return err
	}
	if val < 0 || val > 255 {
		return fmt.Errorf("%w: digital value %d must be between 0 and 255", ErrInvalidValue, val)
	}
	started := s.clk.Now()
	err := s.store.SetDigital(boardID, pin, val, source)
	s.recordAPI(trace.KindPinWrite, boardID, pin, "write", val, source, started, err)
	return err
}

// PressButton presses a button or captor.
func (s *Simulator) PressButton(boardID, deviceID string) error {
	return s.deviceAction(boardID, deviceID, "press", s.store.PressButton)
}

// ReleaseButton releases a button or captor.
func (s *Simulator) ReleaseButton(boardID, deviceID string) error {
	return s.deviceAction(boardID, deviceID, "release", s.store.ReleaseButton)
}

// ToggleDevice toggles a device. Outputs are toggled through their attached
// driver so cached driver state stays consistent; inputs flip their pin.
func (s *Simulator) ToggleDevice(boardID, deviceID string) error {
	if err := state.ValidateIdentifier("device", deviceID); err != nil {
		return err
	}
	device, err := s.store.Device(boardID, deviceID)
	if err != nil {
		return err
	}
	if state.IsOutput(device.Kind) {
		return s.CommandDevice(boardID, deviceID, "toggle", nil)
	}
	return s.deviceAction(boardID, deviceID, "toggle", s.store.ToggleDevice)
}

func (s *Simulator) deviceAction(boardID, deviceID, operation string, fn func(string, string) error) error {
	if err := state.ValidateIdentifier("device", deviceID); err != nil {
		return err
	}
	started := s.clk.Now()
	err := fn(boardID, deviceID)
	s.recordAPI(trace.KindDriverEvent, boardID, deviceID, operation, nil, SourceUI, started, err)
	return err
}

// SetPWM writes a PWM value.
func (s *Simulator) SetPWM(boardID, pin string, val byte, source EventSource) error {
	if err := state.ValidateIdentifier("pin", pin); err != nil {
		return err
	}
	started := s.clk.Now()
	err := s.store.SetPWM(boardID, pin, val, source)
	s.recordAPI(trace.KindPWM, boardID, pin, "pwm", int(val), source, started, err)
	return err
}

// SetServo writes a servo angle.
func (s *Simulator) SetServo(boardID, pin string, angle byte, source EventSource) error {
	if err := state.ValidateIdentifier("pin", pin); err != nil {
		return err
	}
	started := s.clk.Now()
	err := s.store.SetServo(boardID, pin, angle, source)
	s.recordAPI(trace.KindServo, boardID, pin, "servo", int(angle), source, started, err)
	return err
}

// SetAnalog writes an analog value.
func (s *Simulator) SetAnalog(boardID, pin string, val int, source EventSource) error {
	if err := state.ValidateIdentifier("pin", pin); err != nil {
		return err
	}
	started := s.clk.Now()
	err := s.store.SetAnalog(boardID, pin, val, source)
	s.recordAPI(trace.KindAnalog, boardID, pin, "write", val, source, started, err)
	return err
}

// SetValue stores an extra value.
func (s *Simulator) SetValue(boardID, name string, val interface{}, source EventSource) error {
	if err := state.ValidateIdentifier("value", name); err != nil {
		return err
	}
	started := s.clk.Now()
	err := s.store.SetValue(boardID, name, val, source)
	s.recordAPI(trace.KindValueWrite, boardID, name, "write", val, source, started, err)
	return err
}

// CallFunction invokes an extra function.
func (s *Simulator) CallFunction(boardID, name, params string, source EventSource) (int, error) {
	if err := state.ValidateIdentifier("function", name); err != nil {
		return 0, err
	}
	started := s.clk.Now()
	result, err := s.store.CallFunction(boardID, name, params, source)
	s.recordAPI(trace.KindFunctionCall, boardID, name, params, result, source, started, err)
	return result, err
}

func (s *Simulator) recordAPI(kind trace.Kind, boardID, target, operation string, value interface{}, source EventSource, started time.Time, err error) {
	if s.traces == nil {
		return
	}
	rec := trace.Record{
		BoardID:    boardID,
		Kind:       kind,
		Target:     target,
		Operation:  operation,
		Value:      value,
		Source:     source,
		DurationNs: s.clk.Since(started).Nanoseconds(),
	}
	if err != nil {
		rec.Error = err.Error()
	}
	s.traces.Append(rec)
}

// SetFunctionCode configures a function's mocked return code.
func (s *Simulator) SetFunctionCode(boardID, name string, code int, source EventSource) error {
	if err := state.ValidateIdentifier("function", name); err != nil {
		return err
	}
	return s.store.SetFunctionCode(boardID, name, code, source)
}

// SetFault injects a board-wide fault.
func (s *Simulator) SetFault(boardID string, faultErr error) error {
	return s.store.SetFault(boardID, faultErr)
}

// ClearFault clears a board-wide fault.
func (s *Simulator) ClearFault(boardID string) error { return s.store.ClearFault(boardID) }

// Subscribe returns a new event channel.
func (s *Simulator) Subscribe() <-chan Event { return s.store.Subscribe() }

// SubscribeWithReplay subscribes and returns events after lastEventID.
func (s *Simulator) SubscribeWithReplay(lastEventID uint64) (<-chan Event, []Event, bool) {
	return s.store.SubscribeWithReplay(lastEventID)
}

// Unsubscribe removes an event channel.
func (s *Simulator) Unsubscribe(ch <-chan Event) { s.store.Unsubscribe(ch) }

// DroppedEvents returns the number of dropped subscriber events.
func (s *Simulator) DroppedEvents() uint64 { return s.store.DroppedEvents() }

// CurrentSeq returns the current event sequence.
func (s *Simulator) CurrentSeq() uint64 { return s.store.CurrentSeq() }

// ARESTBaseURL returns the aREST base URL for one board.
func (s *Simulator) ARESTBaseURL(boardID string) string {
	base := s.URL()
	if base == "" {
		return ""
	}
	return base + "/arest/" + boardID
}

// Embedded loads configuration (including SIMULATE_TOPOLOGY), starts the HTTP
// server with the embedded UI and returns a ready simulator. Hosts that also
// run gobot robots should stop the robots first, then call Stop, so no driver
// writes race server shutdown.
func Embedded(cfg Config) (*Simulator, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	sim := New(WithConfig(cfg))
	if err := sim.Start(context.Background()); err != nil {
		return nil, err
	}
	return sim, nil
}

// Start starts the HTTP server without blocking.
func (s *Simulator) Start(ctx context.Context) error {
	if s.loadErr != nil {
		return s.loadErr
	}
	s.startRuntime(ctx)
	srv, err := s.ensureServer()
	if err != nil {
		return err
	}
	return srv.Start(ctx)
}

// Serve starts the HTTP server and blocks until ctx is cancelled.
func (s *Simulator) Serve(ctx context.Context) error {
	srv, err := s.ensureServer()
	if err != nil {
		return err
	}
	return srv.Serve(ctx)
}

// Stop gracefully shuts down the HTTP server.
func (s *Simulator) Stop(ctx context.Context) error {
	s.runtimeMu.Lock()
	if s.runtimeCancel != nil {
		s.runtimeCancel()
		s.runtimeCancel = nil
	}
	s.running = false
	s.runtimeMu.Unlock()
	if scenarios := s.currentScenarios(); scenarios != nil {
		scenarios.Stop()
	}
	srv := s.currentServer()
	if srv == nil {
		return nil
	}
	return srv.Stop(ctx)
}

// Close stops the HTTP server with a background context.
func (s *Simulator) Close() error { return s.Stop(context.Background()) }

// Hertz returns the underlying Hertz engine (nil before Start).
func (s *Simulator) Hertz() *server.Hertz {
	srv := s.currentServer()
	if srv == nil {
		return nil
	}
	return srv.Hertz()
}

// URL returns the base URL of the running server.
func (s *Simulator) URL() string {
	srv := s.currentServer()
	if srv == nil {
		return ""
	}
	return srv.URL()
}

func (s *Simulator) ensureServer() (*server.Server, error) {
	if srv := s.currentServer(); srv != nil {
		return srv, nil
	}

	s.srvMu.Lock()
	defer s.srvMu.Unlock()
	if s.srv != nil {
		return s.srv, nil
	}
	srv, err := server.New(s, server.Config{
		Addr:         s.cfg.Addr,
		Port:         s.cfg.Port,
		UIEnabled:    s.cfg.UIEnabled,
		CORSOrigins:  s.cfg.CORSOrigins,
		MaxClients:   s.cfg.MaxClients,
		AuthToken:    s.cfg.AuthToken,
		Version:      s.version,
		ARESTEnabled: s.cfg.AREST,
		Assets:       simweb.DistFS(),
	})
	if err != nil {
		return nil, err
	}
	s.srv = srv
	return srv, nil
}

func (s *Simulator) currentServer() *server.Server {
	s.srvMu.RLock()
	defer s.srvMu.RUnlock()
	return s.srv
}

// Uptime returns the simulator's uptime.
func (s *Simulator) Uptime() time.Duration { return s.clk.Since(s.startedAt) }

// ClientCount returns active SSE clients.
func (s *Simulator) ClientCount() int {
	if srv := s.currentServer(); srv != nil {
		return srv.ClientCount()
	}
	return 0
}

// Version returns the simulator version.
func (s *Simulator) Version() string { return s.version }

// WaitEvent waits for a gobot event, matching gobot-fat/mock.WaitEvent.
func WaitEvent(e gobot.Eventer, eventName string, timeout time.Duration) chan bool {
	out := e.Subscribe()
	status := make(chan bool)

	go func() {
		defer e.Unsubscribe(out)
		for {
			select {
			case evt := <-out:
				if evt.Name == eventName {
					status <- true
					return
				}
			case <-time.After(timeout):
				status <- false
				return
			}
		}
	}()

	return status
}

func devicePins(spec DeviceSpec) []string {
	return dedupeStrings(append(append([]string{}, spec.Pins...), spec.Pin))
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func sanitizeID(candidates ...string) string {
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if state.ValidateID(candidate) == nil {
			return candidate
		}
	}
	for _, candidate := range candidates {
		if candidate != "" {
			return state.NormalizeIdentifier(candidate)
		}
	}
	return "test"
}

func normalizeConfig(cfg Config) Config {
	def := DefaultConfig()
	if strings.TrimSpace(cfg.Addr) == "" {
		cfg.Addr = def.Addr
	}
	if cfg.MaxClients < 1 {
		cfg.MaxClients = def.MaxClients
	}
	if cfg.EventBuffer < 16 {
		cfg.EventBuffer = def.EventBuffer
	}
	if cfg.TraceBuffer < 16 {
		cfg.TraceBuffer = def.TraceBuffer
	}
	if cfg.CORSOrigins == nil {
		cfg.CORSOrigins = def.CORSOrigins
	}
	return cfg
}
