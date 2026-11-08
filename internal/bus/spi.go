package bus

import (
	"fmt"
	"sync"

	"github.com/disaster37/gobot-simulate/internal/state"
	"github.com/disaster37/gobot-simulate/internal/trace"
	gobotSPI "gobot.io/x/gobot/v2/drivers/spi"
)

// SPIBus implements gobot.SpiSystemDevicer for one bus/chip pair.
type SPIBus struct {
	manager *Manager
	bus     int
	chip    int

	mu    sync.RWMutex
	model ChipModel
}

// BusNumber returns the SPI bus number.
func (s *SPIBus) BusNumber() int { return s.bus }

// ChipNumber returns the SPI chip-select number.
func (s *SPIBus) ChipNumber() int { return s.chip }

// Model returns the attached chip model, if any.
func (s *SPIBus) Model() ChipModel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.model
}

func (s *SPIBus) txrx(tx []byte, rx []byte) error {
	started := s.manager.clk.Now()
	target := fmt.Sprintf("spi-%d/%d", s.bus, s.chip)
	s.mu.RLock()
	model := s.model
	s.mu.RUnlock()

	var response Response
	var err error
	switch {
	case s.manager.busFault(target, state.FaultBusTimeout):
		err = ErrTimeout
	case s.manager.busFault(target, state.FaultBusNAK):
		err = ErrNAK
	case model == nil:
		err = fmt.Errorf("%w: %s", ErrChipNotFound, target)
	default:
		response, err = model.Transfer(Transaction{
			Transport: "spi",
			Bus:       s.bus,
			Address:   s.chip,
			Write:     append([]byte(nil), tx...),
			ReadLen:   len(rx),
		})
		switch {
		case err != nil || len(rx) == 0:
		case len(response.Data) == len(rx):
			copy(rx, response.Data)
		case len(response.Data) == 0:
			// Write-only transfer: leave rx untouched.
		default:
			err = fmt.Errorf("simulate: spi chip %s returned %d bytes for %d-byte read", target, len(response.Data), len(rx))
		}
	}
	s.manager.record(trace.KindBusTxn, target, "spi", map[string]interface{}{
		"bus":     s.bus,
		"chip":    s.chip,
		"write":   tx,
		"readLen": len(rx),
		"data":    response.Data,
	}, state.SourceHardwareCode, started, err)
	return err
}

// TxRx implements gobot.SpiSystemDevicer.
func (s *SPIBus) TxRx(tx []byte, rx []byte) error { return s.txrx(tx, rx) }

// Close implements gobot.SpiSystemDevicer.
func (s *SPIBus) Close() error { return nil }

// SPIConnector implements gobot's spi.Connector.
type SPIConnector struct {
	manager  *Manager
	defBus   int
	defChip  int
	defMode  int
	defBits  int
	maxSpeed int64
}

// NewSPIConnector creates a SPI connector.
func NewSPIConnector(manager *Manager, bus, chip, mode, bits int, maxSpeed int64) *SPIConnector {
	return &SPIConnector{
		manager:  manager,
		defBus:   bus,
		defChip:  chip,
		defMode:  mode,
		defBits:  bits,
		maxSpeed: maxSpeed,
	}
}

// GetSpiConnection implements spi.Connector.
func (c *SPIConnector) GetSpiConnection(busNum, chip, mode, bits int, maxSpeed int64) (gobotSPI.Connection, error) {
	_ = mode
	_ = bits
	_ = maxSpeed
	return gobotSPI.NewConnection(c.manager.SPIConnection(busNum, chip)), nil
}

// SpiDefaultBusNumber implements spi.Connector.
func (c *SPIConnector) SpiDefaultBusNumber() int { return c.defBus }

// SpiDefaultChipNumber implements spi.Connector.
func (c *SPIConnector) SpiDefaultChipNumber() int { return c.defChip }

// SpiDefaultMode implements spi.Connector.
func (c *SPIConnector) SpiDefaultMode() int { return c.defMode }

// SpiDefaultBitCount implements spi.Connector.
func (c *SPIConnector) SpiDefaultBitCount() int { return c.defBits }

// SpiDefaultMaxSpeed implements spi.Connector.
func (c *SPIConnector) SpiDefaultMaxSpeed() int64 { return c.maxSpeed }
