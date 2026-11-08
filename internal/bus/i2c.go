package bus

import (
	"fmt"
	"sync"

	"github.com/disaster37/gobot-simulate/internal/state"
	"github.com/disaster37/gobot-simulate/internal/trace"
	gobotI2C "gobot.io/x/gobot/v2/drivers/i2c"
)

// I2CBus implements gobot.I2cSystemDevicer over modelled chips.
type I2CBus struct {
	manager *Manager
	number  int

	mu      sync.RWMutex
	devices map[int]ChipModel
}

// Number returns the bus index.
func (b *I2CBus) Number() int { return b.number }

// Connector returns an i2c.Connector rooted at this bus.
func (b *I2CBus) Connector() *Connector {
	return NewConnector(b.manager, b.number)
}

func (b *I2CBus) chip(address int) (ChipModel, error) {
	b.mu.RLock()
	chip, ok := b.devices[address]
	b.mu.RUnlock()
	if ok {
		return chip, nil
	}
	return nil, fmt.Errorf("%w: address 0x%02x on bus %d", ErrNAK, address, b.number)
}

func (b *I2CBus) checkFault(address int) error {
	target := fmt.Sprintf("i2c-%d", b.number)
	if b.manager.busFault(target, state.FaultBusTimeout) {
		return ErrTimeout
	}
	if b.manager.busFault(target, state.FaultBusNAK) {
		return ErrNAK
	}
	return nil
}

func (b *I2CBus) transfer(address int, write []byte, readLen int) ([]byte, error) {
	started := b.manager.clk.Now()
	target := fmt.Sprintf("i2c-%d/0x%02x", b.number, address)
	operation := "write"
	if readLen > 0 {
		operation = "read"
	}
	var response Response
	var err error
	if b.checkFault(address) != nil {
		err = b.checkFault(address)
	} else {
		var chip ChipModel
		chip, err = b.chip(address)
		if err == nil {
			response, err = chip.Transfer(Transaction{
				Transport: "i2c",
				Bus:       b.number,
				Address:   address,
				Write:     write,
				ReadLen:   readLen,
			})
		}
	}
	b.manager.record(trace.KindBusTxn, target, operation, map[string]interface{}{
		"bus":     b.number,
		"address": address,
		"write":   write,
		"readLen": readLen,
		"data":    response.Data,
	}, state.SourceHardwareCode, started, err)
	if err != nil {
		return nil, err
	}
	return response.Data, nil
}

// ReadByteAt reads a single byte from an address.
func (b *I2CBus) ReadByteAt(address int) (byte, error) {
	data, err := b.transfer(address, nil, 1)
	if err != nil || len(data) == 0 {
		return 0, err
	}
	return data[0], nil
}

// ReadByteData implements gobot.I2cSystemDevicer.
func (b *I2CBus) ReadByteData(address int, reg uint8) (uint8, error) {
	data, err := b.transfer(address, []byte{reg}, 1)
	if err != nil || len(data) == 0 {
		return 0, err
	}
	return data[0], nil
}

// ReadWordData implements gobot.I2cSystemDevicer.
func (b *I2CBus) ReadWordData(address int, reg uint8) (uint16, error) {
	data, err := b.transfer(address, []byte{reg}, 2)
	if err != nil || len(data) < 2 {
		return 0, err
	}
	return uint16(data[0]) | uint16(data[1])<<8, nil
}

// ReadBlockData implements gobot.I2cSystemDevicer.
func (b *I2CBus) ReadBlockData(address int, reg uint8, data []byte) error {
	out, err := b.transfer(address, []byte{reg}, len(data))
	if err != nil {
		return err
	}
	copy(data, out)
	return nil
}

// WriteByteAt writes a single byte to an address.
func (b *I2CBus) WriteByteAt(address int, val byte) error {
	_, err := b.transfer(address, []byte{val}, 0)
	return err
}

// WriteByteData implements gobot.I2cSystemDevicer.
func (b *I2CBus) WriteByteData(address int, reg uint8, val uint8) error {
	_, err := b.transfer(address, []byte{reg, val}, 0)
	return err
}

// WriteBlockData implements gobot.I2cSystemDevicer.
func (b *I2CBus) WriteBlockData(address int, reg uint8, data []byte) error {
	write := append([]byte{reg}, data...)
	_, err := b.transfer(address, write, 0)
	return err
}

// WriteWordData implements gobot.I2cSystemDevicer.
func (b *I2CBus) WriteWordData(address int, reg uint8, val uint16) error {
	_, err := b.transfer(address, []byte{reg, uint8(val), uint8(val >> 8)}, 0) //nolint:gosec // intentional width conversion
	return err
}

// WriteBytes implements gobot.I2cSystemDevicer.
func (b *I2CBus) WriteBytes(address int, data []byte) error {
	_, err := b.transfer(address, data, 0)
	return err
}

// Read implements gobot.I2cSystemDevicer.
func (b *I2CBus) Read(address int, buf []byte) (int, error) {
	data, err := b.transfer(address, nil, len(buf))
	if err != nil {
		return 0, err
	}
	return copy(buf, data), nil
}

// Write implements gobot.I2cSystemDevicer.
func (b *I2CBus) Write(address int, data []byte) (int, error) {
	if _, err := b.transfer(address, data, 0); err != nil {
		return 0, err
	}
	return len(data), nil
}

// Close implements gobot.I2cSystemDevicer.
func (b *I2CBus) Close() error { return nil }

// Connector implements gobot's i2c.Connector for an I2C bus.
type Connector struct {
	manager *Manager
	defBus  int
}

// NewConnector creates an i2c connector rooted at the manager.
func NewConnector(manager *Manager, defaultBus int) *Connector {
	return &Connector{manager: manager, defBus: defaultBus}
}

// DefaultI2cBus returns the default bus index.
func (c *Connector) DefaultI2cBus() int { return c.defBus }

// GetI2cConnection implements i2c.Connector.
func (c *Connector) GetI2cConnection(address, busNr int) (gobotI2C.Connection, error) {
	return &DeviceConnection{bus: c.manager.I2C(busNr), address: address}, nil
}

// DeviceConnection is a fixed-address i2c connection. It implements
// gobot.I2cOperations directly so the bus type is not required to expose
// address-taking ReadByte/WriteByte methods (which go vet's stdmethods flags).
type DeviceConnection struct {
	bus     *I2CBus
	address int
}

var _ gobotI2C.Connection = (*DeviceConnection)(nil)

// Read implements io.Reader.
func (c *DeviceConnection) Read(data []byte) (int, error) {
	out, err := c.bus.transfer(c.address, nil, len(data))
	if err != nil {
		return 0, err
	}
	return copy(data, out), nil
}

// Write implements io.Writer.
func (c *DeviceConnection) Write(data []byte) (int, error) {
	if _, err := c.bus.transfer(c.address, data, 0); err != nil {
		return 0, err
	}
	return len(data), nil
}

// Close implements io.Closer.
func (c *DeviceConnection) Close() error { return nil }

// ReadByte reads a byte from the current register.
func (c *DeviceConnection) ReadByte() (byte, error) {
	data, err := c.bus.transfer(c.address, nil, 1)
	if err != nil || len(data) == 0 {
		return 0, err
	}
	return data[0], nil
}

// ReadByteData reads a byte from a register.
func (c *DeviceConnection) ReadByteData(reg uint8) (uint8, error) {
	data, err := c.bus.transfer(c.address, []byte{reg}, 1)
	if err != nil || len(data) == 0 {
		return 0, err
	}
	return data[0], nil
}

// ReadWordData reads a little-endian word from a register.
func (c *DeviceConnection) ReadWordData(reg uint8) (uint16, error) {
	data, err := c.bus.transfer(c.address, []byte{reg}, 2)
	if err != nil || len(data) < 2 {
		return 0, err
	}
	return uint16(data[0]) | uint16(data[1])<<8, nil
}

// ReadBlockData reads a block from a register.
func (c *DeviceConnection) ReadBlockData(reg uint8, data []byte) error {
	out, err := c.bus.transfer(c.address, []byte{reg}, len(data))
	if err != nil {
		return err
	}
	copy(data, out)
	return nil
}

// WriteByte writes a byte to the current register.
func (c *DeviceConnection) WriteByte(val byte) error {
	_, err := c.bus.transfer(c.address, []byte{val}, 0)
	return err
}

// WriteByteData writes a byte to a register.
func (c *DeviceConnection) WriteByteData(reg uint8, val uint8) error {
	_, err := c.bus.transfer(c.address, []byte{reg, val}, 0)
	return err
}

// WriteWordData writes a little-endian word to a register.
func (c *DeviceConnection) WriteWordData(reg uint8, val uint16) error {
	_, err := c.bus.transfer(c.address, []byte{reg, uint8(val), uint8(val >> 8)}, 0) //nolint:gosec // intentional width conversion
	return err
}

// WriteBlockData writes a block to a register.
func (c *DeviceConnection) WriteBlockData(reg uint8, data []byte) error {
	_, err := c.bus.transfer(c.address, append([]byte{reg}, data...), 0)
	return err
}

// WriteBytes writes data starting at the current register.
func (c *DeviceConnection) WriteBytes(data []byte) error {
	_, err := c.bus.transfer(c.address, data, 0)
	return err
}
