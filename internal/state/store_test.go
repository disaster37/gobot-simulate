package state

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/disaster37/gobot-simulate/internal/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore() *Store {
	clk := clock.NewMockClock(time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC))
	return NewStore("0.1.0", 32, clk)
}

func TestStoreRegisterAndResolveBoard(t *testing.T) {
	store := newTestStore()
	board, err := store.RegisterBoard("dfp", "fat", "raspberry-pi", "", 0)
	require.NoError(t, err)
	require.NotNil(t, board)

	again, err := store.RegisterBoard("dfp", "fat", "raspberry-pi", "", 0)
	require.NoError(t, err)
	assert.Same(t, board, again)

	resolved, err := store.Resolve("fat")
	require.NoError(t, err)
	assert.Equal(t, "dfp", resolved.ID())

	_, err = store.Resolve("missing")
	assert.ErrorIs(t, err, ErrBoardNotFound)

	_, err = store.RegisterBoard("bad id", "bad", "", "", 0)
	assert.ErrorIs(t, err, ErrInvalidID)
}

func TestStorePullupAndInvertedInitialState(t *testing.T) {
	store := newTestStore()
	board, err := store.RegisterBoard("dfp", "fat", "", "", 0)
	require.NoError(t, err)

	board.SetInvertInitialPinState("10")
	board.SetInputPullup("10", "11")

	inverted, ok := board.Pin("10")
	require.True(t, ok)
	assert.True(t, inverted.InvertInitial)
	assert.True(t, inverted.Pullup)
	assert.Equal(t, PinModeInputPullup, inverted.Mode)
	assert.Equal(t, 0, inverted.DigitalValue)

	pulled, ok := board.Pin("11")
	require.True(t, ok)
	assert.False(t, pulled.InvertInitial)
	assert.Equal(t, 1, pulled.DigitalValue)
	assert.Equal(t, byte(1), pulled.ByteValue)
}

func TestStoreDigitalWriteUpdatesBoundRelay(t *testing.T) {
	store := newTestStore()
	board, err := store.RegisterBoard("tfp", "tfp", "", "", 0)
	require.NoError(t, err)

	board.RegisterDevice(DeviceState{
		ID: "relay_pond", Name: "Pond", Label: "Pond", Kind: DeviceKindRelay,
		Pins: []string{"4"}, Inverted: true,
	})
	board.RegisterDevice(DeviceState{
		ID: "relay_pump", Name: "Pump", Label: "Pump", Kind: DeviceKindRelay,
		Pins: []string{"5"},
	})

	board.DigitalWritePin("4", 0, true, PinModeOutput, SourceHardwareCode)
	device, ok := board.Device("relay_pond")
	require.True(t, ok)
	assert.True(t, device.Active)

	board.DigitalWritePin("5", 1, true, PinModeOutput, SourceHardwareCode)
	device, ok = board.Device("relay_pump")
	require.True(t, ok)
	assert.True(t, device.Active)
}

func TestStorePressAndReleaseButton(t *testing.T) {
	store := newTestStore()
	board, err := store.RegisterBoard("dfp", "fat", "", "", 0)
	require.NoError(t, err)

	board.SetInputPullup("11")
	board.RegisterDevice(DeviceState{
		ID: "button_start", Name: "start", Label: "Start", Kind: DeviceKindButton,
		Pins: []string{"11"}, DefaultState: 1,
	})

	require.NoError(t, store.PressButton("dfp", "button_start"))
	pin, err := store.Pin("dfp", "11")
	require.NoError(t, err)
	assert.Equal(t, 0, pin.DigitalValue)
	device, err := store.Device("dfp", "button_start")
	require.NoError(t, err)
	assert.True(t, device.Active)

	require.NoError(t, store.ReleaseButton("dfp", "button_start"))
	pin, err = store.Pin("dfp", "11")
	require.NoError(t, err)
	assert.Equal(t, 1, pin.DigitalValue)
	device, err = store.Device("dfp", "button_start")
	require.NoError(t, err)
	assert.False(t, device.Active)
}

func TestStoreFaultAndValues(t *testing.T) {
	store := newTestStore()
	board, err := store.RegisterBoard("dfp", "fat", "", "", 0)
	require.NoError(t, err)

	fault := errors.New("i2c timeout")
	require.NoError(t, store.SetFault("dfp", fault))
	assert.ErrorIs(t, board.FaultError(), fault)

	snapshot := board.Snapshot()
	assert.True(t, snapshot.Fault.Active)
	assert.Equal(t, "i2c timeout", snapshot.Fault.Message)

	require.NoError(t, store.ClearFault("dfp"))
	assert.NoError(t, board.FaultError())

	require.NoError(t, store.SetValue("dfp", "distance", float64(12.5), SourceUI))
	values, err := store.Values("dfp")
	require.NoError(t, err)
	assert.Equal(t, float64(12.5), values["distance"].Value)
}

func TestStoreEventsReplayAndSnapshot(t *testing.T) {
	store := newTestStore()
	_, err := store.RegisterBoard("dfp", "fat", "", "", 0)
	require.NoError(t, err)

	board, err := store.Resolve("dfp")
	require.NoError(t, err)
	board.SetInputPullup("11")
	board.SetDigitalValue("11", 0, SourceUI)

	current := store.CurrentSeq()
	require.Greater(t, current, uint64(0))

	ch, missed, ok := store.SubscribeWithReplay(current - 1)
	require.True(t, ok)
	require.NotEmpty(t, missed)
	defer store.Unsubscribe(ch)

	snapshot := store.Snapshot()
	assert.Equal(t, current, snapshot.Meta.CurrentSeq)
	require.Len(t, snapshot.Boards, 1)
	assert.Equal(t, "dfp", snapshot.Boards[0].ID)
}

func TestStoreCopyOnRead(t *testing.T) {
	store := newTestStore()
	board, err := store.RegisterBoard("dfp", "fat", "", "", 0)
	require.NoError(t, err)
	board.SetInputPullup("11")
	board.RegisterDevice(DeviceState{
		ID: "button_start", Kind: DeviceKindButton, Pins: []string{"11"}, DefaultState: 1,
	})

	snapshot, err := store.BoardState("dfp")
	require.NoError(t, err)
	snapshot.Pins["11"] = PinState{Pin: "11", DigitalValue: 42}
	snapshot.Devices["button_start"].Pins[0] = "broken"

	fresh, err := store.BoardState("dfp")
	require.NoError(t, err)
	assert.Equal(t, 1, fresh.Pins["11"].DigitalValue)
	assert.Equal(t, []string{"11"}, fresh.Devices["button_start"].Pins)
}

func TestStoreConcurrentAccess(t *testing.T) {
	store := newTestStore()
	_, err := store.RegisterBoard("dfp", "fat", "", "", 0)
	require.NoError(t, err)
	board, err := store.Resolve("dfp")
	require.NoError(t, err)
	board.RegisterDevice(DeviceState{
		ID: "button_start", Kind: DeviceKindButton, Pins: []string{"11"}, DefaultState: 1,
	})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pin := "11"
			if i%2 == 0 {
				board.SetDigitalValue(pin, i%2, SourceHardwareCode)
			} else {
				_, _ = board.Pin(pin)
			}
			_ = board.Values()
			_ = board.Snapshot()
			_ = store.Snapshot()

			ch := store.Subscribe()
			store.Unsubscribe(ch)
		}(i)
	}
	wg.Wait()
}

func TestStorePinAndDeviceOperations(t *testing.T) {
	store := newTestStore()
	board, err := store.RegisterBoard("dfp", "fat", "raspberry-pi", "", 12*time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, 12*time.Millisecond, board.Latency())

	board.SetConnected(true)
	assert.True(t, board.Connected())
	board.IncrementReconnect()
	snapshot := board.Snapshot()
	assert.True(t, snapshot.Connected)
	assert.Equal(t, uint64(1), snapshot.ReconnectCount)

	require.NoError(t, board.BindPin("9", "servo", PinModeServo))
	pin, ok := board.Pin("9")
	require.True(t, ok)
	assert.Equal(t, "servo", pin.Label)

	require.ErrorIs(t, board.BindPin("9", "", PinMode("bogus")), ErrInvalidPinMode)

	board.PwmWritePin("5", 128, SourceUI)
	pin, _ = board.Pin("5")
	assert.Equal(t, PinModePWM, pin.Mode)
	assert.Equal(t, byte(128), pin.ByteValue)

	board.ServoWritePin("9", 90, SourceUI)
	pin, _ = board.Pin("9")
	assert.Equal(t, PinModeServo, pin.Mode)
	assert.Equal(t, byte(90), pin.ByteValue)

	board.AnalogWritePin("A0", 512, SourceUI)
	pin, _ = board.Pin("A0")
	assert.Equal(t, PinModeAnalog, pin.Mode)
	assert.Equal(t, 512, pin.AnalogValue)
	assert.Equal(t, 512, board.AnalogReadPin("A0").AnalogValue)

	board.EnsurePin("11", PinModeInput)
	board.LabelPin("11", "button")
	pin, _ = board.Pin("11")
	assert.Equal(t, "button", pin.Label)
}

func TestStoreDevicesValuesAndFunctions(t *testing.T) {
	store := newTestStore()
	board, err := store.RegisterBoard("dfp", "fat", "", "", 0)
	require.NoError(t, err)

	board.RegisterDevice(DeviceState{ID: "value_isRebooted", Kind: DeviceKindValue, ValueName: "isRebooted"})
	board.RegisterDevice(DeviceState{ID: "function_ack", Kind: DeviceKindFunction, FunctionName: "ack"})
	board.RegisterDevice(DeviceState{ID: "pwm", Kind: DeviceKindPWM, Pins: []string{"5"}})
	board.RegisterDevice(DeviceState{ID: "sensor", Kind: DeviceKindSensor, Pins: []string{"A0"}})

	board.SetValue("isRebooted", true, SourceUI)
	value, ok := board.Value("isRebooted")
	require.True(t, ok)
	assert.Equal(t, true, value.Value)
	device, ok := board.Device("value_isRebooted")
	require.True(t, ok)
	assert.True(t, device.Active)

	board.SetFunctionCode("ack", 2, SourceUI)
	fn, ok := board.Function("ack")
	require.True(t, ok)
	assert.Equal(t, 2, fn.ReturnCode)
	fn, result := board.CallFunction("ack", "42", SourceUI)
	assert.Equal(t, 2, result)
	assert.Equal(t, uint64(1), fn.CallCount)

	board.PwmWritePin("5", 64, SourceUI)
	pwm, _ := board.Device("pwm")
	assert.Equal(t, 64, pwm.State["byteValue"])

	board.AnalogWritePin("A0", 300, SourceUI)
	sensor, _ := board.Device("sensor")
	assert.Equal(t, 300, sensor.State["analogValue"])

	devices := board.Devices()
	assert.Len(t, devices, 4)
}

func TestStoreToggleAndValues(t *testing.T) {
	store := newTestStore()
	board, err := store.RegisterBoard("dfp", "fat", "", "", 0)
	require.NoError(t, err)
	board.SetInputPullup("11")
	board.RegisterDevice(DeviceState{ID: "relay", Kind: DeviceKindRelay, Pins: []string{"3"}})
	board.RegisterDevice(DeviceState{ID: "inverted", Kind: DeviceKindRelay, Pins: []string{"4"}, Inverted: true})
	board.RegisterDevice(DeviceState{ID: "button", Kind: DeviceKindButton, Pins: []string{"11"}, DefaultState: 1})
	board.RegisterDevice(DeviceState{ID: "value", Kind: DeviceKindValue, ValueName: "value"})

	require.NoError(t, store.ToggleDevice("dfp", "relay"))
	relay, _ := board.Device("relay")
	assert.True(t, relay.Active)
	require.NoError(t, store.ToggleDevice("dfp", "inverted"))
	inverted, _ := board.Device("inverted")
	assert.True(t, inverted.Active)
	require.NoError(t, store.ToggleDevice("dfp", "button"))
	button, _ := board.Device("button")
	assert.True(t, button.Active)
	assert.ErrorIs(t, store.ToggleDevice("dfp", "value"), ErrReadOnly)

	require.NoError(t, store.SetPWM("dfp", "5", 1, SourceUI))
	require.NoError(t, store.SetServo("dfp", "9", 90, SourceUI))
	require.NoError(t, store.SetAnalog("dfp", "A0", 1, SourceUI))
	require.ErrorIs(t, store.SetServo("dfp", "9", 181, SourceUI), ErrInvalidValue)
}

func TestStoreAliasesAndReplayEdgeCases(t *testing.T) {
	store := newTestStore()
	board, err := store.RegisterBoard("tank_pond", "TankPond", "", "", 0)
	require.NoError(t, err)
	assert.Equal(t, "tank_pond", board.ID())

	resolved, err := store.Resolve("tankpond")
	require.NoError(t, err)
	assert.Same(t, board, resolved)

	require.NoError(t, board.SetIdentity("garden", "", ""))
	resolved, err = store.Resolve("garden")
	require.NoError(t, err)
	assert.Same(t, board, resolved)

	ch, missed, ok := store.SubscribeWithReplay(0)
	assert.False(t, ok)
	assert.Empty(t, missed)
	store.Unsubscribe(ch)

	_, err = store.BoardState("missing")
	assert.ErrorIs(t, err, ErrBoardNotFound)
}
