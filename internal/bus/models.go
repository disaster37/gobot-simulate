package bus

import (
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
)

// DisplaySnapshot is the renderable content of a simulated display.
type DisplaySnapshot struct {
	Model   string   `json:"model"`
	Width   int      `json:"width"`
	Height  int      `json:"height"`
	Pixels  []byte   `json:"pixels,omitempty"`
	Lines   []string `json:"lines,omitempty"`
	Enabled bool     `json:"enabled"`
	Zoom    int      `json:"zoom,omitempty"`
}

// Display is implemented by chips that maintain renderable content.
type Display interface {
	Snapshot() DisplaySnapshot
}

// SSD1306 is an I2C/SPI monochrome OLED model with a real framebuffer.
type SSD1306 struct {
	mu        sync.RWMutex
	transport string
	width     int
	height    int
	buffer    []byte
	col       int
	page      int
	enabled   bool
	contrast  byte
}

// NewSSD1306 creates a 128x64 OLED model.
func NewSSD1306(transport string) *SSD1306 {
	return &SSD1306{
		transport: transport,
		width:     128,
		height:    64,
		buffer:    make([]byte, 128*64/8),
		enabled:   true,
		contrast:  0xCF,
	}
}

// Describe implements ChipModel.
func (d *SSD1306) Describe() ChipDescriptor {
	return ChipDescriptor{
		Model:     "ssd1306",
		Name:      "SSD1306 OLED display",
		Transport: d.transport,
		Widget:    "display",
		Fields: []FieldDescriptor{
			{Name: "enabled", Widget: "switch"},
			{Name: "contrast", Min: 0, Max: 255, Widget: "number"},
			{Name: "text", Widget: "text"},
		},
	}
}

// Transfer implements ChipModel.
func (d *SSD1306) Transfer(txn Transaction) (Response, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if txn.Transport == "spi" {
		if len(txn.Write) < 1 {
			return Response{Data: make([]byte, txn.ReadLen)}, nil
		}
		control := txn.Write[0]
		payload := txn.Write[1:]
		if control&0x80 != 0 {
			d.command(payload)
		} else {
			d.data(payload)
		}
		return Response{Data: make([]byte, txn.ReadLen), Values: d.valuesLocked()}, nil
	}
	if len(txn.Write) > 0 {
		control := txn.Write[0]
		payload := txn.Write[1:]
		switch control {
		case 0x00:
			d.command(payload)
		case 0x40:
			d.data(payload)
		default:
			// Some controllers accept command-only writes without a control
			// byte; treat the whole write as commands.
			d.command(txn.Write)
		}
	}
	return Response{Data: make([]byte, txn.ReadLen), Values: d.valuesLocked()}, nil
}

func (d *SSD1306) command(data []byte) {
	for i := 0; i < len(data); i++ {
		cmd := data[i]
		switch cmd {
		case 0x20:
			if i+1 < len(data) {
				i++
			}
		case 0x21:
			if i+2 < len(data) {
				d.col = int(data[i+1])
				i += 2
			}
		case 0x22:
			if i+2 < len(data) {
				d.page = int(data[i+1]) & 0x07
				i += 2
			}
		case 0x81:
			if i+1 < len(data) {
				d.contrast = data[i+1]
				i++
			}
		case 0xAE:
			d.enabled = false
		case 0xAF:
			d.enabled = true
		default:
			switch {
			case cmd >= 0xB0 && cmd <= 0xB7:
				d.page = int(cmd & 0x07)
			case cmd <= 0x0F:
				d.col = (d.col & 0xF0) | int(cmd)
			case cmd >= 0x10 && cmd <= 0x1F:
				d.col = (d.col & 0x0F) | (int(cmd&0x0F) << 4)
			}
		}
	}
}

func (d *SSD1306) data(data []byte) {
	for _, b := range data {
		if d.page >= 0 && d.page < d.height/8 && d.col >= 0 && d.col < d.width {
			d.buffer[d.page*d.width+d.col] = b
		}
		d.col++
		if d.col >= d.width {
			d.col = 0
			d.page = (d.page + 1) % (d.height / 8)
		}
	}
}

// Set implements ChipModel.
func (d *SSD1306) Set(field string, value interface{}) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch field {
	case "enabled":
		if b, ok := value.(bool); ok {
			d.enabled = b
			return nil
		}
	case "contrast":
		d.contrast = byte(intValue(value)) //nolint:gosec // intentional width conversion
		return nil
	case "text":
		d.clearLocked()
		lines := strings.Split(fmt.Sprint(value), "\n")
		for row, line := range lines {
			if row >= d.height/8 {
				break
			}
			d.writeTextLocked(row, line)
		}
		return nil
	case "clear":
		d.clearLocked()
		return nil
	}
	return fmt.Errorf("simulate: ssd1306 has no settable field %q", field)
}

func (d *SSD1306) clearLocked() {
	for i := range d.buffer {
		d.buffer[i] = 0
	}
	d.col = 0
	d.page = 0
}

func (d *SSD1306) writeTextLocked(page int, text string) {
	// 5x7 font using a compact subset is overkill for the simulator; store a
	// deterministic pseudo-glyph so the UI shows activity in the right cells.
	for col, r := range text {
		if col*6+5 >= d.width {
			break
		}
		d.buffer[page*d.width+col*6] = byte(r)        //nolint:gosec // intentional width conversion
		d.buffer[page*d.width+col*6+1] = byte(r >> 8) //nolint:gosec // intentional width conversion
	}
}

// Values implements ValuesProvider.
func (d *SSD1306) Values() map[string]interface{} {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.valuesLocked()
}

func (d *SSD1306) valuesLocked() map[string]interface{} {
	return map[string]interface{}{
		"enabled":  d.enabled,
		"contrast": int(d.contrast),
		"width":    d.width,
		"height":   d.height,
		"page":     d.page,
		"column":   d.col,
		"pixels":   append([]byte(nil), d.buffer...),
	}
}

// Snapshot implements Display.
func (d *SSD1306) Snapshot() DisplaySnapshot {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return DisplaySnapshot{
		Model:   "ssd1306",
		Width:   d.width,
		Height:  d.height,
		Pixels:  append([]byte(nil), d.buffer...),
		Enabled: d.enabled,
	}
}

// MCP23017 is a 16-bit I2C GPIO expander.
type MCP23017 struct {
	mu   sync.RWMutex
	regs map[uint8]byte
	ptr  uint8
}

// NewMCP23017 creates an expander with all pins as inputs.
func NewMCP23017(_ int) *MCP23017 {
	regs := map[uint8]byte{
		0x00: 0xFF, 0x01: 0xFF, // IODIR A/B
		0x0C: 0x00, 0x0D: 0x00, // GPPU
	}
	return &MCP23017{regs: regs}
}

// Describe implements ChipModel.
func (m *MCP23017) Describe() ChipDescriptor {
	return ChipDescriptor{
		Model:     "mcp23017",
		Name:      "MCP23017 16-bit GPIO expander",
		Transport: "i2c",
		Widget:    "expander",
		Registers: []RegisterDescriptor{
			{Address: 0x00, Name: "IODIRA", Width: 8, Access: "rw", Reset: 0xFF},
			{Address: 0x01, Name: "IODIRB", Width: 8, Access: "rw", Reset: 0xFF},
			{Address: 0x12, Name: "GPIOA", Width: 8, Access: "rw"},
			{Address: 0x13, Name: "GPIOB", Width: 8, Access: "rw"},
		},
		Fields: []FieldDescriptor{
			{Name: "portA", Widget: "register"},
			{Name: "portB", Widget: "register"},
		},
	}
}

// Transfer implements ChipModel.
func (m *MCP23017) Transfer(txn Transaction) (Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(txn.Write) > 0 {
		m.ptr = txn.Write[0]
		for _, value := range txn.Write[1:] {
			m.regs[m.ptr] = value
			m.ptr++
		}
	}
	data := make([]byte, 0, txn.ReadLen)
	for i := 0; i < txn.ReadLen; i++ {
		value := m.regs[m.ptr]
		if m.ptr == 0x12 || m.ptr == 0x13 {
			direction := m.regs[m.ptr-0x12]
			if direction == 0xFF {
				// Inputs are pulled up unless the caller drives them low.
				value = m.regs[m.ptr] | 0xFF
			}
		}
		data = append(data, value)
		m.ptr++
	}
	return Response{Data: data, Values: m.valuesLocked()}, nil
}

// Set implements ChipModel.
func (m *MCP23017) Set(field string, value interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch field {
	case "portA", "portB":
		reg := uint8(0x12)
		if field == "portB" {
			reg = 0x13
		}
		m.regs[reg] = byte(intValue(value)) //nolint:gosec // intentional width conversion
		return nil
	default:
		return fmt.Errorf("simulate: mcp23017 has no settable field %q", field)
	}
}

// Values implements ValuesProvider.
func (m *MCP23017) Values() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.valuesLocked()
}

func (m *MCP23017) valuesLocked() map[string]interface{} {
	return map[string]interface{}{
		"portA": int(m.regs[0x12]),
		"portB": int(m.regs[0x13]),
		"dirA":  int(m.regs[0x00]),
		"dirB":  int(m.regs[0x01]),
	}
}

// PCA9685 is a 16-channel PWM driver.
type PCA9685 struct {
	mu   sync.RWMutex
	regs map[uint8]byte
	ptr  uint8
}

// NewPCA9685 creates a PWM controller.
func NewPCA9685() *PCA9685 {
	return &PCA9685{regs: map[uint8]byte{0x00: 0x11}}
}

// Describe implements ChipModel.
func (p *PCA9685) Describe() ChipDescriptor {
	return ChipDescriptor{
		Model:     "pca9685",
		Name:      "PCA9685 16-channel PWM",
		Transport: "i2c",
		Widget:    "pwm",
		Fields: []FieldDescriptor{
			{Name: "channel0", Min: 0, Max: 4095, Widget: "number"},
			{Name: "channel1", Min: 0, Max: 4095, Widget: "number"},
			{Name: "prescale", Min: 3, Max: 255, Widget: "number"},
		},
	}
}

// Transfer implements ChipModel.
func (p *PCA9685) Transfer(txn Transaction) (Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(txn.Write) > 0 {
		p.ptr = txn.Write[0]
		for _, value := range txn.Write[1:] {
			p.regs[p.ptr] = value
			p.ptr++
		}
	}
	data := make([]byte, 0, txn.ReadLen)
	for i := 0; i < txn.ReadLen; i++ {
		data = append(data, p.regs[p.ptr])
		p.ptr++
	}
	return Response{Data: data, Values: p.valuesLocked()}, nil
}

// Set implements ChipModel.
func (p *PCA9685) Set(field string, value interface{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if strings.HasPrefix(field, "channel") {
		var channel int
		if _, err := fmt.Sscanf(field, "channel%d", &channel); err != nil || channel < 0 || channel > 15 {
			return fmt.Errorf("simulate: invalid pca9685 channel %q", field)
		}
		off := intValue(value)
		if off < 0 || off > 4095 {
			return fmt.Errorf("simulate: pca9685 channel value out of range")
		}
		base := uint8(0x06 + channel*4)
		p.regs[base] = 0
		p.regs[base+1] = 0
		p.regs[base+2] = byte(off) //nolint:gosec // intentional width conversion
		p.regs[base+3] = byte(off >> 8)
		return nil
	}
	if field == "prescale" {
		p.regs[0xFE] = byte(intValue(value)) //nolint:gosec // intentional width conversion
		return nil
	}
	return fmt.Errorf("simulate: pca9685 has no settable field %q", field)
}

// Values implements ValuesProvider.
func (p *PCA9685) Values() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.valuesLocked()
}

func (p *PCA9685) valuesLocked() map[string]interface{} {
	values := map[string]interface{}{
		"prescale": int(p.regs[0xFE]),
		"mode1":    int(p.regs[0x00]),
	}
	for channel := 0; channel < 16; channel++ {
		base := uint8(0x06 + channel*4)
		values[fmt.Sprintf("channel%d", channel)] = int(binary.LittleEndian.Uint16([]byte{p.regs[base+2], p.regs[base+3]}))
	}
	return values
}

// ADS1x15 is an ADC model with settable input voltages.
type ADS1x15 struct {
	mu       sync.RWMutex
	bits     int
	config   uint16
	voltages map[int]float64
	ref      float64
	ptr      uint8
}

// NewADS1x15 creates an ADC with four input channels at 1 V.
func NewADS1x15(bits int) *ADS1x15 {
	voltages := make(map[int]float64, 4)
	for channel := 0; channel < 4; channel++ {
		voltages[channel] = 1.0
	}
	return &ADS1x15{bits: bits, voltages: voltages, ref: 4.096}
}

// Describe implements ChipModel.
func (a *ADS1x15) Describe() ChipDescriptor {
	return ChipDescriptor{
		Model:     "ads1x15",
		Name:      "ADS1x15 ADC",
		Transport: "i2c",
		Widget:    "adc",
		Fields: []FieldDescriptor{
			{Name: "channel0", Unit: "V", Min: 0, Max: 4.096, Widget: "number"},
			{Name: "channel1", Unit: "V", Min: 0, Max: 4.096, Widget: "number"},
			{Name: "channel2", Unit: "V", Min: 0, Max: 4.096, Widget: "number"},
			{Name: "channel3", Unit: "V", Min: 0, Max: 4.096, Widget: "number"},
		},
	}
}

// Transfer implements ChipModel.
func (a *ADS1x15) Transfer(txn Transaction) (Response, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(txn.Write) > 0 {
		a.ptr = txn.Write[0]
	}
	if len(txn.Write) >= 3 {
		a.ptr = txn.Write[0]
		a.config = uint16(txn.Write[1])<<8 | uint16(txn.Write[2])
	}
	data := make([]byte, 0, txn.ReadLen)
	if len(txn.Write) == 1 {
		switch txn.Write[0] {
		case 0x00:
			raw := a.conversionLocked()
			data = append(data, byte(raw>>8), byte(raw)) //nolint:gosec // intentional width conversion
		case 0x01:
			// The OS bit reads back set once a conversion has completed,
			// which is what the gobot driver polls for.
			config := a.config | 0x8000
			data = append(data, byte(config>>8), byte(config)) //nolint:gosec // intentional width conversion
		}
	}
	for len(data) < txn.ReadLen {
		data = append(data, 0)
	}
	return Response{Data: data, Values: a.valuesLocked()}, nil
}

// ads1x15FullScale mirrors the driver's PGA full-scale-range table.
var ads1x15FullScale = [8]float64{6.144, 4.096, 2.048, 1.024, 0.512, 0.256, 0.256, 0.256}

// conversionLocked returns the two's-complement conversion register value for
// the currently selected mux and gain.
func (a *ADS1x15) conversionLocked() uint16 {
	mux := int((a.config >> 12) & 0x07)
	gain := int((a.config >> 9) & 0x07)
	fsr := ads1x15FullScale[gain]
	var voltage float64
	if mux >= 4 {
		channel := mux - 4
		if channel > 3 {
			channel = 3
		}
		voltage = a.voltages[channel]
	} else {
		pairs := [4][2]int{{0, 1}, {0, 3}, {1, 3}, {2, 3}}
		if mux < 0 || mux >= len(pairs) {
			mux = 0
		}
		voltage = a.voltages[pairs[mux][0]] - a.voltages[pairs[mux][1]] //nolint:gosec // intentional width conversion
	}
	raw := int(voltage / fsr * 32768)
	if raw > 32767 {
		raw = 32767
	}
	if raw < -32768 {
		raw = -32768
	}
	return uint16(int16(raw)) //nolint:gosec // intentional width conversion
}

// Set implements ChipModel.
func (a *ADS1x15) Set(field string, value interface{}) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	var channel int
	if _, err := fmt.Sscanf(field, "channel%d", &channel); err != nil {
		return fmt.Errorf("simulate: ads1x15 has no settable field %q", field)
	}
	if channel < 0 || channel > 3 {
		return fmt.Errorf("simulate: ads1x15 channel out of range")
	}
	f, ok := toFloat(value)
	if !ok || f < 0 || f > a.ref {
		return fmt.Errorf("simulate: ads1x15 voltage out of range")
	}
	a.voltages[channel] = f
	return nil
}

// Values implements ValuesProvider.
func (a *ADS1x15) Values() map[string]interface{} {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.valuesLocked()
}

func (a *ADS1x15) valuesLocked() map[string]interface{} {
	values := make(map[string]interface{}, len(a.voltages)+2)
	for channel, voltage := range a.voltages {
		values[fmt.Sprintf("channel%d", channel)] = voltage
	}
	values["config"] = int(a.config)
	values["bits"] = a.bits
	values["fullScale"] = ads1x15FullScale[int((a.config>>9)&0x07)]
	return values
}

// BH1750 is a digital ambient light sensor.
type BH1750 struct {
	mu      sync.RWMutex
	lux     float64
	power   bool
	measure bool
}

// NewBH1750 creates an ambient light sensor at 100 lux.
func NewBH1750() *BH1750 { return &BH1750{lux: 100} }

// Describe implements ChipModel.
func (b *BH1750) Describe() ChipDescriptor {
	return ChipDescriptor{
		Model:     "bh1750",
		Name:      "BH1750 light sensor",
		Transport: "i2c",
		Widget:    "sensor",
		Fields: []FieldDescriptor{
			{Name: "lux", Unit: "lx", Min: 0, Max: 65535, Widget: "number"},
		},
	}
}

// Transfer implements ChipModel.
func (b *BH1750) Transfer(txn Transaction) (Response, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(txn.Write) > 0 {
		switch txn.Write[0] {
		case 0x01:
			b.power = true
		case 0x00:
			b.power = false
		case 0x10, 0x11, 0x20, 0x21:
			b.power = true
			b.measure = true
		case 0x23:
			b.power = true
			b.measure = true
		}
	}
	data := make([]byte, 0, txn.ReadLen)
	if txn.ReadLen >= 2 {
		raw := uint16(b.lux * 1.2)
		data = append(data, byte(raw>>8), byte(raw)) //nolint:gosec // intentional width conversion
	}
	for len(data) < txn.ReadLen {
		data = append(data, 0)
	}
	return Response{Data: data, Values: b.valuesLocked()}, nil
}

// Set implements ChipModel.
func (b *BH1750) Set(field string, value interface{}) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch field {
	case "lux":
		lux, ok := toFloat(value)
		if !ok || lux < 0 || lux > 65535 {
			return fmt.Errorf("simulate: lux out of range")
		}
		b.lux = lux
		return nil
	case "power":
		if v, ok := value.(bool); ok {
			b.power = v
			return nil
		}
	}
	return fmt.Errorf("simulate: bh1750 has no settable field %q", field)
}

// Values implements ValuesProvider.
func (b *BH1750) Values() map[string]interface{} {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.valuesLocked()
}

func (b *BH1750) valuesLocked() map[string]interface{} {
	return map[string]interface{}{"lux": b.lux, "power": b.power, "measuring": b.measure}
}

func toFloat(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	default:
		return 0, false
	}
}

func init() {
	Register("ssd1306", func(cfg ChipConfig) (ChipModel, error) {
		transport := cfg.Transport
		if transport == "" {
			transport = "i2c"
		}
		return NewSSD1306(transport), nil
	})
	Register("mcp23017", func(cfg ChipConfig) (ChipModel, error) {
		address := cfg.Address
		if address == 0 {
			address = 0x20
		}
		return NewMCP23017(address), nil
	})
	Register("pca9685", func(ChipConfig) (ChipModel, error) { return NewPCA9685(), nil })
	Register("ads1115", func(ChipConfig) (ChipModel, error) { return NewADS1x15(16), nil })
	Register("ads1015", func(ChipConfig) (ChipModel, error) { return NewADS1x15(12), nil })
	Register("bh1750", func(ChipConfig) (ChipModel, error) { return NewBH1750(), nil })
	// Register a generic register-map fallback for the remaining long-tail
	// models so unmodelled drivers still attach and expose raw registers.
	for _, model := range longTailModels {
		name := model
		registryMu.RLock()
		_, exists := registry[name]
		registryMu.RUnlock()
		if exists {
			continue
		}
		Register(name, func(cfg ChipConfig) (ChipModel, error) {
			desc, opts := GenericConfig(cfg.Options)
			desc.Model = name
			desc.Name = name + " (generic register map)"
			if cfg.Address != 0 {
				desc.Address = cfg.Address
			}
			return NewRegisterModel(desc, opts...), nil
		})
	}
}

// longTailModels are registered with the generic register-map fallback. Models
// with a dedicated implementation are intentionally omitted so their factory
// keeps precedence regardless of file init order.
var longTailModels = []string{
	"adafruit1109", "adafruit2327", "adafruit2348", "adxl345", "aip1640",
	"bme280", "bmp180", "bmp280", "bmp388", "blinkm", "ccs811",
	"drv2605l", "ds3231", "grovepi", "hd44780", "hmc5883l", "hmc6352",
	"ina3221", "jhd1313m1", "l3gd20h", "lidarlite", "max7219",
	"mfrc522", "mma7660", "mpu6050", "pcf8574", "pcf8591", "pca9501",
	"pca953x", "sht2x", "sht3x", "ssd1306-spi", "tm1638", "tsl2561",
	"wiichuck", "yl40",
}

// ChipModelJSON returns a sorted snapshot of a chip's documented values.
func ChipModelJSON(model ChipModel) map[string]interface{} {
	out := map[string]interface{}{"descriptor": model.Describe()}
	if provider, ok := model.(ValuesProvider); ok {
		out["values"] = provider.Values()
	}
	if display, ok := model.(Display); ok {
		out["display"] = display.Snapshot()
	}
	return out
}
