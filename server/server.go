// Package server exposes the simulator's state over the CloudWeGo Hertz HTTP
// engine, including REST endpoints and a Server-Sent Events stream.
package server

import (
	"context"
	"io/fs"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	hertz "github.com/cloudwego/hertz/pkg/app/server"
	"github.com/disaster37/gobot-simulate/internal/bus"
	"github.com/disaster37/gobot-simulate/internal/scenario"
	"github.com/disaster37/gobot-simulate/internal/state"
	"github.com/disaster37/gobot-simulate/internal/topology"
	"github.com/disaster37/gobot-simulate/internal/trace"
)

const maxRequestBodySize = 64 * 1024

// Hertz is re-exported so callers do not need to import the Hertz server package.
type Hertz = hertz.Hertz

// Backend is implemented by simulate.Simulator without creating an import cycle.
type Backend interface {
	Snapshot() state.Snapshot
	Boards() []state.BoardState
	Board(boardID string) (state.BoardState, error)
	Devices(boardID string) ([]state.DeviceState, error)
	Device(boardID, deviceID string) (state.DeviceState, error)
	Pin(boardID, pin string) (state.PinState, error)
	Values(boardID string) (map[string]state.ExtraValue, error)
	BindPin(boardID, pin, label string, mode state.PinMode) error
	SetDigital(boardID, pin string, val int, source state.EventSource) error
	PressButton(boardID, deviceID string) error
	ReleaseButton(boardID, deviceID string) error
	ToggleDevice(boardID, deviceID string) error
	SetPWM(boardID, pin string, val byte, source state.EventSource) error
	SetServo(boardID, pin string, angle byte, source state.EventSource) error
	SetAnalog(boardID, pin string, val int, source state.EventSource) error
	SetValue(boardID, name string, val interface{}, source state.EventSource) error
	CallFunction(boardID, name, params string, source state.EventSource) (int, error)
	SetFunctionCode(boardID, name string, code int, source state.EventSource) error
	SetFault(boardID string, faultErr error) error
	ClearFault(boardID string) error
	Subscribe() <-chan state.Event
	SubscribeWithReplay(lastEventID uint64) (<-chan state.Event, []state.Event, bool)
	Unsubscribe(ch <-chan state.Event)
	DroppedEvents() uint64
	CurrentSeq() uint64
	ClientCount() int
	Uptime() time.Duration
	Version() string

	CommandDevice(boardID, deviceID, action string, value *int) error
	SetDeviceFault(boardID, deviceID, fault string) (state.DeviceState, error)
	ClearDeviceFault(boardID, deviceID, fault string) (state.DeviceState, error)
	ClearDeviceFaults(boardID, deviceID string) (state.DeviceState, error)
	Step(boardID, name, status string, detail map[string]interface{}) error
	Trace(boardID string, since uint64, kinds []string, target string) ([]trace.Record, error)
	Waveform(boardID string, pins []string, from, to time.Time, maxPoints int) ([]trace.Series, error)
	SubscribeTrace() <-chan trace.Record
	UnsubscribeTrace(ch <-chan trace.Record)
	Topology() (topology.Spec, []byte)
	Scenarios() []scenario.Rule
	SetScenarioEnabled(id string, enabled bool) error
	TriggerScenario(ctx context.Context, id string) error
	Buses(boardID string) ([]map[string]interface{}, error)
	Chips(boardID string) ([]bus.ChipEntry, error)
	FindChip(chipID string) (bus.ChipEntry, error)
	SetChipField(boardID, chipID, field string, value interface{}) (bus.ChipEntry, error)
	RawChip(boardID, chipID string, txn bus.Transaction) (bus.Response, error)
	ChipValues(chipID string) (map[string]interface{}, error)
	DigitalRead(boardID, pin string) (int, error)
	AnalogRead(boardID, pin string) (int, error)
	BindPinMode(boardID, pin, mode string) error
	BoardLatency(boardID string) time.Duration
	BoardFault(boardID string) error
}

// Config configures the HTTP server.
type Config struct {
	Addr         string
	Port         int
	UIEnabled    bool
	CORSOrigins  []string
	MaxClients   int
	AuthToken    string
	Version      string
	ARESTEnabled bool
	Assets       fs.FS
}

// Server wraps a Hertz engine and its lifecycle.
type Server struct {
	backend               Backend
	cfg                   Config
	contentSecurityPolicy string

	mu       sync.Mutex
	h        *Hertz
	listener net.Listener
	port     int
	running  bool
	runErr   error
	stopCh   chan struct{}

	clients atomic.Int64
}

// New creates a server. Start must be called before Hertz is non-nil.
func New(backend Backend, cfg Config) (*Server, error) {
	if backend == nil {
		return nil, state.ErrBoardNotFound
	}
	if cfg.MaxClients < 1 {
		cfg.MaxClients = 64
	}
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1"
	}
	return &Server{
		backend:               backend,
		cfg:                   cfg,
		contentSecurityPolicy: buildContentSecurityPolicy(cfg.Assets),
		stopCh:                make(chan struct{}),
	}, nil
}

// Start binds the listener and starts the Hertz engine without blocking.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}

	listenCtx := ctx
	if listenCtx == nil {
		listenCtx = context.Background()
	}
	lc := net.ListenConfig{}
	listener, err := lc.Listen(listenCtx, "tcp", net.JoinHostPort(s.cfg.Addr, strconv.Itoa(s.cfg.Port)))
	if err != nil {
		s.mu.Unlock()
		return err
	}
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		_ = listener.Close()
		s.mu.Unlock()
		return &net.AddrError{Err: "unexpected listener address", Addr: listener.Addr().String()}
	}

	h := hertz.New(
		hertz.WithListener(listener),
		hertz.WithReadTimeout(10*time.Second),
		hertz.WithIdleTimeout(60*time.Second),
		hertz.WithExitWaitTime(5*time.Second),
		hertz.WithMaxRequestBodySize(maxRequestBodySize),
		hertz.WithSenseClientDisconnection(true),
		hertz.WithDisablePrintRoute(true),
	)
	s.registerRoutes(h)

	stopCh := make(chan struct{})
	s.h = h
	s.listener = listener
	s.port = addr.Port
	s.running = true
	s.runErr = nil
	s.stopCh = stopCh
	s.mu.Unlock()

	go func() {
		if err := h.Run(); err != nil {
			s.mu.Lock()
			s.runErr = err
			s.mu.Unlock()
		}
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !h.IsRunning() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	s.mu.Lock()
	runErr := s.runErr
	s.mu.Unlock()
	if runErr != nil {
		return runErr
	}

	if ctx != nil && ctx.Done() != nil {
		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_ = s.Stop(shutdownCtx)
		}()
	}
	return nil
}

// Serve starts the server and blocks until ctx is cancelled.
func (s *Server) Serve(ctx context.Context) error {
	if err := s.Start(ctx); err != nil {
		return err
	}
	<-ctx.Done()
	if err := s.Stop(ctx); err != nil && ctx.Err() == nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runErr
}

// Stop shuts down the server and unblocks active SSE handlers.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	close(s.stopCh)
	h := s.h
	s.mu.Unlock()

	if ctx == nil {
		ctx = context.Background()
	}
	if err := h.Shutdown(ctx); err != nil && !strings.Contains(err.Error(), "engine is not running") {
		return err
	}
	return nil
}

// Hertz returns the underlying Hertz engine.
func (s *Server) Hertz() *Hertz {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.h
}

// URL returns the base URL, or an empty string before Start.
func (s *Server) URL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.port == 0 {
		return ""
	}
	host := s.cfg.Addr
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(s.port))
}

// Port returns the effective port.
func (s *Server) Port() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.port
}

// ClientCount returns the number of active SSE clients.
func (s *Server) ClientCount() int { return int(s.clients.Load()) }

func (s *Server) registerRoutes(h *Hertz) {
	h.Use(s.recoveryMiddleware, s.securityHeaders, s.corsMiddleware, s.authMiddleware, s.bodyLimitMiddleware)

	v1 := h.Group("/api/v1")
	v1.GET("/health", s.handleHealth)
	v1.GET("/snapshot", s.handleSnapshot)
	v1.GET("/boards", s.handleBoards)
	v1.GET("/boards/:board", s.handleBoard)
	v1.GET("/boards/:board/devices", s.handleDevices)
	v1.GET("/boards/:board/devices/:device", s.handleDevice)
	v1.PATCH("/boards/:board/devices/:device", s.handlePatchDevice)
	v1.POST("/boards/:board/devices/:device/press", s.handlePress)
	v1.POST("/boards/:board/devices/:device/release", s.handleRelease)
	v1.POST("/boards/:board/devices/:device/command", s.handleCommand)
	v1.POST("/boards/:board/devices/:device/fault", s.handleSetDeviceFault)
	v1.DELETE("/boards/:board/devices/:device/fault", s.handleClearDeviceFault)
	v1.POST("/boards/:board/pins/:pin", s.handlePin)
	v1.GET("/boards/:board/values/:name", s.handleGetValue)
	v1.PUT("/boards/:board/values/:name", s.handleSetValue)
	v1.POST("/boards/:board/functions/:name", s.handleFunction)
	v1.POST("/boards/:board/fault", s.handleSetFault)
	v1.DELETE("/boards/:board/fault", s.handleClearFault)
	v1.GET("/boards/:board/trace", s.handleTrace)
	v1.GET("/boards/:board/waveform", s.handleWaveform)
	v1.POST("/boards/:board/steps", s.handleStep)
	v1.GET("/boards/:board/buses", s.handleBuses)
	v1.GET("/boards/:board/chips", s.handleChips)
	v1.GET("/chips/:chip", s.handleChip)
	v1.PATCH("/chips/:chip", s.handlePatchChip)
	v1.POST("/chips/:chip/raw", s.handleRawChip)
	v1.GET("/topology", s.handleTopology)
	v1.GET("/topology/source", s.handleTopologySource)
	v1.GET("/scenarios", s.handleScenarios)
	v1.PATCH("/scenarios/:id", s.handlePatchScenario)
	v1.POST("/scenarios/:id/trigger", s.handleTriggerScenario)
	v1.GET("/boardprofiles", s.handleBoardProfiles)
	v1.GET("/events", s.handleEvents)

	if s.cfg.ARESTEnabled {
		h.Any("/arest/:board/*action", s.handleAREST)
		h.Any("/arest/:board", s.handleAREST)
		h.Any("/v1/devices/:board/*action", s.handleParticle)
		h.Any("/v1/devices/:board", s.handleParticle)
	}

	h.NoRoute(s.handleNoRoute)
}
