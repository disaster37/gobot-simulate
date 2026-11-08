package simulate

import (
	"github.com/disaster37/gobot-simulate/internal/bus"
	"gobot.io/x/gobot/v2"
	gobotI2C "gobot.io/x/gobot/v2/drivers/i2c"
	gobotOneWire "gobot.io/x/gobot/v2/drivers/onewire"
	"gobot.io/x/gobot/v2/drivers/serial"
	gobotSPI "gobot.io/x/gobot/v2/drivers/spi"
)

// BusManager returns the board's bus manager (used by protocol emulation and
// the HTTP chip console).
func (a *Adaptor) BusManager() *bus.Manager { return a.buses }

// GetI2cConnection implements i2c.Connector.
func (a *Adaptor) GetI2cConnection(address, busNr int) (gobotI2C.Connection, error) {
	return a.buses.I2C(busNr).Connector().GetI2cConnection(address, busNr)
}

// DefaultI2cBus implements i2c.Connector.
func (a *Adaptor) DefaultI2cBus() int { return a.buses.DefaultI2CBus() }

// GetSpiConnection implements spi.Connector.
func (a *Adaptor) GetSpiConnection(busNum, chip, mode, bits int, maxSpeed int64) (gobotSPI.Connection, error) {
	return bus.NewSPIConnector(a.buses, 0, 0, 0, 8, 500000).
		GetSpiConnection(busNum, chip, mode, bits, maxSpeed)
}

// SpiDefaultBusNumber implements spi.Connector.
func (a *Adaptor) SpiDefaultBusNumber() int { return 0 }

// SpiDefaultChipNumber implements spi.Connector.
func (a *Adaptor) SpiDefaultChipNumber() int { return 0 }

// SpiDefaultMode implements spi.Connector.
func (a *Adaptor) SpiDefaultMode() int { return 0 }

// SpiDefaultBitCount implements spi.Connector.
func (a *Adaptor) SpiDefaultBitCount() int { return 8 }

// SpiDefaultMaxSpeed implements spi.Connector.
func (a *Adaptor) SpiDefaultMaxSpeed() int64 { return 500000 }

// GetOneWireConnection implements gobot's onewire connector contract.
func (a *Adaptor) GetOneWireConnection(familyCode byte, serialNumber uint64) (gobotOneWire.Connection, error) {
	return a.buses.OneWire().GetOneWireConnection(familyCode, serialNumber)
}

// SerialRead implements serial.SerialReader.
func (a *Adaptor) SerialRead(b []byte) (int, error) { return a.buses.Serial(a.Port()).SerialRead(b) }

// SerialWrite implements serial.SerialWriter.
func (a *Adaptor) SerialWrite(b []byte) (int, error) { return a.buses.Serial(a.Port()).SerialWrite(b) }

// Address implements gobot.BLEConnector.
func (a *Adaptor) Address() string { return a.buses.BLE().Address() }

// Disconnect implements gobot.BLEConnector.
func (a *Adaptor) Disconnect() error { return a.Finalize() }

// ReadCharacteristic implements gobot.BLEConnector.
func (a *Adaptor) ReadCharacteristic(cUUID string) ([]byte, error) {
	return a.buses.BLE().ReadCharacteristic(cUUID)
}

// WriteCharacteristic implements gobot.BLEConnector.
func (a *Adaptor) WriteCharacteristic(cUUID string, data []byte) error {
	return a.buses.BLE().WriteCharacteristic(cUUID, data)
}

// Subscribe implements gobot.BLEConnector.
func (a *Adaptor) Subscribe(cUUID string, f func(data []byte)) error {
	return a.buses.BLE().Subscribe(cUUID, f)
}

// WithoutResponses implements gobot.BLEConnector.
func (a *Adaptor) WithoutResponses(use bool) { a.buses.BLE().WithoutResponses(use) }

var (
	_ gobotI2C.Connector  = (*Adaptor)(nil)
	_ gobotSPI.Connector  = (*Adaptor)(nil)
	_ serial.SerialReader = (*Adaptor)(nil)
	_ serial.SerialWriter = (*Adaptor)(nil)
	_ gobot.BLEConnector  = (*Adaptor)(nil)
)
