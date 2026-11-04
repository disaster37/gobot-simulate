package simulate

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/disaster37/gobot-simulate/internal/clock"
	"github.com/disaster37/gobot-simulate/internal/state"
	"github.com/disaster37/gobot-simulate/server"
	simweb "github.com/disaster37/gobot-simulate/webui"
	"gobot.io/x/gobot/v2"
)

// Version is the simulator release version.
const Version = "0.1.0"

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
	DeviceKindRelay    = state.DeviceKindRelay
	DeviceKindLED      = state.DeviceKindLED
	DeviceKindButton   = state.DeviceKindButton
	DeviceKindCaptor   = state.DeviceKindCaptor
	DeviceKindSensor   = state.DeviceKindSensor
	DeviceKindPWM      = state.DeviceKindPWM
	DeviceKindServo    = state.DeviceKindServo
	DeviceKindAnalog   = state.DeviceKindAnalog
	DeviceKindValue    = state.DeviceKindValue
	DeviceKindFunction = state.DeviceKindFunction
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
	Pin          string
	Pins         []string
	ValueName    string
	FunctionName string
	Inverted     bool
	DefaultState *int
	Labels       map[string]string
	Capabilities *DeviceCapabilities
}

// Simulator owns a state store, event hub, and optional Hertz server.
type Simulator struct {
	cfg        Config
	store      *state.Store
	clk        clock.Clock
	version    string
	startedAt  time.Time
	adaptorsMu sync.Mutex
	adaptors   map[string]*Adaptor
	srvMu      sync.RWMutex
	srv        *server.Server
}

// New creates a simulator with optional configuration.
func New(opts ...Option) *Simulator {
	s := &Simulator{
		cfg:       DefaultConfig(),
		clk:       clock.RealClock{},
		version:   Version,
		adaptors:  make(map[string]*Adaptor),
		startedAt: time.Now(),
	}
	for _, opt := range opts {
		opt(s)
	}
	s.cfg = normalizeConfig(s.cfg)
	s.startedAt = s.clk.Now()
	s.store = state.NewStore(s.version, s.cfg.EventBuffer, s.clk)
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

// WithAuthToken enables bearer-token authentication.
func WithAuthToken(token string) Option { return func(s *Simulator) { s.cfg.AuthToken = token } }

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
	a := &Adaptor{sim: s, board: board, id: cfg.ID, name: cfg.Name, port: cfg.Port}
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
		Pins:         devicePins(spec),
		ValueName:    spec.ValueName,
		FunctionName: spec.FunctionName,
		Inverted:     spec.Inverted,
		DefaultState: defaultState,
		Capabilities: caps,
		Labels:       spec.Labels,
		State:        make(map[string]interface{}),
	}
	return board.RegisterDevice(d), nil
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

// SetDigital sets a digital pin value. Values are truncated to a byte when
// stored; callers should pass 0 or 1. (The HTTP layer validates 0/1; this is
// a low-level mutation API.)
func (s *Simulator) SetDigital(boardID, pin string, val int, source EventSource) error {
	if err := state.ValidateIdentifier("pin", pin); err != nil {
		return err
	}
	return s.store.SetDigital(boardID, pin, val, source)
}

// PressButton presses a button or captor.
func (s *Simulator) PressButton(boardID, deviceID string) error {
	if err := state.ValidateIdentifier("device", deviceID); err != nil {
		return err
	}
	return s.store.PressButton(boardID, deviceID)
}

// ReleaseButton releases a button or captor.
func (s *Simulator) ReleaseButton(boardID, deviceID string) error {
	if err := state.ValidateIdentifier("device", deviceID); err != nil {
		return err
	}
	return s.store.ReleaseButton(boardID, deviceID)
}

// ToggleDevice toggles a toggleable device.
func (s *Simulator) ToggleDevice(boardID, deviceID string) error {
	if err := state.ValidateIdentifier("device", deviceID); err != nil {
		return err
	}
	return s.store.ToggleDevice(boardID, deviceID)
}

// SetPWM writes a PWM value.
func (s *Simulator) SetPWM(boardID, pin string, val byte, source EventSource) error {
	if err := state.ValidateIdentifier("pin", pin); err != nil {
		return err
	}
	return s.store.SetPWM(boardID, pin, val, source)
}

// SetServo writes a servo angle.
func (s *Simulator) SetServo(boardID, pin string, angle byte, source EventSource) error {
	if err := state.ValidateIdentifier("pin", pin); err != nil {
		return err
	}
	return s.store.SetServo(boardID, pin, angle, source)
}

// SetAnalog writes an analog value.
func (s *Simulator) SetAnalog(boardID, pin string, val int, source EventSource) error {
	if err := state.ValidateIdentifier("pin", pin); err != nil {
		return err
	}
	return s.store.SetAnalog(boardID, pin, val, source)
}

// SetValue stores an extra value.
func (s *Simulator) SetValue(boardID, name string, val interface{}, source EventSource) error {
	if err := state.ValidateIdentifier("value", name); err != nil {
		return err
	}
	return s.store.SetValue(boardID, name, val, source)
}

// CallFunction invokes an extra function.
func (s *Simulator) CallFunction(boardID, name, params string, source EventSource) (int, error) {
	if err := state.ValidateIdentifier("function", name); err != nil {
		return 0, err
	}
	return s.store.CallFunction(boardID, name, params, source)
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

// Start starts the HTTP server without blocking.
func (s *Simulator) Start(ctx context.Context) error {
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
		Addr:        s.cfg.Addr,
		Port:        s.cfg.Port,
		UIEnabled:   s.cfg.UIEnabled,
		CORSOrigins: s.cfg.CORSOrigins,
		MaxClients:  s.cfg.MaxClients,
		AuthToken:   s.cfg.AuthToken,
		Version:     s.version,
		Assets:      simweb.DistFS(),
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
	if cfg.CORSOrigins == nil {
		cfg.CORSOrigins = def.CORSOrigins
	}
	return cfg
}
