// Package topology defines the declarative board topology schema and loader.
package topology

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/disaster37/gobot-simulate/internal/scenario"
	"github.com/disaster37/gobot-simulate/internal/state"
	"gopkg.in/yaml.v3"
)

// SchemaVersion is the supported topology schema version.
const SchemaVersion = "1"

// MaxTopologySize bounds a topology file read from disk or a request body.
const MaxTopologySize = 4 * 1024 * 1024

// Spec is the effective topology document.
type Spec struct {
	Schema    string          `json:"schema" yaml:"schema"`
	Boards    []Board         `json:"boards" yaml:"boards"`
	Scenarios []scenario.Rule `json:"scenarios,omitempty" yaml:"scenarios,omitempty"`
}

// Board declares one simulated board.
type Board struct {
	ID        string              `json:"id" yaml:"id"`
	Name      string              `json:"name,omitempty" yaml:"name,omitempty"`
	Profile   string              `json:"profile,omitempty" yaml:"profile,omitempty"`
	Platform  string              `json:"platform,omitempty" yaml:"platform,omitempty"`
	Port      string              `json:"port,omitempty" yaml:"port,omitempty"`
	Latency   string              `json:"latency,omitempty" yaml:"latency,omitempty"`
	Pins      []Pin               `json:"pins,omitempty" yaml:"pins,omitempty"`
	Devices   []Device            `json:"devices,omitempty" yaml:"devices,omitempty"`
	Chips     []Chip              `json:"chips,omitempty" yaml:"chips,omitempty"`
	Values    map[string]any      `json:"values,omitempty" yaml:"values,omitempty"`
	Functions map[string]Function `json:"functions,omitempty" yaml:"functions,omitempty"`
}

// Pin declares pin metadata.
type Pin struct {
	Pin    string        `json:"pin" yaml:"pin"`
	Label  string        `json:"label,omitempty" yaml:"label,omitempty"`
	Mode   state.PinMode `json:"mode,omitempty" yaml:"mode,omitempty"`
	Pull   state.Pull    `json:"pull,omitempty" yaml:"pull,omitempty"`
	Invert bool          `json:"invert,omitempty" yaml:"invert,omitempty"`
}

// Device declares one device binding.
type Device struct {
	ID           string            `json:"id" yaml:"id"`
	Name         string            `json:"name,omitempty" yaml:"name,omitempty"`
	Label        string            `json:"label,omitempty" yaml:"label,omitempty"`
	Kind         state.DeviceKind  `json:"kind" yaml:"kind"`
	DisplayModel string            `json:"displayModel,omitempty" yaml:"displayModel,omitempty"`
	Pin          string            `json:"pin,omitempty" yaml:"pin,omitempty"`
	Pins         []string          `json:"pins,omitempty" yaml:"pins,omitempty"`
	Wiring       state.Wiring      `json:"wiring,omitempty" yaml:"wiring,omitempty"`
	DriverHints  map[string]string `json:"driverHints,omitempty" yaml:"driverHints,omitempty"`
	ValueName    string            `json:"valueName,omitempty" yaml:"valueName,omitempty"`
	FunctionName string            `json:"functionName,omitempty" yaml:"functionName,omitempty"`
	Value        any               `json:"value,omitempty" yaml:"value,omitempty"`
	Function     *int              `json:"function,omitempty" yaml:"function,omitempty"`
	DefaultState *int              `json:"defaultState,omitempty" yaml:"defaultState,omitempty"`
	Inverted     *bool             `json:"inverted,omitempty" yaml:"inverted,omitempty"`
	Labels       map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	State        map[string]any    `json:"state,omitempty" yaml:"state,omitempty"`
}

// Chip declares one modelled bus chip.
type Chip struct {
	ID      string         `json:"id" yaml:"id"`
	Bus     string         `json:"bus" yaml:"bus"`
	Address int            `json:"address,omitempty" yaml:"address,omitempty"`
	Model   string         `json:"model" yaml:"model"`
	Options map[string]any `json:"options,omitempty" yaml:"options,omitempty"`
	Faults  []string       `json:"faults,omitempty" yaml:"faults,omitempty"`
}

// Function declares an aREST function return code.
type Function struct {
	ReturnCode int `json:"returnCode" yaml:"returnCode"`
}

// Loaded is a parsed topology plus its original bytes and source path.
type Loaded struct {
	Spec   Spec
	Source []byte
	Path   string
}

// Load parses and validates a topology document.
func Load(r io.Reader) (*Loaded, error) {
	if r == nil {
		return nil, errors.New("topology: nil reader")
	}
	source, err := io.ReadAll(io.LimitReader(r, MaxTopologySize+1))
	if err != nil {
		return nil, fmt.Errorf("topology: read: %w", err)
	}
	if len(source) > MaxTopologySize {
		return nil, fmt.Errorf("topology: document exceeds %d bytes", MaxTopologySize)
	}
	return Parse(source)
}

// Parse parses raw topology bytes.
func Parse(source []byte) (*Loaded, error) {
	var spec Spec
	decoder := yaml.NewDecoder(bytes.NewReader(source))
	decoder.KnownFields(true)
	if err := decoder.Decode(&spec); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("topology: empty document")
		}
		return nil, fmt.Errorf("topology: parse: %w", err)
	}
	if spec.Schema == "" {
		spec.Schema = SchemaVersion
	}
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	return &Loaded{Spec: spec, Source: append([]byte(nil), source...)}, nil
}

// LoadFile reads, parses and validates a topology file. The path is cleaned
// and must be a regular file; it is never resolved relative to user input.
func LoadFile(path string) (*Loaded, error) {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" || clean == "." {
		return nil, errors.New("topology: empty path")
	}
	info, err := os.Stat(clean)
	if err != nil {
		return nil, fmt.Errorf("topology: stat %q: %w", clean, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("topology: %q is a directory", clean)
	}
	file, err := os.Open(clean)
	if err != nil {
		return nil, fmt.Errorf("topology: open %q: %w", clean, err)
	}
	defer func() { _ = file.Close() }()
	loaded, err := Load(file)
	if err != nil {
		return nil, err
	}
	loaded.Path = clean
	return loaded, nil
}

// Validate returns path-qualified validation errors.
func (s Spec) Validate() error {
	var errs []error
	if s.Schema != SchemaVersion {
		errs = append(errs, fmt.Errorf("schema: unsupported version %q (want %q)", s.Schema, SchemaVersion))
	}
	if len(s.Boards) == 0 {
		errs = append(errs, errors.New("boards: at least one board is required"))
	}
	seenBoards := make(map[string]struct{}, len(s.Boards))
	for i, board := range s.Boards {
		path := fmt.Sprintf("boards[%d]", i)
		if err := state.ValidateIdentifier("board", board.ID); err != nil {
			errs = append(errs, fmt.Errorf("%s.id: %w", path, err))
		}
		if _, ok := seenBoards[board.ID]; ok {
			errs = append(errs, fmt.Errorf("%s.id: duplicate board %q", path, board.ID))
		}
		seenBoards[board.ID] = struct{}{}
		if board.Latency != "" {
			if _, err := time.ParseDuration(board.Latency); err != nil {
				errs = append(errs, fmt.Errorf("%s.latency: invalid duration %q", path, board.Latency))
			}
		}
		errs = append(errs, validateBoard(board, path)...)
	}
	seenRules := make(map[string]struct{}, len(s.Scenarios))
	for i, rule := range s.Scenarios {
		path := fmt.Sprintf("scenarios[%d]", i)
		if _, ok := seenRules[rule.ID]; ok {
			errs = append(errs, fmt.Errorf("%s.id: duplicate scenario %q", path, rule.ID))
		}
		seenRules[rule.ID] = struct{}{}
		if err := rule.Validate(path); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func validateBoard(board Board, path string) []error {
	var errs []error
	seenPins := make(map[string]struct{}, len(board.Pins))
	for i, pin := range board.Pins {
		pinPath := fmt.Sprintf("%s.pins[%d]", path, i)
		if err := state.ValidateIdentifier("pin", pin.Pin); err != nil {
			errs = append(errs, fmt.Errorf("%s.pin: %w", pinPath, err))
		}
		seenPins[pin.Pin] = struct{}{}
		if pin.Mode != "" && !pin.Mode.Valid() {
			errs = append(errs, fmt.Errorf("%s.mode: unknown value %q", pinPath, pin.Mode))
		}
		if pin.Pull != "" && pin.Pull != state.PullNone && pin.Pull != state.PullUp &&
			pin.Pull != state.PullDown && pin.Pull != state.PullExternal {
			errs = append(errs, fmt.Errorf("%s.pull: unknown value %q", pinPath, pin.Pull))
		}
	}
	seenDevices := make(map[string]struct{}, len(board.Devices))
	for i, device := range board.Devices {
		devicePath := fmt.Sprintf("%s.devices[%d]", path, i)
		if err := state.ValidateIdentifier("device", device.ID); err != nil {
			errs = append(errs, fmt.Errorf("%s.id: %w", devicePath, err))
		}
		if _, ok := seenDevices[device.ID]; ok {
			errs = append(errs, fmt.Errorf("%s.id: duplicate device %q", devicePath, device.ID))
		}
		seenDevices[device.ID] = struct{}{}
		if !device.Kind.Valid() {
			errs = append(errs, fmt.Errorf("%s.kind: unknown value %q", devicePath, device.Kind))
		}
		for j, pin := range devicePins(device) {
			if err := state.ValidateIdentifier("pin", pin); err != nil {
				errs = append(errs, fmt.Errorf("%s.pins[%d]: %w", devicePath, j, err))
			}
		}
		if err := device.Wiring.Validate(devicePath + ".wiring"); err != nil {
			errs = append(errs, err)
		}
		if device.Kind == state.DeviceKindValue && device.ValueName == "" {
			errs = append(errs, fmt.Errorf("%s.valueName: required for value devices", devicePath))
		}
		if device.Kind == state.DeviceKindFunction && device.FunctionName == "" {
			errs = append(errs, fmt.Errorf("%s.functionName: required for function devices", devicePath))
		}
	}
	for i, chip := range board.Chips {
		chipPath := fmt.Sprintf("%s.chips[%d]", path, i)
		if err := state.ValidateIdentifier("chip", chip.ID); err != nil {
			errs = append(errs, fmt.Errorf("%s.id: %w", chipPath, err))
		}
		if chip.Bus == "" {
			errs = append(errs, fmt.Errorf("%s.bus: required", chipPath))
		}
		if chip.Model == "" {
			errs = append(errs, fmt.Errorf("%s.model: required", chipPath))
		}
		for _, fault := range chip.Faults {
			if _, err := state.ParseDeviceFault(fault); err != nil {
				errs = append(errs, fmt.Errorf("%s.faults: %w", chipPath, err))
			}
		}
	}
	for name := range board.Values {
		if err := state.ValidateIdentifier("value", name); err != nil {
			errs = append(errs, fmt.Errorf("%s.values.%s: %w", path, name, err))
		}
	}
	for name := range board.Functions {
		if err := state.ValidateIdentifier("function", name); err != nil {
			errs = append(errs, fmt.Errorf("%s.functions.%s: %w", path, name, err))
		}
	}
	_ = seenPins
	return errs
}

func devicePins(device Device) []string {
	seen := make(map[string]struct{}, len(device.Pins)+1)
	var out []string
	for _, pin := range append(append([]string{}, device.Pins...), device.Pin) {
		if pin == "" {
			continue
		}
		if _, ok := seen[pin]; ok {
			continue
		}
		seen[pin] = struct{}{}
		out = append(out, pin)
	}
	return out
}
