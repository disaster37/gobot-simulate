package simulate

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/disaster37/gobot-simulate/internal/protocol/firmata"
	"github.com/disaster37/gobot-simulate/internal/state"
)

// FirmataPipe returns the client end of an in-memory Firmata connection bound
// to a simulated board. Pass it to firmata.NewAdaptor.
func (s *Simulator) FirmataPipe(boardID string) (io.ReadWriteCloser, error) {
	adaptor, err := s.adaptorFor(boardID)
	if err != nil {
		return nil, err
	}
	server, client := net.Pipe()
	responder, err := firmata.New(server, &firmataBackend{adaptor: adaptor}, 24, 8)
	if err != nil {
		_ = server.Close()
		_ = client.Close()
		return nil, err
	}
	go func() { _ = responder.Serve(context.Background()) }()
	return client, nil
}

// FirmataTCP listens for Firmata connections and serves each one.
func (s *Simulator) FirmataTCP(ctx context.Context, boardID, addr string) (net.Listener, error) {
	adaptor, err := s.adaptorFor(boardID)
	if err != nil {
		return nil, err
	}
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			responder, err := firmata.New(conn, &firmataBackend{adaptor: adaptor}, 24, 8)
			if err != nil {
				_ = conn.Close()
				continue
			}
			go func() { _ = responder.Serve(ctx) }()
		}
	}()
	return listener, nil
}

type firmataBackend struct {
	adaptor *Adaptor
}

func (b *firmataBackend) SetFirmataPinMode(pin, mode int) error {
	name := fmt.Sprintf("%d", pin)
	switch mode {
	case firmata.ModeInput:
		return b.adaptor.board.BindPin(name, "", state.PinModeInput)
	case firmata.ModeOutput:
		return b.adaptor.board.BindPin(name, "", state.PinModeOutput)
	case firmata.ModeAnalog:
		return b.adaptor.board.BindPin(name, "", state.PinModeAnalog)
	case firmata.ModePWM:
		return b.adaptor.board.BindPin(name, "", state.PinModePWM)
	case firmata.ModeServo:
		return b.adaptor.board.BindPin(name, "", state.PinModeServo)
	default:
		return fmt.Errorf("%w: firmata mode %d", state.ErrInvalidPinMode, mode)
	}
}

func (b *firmataBackend) FirmataDigitalWrite(pin, value int) error {
	b.adaptor.board.DigitalWritePin(fmt.Sprintf("%d", pin), byte(value), true, state.PinModeOutput, state.SourceHardwareCode) //nolint:gosec // intentional width conversion
	return nil
}

func (b *firmataBackend) FirmataDigitalRead(pin int) (int, error) {
	return b.adaptor.board.DigitalReadPin(fmt.Sprintf("%d", pin)).DigitalValue, nil
}

func (b *firmataBackend) FirmataAnalogRead(pin int) (int, error) {
	return b.adaptor.board.AnalogReadPin(fmt.Sprintf("%d", pin+14)).AnalogValue, nil
}

func (b *firmataBackend) FirmataI2C(address int, write []byte, readLen int) ([]byte, error) {
	bus := b.adaptor.buses.I2C(b.adaptor.buses.DefaultI2CBus())
	if readLen > 0 {
		buf := make([]byte, readLen)
		if err := bus.ReadBlockData(address, firstRegister(write), buf); err != nil {
			return nil, err
		}
		return buf, nil
	}
	if len(write) > 0 {
		return nil, bus.WriteBytes(address, write)
	}
	return nil, nil
}

func firstRegister(write []byte) uint8 {
	if len(write) > 0 {
		return write[0]
	}
	return 0
}
