package bus

import (
	"fmt"
	"strings"
	"sync"
)

// JHD1313M1 is an I2C character LCD (HD44780-compatible) with an RGB
// backlight controller on a second address.
type JHD1313M1 struct {
	mu      sync.RWMutex
	lines   [2][16]byte
	row     int
	column  int
	enabled bool
	red     byte
	green   byte
	blue    byte
}

// NewJHD1313M1 creates an empty 16x2 character display.
func NewJHD1313M1() *JHD1313M1 {
	return &JHD1313M1{enabled: true}
}

// Describe implements ChipModel.
func (d *JHD1313M1) Describe() ChipDescriptor {
	return ChipDescriptor{
		Model:     "jhd1313m1",
		Name:      "JHD1313M1 16x2 RGB character LCD",
		Transport: "i2c",
		Widget:    "display",
		Fields: []FieldDescriptor{
			{Name: "text", Widget: "text"},
			{Name: "clear", Widget: "button"},
			{Name: "red", Min: 0, Max: 255, Widget: "number"},
			{Name: "green", Min: 0, Max: 255, Widget: "number"},
			{Name: "blue", Min: 0, Max: 255, Widget: "number"},
		},
	}
}

// Transfer implements ChipModel for the LCD command/data stream.
func (d *JHD1313M1) Transfer(txn Transaction) (Response, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(txn.Write) > 0 {
		switch txn.Write[0] {
		case 0x00:
			d.commandLocked(txn.Write[1:])
		case 0x40:
			d.dataLocked(txn.Write[1:])
		default:
			// RGB backlight controller uses [register, value].
			if len(txn.Write) >= 2 {
				switch txn.Write[0] {
				case 0x04:
					d.red = txn.Write[1]
				case 0x03:
					d.green = txn.Write[1]
				case 0x02:
					d.blue = txn.Write[1]
				}
			}
		}
	}
	return Response{Values: d.valuesLocked()}, nil
}

func (d *JHD1313M1) commandLocked(data []byte) {
	for _, command := range data {
		switch {
		case command&0x80 != 0:
			addr := int(command & 0x7F)
			d.row = addr / 0x40
			d.column = addr % 0x40
			if d.row > 1 {
				d.row = 1
			}
			if d.column > 15 {
				d.column = 15
			}
		case command == 0x01:
			d.lines = [2][16]byte{}
			d.row, d.column = 0, 0
		case command == 0x02:
			d.row, d.column = 0, 0
		case command == 0x08:
			d.enabled = false
		case command == 0x0C:
			d.enabled = true
		}
	}
}

func (d *JHD1313M1) dataLocked(data []byte) {
	for _, char := range data {
		if d.row < 2 && d.column < 16 {
			d.lines[d.row][d.column] = char
		}
		d.column++
		if d.column >= 16 {
			d.column = 0
			d.row++
			if d.row > 1 {
				d.row = 1
			}
		}
	}
}

// Set implements ChipModel.
func (d *JHD1313M1) Set(field string, value interface{}) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch field {
	case "text":
		d.lines = [2][16]byte{}
		d.row, d.column = 0, 0
		d.dataLocked([]byte(fmt.Sprint(value)))
		return nil
	case "clear":
		d.lines = [2][16]byte{}
		d.row, d.column = 0, 0
		return nil
	case "red":
		d.red = byte(intValue(value)) //nolint:gosec // intentional width conversion
		return nil
	case "green":
		d.green = byte(intValue(value)) //nolint:gosec // intentional width conversion
		return nil
	case "blue":
		d.blue = byte(intValue(value)) //nolint:gosec // intentional width conversion
		return nil
	default:
		return fmt.Errorf("simulate: jhd1313m1 has no settable field %q", field)
	}
}

// Values implements ValuesProvider.
func (d *JHD1313M1) Values() map[string]interface{} {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.valuesLocked()
}

func (d *JHD1313M1) valuesLocked() map[string]interface{} {
	return map[string]interface{}{
		"text":    d.textLocked(),
		"enabled": d.enabled,
		"red":     int(d.red),
		"green":   int(d.green),
		"blue":    int(d.blue),
	}
}

func (d *JHD1313M1) textLocked() string {
	lines := make([]string, 0, 2)
	for _, line := range d.lines {
		lines = append(lines, strings.TrimRight(string(line[:]), "\x00 "))
	}
	return strings.Join(lines, "\n")
}

// Snapshot implements Display.
func (d *JHD1313M1) Snapshot() DisplaySnapshot {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return DisplaySnapshot{
		Model:   "jhd1313m1",
		Width:   16,
		Height:  2,
		Lines:   strings.Split(d.textLocked(), "\n"),
		Enabled: d.enabled,
	}
}

func init() {
	Register("jhd1313m1", func(ChipConfig) (ChipModel, error) { return NewJHD1313M1(), nil })
	Register("rgb-backlight", func(ChipConfig) (ChipModel, error) { return NewJHD1313M1(), nil })
	Register("ssd1306-spi", func(ChipConfig) (ChipModel, error) { return NewSSD1306("spi"), nil })
}
