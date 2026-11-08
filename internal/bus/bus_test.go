package bus

import (
	"io"
	"testing"
	"time"

	"github.com/disaster37/gobot-simulate/internal/clock"
	"github.com/disaster37/gobot-simulate/internal/state"
	"github.com/disaster37/gobot-simulate/internal/trace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestManager() (*Manager, *trace.Store) {
	clk := clock.NewMockClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	traces := trace.New(256, clk)
	return NewManager("dfp", clk, traces), traces
}

func TestI2CRegisterRoundTripAndNAK(t *testing.T) {
	manager, traces := newTestManager()
	model := NewRegisterModel(ChipDescriptor{Model: "generic"})
	manager.AddI2CChip(1, 0x10, model)

	conn, err := NewConnector(manager, 1).GetI2cConnection(0x10, 1)
	require.NoError(t, err)
	require.NoError(t, conn.WriteByteData(0x01, 0x42))
	value, err := conn.ReadByteData(0x01)
	require.NoError(t, err)
	assert.Equal(t, uint8(0x42), value)

	_, err = conn.ReadByteData(0x20)
	require.NoError(t, err, "register reads stay on the same addressed chip")

	other, err := NewConnector(manager, 1).GetI2cConnection(0x11, 1)
	require.NoError(t, err)
	_, err = other.ReadByte()
	require.ErrorIs(t, err, ErrNAK)

	records := traces.Query("dfp", 0, []trace.Kind{trace.KindBusTxn}, "")
	assert.NotEmpty(t, records)
	for _, record := range records {
		assert.Contains(t, record.Target, "i2c-1/")
	}
}

func TestI2CBusFaults(t *testing.T) {
	manager, _ := newTestManager()
	manager.AddI2CChip(1, 0x10, NewRegisterModel(ChipDescriptor{Model: "generic"}))
	conn, err := NewConnector(manager, 1).GetI2cConnection(0x10, 1)
	require.NoError(t, err)

	require.NoError(t, manager.SetBusFault("i2c-1", "bus-nak"))
	_, err = conn.ReadByte()
	require.ErrorIs(t, err, ErrNAK)

	require.NoError(t, manager.ClearBusFault("i2c-1", "bus-nak"))
	require.NoError(t, manager.SetBusFault("i2c-1", "bus-timeout"))
	_, err = conn.ReadByte()
	require.ErrorIs(t, err, ErrTimeout)
	require.NoError(t, manager.ClearBusFault("i2c-1", "bus-timeout"))

	require.Error(t, manager.SetBusFault("i2c-1", "stuck-high"))
}

type echoModel struct {
	last []byte
}

func (e *echoModel) Describe() ChipDescriptor { return ChipDescriptor{Model: "echo"} }
func (e *echoModel) Transfer(txn Transaction) (Response, error) {
	e.last = append([]byte(nil), txn.Write...)
	data := make([]byte, txn.ReadLen)
	for i := range data {
		data[i] = 0xA0 + byte(i)
	}
	return Response{Data: data}, nil
}
func (e *echoModel) Set(string, interface{}) error { return nil }

func TestSPIFullDuplexAndBusFault(t *testing.T) {
	manager, traces := newTestManager()
	model := &echoModel{}
	manager.AddSPIChip(0, 0, model)
	spi := manager.SPIConnection(0, 0)
	rx := make([]byte, 3)
	require.NoError(t, spi.TxRx([]byte{0x01, 0x02}, rx))
	assert.Equal(t, []byte{0x01, 0x02}, model.last)
	assert.Equal(t, []byte{0xA0, 0xA1, 0xA2}, rx)

	require.NoError(t, manager.SetBusFault("spi-0/0", "bus-timeout"))
	require.ErrorIs(t, spi.TxRx([]byte{1}, make([]byte, 1)), ErrTimeout)
	require.NoError(t, manager.ClearBusFault("spi-0/0", "bus-timeout"))
	assert.NotEmpty(t, traces.Query("dfp", 0, []trace.Kind{trace.KindBusTxn}, "spi-0/0"))
}

func TestSPIUnknownChip(t *testing.T) {
	manager, _ := newTestManager()
	err := manager.SPIConnection(0, 3).TxRx([]byte{1}, make([]byte, 1))
	require.ErrorIs(t, err, ErrChipNotFound)
}

func TestDS18B20Conversion(t *testing.T) {
	sensor := NewDS18B20(0x28, 42)
	require.NoError(t, sensor.Set("temperature", 21.5))
	require.NoError(t, sensor.Set("resolution", 10))
	require.NoError(t, sensor.Set("conversionTime", 375))
	require.NoError(t, sensor.Set("externalPower", true))
	values := sensor.Values()
	assert.Equal(t, 21.5, values["temperature"])
	assert.Equal(t, 10, values["resolution"])
	assert.Equal(t, 375, values["conversionTime"])
	assert.Equal(t, true, values["externalPower"])
	assert.Equal(t, "28-000000000000002a", sensor.ID())

	data := make([]byte, 4)
	require.NoError(t, sensor.ReadData("temperature", data))
	value, err := sensor.ReadInteger("temperature")
	require.NoError(t, err)
	assert.Equal(t, 21500, value)
	require.NoError(t, sensor.WriteInteger("resolution", 9))
	resolution, err := sensor.ReadInteger("resolution")
	require.NoError(t, err)
	assert.Equal(t, 9, resolution)

	require.Error(t, sensor.Set("resolution", 8))
	require.Error(t, sensor.Set("bogus", 1))
	_, err = sensor.ReadInteger("bogus")
	require.Error(t, err)
	require.Error(t, sensor.WriteInteger("bogus", 1))
}

func TestOneWireBusConnectorAndChipEntry(t *testing.T) {
	manager, _ := newTestManager()
	connection, err := manager.OneWire().GetOneWireConnection(0x28, 7)
	require.NoError(t, err)
	require.NotNil(t, connection)

	chip := manager.AddDS18B20Chip("temp", 7)
	require.NotNil(t, chip)
	entries := manager.Chips()
	require.Len(t, entries, 1)
	assert.Equal(t, "ds18b20", entries[0].Model)
	assert.Equal(t, "onewire", entries[0].Transport)
}

func TestSerialRoundTripAndHandler(t *testing.T) {
	port := newSerialPort("tty")
	port.SetHandler(func(request string) string {
		if request == "PING" {
			return "PONG\n"
		}
		return ""
	})
	n, err := port.SerialWrite([]byte("PING"))
	require.NoError(t, err)
	assert.Equal(t, 4, n)
	buf := make([]byte, 16)
	n, err = port.SerialRead(buf)
	require.NoError(t, err)
	assert.Equal(t, "PONG\n", string(buf[:n]))

	port.SetHandler(nil)
	_, err = port.SerialWrite([]byte("echo"))
	require.NoError(t, err)
	n, err = port.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, "echo", string(buf[:n]))

	require.NoError(t, port.Close())
	_, err = port.Read(buf)
	require.ErrorIs(t, err, io.EOF)
}

func TestSerialOversizeWriteDoesNotPanic(t *testing.T) {
	port := newSerialPort("tty")
	big := make([]byte, defaultSerialBuffer+1024)
	for i := range big {
		big[i] = byte('a' + i%26)
	}
	n, err := port.SerialWrite(big)
	require.NoError(t, err)
	assert.Equal(t, len(big), n)
	assert.LessOrEqual(t, len(port.Buffered()), defaultSerialBuffer)

	// The retained bytes are the tail of the write, matching a bounded FIFO.
	buffered := port.Buffered()
	assert.Equal(t, big[len(big)-len(buffered):], buffered)
}

func TestBLEDeviceCharacteristicsAndNotifications(t *testing.T) {
	device := newBLEDevice()
	assert.Equal(t, "simulator", device.Address())
	device.SetAddress("aa:bb")
	assert.Equal(t, "aa:bb", device.Address())

	level, err := device.ReadCharacteristic("2A19")
	require.NoError(t, err)
	assert.Equal(t, []byte{100}, level)

	var notified []byte
	require.NoError(t, device.Subscribe("2a19", func(data []byte) { notified = data }))
	device.Notify("2A19", []byte{90})
	assert.Equal(t, []byte{90}, notified)

	require.NoError(t, device.WriteCharacteristic("2a19", []byte{80}))
	level, err = device.ReadCharacteristic("2a19")
	require.NoError(t, err)
	assert.Equal(t, []byte{80}, level)

	_, err = device.ReadCharacteristic("ffff")
	require.Error(t, err)
	require.Error(t, device.Subscribe("2a19", nil))
}

func TestManagerChipsAndBuses(t *testing.T) {
	manager, _ := newTestManager()
	model := NewRegisterModel(ChipDescriptor{Model: "generic", Name: "generic"})
	manager.AddChipWithModel(&ChipEntry{
		ID: "chip", BoardID: "dfp", Transport: "i2c", Bus: "i2c-1",
		BusNumber: 1, Address: 0x10, Model: "generic",
	}, model)

	buses := manager.Buses()
	require.Len(t, buses, 1)
	assert.Equal(t, "i2c-1", buses[0]["id"])

	entry, err := manager.Chip("chip")
	require.NoError(t, err)
	assert.Equal(t, 0x10, entry.Address)

	chipModel, err := manager.ChipModel("chip")
	require.NoError(t, err)
	assert.Same(t, model, chipModel)

	_, err = manager.Chip("missing")
	require.ErrorIs(t, err, ErrChipNotFound)
	_, err = manager.ChipModel("missing")
	require.ErrorIs(t, err, ErrChipNotFound)
}

func TestADSChipConversions(t *testing.T) {
	ads := NewADS1x15(16)
	// Default gain 1 (FSR 4.096) single-ended channel 0 at 1 V.
	config := uint16(0x8000 | (4 << 12) | (1 << 9) | 0x0103)
	response, err := ads.Transfer(Transaction{Write: []byte{0x01, byte(config >> 8), byte(config)}, ReadLen: 0}) //nolint:gosec // intentional width conversion
	require.NoError(t, err)
	assert.Empty(t, response.Data)
	response, err = ads.Transfer(Transaction{Write: []byte{0x00}, ReadLen: 2})
	require.NoError(t, err)
	raw := int16(uint16(response.Data[0])<<8 | uint16(response.Data[1])) //nolint:gosec // intentional width conversion
	assert.InDelta(t, 8000, int(raw), 5)

	require.NoError(t, ads.Set("channel0", 2.0))
	response, _ = ads.Transfer(Transaction{Write: []byte{0x00}, ReadLen: 2})
	raw = int16(uint16(response.Data[0])<<8 | uint16(response.Data[1])) //nolint:gosec // intentional width conversion
	assert.InDelta(t, 16000, int(raw), 10)

	require.Error(t, ads.Set("channel9", 1.0))
	require.Error(t, ads.Set("channel0", -1.0))
	require.Error(t, ads.Set("bogus", 1.0))
}

func TestMCP23017RoundTrip(t *testing.T) {
	expander := NewMCP23017(0x20)
	// Direction A: bit 0 output, rest input.
	_, err := expander.Transfer(Transaction{Write: []byte{0x00, 0xFE}})
	require.NoError(t, err)
	// Write GPIOA bit 0 high.
	_, err = expander.Transfer(Transaction{Write: []byte{0x12, 0x01}})
	require.NoError(t, err)
	response, err := expander.Transfer(Transaction{Write: []byte{0x12}, ReadLen: 1})
	require.NoError(t, err)
	assert.Equal(t, byte(0x01), response.Data[0])

	require.NoError(t, expander.Set("portA", 0x80))
	values := expander.Values()
	assert.Equal(t, 0x80, values["portA"])
	require.Error(t, expander.Set("bogus", 1))
}

func TestPCA9685Channels(t *testing.T) {
	pca := NewPCA9685()
	require.NoError(t, pca.Set("channel0", 2048))
	values := pca.Values()
	assert.Equal(t, 2048, values["channel0"])
	require.NoError(t, pca.Set("prescale", 121))
	assert.Equal(t, 121, pca.Values()["prescale"])
	require.Error(t, pca.Set("channel16", 1))
	require.Error(t, pca.Set("channel0", 5000))
	require.Error(t, pca.Set("bogus", 1))
}

func TestBH1750Measurement(t *testing.T) {
	sensor := NewBH1750()
	require.NoError(t, sensor.Set("lux", 500.0))
	response, err := sensor.Transfer(Transaction{ReadLen: 2})
	require.NoError(t, err)
	raw := uint16(response.Data[0])<<8 | uint16(response.Data[1])
	assert.Equal(t, uint16(600), raw)
	require.NoError(t, sensor.Set("power", false))
	power, ok := sensor.Values()["power"].(bool)
	require.True(t, ok)
	assert.False(t, power)
	require.Error(t, sensor.Set("lux", -1.0))
	require.Error(t, sensor.Set("bogus", 1))
}

func TestSSD1306FramebufferAndText(t *testing.T) {
	display := NewSSD1306("i2c")
	_, err := display.Transfer(Transaction{Write: []byte{0x00, 0x21, 0, 127, 0x22, 0, 7}})
	require.NoError(t, err)
	_, err = display.Transfer(Transaction{Write: append([]byte{0x40}, []byte{0xFF, 0xAA}...)})
	require.NoError(t, err)
	snapshot := display.Snapshot()
	assert.Equal(t, 128*64/8, len(snapshot.Pixels))
	assert.Equal(t, byte(0xFF), snapshot.Pixels[0])

	require.NoError(t, display.Set("text", "hello\nworld"))
	assert.NotEmpty(t, display.Snapshot().Pixels)
	require.NoError(t, display.Set("clear", nil))
	assert.Equal(t, byte(0), display.Snapshot().Pixels[0])
	require.Error(t, display.Set("bogus", 1))
}

func TestRegisterModelGenericDefaultsAndSPI(t *testing.T) {
	desc, opts := GenericConfig(map[string]interface{}{
		"defaults": map[string]interface{}{"01": 5, "2": 7},
	})
	model := NewRegisterModel(desc, opts...)
	response, err := model.Transfer(Transaction{Write: []byte{0x01}, ReadLen: 2})
	require.NoError(t, err)
	assert.Equal(t, byte(5), response.Data[0])
	assert.Equal(t, byte(7), response.Data[1])

	require.Error(t, model.Set("nothing", 1)) // no setter configured
}

func TestRegistryAndUnknownModel(t *testing.T) {
	assert.Contains(t, Models(), "ssd1306")
	assert.Contains(t, Models(), "generic")
	_, err := NewChip("does-not-exist", ChipConfig{})
	require.ErrorIs(t, err, ErrChipNotFound)

	for _, name := range Models() {
		model, err := NewChip(name, ChipConfig{Transport: "i2c", Address: 0x20, Options: map[string]interface{}{}})
		require.NoError(t, err, name)
		assert.NotEmpty(t, model.Describe().Model)
	}
}

func TestChipModelJSON(t *testing.T) {
	model := NewSSD1306("i2c")
	out := ChipModelJSON(model)
	assert.Contains(t, out, "descriptor")
	assert.Contains(t, out, "values")
	assert.Contains(t, out, "display")
}

func TestIntValueAndToFloat(t *testing.T) {
	assert.Equal(t, 1, intValue(int64(1)))
	assert.Equal(t, 1, intValue(float64(1.9)))
	assert.Equal(t, 1, intValue(true))
	assert.Equal(t, 0, intValue("x"))
	_, ok := toFloat(state.FaultStuckLow)
	assert.False(t, ok)
}
