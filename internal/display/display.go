// Package display implements pin-level decoders for bit-banged displays.
package display

import "sync"

// Snapshot is the renderable content of a display.
type Snapshot struct {
	Model   string   `json:"model"`
	Width   int      `json:"width"`
	Height  int      `json:"height"`
	Pixels  []byte   `json:"pixels,omitempty"`
	Lines   []string `json:"lines,omitempty"`
	Enabled bool     `json:"enabled"`
}

// Decoder turns digital pin transitions into display content.
type Decoder interface {
	Pin(pin string, level int)
	Snapshot() Snapshot
}

// New creates a decoder for a model and its pin role order.
func New(model string, pins []string) Decoder {
	switch model {
	case "hd44780":
		return newHD44780(pins)
	case "max7219":
		return newMAX7219(pins)
	case "tm1638", "aip1640":
		return newShiftRegister(model, pins)
	default:
		return nil
	}
}

type levelTracker struct {
	mu     sync.Mutex
	levels map[string]int
}

func newLevelTracker(pins []string) levelTracker {
	levels := make(map[string]int, len(pins))
	for _, pin := range pins {
		levels[pin] = 0
	}
	return levelTracker{levels: levels}
}

func (t *levelTracker) set(pin string, level int) (int, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	old, ok := t.levels[pin]
	t.levels[pin] = level
	return old, ok
}

func (t *levelTracker) get(pin string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.levels[pin]
}

func (t *levelTracker) rising(pin string, level int) bool {
	old, ok := t.set(pin, level)
	return ok && old == 0 && level == 1
}

func (t *levelTracker) falling(pin string, level int) bool {
	old, ok := t.set(pin, level)
	return ok && old == 1 && level == 0
}

// hd44780 decodes a 4-bit HD44780 character LCD.
type hd44780 struct {
	levels levelTracker
	pins   []string
	mu     sync.Mutex
	high   byte
	half   bool
	cursor int
	lines  [2][40]byte
	on     bool
}

func newHD44780(pins []string) Decoder {
	return &hd44780{levels: newLevelTracker(pins), pins: pins, on: true}
}

func (d *hd44780) Pin(pin string, level int) {
	if len(d.pins) < 6 {
		return
	}
	if pin == pinsAt(d.pins, 1) {
		if d.levels.falling(pin, level) {
			d.latch()
		}
		return
	}
	d.levels.set(pin, level)
}

func (d *hd44780) latch() {
	nibble := byte(d.levels.get(pinsAt(d.pins, 2)) | //nolint:gosec // intentional width conversion
		d.levels.get(pinsAt(d.pins, 3))<<1 |
		d.levels.get(pinsAt(d.pins, 4))<<2 |
		d.levels.get(pinsAt(d.pins, 5))<<3)
	registerSelect := d.levels.get(pinsAt(d.pins, 0)) == 1
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.half {
		d.high = nibble
		d.half = true
		return
	}
	d.half = false
	d.byteLocked(registerSelect, d.high<<4|nibble)
}

func (d *hd44780) byteLocked(registerSelect bool, value byte) {
	if registerSelect {
		if d.cursor < 80 {
			d.lines[d.cursor/40][d.cursor%40] = value
		}
		d.cursor++
		return
	}
	switch {
	case value&0x80 != 0:
		addr := int(value & 0x7F)
		if addr >= 0x40 {
			d.cursor = 40 + (addr - 0x40)
		} else {
			d.cursor = addr
		}
	case value == 0x01:
		d.lines = [2][40]byte{}
		d.cursor = 0
	case value == 0x02:
		d.cursor = 0
	case value&0x08 != 0:
		d.on = value&0x04 != 0
	}
}

func (d *hd44780) Snapshot() Snapshot {
	d.mu.Lock()
	defer d.mu.Unlock()
	lines := make([]string, 0, 2)
	for _, row := range d.lines {
		lines = append(lines, trim(row[:]))
	}
	return Snapshot{Model: "hd44780", Width: 16, Height: 2, Lines: lines, Enabled: d.on}
}

// max7219 decodes the MAX7219 SPI-like 16-bit command stream.
type max7219 struct {
	levels levelTracker
	pins   []string
	mu     sync.Mutex
	word   uint16
	bits   int
	digits [8]byte
	on     bool
}

func newMAX7219(pins []string) Decoder {
	return &max7219{levels: newLevelTracker(pins), pins: pins, on: true}
}

func (d *max7219) Pin(pin string, level int) {
	if len(d.pins) < 3 {
		return
	}
	if pin == pinsAt(d.pins, 2) {
		if d.levels.rising(pin, level) {
			d.latch()
		}
		return
	}
	if pin == pinsAt(d.pins, 0) && d.levels.rising(pin, level) {
		d.mu.Lock()
		d.word = d.word<<1 | uint16(d.levels.get(pinsAt(d.pins, 1))&1)
		d.bits++
		d.mu.Unlock()
		return
	}
	d.levels.set(pin, level)
}

func (d *max7219) latch() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.bits < 16 {
		d.bits = 0
		d.word = 0
		return
	}
	address := byte(d.word >> 8)
	value := byte(d.word) //nolint:gosec // intentional width conversion
	d.word = 0
	d.bits = 0
	if address >= 1 && address <= 8 {
		d.digits[address-1] = value
	}
	switch address {
	case 0x0C:
		d.on = value&1 != 0
	case 0x0F:
		if value&1 != 0 {
			for i := range d.digits {
				d.digits[i] = 0xFF
			}
		}
	}
}

func (d *max7219) Snapshot() Snapshot {
	d.mu.Lock()
	defer d.mu.Unlock()
	pixels := make([]byte, len(d.digits))
	copy(pixels, d.digits[:])
	lines := make([]string, 0, len(d.digits))
	for _, digit := range d.digits {
		lines = append(lines, hexDigit(digit))
	}
	return Snapshot{Model: "max7219", Width: 8, Height: 8, Pixels: pixels, Lines: lines, Enabled: d.on}
}

// shiftRegister decodes TM1638/AIP1640 LSB-first streams.
type shiftRegister struct {
	model       string
	levels      levelTracker
	pins        []string
	transaction bool
	mu          sync.Mutex
	bits        int
	value       byte
	bytes       []byte
	buffer      [16]byte
	on          bool
}

func newShiftRegister(model string, pins []string) Decoder {
	return &shiftRegister{
		model:       model,
		levels:      newLevelTracker(pins),
		pins:        pins,
		transaction: model == "tm1638",
		on:          true,
	}
}

func (d *shiftRegister) Pin(pin string, level int) {
	if len(d.pins) < 2 {
		return
	}
	// A transaction starts when the data line falls while the clock is high
	// (AIP1640) or when the strobe falls (TM1638). Resetting the bit phase
	// there keeps initialization pulses from shifting the byte stream.
	if d.transaction && pin == pinsAt(d.pins, 2) {
		old, ok := d.levels.set(pin, level)
		if ok && old == 1 && level == 0 {
			d.mu.Lock()
			d.bits = 0
			d.value = 0
			d.bytes = d.bytes[:0]
			d.mu.Unlock()
		}
		if ok && old == 0 && level == 1 {
			d.mu.Lock()
			d.processTransactionLocked()
			d.bytes = d.bytes[:0]
			d.mu.Unlock()
		}
		return
	}
	if pin == pinsAt(d.pins, 1) && level == 0 && d.levels.get(pinsAt(d.pins, 0)) == 1 {
		d.mu.Lock()
		d.bits = 0
		d.value = 0
		d.mu.Unlock()
	}
	if pin == pinsAt(d.pins, 0) && d.levels.rising(pin, level) {
		d.mu.Lock()
		d.value |= byte(d.levels.get(pinsAt(d.pins, 1))&1) << d.bits
		d.bits++
		if d.bits == 8 {
			d.bytes = append(d.bytes, d.value)
			d.value = 0
			d.bits = 0
			if !d.transaction {
				d.processLocked()
			}
		}
		d.mu.Unlock()
		return
	}
	d.levels.set(pin, level)
}

func (d *shiftRegister) processLocked() {
	if len(d.bytes) < 3 {
		return
	}
	command := d.bytes[0]
	address := d.bytes[1]
	value := d.bytes[2]
	d.bytes = d.bytes[:0]
	if command&0x40 == 0 {
		return
	}
	index := int(address & 0x0F)
	if index < len(d.buffer) {
		d.buffer[index] = value
	}
}

func (d *shiftRegister) processTransactionLocked() {
	if len(d.bytes) == 0 {
		return
	}
	if d.bytes[0]&0xC0 != 0xC0 {
		// Command transaction: the next transaction carries address/data.
		return
	}
	address := d.bytes[0] & 0x0F
	for index, value := range d.bytes[1:] {
		d.buffer[(address+byte(index))&0x0F] = value
	}
}

func (d *shiftRegister) Snapshot() Snapshot {
	d.mu.Lock()
	defer d.mu.Unlock()
	pixels := make([]byte, len(d.buffer))
	copy(pixels, d.buffer[:])
	lines := make([]string, 0, len(d.buffer))
	for _, value := range d.buffer {
		lines = append(lines, hexDigit(value))
	}
	return Snapshot{Model: d.model, Width: 16, Height: 8, Pixels: pixels, Lines: lines, Enabled: d.on}
}

func pinsAt(pins []string, index int) string {
	if index < len(pins) {
		return pins[index]
	}
	return ""
}

func trim(data []byte) string {
	end := len(data)
	for end > 0 && (data[end-1] == 0 || data[end-1] == ' ') {
		end--
	}
	return string(data[:end])
}

func hexDigit(value byte) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[value>>4], digits[value&0x0F]})
}
