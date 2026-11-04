package simulate

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gobot.io/x/gobot/v2"
)

type legacyMockPlateform interface {
	Name() string
	SetName(name string)
	Port() string
	Connect() error
	Finalize() error
	SetInvertInitialPinState(pin string)
	SetInputPullup(listPins ...string)
	GetDigitalPinState(pin string) int
	SetDigitalPinState(pin string, value int)
	GetValueReadState(name string) interface{}
	SetValueReadState(name string, value interface{})
	GetCallFunctionState(name string) int
	SetCallFunctionState(name string, value int)
	SetError(err error)
	TestReconnect(f func() error)
	Reconnect() error
	TestAdaptorDigitalWrite(f func(pin string, val byte) error)
	TestAdaptorServoWrite(f func(pin string, val byte) error)
	TestAdaptorPwmWrite(f func(pin string, val byte) error)
	TestAdaptorAnalogRead(f func(pin string) (int, error))
	TestAdaptorDigitalRead(f func(pin string) (int, error))
	TestAdaptorReadValue(f func(pin string) (int, error))
	TestAdaptorValueRead(f func(name string) (interface{}, error))
	TestAdaptorValuesRead(f func() (map[string]interface{}, error))
	TestAdaptorFunctionCall(f func(name, parameters string) (int, error))
}

var _ legacyMockPlateform = (*MockPlateform)(nil)

func TestNewMockPlateformDefaults(t *testing.T) {
	board := NewMockPlateform()
	assert.Equal(t, "test", board.Name())
	assert.Equal(t, "", board.Port())
	assert.NoError(t, board.Connect())
	assert.NoError(t, board.Finalize())
	assert.NoError(t, board.Reconnect())

	values, err := board.ValuesRead()
	require.NoError(t, err)
	assert.Equal(t, 99, values["test"])
}

func TestValuesReadAlwaysIncludesTestSeed(t *testing.T) {
	board := NewMockPlateform()
	board.SetValueReadState("distance", float64(42))
	board.SetValueReadState("isRebooted", false)

	values, err := board.ValuesRead()
	require.NoError(t, err)
	assert.Equal(t, 99, values["test"])
	assert.Equal(t, float64(42), values["distance"])
	assert.Equal(t, false, values["isRebooted"])

	board.SetValueReadState("test", 7)
	values, err = board.ValuesRead()
	require.NoError(t, err)
	assert.Equal(t, 7, values["test"])
}

func TestPullupAndInvertedInitialPinState(t *testing.T) {
	board := NewMockPlateform()
	board.SetInvertInitialPinState("10")
	board.SetInputPullup("10", "11", "12")

	assert.Equal(t, 0, board.GetDigitalPinState("10"))
	assert.Equal(t, 1, board.GetDigitalPinState("11"))
	assert.Equal(t, 1, board.GetDigitalPinState("12"))

	pin, err := board.sim.Pin("test", "10")
	require.NoError(t, err)
	assert.True(t, pin.InvertInitial)
	assert.True(t, pin.Pullup)
	assert.Equal(t, PinModeInputPullup, pin.Mode)
}

func TestUnknownPinsAreAutoCreated(t *testing.T) {
	board := NewMockPlateform()

	value, err := board.DigitalRead("99")
	require.NoError(t, err)
	assert.Equal(t, 0, value)
	pin, err := board.sim.Pin("test", "99")
	require.NoError(t, err)
	assert.Equal(t, PinModeInput, pin.Mode)

	require.NoError(t, board.DigitalWrite("100", 1))
	pin, err = board.sim.Pin("test", "100")
	require.NoError(t, err)
	assert.Equal(t, PinModeOutput, pin.Mode)
	assert.Equal(t, 1, pin.DigitalValue)
}

func TestPressAndReleaseButtonDefaultStates(t *testing.T) {
	sim := New()
	board := sim.Adaptor("dfp", WithAdaptorName("fat"))
	board.SetInputPullup("11", "12")
	_, err := sim.RegisterDevice("dfp", DeviceSpec{
		ID: "button_start", Label: "Start", Kind: DeviceKindButton, Pin: "11", DefaultState: IntPtr(1),
	})
	require.NoError(t, err)
	_, err = sim.RegisterDevice("dfp", DeviceSpec{
		ID: "captor_upper", Label: "Upper", Kind: DeviceKindCaptor, Pin: "12", DefaultState: IntPtr(0),
	})
	require.NoError(t, err)

	require.NoError(t, sim.PressButton("dfp", "button_start"))
	assert.Equal(t, 0, board.GetDigitalPinState("11"))
	device, err := sim.Device("dfp", "button_start")
	require.NoError(t, err)
	assert.True(t, device.Active)

	require.NoError(t, sim.ReleaseButton("dfp", "button_start"))
	assert.Equal(t, 1, board.GetDigitalPinState("11"))

	require.NoError(t, sim.PressButton("dfp", "captor_upper"))
	assert.Equal(t, 1, board.GetDigitalPinState("12"))
	require.NoError(t, sim.ReleaseButton("dfp", "captor_upper"))
	assert.Equal(t, 0, board.GetDigitalPinState("12"))
}

func TestSetErrorFaultPropagation(t *testing.T) {
	board := NewMockPlateform()
	fault := errors.New("hardware offline")
	board.SetError(fault)

	_, err := board.DigitalRead("1")
	assert.ErrorIs(t, err, fault)
	assert.ErrorIs(t, board.DigitalWrite("1", 1), fault)
	assert.ErrorIs(t, board.PwmWrite("1", 1), fault)
	assert.ErrorIs(t, board.ServoWrite("1", 1), fault)
	_, err = board.AnalogRead("1")
	assert.ErrorIs(t, err, fault)
	assert.ErrorIs(t, board.AnalogWrite("1", 1), fault)
	_, err = board.ValueRead("distance")
	assert.ErrorIs(t, err, fault)
	_, err = board.ValuesRead()
	assert.ErrorIs(t, err, fault)
	_, err = board.FunctionCall("acknoledgeRebooted", "")
	assert.ErrorIs(t, err, fault)
	assert.ErrorIs(t, board.Connect(), fault)
	assert.ErrorIs(t, board.Finalize(), fault)
	assert.ErrorIs(t, board.Reconnect(), fault)

	board.SetError(nil)
	values, err := board.ValuesRead()
	require.NoError(t, err)
	assert.Equal(t, 99, values["test"])
}

func TestReconnectHookAndCounter(t *testing.T) {
	board := NewMockPlateform()
	called := false
	board.TestReconnect(func() error {
		called = true
		return nil
	})
	require.NoError(t, board.Reconnect())
	assert.True(t, called)

	board = NewMockPlateform()
	require.NoError(t, board.Reconnect())
	state, err := board.sim.Board("test")
	require.NoError(t, err)
	assert.Equal(t, uint64(1), state.ReconnectCount)
}

func TestReentrantTestHooks(t *testing.T) {
	board := NewMockPlateform()
	board.TestAdaptorDigitalRead(func(pin string) (int, error) {
		board.SetDigitalPinState(pin, 1)
		return board.GetDigitalPinState(pin), nil
	})
	value, err := board.DigitalRead("11")
	require.NoError(t, err)
	assert.Equal(t, 1, value)

	board.TestAdaptorDigitalWrite(func(pin string, val byte) error {
		board.SetDigitalPinState(pin, int(val))
		return nil
	})
	require.NoError(t, board.DigitalWrite("12", 1))
	assert.Equal(t, 1, board.GetDigitalPinState("12"))
}

func TestValueAndFunctionLegacyState(t *testing.T) {
	board := NewMockPlateform()
	assert.Nil(t, board.GetValueReadState("distance"))
	board.SetValueReadState("distance", 12.5)
	assert.Equal(t, 12.5, board.GetValueReadState("distance"))

	assert.Equal(t, 0, board.GetCallFunctionState("acknoledgeRebooted"))
	board.SetCallFunctionState("acknoledgeRebooted", 1)
	assert.Equal(t, 1, board.GetCallFunctionState("acknoledgeRebooted"))
	result, err := board.FunctionCall("acknoledgeRebooted", "1")
	require.NoError(t, err)
	assert.Equal(t, 1, result)
}

func TestSetNameMutatesBoard(t *testing.T) {
	board := NewMockPlateform()
	board.SetName("fat")
	assert.Equal(t, "fat", board.Name())
	state, err := board.sim.Board("test")
	require.NoError(t, err)
	assert.Equal(t, "fat", state.Name)
}

func TestWaitEvent(t *testing.T) {
	eventer := gobot.NewEventer()
	eventer.AddEvent("ready")
	go func() {
		time.Sleep(10 * time.Millisecond)
		eventer.Publish("ready", nil)
	}()
	assert.True(t, <-WaitEvent(eventer, "ready", time.Second))

	eventer2 := gobot.NewEventer()
	assert.False(t, <-WaitEvent(eventer2, "never", 20*time.Millisecond))
}

func TestConcurrentAdaptorAccess(t *testing.T) {
	board := NewMockPlateform()
	board.SetInputPullup("11")
	board.SetValueReadState("distance", 1.0)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				board.SetDigitalPinState("11", i%2)
			}
			_, _ = board.DigitalRead("11")
			_, _ = board.ValuesRead()
			_, _ = board.ValueRead("distance")
			_, _ = board.FunctionCall("fn", "1")
			ch := board.sim.Subscribe()
			board.sim.Unsubscribe(ch)
		}(i)
	}
	wg.Wait()
}

func TestAdaptorIOMethods(t *testing.T) {
	board := NewMockPlateform()

	require.NoError(t, board.PwmWrite("9", 128))
	pin, err := board.sim.Pin("test", "9")
	require.NoError(t, err)
	assert.Equal(t, PinModePWM, pin.Mode)
	assert.Equal(t, byte(128), pin.ByteValue)

	require.NoError(t, board.ServoWrite("9", 90))
	pin, err = board.sim.Pin("test", "9")
	require.NoError(t, err)
	assert.Equal(t, PinModeServo, pin.Mode)
	assert.Equal(t, byte(90), pin.ByteValue)

	require.NoError(t, board.AnalogWrite("A0", 512))
	value, err := board.AnalogRead("A0")
	require.NoError(t, err)
	assert.Equal(t, 512, value)

	readValue, err := board.ValueRead("missing")
	require.NoError(t, err)
	assert.Nil(t, readValue)
}

func TestAdaptorOverrideHooks(t *testing.T) {
	board := NewMockPlateform()
	board.TestAdaptorPwmWrite(func(string, byte) error { return errors.New("pwm") })
	board.TestAdaptorServoWrite(func(string, byte) error { return errors.New("servo") })
	board.TestAdaptorAnalogRead(func(string) (int, error) { return 7, nil })
	board.TestAdaptorValueRead(func(string) (interface{}, error) { return "value", nil })
	board.TestAdaptorValuesRead(func() (map[string]interface{}, error) {
		return map[string]interface{}{"custom": true}, nil
	})
	board.TestAdaptorFunctionCall(func(string, string) (int, error) { return 3, nil })

	assert.ErrorContains(t, board.PwmWrite("1", 1), "pwm")
	assert.ErrorContains(t, board.ServoWrite("1", 1), "servo")
	value, err := board.AnalogRead("1")
	require.NoError(t, err)
	assert.Equal(t, 7, value)
	read, err := board.ValueRead("anything")
	require.NoError(t, err)
	assert.Equal(t, "value", read)
	values, err := board.ValuesRead()
	require.NoError(t, err)
	assert.Equal(t, true, values["custom"])
	result, err := board.FunctionCall("fn", "")
	require.NoError(t, err)
	assert.Equal(t, 3, result)
}

func TestSimulatorMutationAPI(t *testing.T) {
	sim := New()
	board := sim.Adaptor("dfp", WithAdaptorName("fat"))
	board.SetInputPullup("11")
	_, err := sim.RegisterDevice("dfp", DeviceSpec{
		ID: "relay_drum", Kind: DeviceKindRelay, Pin: "3",
	})
	require.NoError(t, err)
	_, err = sim.RegisterDevice("dfp", DeviceSpec{
		ID: "button_start", Kind: DeviceKindButton, Pin: "11", DefaultState: IntPtr(1),
	})
	require.NoError(t, err)
	_, err = sim.RegisterDevice("dfp", DeviceSpec{
		ID: "servo", Kind: DeviceKindServo, Pin: "9",
	})
	require.NoError(t, err)
	_, err = sim.RegisterDevice("dfp", DeviceSpec{
		ID: "distance", Kind: DeviceKindValue, ValueName: "distance",
	})
	require.NoError(t, err)

	require.NoError(t, sim.SetPWM("dfp", "5", 200, SourceUI))
	require.NoError(t, sim.SetServo("dfp", "9", 180, SourceUI))
	assert.ErrorIs(t, sim.SetServo("dfp", "9", 181, SourceUI), ErrInvalidValue)
	require.NoError(t, sim.SetAnalog("dfp", "A0", 1023, SourceUI))
	require.NoError(t, sim.SetValue("dfp", "distance", float64(12), SourceUI))
	assert.ErrorIs(t, sim.SetValue("dfp", "bad name", 1, SourceUI), ErrInvalidID)

	require.NoError(t, sim.ToggleDevice("dfp", "relay_drum"))
	device, err := sim.Device("dfp", "relay_drum")
	require.NoError(t, err)
	assert.True(t, device.Active)
	require.NoError(t, sim.ToggleDevice("dfp", "relay_drum"))
	device, err = sim.Device("dfp", "relay_drum")
	require.NoError(t, err)
	assert.False(t, device.Active)

	require.NoError(t, sim.ToggleDevice("dfp", "button_start"))
	device, err = sim.Device("dfp", "button_start")
	require.NoError(t, err)
	assert.True(t, device.Active)
	require.NoError(t, sim.ToggleDevice("dfp", "button_start"))
	device, err = sim.Device("dfp", "button_start")
	require.NoError(t, err)
	assert.False(t, device.Active)

	require.NoError(t, sim.BindPin("dfp", "7", "green", PinModeOutput))
	pin, err := sim.Pin("dfp", "7")
	require.NoError(t, err)
	assert.Equal(t, "green", pin.Label)

	fault := errors.New("boom")
	require.NoError(t, sim.SetFault("dfp", fault))
	state, err := sim.Board("dfp")
	require.NoError(t, err)
	assert.True(t, state.Fault.Active)
	require.NoError(t, sim.ClearFault("dfp"))
	state, err = sim.Board("dfp")
	require.NoError(t, err)
	assert.False(t, state.Fault.Active)

	assert.Greater(t, sim.CurrentSeq(), uint64(0))
	assert.Equal(t, Version, sim.Version())
	assert.GreaterOrEqual(t, sim.Uptime(), time.Duration(0))
	assert.Equal(t, 0, sim.ClientCount())
	assert.Equal(t, uint64(0), sim.DroppedEvents())

	require.NoError(t, sim.RegisterBoard(board))
}

func TestSimulatorEventSubscription(t *testing.T) {
	sim := New()
	board := sim.Adaptor("dfp")
	_ = board

	ch := sim.Subscribe()
	defer sim.Unsubscribe(ch)

	board.SetDigitalPinState("1", 1)
	select {
	case evt := <-ch:
		assert.Greater(t, evt.Seq, uint64(0))
	case <-time.After(time.Second):
		t.Fatal("expected an event")
	}
}

func TestSimulatorLifecycleAndOptions(t *testing.T) {
	sim := New(
		WithAddr("127.0.0.1"),
		WithPort(0),
		WithUIEnabled(false),
		WithCORSOrigins("http://a.test", "http://a.test"),
		WithMaxClients(2),
		WithEventBuffer(32),
		WithLatency(5*time.Millisecond),
		WithAuthToken("token"),
	)
	board := sim.Adaptor("dfp",
		WithAdaptorID("dfp"),
		WithAdaptorName("fat"),
		WithAdaptorPort("/dev/ttyUSB0"),
		WithPlatform("raspberry-pi"),
		WithInputPullup("11"),
		WithInvertedInitialPins("12"),
		WithInitialValue("distance", float64(1)),
		WithInitialFunctionCode("ack", 1),
		WithSimulatedLatency(2*time.Millisecond),
	)
	assert.Equal(t, "/dev/ttyUSB0", board.Port())
	boardState, err := sim.Board("dfp")
	require.NoError(t, err)
	assert.Equal(t, int64(2), boardState.SimulatedLagMs)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, sim.Start(ctx))
	assert.NotEmpty(t, sim.URL())
	assert.NotNil(t, sim.Hertz())
	assert.NoError(t, sim.Stop(context.Background()))

	standalone := NewAdaptor(WithAdaptorID("standalone"), WithAdaptorName("standalone"))
	assert.Equal(t, "standalone", standalone.BoardID())
	assert.Equal(t, "standalone", standalone.Name())
}
