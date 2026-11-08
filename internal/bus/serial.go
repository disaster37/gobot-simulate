package bus

import (
	"errors"
	"io"
	"strings"
	"sync"
)

// SerialPort is an in-memory serial endpoint. Writes are optionally passed to
// a handler that appends a textual response, which lets protocol responders
// such as aREST serial run over the same model.
type SerialPort struct {
	mu      sync.Mutex
	buf     []byte
	max     int
	handler func(string) string
	closed  bool
}

const defaultSerialBuffer = 64 * 1024

func newSerialPort(_ string) *SerialPort {
	return &SerialPort{max: defaultSerialBuffer}
}

// SetHandler installs a line-oriented request/response handler.
func (p *SerialPort) SetHandler(handler func(string) string) {
	p.mu.Lock()
	p.handler = handler
	p.mu.Unlock()
}

// SerialRead implements serial.SerialReader.
func (p *SerialPort) SerialRead(b []byte) (int, error) {
	return p.Read(b)
}

// SerialWrite implements serial.SerialWriter.
func (p *SerialPort) SerialWrite(b []byte) (int, error) {
	return p.Write(b)
}

// Read drains buffered bytes.
func (p *SerialPort) Read(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed && len(p.buf) == 0 {
		return 0, io.EOF
	}
	n := copy(b, p.buf)
	p.buf = p.buf[n:]
	return n, nil
}

// Write buffers bytes and invokes the optional handler.
func (p *SerialPort) Write(b []byte) (int, error) {
	p.mu.Lock()
	handler := p.handler
	closed := p.closed
	p.mu.Unlock()
	if closed {
		return 0, errors.New("simulate: serial port closed")
	}
	if handler != nil {
		// Request/response endpoint: only the handler's response is readable
		// back, the request itself is consumed by the device.
		response := handler(strings.TrimSpace(string(b)))
		p.mu.Lock()
		p.appendLocked([]byte(response))
		p.mu.Unlock()
		return len(b), nil
	}
	p.mu.Lock()
	p.appendLocked(b)
	p.mu.Unlock()
	return len(b), nil
}

func (p *SerialPort) appendLocked(data []byte) {
	if len(data) == 0 {
		return
	}
	if len(data) >= p.max {
		// A single write larger than the whole buffer would otherwise slice
		// p.buf past its bounds. Keep only the most recent max bytes.
		p.buf = append(p.buf[:0], data[len(data)-p.max:]...)
		return
	}
	if len(p.buf)+len(data) > p.max {
		p.buf = p.buf[len(p.buf)+len(data)-p.max:]
	}
	p.buf = append(p.buf, data...)
}

// Buffered returns a copy of buffered bytes.
func (p *SerialPort) Buffered() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]byte(nil), p.buf...)
}

// Close marks the port closed.
func (p *SerialPort) Close() error {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	return nil
}

// BLEDevice is a generic GATT peripheral model.
type BLEDevice struct {
	mu        sync.RWMutex
	values    map[string][]byte
	notify    map[string][]func([]byte)
	address   string
	responses bool
}

func newBLEDevice() *BLEDevice {
	d := &BLEDevice{
		values:  make(map[string][]byte),
		notify:  make(map[string][]func([]byte)),
		address: "simulator",
	}
	d.values["2a19"] = []byte{100}
	d.values["2a00"] = []byte("gobot-simulate")
	d.values["2a01"] = []byte{0x00, 0x00}
	d.values["2a24"] = []byte("simulated")
	d.values["2a26"] = []byte("0.1.0")
	d.values["2a27"] = []byte("virtual")
	d.values["2a29"] = []byte("disaster37")
	d.values["2a50"] = []byte("sim")
	return d
}

// SetAddress sets the advertised address.
func (d *BLEDevice) SetAddress(address string) {
	d.mu.Lock()
	d.address = address
	d.mu.Unlock()
}

// Address returns the advertised address.
func (d *BLEDevice) Address() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.address
}

// SetCharacteristic stores a characteristic value.
func (d *BLEDevice) SetCharacteristic(uuid string, data []byte) {
	d.mu.Lock()
	d.values[strings.ToLower(uuid)] = append([]byte(nil), data...)
	handlers := append([]func([]byte){}, d.notify[strings.ToLower(uuid)]...)
	d.mu.Unlock()
	for _, handler := range handlers {
		handler(append([]byte(nil), data...))
	}
}

// ReadCharacteristic returns a characteristic value.
func (d *BLEDevice) ReadCharacteristic(uuid string) ([]byte, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	data, ok := d.values[strings.ToLower(uuid)]
	if !ok {
		return nil, errors.New("simulate: ble characteristic not found: " + uuid)
	}
	return append([]byte(nil), data...), nil
}

// WriteCharacteristic stores a characteristic value.
func (d *BLEDevice) WriteCharacteristic(uuid string, data []byte) error {
	d.SetCharacteristic(uuid, data)
	return nil
}

// Subscribe registers a notification handler.
func (d *BLEDevice) Subscribe(uuid string, handler func([]byte)) error {
	if handler == nil {
		return errors.New("simulate: nil ble subscription handler")
	}
	d.mu.Lock()
	d.notify[strings.ToLower(uuid)] = append(d.notify[strings.ToLower(uuid)], handler)
	d.mu.Unlock()
	return nil
}

// WithoutResponses toggles write-response emulation.
func (d *BLEDevice) WithoutResponses(use bool) {
	d.mu.Lock()
	d.responses = !use
	d.mu.Unlock()
}

// Notify pushes data to subscribers of a characteristic.
func (d *BLEDevice) Notify(uuid string, data []byte) {
	d.SetCharacteristic(uuid, data)
}
