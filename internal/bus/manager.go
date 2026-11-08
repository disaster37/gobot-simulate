// Package bus implements addressable simulated I2C, SPI, 1-Wire, serial and
// BLE transports backed by register-accurate chip models.
package bus

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/disaster37/gobot-simulate/internal/clock"
	"github.com/disaster37/gobot-simulate/internal/state"
	"github.com/disaster37/gobot-simulate/internal/trace"
)

// ErrNAK is returned when no chip acknowledges an address.
var ErrNAK = errors.New("simulate: i2c address not acknowledged")

// ErrTimeout is returned when a simulated bus timeout is injected.
var ErrTimeout = errors.New("simulate: bus timeout")

// ErrBusNotFound is returned for an unknown bus.
var ErrBusNotFound = errors.New("simulate: bus not found")

// ErrChipNotFound is returned for an unknown chip.
var ErrChipNotFound = errors.New("simulate: chip not found")

// RegisterDescriptor documents one register.
type RegisterDescriptor struct {
	Address int               `json:"address"`
	Name    string            `json:"name"`
	Width   int               `json:"width"`
	Access  string            `json:"access,omitempty"`
	Reset   uint32            `json:"reset,omitempty"`
	Fields  []FieldDescriptor `json:"fields,omitempty"`
}

// FieldDescriptor documents a value derived from one or more registers.
type FieldDescriptor struct {
	Name   string  `json:"name"`
	Bits   string  `json:"bits,omitempty"`
	Unit   string  `json:"unit,omitempty"`
	Min    float64 `json:"min,omitempty"`
	Max    float64 `json:"max,omitempty"`
	Widget string  `json:"widget,omitempty"`
}

// ChipDescriptor describes a chip model to the UI and API.
type ChipDescriptor struct {
	Model     string               `json:"model"`
	Name      string               `json:"name"`
	Transport string               `json:"transport"`
	Address   int                  `json:"address,omitempty"`
	Registers []RegisterDescriptor `json:"registers,omitempty"`
	Fields    []FieldDescriptor    `json:"fields,omitempty"`
	Widget    string               `json:"widget,omitempty"`
}

// Transaction is one bus operation presented to a chip model.
type Transaction struct {
	Transport string `json:"transport,omitempty"`
	Bus       int    `json:"bus,omitempty"`
	Address   int    `json:"address,omitempty"`
	Write     []byte `json:"write,omitempty"`
	ReadLen   int    `json:"readLen,omitempty"`
}

// Response is the result of a chip transaction.
type Response struct {
	Data   []byte                 `json:"data,omitempty"`
	Values map[string]interface{} `json:"values,omitempty"`
}

// ChipModel is the contract every simulated chip implements.
type ChipModel interface {
	Describe() ChipDescriptor
	Transfer(txn Transaction) (Response, error)
	Set(field string, value interface{}) error
}

// ValuesProvider is optionally implemented by chips exposing derived fields.
type ValuesProvider interface {
	Values() map[string]interface{}
}

// Factory constructs a chip model from topology options.
type Factory func(cfg ChipConfig) (ChipModel, error)

// ChipConfig carries construction options.
type ChipConfig struct {
	BoardID   string
	ID        string
	Transport string
	Bus       int
	Address   int
	Options   map[string]interface{}
}

var (
	registryMu sync.RWMutex
	registry   = make(map[string]Factory)
)

// Register adds a chip model factory to the global registry.
func Register(model string, factory Factory) {
	registryMu.Lock()
	registry[model] = factory
	registryMu.Unlock()
}

// Models returns every registered model name.
func Models() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]string, 0, len(registry))
	for name := range registry {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// NewChip constructs a registered chip model.
func NewChip(model string, cfg ChipConfig) (ChipModel, error) {
	registryMu.RLock()
	factory, ok := registry[strings.ToLower(strings.TrimSpace(model))]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrChipNotFound, model)
	}
	return factory(cfg)
}

// ChipEntry is a chip plus its wiring metadata.
type ChipEntry struct {
	ID         string              `json:"id"`
	BoardID    string              `json:"boardId"`
	Transport  string              `json:"transport"`
	Bus        string              `json:"bus"`
	BusNumber  int                 `json:"busNumber,omitempty"`
	Address    int                 `json:"address,omitempty"`
	Model      string              `json:"model"`
	Descriptor ChipDescriptor      `json:"descriptor"`
	Faults     []state.DeviceFault `json:"faults,omitempty"`
	chip       ChipModel
}

// Manager owns all bus transports for one board.
type Manager struct {
	boardID string
	clk     clock.Clock
	traces  *trace.Store

	mu        sync.RWMutex
	i2c       map[int]*I2CBus
	spi       map[string]*SPIBus
	onewire   *OneWireBus
	serial    map[string]*SerialPort
	ble       *BLEDevice
	chips     map[string]*ChipEntry
	busFaults map[string]map[state.DeviceFault]bool
}

// NewManager creates an empty bus manager for a board.
func NewManager(boardID string, clk clock.Clock, traces *trace.Store) *Manager {
	if clk == nil {
		clk = clock.RealClock{}
	}
	return &Manager{
		boardID:   boardID,
		clk:       clk,
		traces:    traces,
		i2c:       make(map[int]*I2CBus),
		spi:       make(map[string]*SPIBus),
		serial:    make(map[string]*SerialPort),
		onewire:   newOneWireBus(),
		ble:       newBLEDevice(),
		chips:     make(map[string]*ChipEntry),
		busFaults: make(map[string]map[state.DeviceFault]bool),
	}
}

func (m *Manager) record(kind trace.Kind, target, operation string, value interface{}, source state.EventSource, started time.Time, err error) {
	if m.traces == nil {
		return
	}
	rec := trace.Record{
		BoardID:    m.boardID,
		Kind:       kind,
		Target:     target,
		Operation:  operation,
		Value:      value,
		Source:     source,
		DurationNs: m.clk.Since(started).Nanoseconds(),
	}
	if err != nil {
		rec.Error = err.Error()
	}
	m.traces.Append(rec)
}

func (m *Manager) busFault(target string, fault state.DeviceFault) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	faults, ok := m.busFaults[target]
	if !ok {
		return false
	}
	return faults[fault]
}

// SetBusFault injects a bus-level fault on an i2c/spi/onewire/serial/ble bus.
func (m *Manager) SetBusFault(busID, fault string) error {
	f, err := state.ParseDeviceFault(fault)
	if err != nil {
		return err
	}
	if f != state.FaultBusNAK && f != state.FaultBusTimeout {
		return fmt.Errorf("%w: %q is not a bus fault", state.ErrInvalidFault, fault)
	}
	m.mu.Lock()
	if m.busFaults[busID] == nil {
		m.busFaults[busID] = make(map[state.DeviceFault]bool)
	}
	m.busFaults[busID][f] = true
	m.mu.Unlock()
	return nil
}

// ClearBusFault removes a bus-level fault.
func (m *Manager) ClearBusFault(busID, fault string) error {
	f, err := state.ParseDeviceFault(fault)
	if err != nil {
		return err
	}
	m.mu.Lock()
	if faults, ok := m.busFaults[busID]; ok {
		delete(faults, f)
		if len(faults) == 0 {
			delete(m.busFaults, busID)
		}
	}
	m.mu.Unlock()
	return nil
}

// I2C returns the addressed bus, creating it on first use.
func (m *Manager) I2C(busNr int) *I2CBus {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.i2c[busNr]
	if !ok {
		b = &I2CBus{manager: m, number: busNr, devices: make(map[int]ChipModel)}
		m.i2c[busNr] = b
	}
	return b
}

// DefaultI2CBus returns the lowest configured I2C bus number.
func (m *Manager) DefaultI2CBus() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.i2c) == 0 {
		return 1
	}
	best := 0
	first := true
	for number := range m.i2c {
		if first || number < best {
			best = number
			first = false
		}
	}
	return best
}

// AddI2CChip attaches a chip model at an address.
func (m *Manager) AddI2CChip(busNr, address int, model ChipModel) {
	m.I2C(busNr).devices[address] = model
}

// AddSPIChip attaches a SPI chip model for a bus/chip pair.
func (m *Manager) AddSPIChip(busNr, chip int, model ChipModel) *SPIBus {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%d/%d", busNr, chip)
	sb, ok := m.spi[key]
	if !ok {
		sb = &SPIBus{manager: m, bus: busNr, chip: chip, model: model}
		m.spi[key] = sb
	}
	sb.model = model
	return sb
}

// SPIConnection returns the SPI system device for a bus/chip pair.
func (m *Manager) SPIConnection(busNr, chip int) *SPIBus {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%d/%d", busNr, chip)
	sb, ok := m.spi[key]
	if !ok {
		sb = &SPIBus{manager: m, bus: busNr, chip: chip}
		m.spi[key] = sb
	}
	return sb
}

// OneWire exposes the 1-Wire bus.
func (m *Manager) OneWire() *OneWireBus { return m.onewire }

// Serial returns a named serial port.
func (m *Manager) Serial(port string) *SerialPort {
	if port == "" {
		port = "default"
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.serial[port]
	if !ok {
		p = newSerialPort(port)
		m.serial[port] = p
	}
	return p
}

// BLE exposes the generic BLE peripheral.
func (m *Manager) BLE() *BLEDevice { return m.ble }

// AddChip registers a chip entry.
func (m *Manager) AddChip(entry *ChipEntry) {
	m.AddChipWithModel(entry, entry.chip)
}

// AddChipWithModel registers a chip entry with an explicit model.
func (m *Manager) AddChipWithModel(entry *ChipEntry, model ChipModel) {
	entry.chip = model
	m.mu.Lock()
	m.chips[entry.ID] = entry
	m.mu.Unlock()
	switch entry.Transport {
	case "i2c":
		m.AddI2CChip(entry.BusNumber, entry.Address, model)
	case "spi":
		m.AddSPIChip(entry.BusNumber, entry.Address, model)
	}
}

// Chips returns chip entries sorted by ID.
func (m *Manager) Chips() []ChipEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ChipEntry, 0, len(m.chips))
	for _, entry := range m.chips {
		out = append(out, *entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Chip returns one chip entry.
func (m *Manager) Chip(id string) (*ChipEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.chips[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrChipNotFound, id)
	}
	return entry, nil
}

// ChipModel returns the model for a chip ID.
func (m *Manager) ChipModel(id string) (ChipModel, error) {
	entry, err := m.Chip(id)
	if err != nil {
		return nil, err
	}
	return entry.chip, nil
}

// Buses returns a description of configured buses.
func (m *Manager) Buses() []map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]map[string]interface{}, 0, len(m.i2c)+len(m.spi))
	for number, b := range m.i2c {
		out = append(out, map[string]interface{}{
			"id":        fmt.Sprintf("i2c-%d", number),
			"transport": "i2c",
			"number":    number,
			"devices":   len(b.devices),
		})
	}
	for key, b := range m.spi {
		out = append(out, map[string]interface{}{
			"id":        "spi-" + strings.ReplaceAll(key, "/", "-"),
			"transport": "spi",
			"bus":       b.bus,
			"chip":      b.chip,
			"hasModel":  b.model != nil,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return fmt.Sprint(out[i]["id"]) < fmt.Sprint(out[j]["id"])
	})
	return out
}
