package state

import "time"

// PinMode describes the configured electrical mode of a simulated pin.
type PinMode string

const (
	PinModeUnset       PinMode = "unset"
	PinModeInput       PinMode = "input"
	PinModeOutput      PinMode = "output"
	PinModeInputPullup PinMode = "input-pullup"
	PinModePWM         PinMode = "pwm"
	PinModeServo       PinMode = "servo"
	PinModeAnalog      PinMode = "analog"
)

// Valid reports whether m is a known pin mode.
func (m PinMode) Valid() bool {
	switch m {
	case PinModeUnset, PinModeInput, PinModeOutput, PinModeInputPullup,
		PinModePWM, PinModeServo, PinModeAnalog:
		return true
	default:
		return false
	}
}

// DeviceKind classifies a Gobot device for UI rendering and interaction rules.
type DeviceKind string

const (
	DeviceKindRelay    DeviceKind = "relay"
	DeviceKindLED      DeviceKind = "led"
	DeviceKindButton   DeviceKind = "button"
	DeviceKindCaptor   DeviceKind = "captor"
	DeviceKindSensor   DeviceKind = "sensor"
	DeviceKindPWM      DeviceKind = "pwm"
	DeviceKindServo    DeviceKind = "servo"
	DeviceKindAnalog   DeviceKind = "analog"
	DeviceKindValue    DeviceKind = "value"
	DeviceKindFunction DeviceKind = "function"
)

// Valid reports whether k is a known device kind.
func (k DeviceKind) Valid() bool {
	switch k {
	case DeviceKindRelay, DeviceKindLED, DeviceKindButton, DeviceKindCaptor,
		DeviceKindSensor, DeviceKindPWM, DeviceKindServo, DeviceKindAnalog,
		DeviceKindValue, DeviceKindFunction:
		return true
	default:
		return false
	}
}

// EventSource identifies what triggered a state transition.
type EventSource string

const (
	SourceHardwareCode EventSource = "hardware-code"
	SourceUI           EventSource = "ui"
	SourceSimulation   EventSource = "simulation"
)

// EventType enumerates SSE and internal notification event names.
type EventType string

const (
	EventSnapshot      EventType = "snapshot"
	EventBoardUpdated  EventType = "board.updated"
	EventDeviceUpdated EventType = "device.updated"
	EventPinUpdated    EventType = "pin.updated"
	EventValueUpdated  EventType = "value.updated"
	EventFaultChanged  EventType = "fault.changed"
	EventHeartbeat     EventType = "heartbeat"
)

// PinState holds the current state of a single pin on a board.
type PinState struct {
	Pin           string    `json:"pin"`
	Label         string    `json:"label,omitempty"`
	Mode          PinMode   `json:"mode"`
	DigitalValue  int       `json:"digitalValue"`
	ByteValue     byte      `json:"byteValue"`
	AnalogValue   int       `json:"analogValue"`
	InvertInitial bool      `json:"invertInitial"`
	Pullup        bool      `json:"pullup"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// DeviceCapabilities defines how the UI/API may interact with a device.
type DeviceCapabilities struct {
	Readable   bool     `json:"readable"`
	Writable   bool     `json:"writable"`
	Pressable  bool     `json:"pressable"`
	Toggleable bool     `json:"toggleable"`
	Callable   bool     `json:"callable"`
	MinValue   *float64 `json:"minValue,omitempty"`
	MaxValue   *float64 `json:"maxValue,omitempty"`
	Unit       string   `json:"unit,omitempty"`
}

// DeviceState represents a registered or auto-discovered Gobot device on a board.
type DeviceState struct {
	ID           string                 `json:"id"`
	BoardID      string                 `json:"boardId"`
	Name         string                 `json:"name"`
	Label        string                 `json:"label"`
	Kind         DeviceKind             `json:"kind"`
	Driver       string                 `json:"driver"`
	Pins         []string               `json:"pins"`
	ValueName    string                 `json:"valueName,omitempty"`
	FunctionName string                 `json:"functionName,omitempty"`
	Inverted     bool                   `json:"inverted"`
	DefaultState int                    `json:"defaultState"`
	Active       bool                   `json:"active"`
	State        map[string]interface{} `json:"state"`
	Capabilities DeviceCapabilities     `json:"capabilities"`
	Labels       map[string]string      `json:"labels,omitempty"`
	UpdatedAt    time.Time              `json:"updatedAt"`
}

// ExtraValue represents an aREST / Extra named value on a board.
type ExtraValue struct {
	Name      string      `json:"name"`
	Value     interface{} `json:"value"`
	UpdatedAt time.Time   `json:"updatedAt"`
}

// ExtraFunction represents an aREST / Extra callable function on a board.
type ExtraFunction struct {
	Name           string    `json:"name"`
	ReturnCode     int       `json:"returnCode"`
	LastParameters string    `json:"lastParameters,omitempty"`
	CallCount      uint64    `json:"callCount"`
	LastCalledAt   time.Time `json:"lastCalledAt,omitempty"`
}

// FaultState represents an injected error on a board.
type FaultState struct {
	Active    bool      `json:"active"`
	Message   string    `json:"message,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

// BoardState represents the complete snapshot of a simulated board.
type BoardState struct {
	ID             string                   `json:"id"`
	Name           string                   `json:"name"`
	Platform       string                   `json:"platform"`
	Port           string                   `json:"port,omitempty"`
	Connected      bool                     `json:"connected"`
	Online         bool                     `json:"online"`
	ReconnectCount uint64                   `json:"reconnectCount"`
	SimulatedLagMs int64                    `json:"simulatedLagMs"`
	Fault          FaultState               `json:"fault"`
	Pins           map[string]PinState      `json:"pins"`
	Devices        map[string]DeviceState   `json:"devices"`
	Values         map[string]ExtraValue    `json:"values"`
	Functions      map[string]ExtraFunction `json:"functions"`
	CreatedAt      time.Time                `json:"createdAt"`
	UpdatedAt      time.Time                `json:"updatedAt"`
}

// Event represents an immutable state change notification.
type Event struct {
	Seq       uint64       `json:"seq"`
	Timestamp time.Time    `json:"timestamp"`
	Type      EventType    `json:"type"`
	Source    EventSource  `json:"source"`
	BoardID   string       `json:"boardId,omitempty"`
	DeviceID  string       `json:"deviceId,omitempty"`
	Pin       string       `json:"pin,omitempty"`
	ValueName string       `json:"valueName,omitempty"`
	OldValue  interface{}  `json:"oldValue,omitempty"`
	NewValue  interface{}  `json:"newValue,omitempty"`
	Board     *BoardState  `json:"board,omitempty"`
	Device    *DeviceState `json:"device,omitempty"`
	PinState  *PinState    `json:"pinState,omitempty"`
	Fault     *FaultState  `json:"fault,omitempty"`
}

// ServerMetadata describes runtime stats of the simulator.
type ServerMetadata struct {
	Version          string    `json:"version"`
	StartedAt        time.Time `json:"startedAt"`
	UptimeSeconds    int64     `json:"uptimeSeconds"`
	CurrentSeq       uint64    `json:"currentSeq"`
	ConnectedClients int       `json:"connectedClients"`
	DroppedEvents    uint64    `json:"droppedEvents"`
}

// Snapshot is the top-level state payload returned by GET /api/v1/snapshot and initial SSE event.
type Snapshot struct {
	Meta   ServerMetadata `json:"meta"`
	Boards []BoardState   `json:"boards"`
}
