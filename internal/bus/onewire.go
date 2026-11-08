package bus

import (
	"fmt"
	"sync"

	gobotOneWire "gobot.io/x/gobot/v2/drivers/onewire"
)

// OneWireBus is a small 1-Wire network backed by modelled devices.
type OneWireBus struct {
	mu      sync.RWMutex
	devices map[string]*DS18B20
}

func newOneWireBus() *OneWireBus {
	return &OneWireBus{devices: make(map[string]*DS18B20)}
}

// AddDS18B20 attaches a DS18B20 device.
func (b *OneWireBus) AddDS18B20(serial uint64) *DS18B20 {
	device := NewDS18B20(0x28, serial)
	b.mu.Lock()
	b.devices[device.ID()] = device
	b.mu.Unlock()
	return device
}

// DS18B20 returns a device by serial number, creating it if needed.
func (b *OneWireBus) DS18B20(serial uint64) *DS18B20 {
	id := fmt.Sprintf("%02x-%016x", 0x28, serial)
	b.mu.RLock()
	device, ok := b.devices[id]
	b.mu.RUnlock()
	if ok {
		return device
	}
	return b.AddDS18B20(serial)
}

// GetOneWireConnection implements gobot's onewire connector contract.
func (b *OneWireBus) GetOneWireConnection(familyCode byte, serialNumber uint64) (gobotOneWire.Connection, error) {
	id := fmt.Sprintf("%02x-%016x", familyCode, serialNumber)
	b.mu.RLock()
	device, ok := b.devices[id]
	b.mu.RUnlock()
	if !ok {
		device = b.AddDS18B20(serialNumber)
	}
	return device, nil
}

// DS18B20 is a register-accurate temperature sensor model.
type DS18B20 struct {
	mu             sync.RWMutex
	familyCode     byte
	serial         uint64
	millidegrees   int
	resolution     uint8
	conversionTime uint16
	externalPower  bool
}

// NewDS18B20 creates a DS18B20 model at 25.0 °C with 12-bit resolution.
func NewDS18B20(family byte, serial uint64) *DS18B20 {
	return &DS18B20{
		familyCode:     family,
		serial:         serial,
		millidegrees:   25000,
		resolution:     12,
		conversionTime: 750,
	}
}

// ID returns the family-serial identifier.
func (d *DS18B20) ID() string {
	return fmt.Sprintf("%02x-%016x", d.familyCode, d.serial)
}

// SerialNumber returns the device serial number.
func (d *DS18B20) SerialNumber() uint64 { return d.serial }

// Describe implements ChipModel.
func (d *DS18B20) Describe() ChipDescriptor {
	return ChipDescriptor{
		Model:     "ds18b20",
		Name:      "DS18B20 1-Wire temperature sensor",
		Transport: "onewire",
		Fields: []FieldDescriptor{
			{Name: "temperature", Unit: "degC", Min: -55, Max: 125, Widget: "number"},
			{Name: "resolution", Unit: "bit", Min: 9, Max: 12, Widget: "number"},
			{Name: "conversionTime", Unit: "ms", Min: 93, Max: 750, Widget: "number"},
			{Name: "externalPower", Widget: "switch"},
		},
	}
}

// Transfer implements ChipModel for the 1-Wire command vocabulary.
func (d *DS18B20) Transfer(txn Transaction) (Response, error) {
	if len(txn.Write) > 0 {
		command := string(txn.Write)
		data := txn.Write
		if err := d.WriteData(command, data); err != nil {
			return Response{}, err
		}
	}
	if txn.ReadLen == 0 {
		return Response{}, nil
	}
	command := "temperature"
	data := make([]byte, txn.ReadLen)
	if err := d.ReadData(command, data); err != nil {
		return Response{}, err
	}
	return Response{Data: data, Values: d.Values()}, nil
}

// Set implements ChipModel.
func (d *DS18B20) Set(field string, value interface{}) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch field {
	case "temperature", "temperatureC":
		switch v := value.(type) {
		case float64:
			d.millidegrees = int(v * 1000)
		case int:
			d.millidegrees = v * 1000
		case float32:
			d.millidegrees = int(float64(v) * 1000)
		default:
			return fmt.Errorf("simulate: invalid temperature %v", value)
		}
	case "millidegrees":
		d.millidegrees = intValue(value)
	case "resolution":
		res := intValue(value)
		if res < 9 || res > 12 {
			return fmt.Errorf("simulate: resolution must be 9..12")
		}
		d.resolution = uint8(res)
	case "conversionTime", "convTime":
		d.conversionTime = uint16(intValue(value)) //nolint:gosec // intentional width conversion
	case "externalPower":
		if v, ok := value.(bool); ok {
			d.externalPower = v
		}
	default:
		return fmt.Errorf("simulate: ds18b20 has no field %q", field)
	}
	return nil
}

// Values implements ValuesProvider.
func (d *DS18B20) Values() map[string]interface{} {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return map[string]interface{}{
		"temperature":    float64(d.millidegrees) / 1000,
		"millidegrees":   d.millidegrees,
		"resolution":     int(d.resolution),
		"conversionTime": int(d.conversionTime),
		"externalPower":  d.externalPower,
	}
}

// ReadData implements gobot.OneWireOperations.
func (d *DS18B20) ReadData(command string, data []byte) error {
	value, err := d.ReadInteger(command)
	if err != nil {
		return err
	}
	for i := range data {
		data[i] = byte(value >> (8 * i)) //nolint:gosec // intentional width conversion
	}
	return nil
}

// WriteData implements gobot.OneWireOperations.
func (d *DS18B20) WriteData(command string, data []byte) error {
	value := 0
	for i := len(data) - 1; i >= 0; i-- {
		value = value<<8 | int(data[i])
	}
	return d.WriteInteger(command, value)
}

// ReadInteger implements gobot.OneWireOperations.
func (d *DS18B20) ReadInteger(command string) (int, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	switch command {
	case "temperature", "":
		return d.millidegrees, nil
	case "resolution":
		return int(d.resolution), nil
	case "ext_power":
		if d.externalPower {
			return 1, nil
		}
		return 0, nil
	case "conv_time":
		return int(d.conversionTime), nil
	default:
		return 0, fmt.Errorf("simulate: unknown ds18b20 command %q", command)
	}
}

// WriteInteger implements gobot.OneWireOperations.
func (d *DS18B20) WriteInteger(command string, val int) error {
	switch command {
	case "temperature":
		return d.Set("millidegrees", val)
	case "resolution":
		return d.Set("resolution", val)
	case "ext_power":
		return d.Set("externalPower", val != 0)
	case "conv_time":
		return d.Set("conversionTime", val)
	default:
		return fmt.Errorf("simulate: unknown ds18b20 command %q", command)
	}
}

// Close implements gobot.OneWireOperations.
func (d *DS18B20) Close() error { return nil }

// AddDS18B20Chip registers a DS18B20 as a chip entry too.
func (m *Manager) AddDS18B20Chip(id string, serial uint64) *DS18B20 {
	device := m.onewire.AddDS18B20(serial)
	m.AddChip(&ChipEntry{
		ID:         id,
		BoardID:    m.boardID,
		Transport:  "onewire",
		Bus:        "onewire",
		Model:      "ds18b20",
		Descriptor: device.Describe(),
		chip:       device,
	})
	return device
}

func init() {
	Register("ds18b20", func(cfg ChipConfig) (ChipModel, error) {
		serial := uint64(1)
		if raw, ok := cfg.Options["serial"]; ok {
			serial = uint64(intValue(raw)) //nolint:gosec // intentional width conversion
		}
		return NewDS18B20(0x28, serial), nil
	})
}

func intValue(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case float32:
		return int(n)
	case byte:
		return int(n)
	case bool:
		if n {
			return 1
		}
		return 0
	default:
		return 0
	}
}

// recordOneWire records a 1-Wire transaction.
