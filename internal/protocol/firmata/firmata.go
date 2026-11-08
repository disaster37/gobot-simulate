// Package firmata implements a Firmata protocol responder over any
// io.ReadWriteCloser (including in-memory pipes and TCP).
package firmata

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"time"
)

// Firmata protocol constants.
const (
	protocolVersion = 0xF9
	systemReset     = 0xFF
	digitalMessage  = 0x90
	analogMessage   = 0xE0
	reportAnalog    = 0xC0
	reportDigital   = 0xD0
	pinMode         = 0xF4
	startSysex      = 0xF0
	endSysex        = 0xF7
	capabilityQuery = 0x6B
	capabilityResp  = 0x6C
	analogMapQuery  = 0x69
	analogMapResp   = 0x6A
	pinStateQuery   = 0x6D
	pinStateResp    = 0x6E
	firmwareQuery   = 0x79
	i2cRequest      = 0x76
	i2cReply        = 0x77
	i2cConfig       = 0x78
	servoConfig     = 0x70
)

// Pin modes.
const (
	ModeInput  = 0x00
	ModeOutput = 0x01
	ModeAnalog = 0x02
	ModePWM    = 0x03
	ModeServo  = 0x04
)

// Backend is implemented by the simulator binding.
type Backend interface {
	SetFirmataPinMode(pin, mode int) error
	FirmataDigitalWrite(pin, value int) error
	FirmataDigitalRead(pin int) (int, error)
	FirmataAnalogRead(pin int) (int, error)
	FirmataI2C(address int, write []byte, readLen int) ([]byte, error)
}

// Responder serves the Firmata protocol for one connection.
type Responder struct {
	conn    io.ReadWriteCloser
	backend Backend
	pins    int
	analog  int

	mu            sync.Mutex
	modes         []int
	digitalOut    []int
	digitalReport []bool
	analogReport  []bool
	lastPort      []int
	lastAnalog    []int
}

// New creates a responder with pinCount digital pins and analogCount analog
// inputs.
func New(conn io.ReadWriteCloser, backend Backend, pinCount, analogCount int) (*Responder, error) {
	if conn == nil {
		return nil, errors.New("firmata: nil connection")
	}
	if backend == nil {
		return nil, errors.New("firmata: nil backend")
	}
	if pinCount < 8 {
		pinCount = 8
	}
	if pinCount < 14+analogCount {
		pinCount = 14 + analogCount
	}
	if analogCount < 0 {
		analogCount = 0
	}
	r := &Responder{
		conn:          conn,
		backend:       backend,
		pins:          pinCount,
		analog:        analogCount,
		modes:         make([]int, pinCount),
		digitalOut:    make([]int, pinCount),
		digitalReport: make([]bool, (pinCount+7)/8),
		analogReport:  make([]bool, pinCount),
		lastPort:      make([]int, (pinCount+7)/8),
		lastAnalog:    make([]int, pinCount),
	}
	// The gobot Firmata client marks every capability-reported pin as an
	// output and therefore does not send a pinMode frame before its first
	// DigitalWrite. Start pins in output mode so those writes take effect.
	for i := range r.modes {
		r.modes[i] = ModeOutput
	}
	for i := range r.lastPort {
		r.lastPort[i] = -1
	}
	for i := range r.lastAnalog {
		r.lastAnalog[i] = -1
	}
	return r, nil
}

// Serve processes protocol frames until the connection closes or ctx ends.
func (r *Responder) Serve(ctx context.Context) error {
	go r.poll(ctx)
	buf := make([]byte, 1)
	for {
		if ctx != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}
		if _, err := io.ReadFull(r.conn, buf); err != nil {
			return err
		}
		if err := r.handle(ctx, buf[0]); err != nil {
			return err
		}
	}
}

func (r *Responder) poll(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.mu.Lock()
			ports := append([]bool(nil), r.digitalReport...)
			analog := append([]bool(nil), r.analogReport...)
			r.mu.Unlock()
			for port, enabled := range ports {
				if !enabled {
					continue
				}
				value := 0
				for bit := 0; bit < 8; bit++ {
					pin := port*8 + bit
					if pin >= r.pins {
						break
					}
					level := r.readPin(pin)
					if level != 0 {
						value |= 1 << bit
					}
				}
				if last := r.lastPort[port]; last == value && r.portReported(port) {
					continue
				}
				r.setPortReported(port, value)
				_ = r.write([]byte{byte(digitalMessage | port), byte(value & 0x7F), byte((value >> 7) & 0x7F)}) //nolint:gosec // intentional width conversion
			}
			for channel, enabled := range analog {
				if !enabled || channel < 14 || channel >= 14+r.analog {
					continue
				}
				value, err := r.backend.FirmataAnalogRead(channel - 14)
				if err != nil {
					continue
				}
				if r.lastAnalog[channel] == value && r.analogReported(channel) {
					continue
				}
				r.setAnalogReported(channel, value)
				_ = r.write([]byte{byte(analogMessage | (channel - 14)), byte(value & 0x7F), byte((value >> 7) & 0x7F)}) //nolint:gosec // intentional width conversion
			}
		}
	}
}

func (r *Responder) portReported(port int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastPort[port] != -1
}

func (r *Responder) setPortReported(port, value int) {
	r.mu.Lock()
	r.lastPort[port] = value
	r.mu.Unlock()
}

func (r *Responder) analogReported(channel int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastAnalog[channel] != -1
}

func (r *Responder) setAnalogReported(channel, value int) {
	r.mu.Lock()
	r.lastAnalog[channel] = value
	r.mu.Unlock()
}

func (r *Responder) readPin(pin int) int {
	r.mu.Lock()
	mode := r.modes[pin]
	out := r.digitalOut[pin]
	r.mu.Unlock()
	if mode == ModeOutput {
		return out
	}
	value, err := r.backend.FirmataDigitalRead(pin)
	if err != nil {
		return 0
	}
	return value
}

func (r *Responder) handle(ctx context.Context, message byte) error {
	switch {
	case message == protocolVersion:
		return r.write([]byte{protocolVersion, 2, 6})
	case message == startSysex:
		return r.handleSysex(ctx)
	case message == pinMode:
		pin, err := r.readByte()
		if err != nil {
			return err
		}
		mode, err := r.readByte()
		if err != nil {
			return err
		}
		if int(pin) < r.pins {
			r.mu.Lock()
			r.modes[pin] = int(mode)
			r.lastPort[int(pin)/8] = -1
			r.mu.Unlock()
			_ = r.backend.SetFirmataPinMode(int(pin), int(mode))
		}
	case message >= digitalMessage && message <= digitalMessage+0x0F:
		return r.handleDigitalWrite(message)
	case message >= analogMessage && message <= analogMessage+0x0F:
		return r.handleAnalogWrite(message)
	case message >= reportAnalog && message <= reportAnalog+0x0F:
		channel := int(message & 0x0F)
		state, err := r.readByte()
		if err != nil {
			return err
		}
		// Firmata reports analog channels by physical pin index after the
		// digital block (14..21).
		index := channel
		if index >= 0 && index < len(r.analogReport) {
			r.mu.Lock()
			r.analogReport[index] = state != 0
			r.lastAnalog[index] = -1
			r.mu.Unlock()
		}
	case message >= reportDigital && message <= reportDigital+0x0F:
		port := int(message & 0x0F)
		state, err := r.readByte()
		if err != nil {
			return err
		}
		if port < len(r.digitalReport) {
			r.mu.Lock()
			r.digitalReport[port] = state != 0
			r.lastPort[port] = -1
			r.mu.Unlock()
		}
	case message == systemReset:
		return nil
	}
	return nil
}

func (r *Responder) handleDigitalWrite(message byte) error {
	port := int(message & 0x0F)
	low, err := r.readByte()
	if err != nil {
		return err
	}
	high, err := r.readByte()
	if err != nil {
		return err
	}
	value := int(low) | int(high)<<7
	var writes []struct {
		pin   int
		level int
	}
	r.mu.Lock()
	for bit := 0; bit < 8; bit++ {
		pin := port*8 + bit
		if pin >= r.pins {
			break
		}
		level := (value >> bit) & 1
		r.digitalOut[pin] = level
		if r.modes[pin] == ModeOutput {
			writes = append(writes, struct {
				pin   int
				level int
			}{pin: pin, level: level})
		}
	}
	r.mu.Unlock()
	for _, write := range writes {
		_ = r.backend.FirmataDigitalWrite(write.pin, write.level)
	}
	return nil
}

func (r *Responder) handleAnalogWrite(message byte) error {
	pin := int(message & 0x0F)
	low, err := r.readByte()
	if err != nil {
		return err
	}
	high, err := r.readByte()
	if err != nil {
		return err
	}
	_ = r.backend.FirmataDigitalWrite(pin, int(low)|int(high)<<7)
	return nil
}

func (r *Responder) handleSysex(ctx context.Context) error {
	var payload []byte
	for {
		value, err := r.readByte()
		if err != nil {
			return err
		}
		if value == endSysex {
			break
		}
		payload = append(payload, value)
		if len(payload) > 4096 {
			return errors.New("firmata: sysex payload too large")
		}
	}
	if len(payload) == 0 {
		return nil
	}
	switch payload[0] {
	case capabilityQuery:
		return r.writeCapabilities()
	case analogMapQuery:
		return r.writeAnalogMapping()
	case firmwareQuery:
		return r.writeSysex([]byte{firmwareQuery, 2, 6, 'S', 'i', 'm', 'u', 'l', 'a', 't', 'o', 'r'})
	case pinStateQuery:
		if len(payload) < 2 {
			return nil
		}
		pin := int(payload[1])
		r.mu.Lock()
		mode := r.modes[pin]
		r.mu.Unlock()
		return r.writeSysex([]byte{pinStateResp, byte(pin), byte(mode), 0, 0}) //nolint:gosec // intentional width conversion
	case i2cConfig:
		return nil
	case servoConfig:
		return nil
	case i2cRequest:
		return r.handleI2C(payload)
	default:
		return nil
	}
}

func (r *Responder) handleI2C(payload []byte) error {
	if len(payload) < 3 {
		return nil
	}
	address := int(payload[1])
	mode := payload[2] >> 3
	var data []byte
	for i := 3; i+1 < len(payload); i += 2 {
		data = append(data, payload[i]|payload[i+1]<<7)
	}
	switch mode {
	case 0: // write
		_, _ = r.backend.FirmataI2C(address, data, 0)
		return nil
	case 1: // read
		readLen := 0
		if len(data) >= 2 {
			readLen = int(data[0]) | int(data[1])<<7
		}
		reply, err := r.backend.FirmataI2C(address, nil, readLen)
		if err != nil {
			return nil
		}
		payload := []byte{i2cReply, byte(address), byte(address >> 7), 0, 0} //nolint:gosec // intentional width conversion
		for _, value := range reply {
			payload = append(payload, value&0x7F, (value>>7)&0x7F)
		}
		return r.writeSysex(payload)
	default:
		return nil
	}
}

func (r *Responder) writeCapabilities() error {
	payload := []byte{capabilityResp}
	for pin := 0; pin < r.pins; pin++ {
		switch {
		case pin >= 14 && pin < 14+r.analog:
			payload = append(payload, ModeInput, 1, ModeOutput, 1, ModeAnalog, 8, ModePWM, 8, ModeServo, 1)
		default:
			payload = append(payload, ModeInput, 1, ModeOutput, 1, ModePWM, 8, ModeServo, 1)
		}
		payload = append(payload, 0x7F)
	}
	return r.writeSysex(payload)
}

func (r *Responder) writeAnalogMapping() error {
	payload := []byte{analogMapResp}
	for pin := 0; pin < r.pins; pin++ {
		if pin >= 14 && pin < 14+r.analog {
			payload = append(payload, byte(pin-14))
		} else {
			payload = append(payload, 0x7F)
		}
	}
	return r.writeSysex(payload)
}

func (r *Responder) writeSysex(payload []byte) error {
	data := make([]byte, 0, len(payload)+2)
	data = append(data, startSysex)
	data = append(data, payload...)
	data = append(data, endSysex)
	return r.write(data)
}

func (r *Responder) readByte() (byte, error) {
	buf := make([]byte, 1)
	_, err := io.ReadFull(r.conn, buf)
	return buf[0], err
}

func (r *Responder) write(data []byte) error {
	_, err := r.conn.Write(data)
	return err
}

// String describes the responder for diagnostics.
func (r *Responder) String() string {
	return strings.Join([]string{"firmata", "responder"}, ":")
}
