package simulate

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/disaster37/gobot-simulate/internal/bus"
	"github.com/disaster37/gobot-simulate/internal/scenario"
	"github.com/disaster37/gobot-simulate/internal/state"
	"github.com/disaster37/gobot-simulate/internal/topology"
)

// LoadTopology parses a topology document without applying it.
func LoadTopology(r io.Reader) (*topology.Loaded, error) { return topology.Load(r) }

// LoadTopologyFile parses a topology file without applying it.
func LoadTopologyFile(path string) (*topology.Loaded, error) { return topology.LoadFile(path) }

// LoadTopology parses and applies a topology document.
func (s *Simulator) LoadTopology(r io.Reader) error {
	loaded, err := topology.Load(r)
	if err != nil {
		return err
	}
	return s.ApplyTopology(loaded)
}

// LoadTopologyFile parses and applies a topology file.
func (s *Simulator) LoadTopologyFile(path string) error {
	loaded, err := topology.LoadFile(path)
	if err != nil {
		return err
	}
	return s.ApplyTopology(loaded)
}

// ApplyTopology applies a parsed topology. The topology file wins over later
// Go hints and AutoDiscover inference.
func (s *Simulator) ApplyTopology(loaded *topology.Loaded) error {
	if loaded == nil {
		return fmt.Errorf("topology: nil document")
	}
	for _, boardSpec := range loaded.Spec.Boards {
		latency := time.Duration(0)
		if boardSpec.Latency != "" {
			parsed, err := time.ParseDuration(boardSpec.Latency)
			if err != nil {
				return fmt.Errorf("topology: boards.%s.latency: %w", boardSpec.ID, err)
			}
			latency = parsed
		}
		platform := boardSpec.Platform
		if platform == "" {
			platform = boardSpec.Profile
		}
		adaptor := s.Adaptor(boardSpec.ID,
			WithAdaptorName(firstNonEmptyString(boardSpec.Name, boardSpec.ID)),
			WithPlatform(platform),
			WithAdaptorPort(boardSpec.Port),
			WithSimulatedLatency(latency),
		)
		for _, pin := range boardSpec.Pins {
			mode := pin.Mode
			if mode == "" {
				mode = state.PinModeUnset
			}
			if pin.Invert {
				adaptor.board.SetInvertInitialPinState(pin.Pin)
			}
			if pin.Pull == state.PullUp || mode == state.PinModeInputPullup {
				adaptor.board.SetInputPullup(pin.Pin)
			}
			if err := adaptor.board.BindPin(pin.Pin, pin.Label, mode); err != nil {
				return fmt.Errorf("topology: boards.%s.pins.%s: %w", boardSpec.ID, pin.Pin, err)
			}
		}
		for name, value := range boardSpec.Values {
			adaptor.board.SetValue(name, value, state.SourceSimulation)
		}
		for name, function := range boardSpec.Functions {
			adaptor.board.SetFunctionCode(name, function.ReturnCode, state.SourceSimulation)
		}
		for _, device := range boardSpec.Devices {
			spec := deviceFromTopology(device)
			if _, err := s.RegisterDevice(boardSpec.ID, spec); err != nil {
				return fmt.Errorf("topology: boards.%s.devices.%s: %w", boardSpec.ID, device.ID, err)
			}
		}
		if err := s.applyChips(adaptor, boardSpec); err != nil {
			return err
		}
	}
	if previous := s.currentScenarios(); previous != nil {
		previous.Stop()
	}
	engine := scenario.New(loaded.Spec.Scenarios, s, s.clk)
	s.scenarioMu.Lock()
	s.scenarios = engine
	s.scenarioMu.Unlock()
	if s.isRunning() {
		engine.Start(s.runtimeContext())
	}
	s.topologyMu.Lock()
	s.topology = loaded
	s.topologyMu.Unlock()
	return nil
}

func deviceFromTopology(device topology.Device) DeviceSpec {
	kind := device.Kind
	if !kind.Valid() {
		kind = state.DeviceKindValue
	}
	spec := DeviceSpec{
		ID:           device.ID,
		Name:         firstNonEmptyString(device.Name, device.ID),
		Label:        firstNonEmptyString(device.Label, device.Name, device.ID),
		Kind:         kind,
		Pin:          device.Pin,
		Pins:         append([]string(nil), device.Pins...),
		ValueName:    device.ValueName,
		FunctionName: device.FunctionName,
		DefaultState: device.DefaultState,
		Wiring:       device.Wiring,
		Labels:       device.Labels,
		State:        device.State,
		Source:       state.SourceTopology,
	}
	if device.Inverted != nil {
		spec.Inverted = *device.Inverted
	}
	if device.Value != nil {
		if spec.State == nil {
			spec.State = make(map[string]interface{})
		}
		spec.State["value"] = device.Value
	}
	return spec
}

func (s *Simulator) applyChips(adaptor *Adaptor, board topology.Board) error {
	for _, chipSpec := range board.Chips {
		if err := s.applyChip(adaptor, board.ID, chipSpec); err != nil {
			return err
		}
	}
	return nil
}

// AddChip attaches a chip model to a board at runtime. It is additive to the
// topology file and uses the same model registry and validation.
func (s *Simulator) AddChip(boardID string, chip topology.Chip) error {
	adaptor, err := s.adaptorFor(boardID)
	if err != nil {
		return err
	}
	return s.applyChip(adaptor, adaptor.board.ID(), chip)
}

func (s *Simulator) applyChip(adaptor *Adaptor, boardID string, chipSpec topology.Chip) error {
	if err := state.ValidateIdentifier("chip", chipSpec.ID); err != nil {
		return err
	}
	transport, busNumber, address, err := parseBusRef(chipSpec)
	if err != nil {
		return fmt.Errorf("topology: boards.%s.chips.%s: %w", boardID, chipSpec.ID, err)
	}
	model, err := bus.NewChip(chipSpec.Model, bus.ChipConfig{
		BoardID:   boardID,
		ID:        chipSpec.ID,
		Transport: transport,
		Bus:       busNumber,
		Address:   address,
		Options:   chipSpec.Options,
	})
	if err != nil {
		return fmt.Errorf("topology: boards.%s.chips.%s: %w", boardID, chipSpec.ID, err)
	}
	faults := make([]state.DeviceFault, 0, len(chipSpec.Faults))
	for _, raw := range chipSpec.Faults {
		fault, err := state.ParseDeviceFault(raw)
		if err != nil {
			return fmt.Errorf("topology: boards.%s.chips.%s: %w", boardID, chipSpec.ID, err)
		}
		faults = append(faults, fault)
		if fault == state.FaultBusNAK || fault == state.FaultBusTimeout {
			_ = adaptor.buses.SetBusFault(chipSpec.Bus, raw)
		}
	}
	entry := &bus.ChipEntry{
		ID:         chipSpec.ID,
		BoardID:    boardID,
		Transport:  transport,
		Bus:        chipSpec.Bus,
		BusNumber:  busNumber,
		Address:    address,
		Model:      chipSpec.Model,
		Descriptor: model.Describe(),
		Faults:     faults,
	}
	if transport == "onewire" {
		if sensor, ok := model.(*bus.DS18B20); ok {
			adaptor.buses.OneWire().AddDS18B20(sensor.SerialNumber())
		}
	}
	adaptor.buses.AddChipWithModel(entry, model)
	return nil
}

func parseBusRef(chip topology.Chip) (transport string, busNumber, address int, err error) {
	raw := strings.ToLower(strings.TrimSpace(chip.Bus))
	switch {
	case strings.HasPrefix(raw, "i2c"):
		transport = "i2c"
		busNumber = parseBusNumber(raw, "i2c", 1)
		address = chip.Address
		if address == 0 {
			return "", 0, 0, fmt.Errorf("i2c chips require an address")
		}
	case strings.HasPrefix(raw, "spi"):
		transport = "spi"
		busNumber = parseBusNumber(raw, "spi", 0)
		address = chip.Address
	case strings.HasPrefix(raw, "onewire") || raw == "1wire":
		transport = "onewire"
	case strings.HasPrefix(raw, "serial"):
		transport = "serial"
	case strings.HasPrefix(raw, "ble"):
		transport = "ble"
	case strings.HasPrefix(raw, "gpio"):
		transport = "gpio"
	default:
		return "", 0, 0, fmt.Errorf("unknown bus %q", chip.Bus)
	}
	return transport, busNumber, address, nil
}

func parseBusNumber(raw, prefix string, fallback int) int {
	trimmed := strings.TrimPrefix(raw, prefix)
	trimmed = strings.TrimLeft(trimmed, "-_/")
	if trimmed == "" {
		return fallback
	}
	if idx := strings.IndexAny(trimmed, "-/"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil {
		return fallback
	}
	return value
}

// Topology returns the effective topology (post-merge) and, when available,
// the raw source document.
func (s *Simulator) Topology() (topology.Spec, []byte) {
	s.topologyMu.RLock()
	loaded := s.topology
	s.topologyMu.RUnlock()
	spec := s.effectiveTopology()
	if loaded != nil && len(loaded.Source) > 0 {
		return spec, append([]byte(nil), loaded.Source...)
	}
	data, _ := json.MarshalIndent(spec, "", "  ")
	return spec, data
}

// TopologySource returns the loaded source bytes, if any.
func (s *Simulator) TopologySource() []byte {
	s.topologyMu.RLock()
	defer s.topologyMu.RUnlock()
	if s.topology == nil {
		return nil
	}
	return append([]byte(nil), s.topology.Source...)
}

func (s *Simulator) effectiveTopology() topology.Spec {
	spec := topology.Spec{Schema: topology.SchemaVersion}
	for _, board := range s.store.Boards() {
		boardSpec := topology.Board{
			ID:        board.ID,
			Name:      board.Name,
			Platform:  board.Platform,
			Port:      board.Port,
			Latency:   (time.Duration(board.SimulatedLagMs) * time.Millisecond).String(),
			Pins:      make([]topology.Pin, 0, len(board.Pins)),
			Devices:   make([]topology.Device, 0, len(board.Devices)),
			Values:    make(map[string]interface{}, len(board.Values)),
			Functions: make(map[string]topology.Function, len(board.Functions)),
		}
		for _, pin := range board.Pins {
			boardSpec.Pins = append(boardSpec.Pins, topology.Pin{
				Pin:    pin.Pin,
				Label:  pin.Label,
				Mode:   pin.Mode,
				Pull:   pinPull(pin),
				Invert: pin.InvertInitial,
			})
		}
		for _, device := range board.Devices {
			boardSpec.Devices = append(boardSpec.Devices, topology.Device{
				ID:           device.ID,
				Name:         device.Name,
				Label:        device.Label,
				Kind:         device.Kind,
				Pins:         device.Pins,
				Wiring:       device.Wiring,
				ValueName:    device.ValueName,
				FunctionName: device.FunctionName,
				DefaultState: intPtrValue(device.DefaultState),
				Inverted:     boolPtrValue(device.Inverted),
				Labels:       device.Labels,
				State:        device.State,
			})
		}
		for name, value := range board.Values {
			boardSpec.Values[name] = value.Value
		}
		for name, function := range board.Functions {
			boardSpec.Functions[name] = topology.Function{ReturnCode: function.ReturnCode}
		}
		if adaptor := s.adaptorByID(board.ID); adaptor != nil {
			for _, chip := range adaptor.buses.Chips() {
				boardSpec.Chips = append(boardSpec.Chips, topology.Chip{
					ID:      chip.ID,
					Bus:     chip.Bus,
					Address: chip.Address,
					Model:   chip.Model,
				})
			}
		}
		spec.Boards = append(spec.Boards, boardSpec)
	}
	if s.scenarios != nil {
		spec.Scenarios = s.scenarios.Rules()
	}
	return spec
}

func pinPull(pin state.PinState) state.Pull {
	if pin.Pullup {
		return state.PullUp
	}
	return state.PullNone
}

func (s *Simulator) adaptorByID(id string) *Adaptor {
	s.adaptorsMu.Lock()
	defer s.adaptorsMu.Unlock()
	return s.adaptors[id]
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func intPtrValue(v int) *int { return &v }

func boolPtrValue(v bool) *bool { return &v }
