package simulate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gobot.io/x/gobot/v2"
	"gobot.io/x/gobot/v2/drivers/aio"
	"gobot.io/x/gobot/v2/drivers/gpio"
)

type fakeDevice struct {
	name       string
	connection gobot.Connection
}

func (f *fakeDevice) Name() string                 { return f.name }
func (f *fakeDevice) SetName(name string)          { f.name = name }
func (f *fakeDevice) Start() error                 { return nil }
func (f *fakeDevice) Halt() error                  { return nil }
func (f *fakeDevice) Connection() gobot.Connection { return f.connection }

type fakeValueDevice struct {
	fakeDevice
	valueName string
}

func (f *fakeValueDevice) ValueName() string { return f.valueName }

type fakeFunctionDevice struct {
	fakeDevice
	functionName string
	parameters   string
}

func (f *fakeFunctionDevice) FunctionName() string            { return f.functionName }
func (f *fakeFunctionDevice) Parameters() string              { return f.parameters }
func (f *fakeFunctionDevice) SetParameters(parameters string) { f.parameters = parameters }

type fakeAdaptor struct{ name string }

func (f *fakeAdaptor) Name() string        { return f.name }
func (f *fakeAdaptor) SetName(name string) { f.name = name }
func (f *fakeAdaptor) Connect() error      { return nil }
func (f *fakeAdaptor) Finalize() error     { return nil }
func (f *fakeAdaptor) Port() string        { return "" }

func TestAutoDiscoverDevicesAndHints(t *testing.T) {
	sim := New()
	board := sim.Adaptor("dfp", WithAdaptorName("test"))
	board.SetInvertInitialPinState("10")
	board.SetInputPullup("10", "11")
	board.SetValueReadState("isRebooted", false)

	relay := gpio.NewRelayDriver(board, "3", gpio.WithRelayInverted())
	led := gpio.NewLedDriver(board, "7")
	button := gpio.NewButtonDriver(board, "11", gpio.WithButtonDefaultState(1))
	captor := gpio.NewButtonDriver(board, "10", gpio.WithButtonDefaultState(0))
	servo := gpio.NewServoDriver(board, "9")
	sensor := aio.NewAnalogSensorDriver(board, "A0")
	value := &fakeValueDevice{fakeDevice: fakeDevice{name: "isRebooted", connection: board}, valueName: "isRebooted"}
	function := &fakeFunctionDevice{
		fakeDevice:   fakeDevice{name: "acknoledgeRebooted", connection: board},
		functionName: "acknoledgeRebooted",
		parameters:   "",
	}

	robot := gobot.NewRobot("fat",
		[]gobot.Connection{board},
		[]gobot.Device{relay, led, button, captor, servo, sensor, value, function},
		func() {},
	)

	require.NoError(t, sim.AutoDiscover(robot,
		DeviceSpec{ID: "relay_drum", Label: "Drum Relay", Kind: DeviceKindRelay, Pin: "3"},
		DeviceSpec{ID: "led_green", Label: "Green", Kind: DeviceKindLED, Pin: "7"},
		DeviceSpec{ID: "button_start", Label: "Start", Kind: DeviceKindButton, Pin: "11", DefaultState: IntPtr(1)},
		DeviceSpec{ID: "captor_security_upper", Label: "Security", Kind: DeviceKindCaptor, Pin: "10", DefaultState: IntPtr(0)},
		DeviceSpec{ID: "servo_valve", Label: "Valve", Kind: DeviceKindServo, Pin: "9"},
		DeviceSpec{ID: "sensor_level", Label: "Level", Kind: DeviceKindSensor, Pin: "A0"},
		DeviceSpec{ID: "value_isRebooted", Name: "isRebooted", Kind: DeviceKindValue},
		DeviceSpec{ID: "function_ack", Name: "acknoledgeRebooted", Kind: DeviceKindFunction},
	))

	state, err := sim.Board("dfp")
	require.NoError(t, err)
	assert.Equal(t, "fat", state.Name)

	relayState, err := sim.Device("dfp", "relay_drum")
	require.NoError(t, err)
	assert.Equal(t, DeviceKindRelay, relayState.Kind)
	assert.True(t, relayState.Inverted)
	assert.Contains(t, relayState.Pins, "3")

	buttonState, err := sim.Device("dfp", "button_start")
	require.NoError(t, err)
	assert.Equal(t, DeviceKindButton, buttonState.Kind)
	assert.Equal(t, 1, buttonState.DefaultState)
	assert.False(t, buttonState.Active)
	assert.True(t, buttonState.Capabilities.Pressable)

	captorState, err := sim.Device("dfp", "captor_security_upper")
	require.NoError(t, err)
	assert.Equal(t, DeviceKindCaptor, captorState.Kind)
	assert.Equal(t, 0, captorState.DefaultState)

	servoState, err := sim.Device("dfp", "servo_valve")
	require.NoError(t, err)
	assert.Equal(t, DeviceKindServo, servoState.Kind)
	require.NotNil(t, servoState.Capabilities.MaxValue)
	assert.Equal(t, float64(180), *servoState.Capabilities.MaxValue)

	sensorState, err := sim.Device("dfp", "sensor_level")
	require.NoError(t, err)
	assert.Equal(t, DeviceKindSensor, sensorState.Kind)

	valueState, err := sim.Device("dfp", "value_isRebooted")
	require.NoError(t, err)
	assert.Equal(t, DeviceKindValue, valueState.Kind)
	assert.Equal(t, "isRebooted", valueState.ValueName)

	functionState, err := sim.Device("dfp", "function_ack")
	require.NoError(t, err)
	assert.Equal(t, DeviceKindFunction, functionState.Kind)
	assert.Equal(t, "acknoledgeRebooted", functionState.FunctionName)
	assert.True(t, functionState.Capabilities.Callable)
}

func TestAutoDiscoverClassifiesCaptorByInvertedPin(t *testing.T) {
	sim := New()
	board := sim.Adaptor("dfp")
	board.SetInvertInitialPinState("10")
	board.SetInputPullup("10")
	button := gpio.NewButtonDriver(board, "10", gpio.WithButtonDefaultState(0))
	robot := gobot.NewRobot("board", []gobot.Connection{board}, []gobot.Device{button}, func() {})

	require.NoError(t, sim.AutoDiscover(robot))
	device := onlyDevice(t, sim, "dfp")
	assert.Equal(t, DeviceKindCaptor, device.Kind)
	assert.Equal(t, 0, device.DefaultState)
}

func TestAutoDiscoverAfterRobotStart(t *testing.T) {
	sim := New()
	board := sim.Adaptor("dfp")
	button := gpio.NewButtonDriver(board, "15", gpio.WithButtonDefaultState(0))
	robot := gobot.NewRobot("board", []gobot.Connection{board}, []gobot.Device{button}, func() {})
	require.NoError(t, robot.Start(false))
	t.Cleanup(func() { _ = robot.Stop() })

	require.NoError(t, sim.AutoDiscover(robot))
	device := onlyDevice(t, sim, "dfp")
	assert.Equal(t, 0, device.DefaultState)
}

func TestAutoDiscoverGeneratesUniqueIDs(t *testing.T) {
	sim := New()
	board := sim.Adaptor("dfp")
	relayA := gpio.NewRelayDriver(board, "3")
	relayB := gpio.NewRelayDriver(board, "4")
	robot := gobot.NewRobot("board", []gobot.Connection{board}, []gobot.Device{relayA, relayB}, func() {})

	require.NoError(t, sim.AutoDiscover(robot))
	devices, err := sim.Devices("dfp")
	require.NoError(t, err)
	require.Len(t, devices, 2)
	assert.NotEqual(t, devices[0].ID, devices[1].ID)
}

func TestAutoDiscoverErrors(t *testing.T) {
	sim := New()
	assert.Error(t, sim.AutoDiscover(nil))

	other := &fakeAdaptor{name: "real"}
	robot := gobot.NewRobot("board", []gobot.Connection{other}, nil, func() {})
	assert.Error(t, sim.AutoDiscover(robot))
}

func onlyDevice(t *testing.T, sim *Simulator, boardID string) DeviceState {
	t.Helper()
	devices, err := sim.Devices(boardID)
	require.NoError(t, err)
	require.Len(t, devices, 1)
	return devices[0]
}
