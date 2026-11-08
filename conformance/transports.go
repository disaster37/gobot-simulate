package conformance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	extra "github.com/disaster37/gobot-arest/v2/drivers/extra"
	arest "github.com/disaster37/gobot-arest/v2/plateforms/arest"
	arestClient "github.com/disaster37/gobot-arest/v2/plateforms/arest/client"
	serialClient "github.com/disaster37/gobot-arest/v2/plateforms/arest/client/serial"
	simulate "github.com/disaster37/gobot-simulate"
	"go.bug.st/serial"
	"gobot.io/x/gobot/v2/platforms/particle"
)

// simSerialPort is an in-memory serial.Port whose writes are served by the
// simulator's aREST serial dispatcher.
type simSerialPort struct {
	sim   *simulate.Simulator
	board string

	mu       sync.Mutex
	buf      []byte
	closed   bool
	signal   chan struct{}
	closedCh chan struct{}
}

func newSimSerialPort(sim *simulate.Simulator, board string) *simSerialPort {
	return &simSerialPort{
		sim:      sim,
		board:    board,
		signal:   make(chan struct{}, 1),
		closedCh: make(chan struct{}),
	}
}

func (p *simSerialPort) SetMode(*serial.Mode) error { return nil }

func (p *simSerialPort) Read(data []byte) (int, error) {
	for {
		p.mu.Lock()
		if len(p.buf) > 0 {
			n := copy(data, p.buf)
			p.buf = p.buf[n:]
			p.mu.Unlock()
			return n, nil
		}
		closed := p.closed
		p.mu.Unlock()
		if closed {
			return 0, io.EOF
		}
		select {
		case <-p.signal:
		case <-p.closedCh:
			return 0, io.EOF
		}
	}
}

func (p *simSerialPort) Write(data []byte) (int, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return 0, errors.New("simulate: serial port closed")
	}
	p.mu.Unlock()

	response := p.sim.ARESTSerialResponse(p.board, string(data))
	p.mu.Lock()
	p.buf = append(p.buf, []byte(response)...)
	p.mu.Unlock()
	select {
	case p.signal <- struct{}{}:
	default:
	}
	return len(data), nil
}

func (p *simSerialPort) Drain() error { return nil }

func (p *simSerialPort) ResetInputBuffer() error {
	p.mu.Lock()
	p.buf = nil
	p.mu.Unlock()
	return nil
}

func (p *simSerialPort) ResetOutputBuffer() error { return nil }
func (p *simSerialPort) SetDTR(bool) error        { return nil }
func (p *simSerialPort) SetRTS(bool) error        { return nil }

func (p *simSerialPort) GetModemStatusBits() (*serial.ModemStatusBits, error) {
	return &serial.ModemStatusBits{}, nil
}

func (p *simSerialPort) SetReadTimeout(time.Duration) error { return nil }

func (p *simSerialPort) Close() error {
	p.mu.Lock()
	if !p.closed {
		p.closed = true
		close(p.closedCh)
	}
	p.mu.Unlock()
	return nil
}

func (p *simSerialPort) Break(time.Duration) error { return nil }

func seedARESTDevices(env *Env) error {
	if _, err := env.Sim.RegisterDevice(env.Board, simulate.DeviceSpec{
		ID: "value_isRebooted", Name: "isRebooted", Kind: simulate.DeviceKindValue, ValueName: "isRebooted",
	}); err != nil {
		return err
	}
	if _, err := env.Sim.RegisterDevice(env.Board, simulate.DeviceSpec{
		ID: "function_ack", Name: "ack", Kind: simulate.DeviceKindFunction, FunctionName: "ack",
	}); err != nil {
		return err
	}
	if err := env.Sim.SetValue(env.Board, "isRebooted", false, simulate.SourceSimulation); err != nil {
		return err
	}
	return env.Sim.SetFunctionCode(env.Board, "ack", 3, simulate.SourceSimulation)
}

func caseARESTHTTP(env *Env) error {
	if err := seedARESTDevices(env); err != nil {
		return err
	}
	if err := env.StartServer(); err != nil {
		return err
	}
	adaptor := arest.NewHTTPAdaptor(env.Sim.ARESTBaseURL(env.Board), env.Board, 2*time.Second)
	if err := adaptor.Connect(); err != nil {
		return err
	}
	defer func() { _ = adaptor.Finalize() }()

	if err := adaptor.DigitalWrite("3", 1); err != nil {
		return err
	}
	pin, err := env.Sim.Pin(env.Board, "3")
	if err != nil {
		return err
	}
	if pin.DigitalValue != 1 || pin.Mode != simulate.PinModeOutput {
		return fmt.Errorf("arest http pin = %d/%s, want 1/output", pin.DigitalValue, pin.Mode)
	}
	value, err := adaptor.ValueRead("isRebooted")
	if err != nil {
		return err
	}
	if value != false {
		return fmt.Errorf("arest http value = %v, want false", value)
	}
	code, err := adaptor.FunctionCall("ack", "now")
	if err != nil {
		return err
	}
	if code != 3 {
		return fmt.Errorf("arest http function = %d, want 3", code)
	}
	return nil
}

func caseARESTSerial(env *Env) error {
	if err := seedARESTDevices(env); err != nil {
		return err
	}
	adaptor := arest.NewSerialAdaptor(env.Board, 2*time.Second)
	client, ok := adaptor.Board.(*serialClient.Client)
	if !ok {
		return fmt.Errorf("arest serial board has unexpected type %T", adaptor.Board)
	}
	port := newSimSerialPort(env.Sim, env.Board)
	client.SetSerial(port)
	if err := adaptor.Connect(); err != nil {
		return err
	}
	defer func() { _ = adaptor.Finalize() }()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := adaptor.Board.SetPinMode(ctx, 3, arestClient.ModeOutput); err != nil {
		return err
	}
	if err := adaptor.Board.DigitalWrite(ctx, 3, 1); err != nil {
		return err
	}
	pin, err := env.Sim.Pin(env.Board, "3")
	if err != nil {
		return err
	}
	if pin.DigitalValue != 1 {
		return fmt.Errorf("arest serial pin = %d, want 1", pin.DigitalValue)
	}

	value, err := adaptor.Board.ReadValue(ctx, "isRebooted")
	if err != nil {
		return err
	}
	if value != false {
		return fmt.Errorf("arest serial value = %v, want false", value)
	}
	code, err := adaptor.Board.CallFunction(ctx, "ack", "x")
	if err != nil {
		return err
	}
	if code != 3 {
		return fmt.Errorf("arest serial function = %d, want 3", code)
	}

	// The extra value driver works over the serial adaptor too.
	valueDriver := extra.NewValueDriver(adaptor, "isRebooted", 10*time.Millisecond)
	if valueDriver.ValueName() != "isRebooted" {
		return fmt.Errorf("unexpected extra value driver name")
	}
	return nil
}

func caseParticle(env *Env) error {
	if err := env.StartServer(); err != nil {
		return err
	}
	adaptor := particle.NewAdaptor(env.Board, "token")
	adaptor.APIServer = env.Sim.URL()
	if err := adaptor.Connect(); err != nil {
		return err
	}
	defer func() { _ = adaptor.Finalize() }()

	if err := adaptor.DigitalWrite("7", 1); err != nil {
		return err
	}
	value, err := adaptor.DigitalRead("7")
	if err != nil {
		return err
	}
	if value != 1 {
		return fmt.Errorf("particle digital read = %d, want 1", value)
	}
	if analogErr := env.Sim.SetAnalog(env.Board, "A0", 512, simulate.SourceSimulation); analogErr != nil {
		return analogErr
	}
	analog, err := adaptor.AnalogRead("A0")
	if err != nil {
		return err
	}
	if analog != 512 {
		return fmt.Errorf("particle analog read = %d, want 512", analog)
	}
	return nil
}
