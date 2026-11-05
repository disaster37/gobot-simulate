package state

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreLifecycleAndLookupMethods(t *testing.T) {
	store := newTestStore()
	board, err := store.RegisterBoard("dfp", "fat", "raspberry-pi", "/dev/ttyS0", 5*time.Millisecond)
	require.NoError(t, err)

	// BoardByID found / not found.
	byID, err := store.BoardByID("dfp")
	require.NoError(t, err)
	assert.Same(t, board, byID)
	_, err = store.BoardByID("nope")
	assert.ErrorIs(t, err, ErrBoardNotFound)

	// Boards returns a sorted snapshot list.
	assert.Len(t, store.Boards(), 1)

	// SetLatency mutates the board lag.
	board.SetLatency(7 * time.Millisecond)
	assert.Equal(t, 7*time.Millisecond, board.Latency())
	assert.Equal(t, int64(7), board.Snapshot().SimulatedLagMs)

	// DroppedEvents starts at zero.
	assert.Equal(t, uint64(0), store.DroppedEvents())
}

func TestStoreHooksRoundTrip(t *testing.T) {
	store := newTestStore()
	board, err := store.RegisterBoard("dfp", "fat", "", "", 0)
	require.NoError(t, err)

	hooks := board.Hooks()
	assert.Nil(t, hooks.DigitalRead)

	board.UpdateHooks(func(h *Hooks) { h.DigitalRead = func(string) (int, error) { return 7, nil } })
	hooks = board.Hooks()
	require.NotNil(t, hooks.DigitalRead)
	val, err := hooks.DigitalRead("1")
	require.NoError(t, err)
	assert.Equal(t, 7, val)
}

func TestStoreDigitalReadPinAutoCreates(t *testing.T) {
	store := newTestStore()
	board, err := store.RegisterBoard("dfp", "fat", "", "", 0)
	require.NoError(t, err)

	pin := board.DigitalReadPin("99")
	assert.Equal(t, PinModeInput, pin.Mode)
	assert.Equal(t, 0, pin.DigitalValue)

	// Reading again returns the now-existing pin without re-publishing.
	again := board.DigitalReadPin("99")
	assert.Equal(t, "99", again.Pin)
}

func TestStoreAnalogReadExistingPin(t *testing.T) {
	store := newTestStore()
	board, err := store.RegisterBoard("dfp", "fat", "", "", 0)
	require.NoError(t, err)

	board.AnalogWritePin("A0", 512, SourceUI)
	assert.Equal(t, 512, board.AnalogReadPin("A0").AnalogValue)
}

func TestStoreHasDevice(t *testing.T) {
	store := newTestStore()
	board, err := store.RegisterBoard("dfp", "fat", "", "", 0)
	require.NoError(t, err)

	assert.False(t, board.HasDevice("relay"))
	board.RegisterDevice(DeviceState{ID: "relay", Kind: DeviceKindRelay, Pins: []string{"3"}})
	assert.True(t, board.HasDevice("relay"))
}

func TestStoreMutationErrorPaths(t *testing.T) {
	store := newTestStore()

	tests := []struct {
		name string
		call func() error
	}{
		{"SetDigital", func() error { return store.SetDigital("nope", "1", 1, SourceUI) }},
		{"PressButton", func() error { return store.PressButton("nope", "b") }},
		{"ReleaseButton", func() error { return store.ReleaseButton("nope", "b") }},
		{"ToggleDevice", func() error { return store.ToggleDevice("nope", "d") }},
		{"SetPWM", func() error { return store.SetPWM("nope", "1", 1, SourceUI) }},
		{"SetServo", func() error { return store.SetServo("nope", "1", 90, SourceUI) }},
		{"SetAnalog", func() error { return store.SetAnalog("nope", "A0", 1, SourceUI) }},
		{"SetValue", func() error { return store.SetValue("nope", "v", 1, SourceUI) }},
		{"SetFault", func() error { return store.SetFault("nope", errors.New("x")) }},
		{"ClearFault", func() error { return store.ClearFault("nope") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.ErrorIs(t, tt.call(), ErrBoardNotFound)
		})
	}

	_, err := store.CallFunction("nope", "fn", "", SourceUI)
	assert.ErrorIs(t, err, ErrBoardNotFound)
	assert.ErrorIs(t, store.SetFunctionCode("nope", "fn", 1, SourceUI), ErrBoardNotFound)

	_, err = store.Devices("nope")
	assert.ErrorIs(t, err, ErrBoardNotFound)
	_, err = store.Device("nope", "d")
	assert.ErrorIs(t, err, ErrBoardNotFound)
	_, err = store.Pin("nope", "1")
	assert.ErrorIs(t, err, ErrBoardNotFound)
	_, err = store.Values("nope")
	assert.ErrorIs(t, err, ErrBoardNotFound)
	_, err = store.BoardState("nope")
	assert.ErrorIs(t, err, ErrBoardNotFound)
}

func TestStoreSetDigitalAndFunctionCode(t *testing.T) {
	store := newTestStore()
	_, err := store.RegisterBoard("dfp", "fat", "", "", 0)
	require.NoError(t, err)

	require.NoError(t, store.SetDigital("dfp", "3", 1, SourceUI))
	pin, err := store.Pin("dfp", "3")
	require.NoError(t, err)
	assert.Equal(t, 1, pin.DigitalValue)

	require.NoError(t, store.SetFunctionCode("dfp", "ack", 3, SourceUI))
	result, err := store.CallFunction("dfp", "ack", "1", SourceUI)
	require.NoError(t, err)
	assert.Equal(t, 3, result)

	state, err := store.BoardState("dfp")
	require.NoError(t, err)
	assert.Equal(t, 3, state.Functions["ack"].ReturnCode)
}

func TestStoreSetDigitalValueRange(t *testing.T) {
	store := newTestStore()
	_, err := store.RegisterBoard("dfp", "fat", "", "", 0)
	require.NoError(t, err)

	tests := []struct {
		name    string
		value   int
		wantErr bool
	}{
		{name: "negative", value: -1, wantErr: true},
		{name: "zero", value: 0},
		{name: "one", value: 1},
		{name: "max byte", value: 255},
		{name: "overflow byte", value: 256, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.SetDigital("dfp", "3", tt.value, SourceUI)
			if tt.wantErr {
				assert.ErrorIs(t, err, ErrInvalidValue)
				return
			}
			require.NoError(t, err)
			pin, err := store.Pin("dfp", "3")
			require.NoError(t, err)
			assert.Equal(t, tt.value, pin.DigitalValue)
			//nolint:gosec // test values are validated into 0..255
			assert.Equal(t, byte(tt.value), pin.ByteValue)
		})
	}
}

func TestStoreInvertAfterPullup(t *testing.T) {
	store := newTestStore()
	board, err := store.RegisterBoard("dfp", "fat", "", "", 0)
	require.NoError(t, err)

	board.SetInputPullup("10")
	pin, _ := board.Pin("10")
	assert.Equal(t, 1, pin.DigitalValue)

	// Inverting after pull-up forces the value to 0.
	board.SetInvertInitialPinState("10")
	pin, _ = board.Pin("10")
	assert.Equal(t, 0, pin.DigitalValue)
}

func TestStoreToggleRelayOffAndCaptor(t *testing.T) {
	store := newTestStore()
	board, err := store.RegisterBoard("dfp", "fat", "", "", 0)
	require.NoError(t, err)
	board.SetInputPullup("11")
	board.RegisterDevice(DeviceState{ID: "relay", Kind: DeviceKindRelay, Pins: []string{"3"}})
	board.RegisterDevice(DeviceState{ID: "captor", Kind: DeviceKindCaptor, Pins: []string{"11"}, DefaultState: 1})

	require.NoError(t, store.ToggleDevice("dfp", "relay"))
	relay, _ := board.Device("relay")
	assert.True(t, relay.Active)
	require.NoError(t, store.ToggleDevice("dfp", "relay"))
	relay, _ = board.Device("relay")
	assert.False(t, relay.Active)

	require.NoError(t, store.ToggleDevice("dfp", "captor"))
	captor, _ := board.Device("captor")
	assert.True(t, captor.Active)

	assert.ErrorIs(t, store.ToggleDevice("dfp", "missing"), ErrDeviceNotFound)
}

func TestIsTruthy(t *testing.T) {
	tests := []struct {
		name string
		in   interface{}
		want bool
	}{
		{"bool true", true, true},
		{"bool false", false, false},
		{"string nonempty", "x", true},
		{"string empty", "", false},
		{"float64 zero", float64(0), false},
		{"float64 nonzero", float64(1), true},
		{"float32 zero", float32(0), false},
		{"float32 nonzero", float32(1), true},
		{"int zero", int(0), false},
		{"int nonzero", int(2), true},
		{"int64 zero", int64(0), false},
		{"int64 nonzero", int64(2), true},
		{"nil", nil, false},
		{"default map", map[string]interface{}{"a": 1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isTruthy(tt.in))
		})
	}
}

func TestDefaultCapabilitiesAllKinds(t *testing.T) {
	for _, kind := range []DeviceKind{
		DeviceKindRelay, DeviceKindLED, DeviceKindButton, DeviceKindCaptor,
		DeviceKindSensor, DeviceKindPWM, DeviceKindServo, DeviceKindAnalog,
		DeviceKindValue, DeviceKindFunction,
	} {
		caps := DefaultCapabilities(kind)
		assert.NotEqual(t, DeviceCapabilities{}, caps, "kind %s should have capabilities", kind)
	}
	assert.Equal(t, DeviceCapabilities{}, DefaultCapabilities(DeviceKind("unknown")))
}

func TestCloneDeviceCopiesStateAndLabels(t *testing.T) {
	original := DeviceState{
		Pins:   []string{"3", "4"},
		State:  map[string]interface{}{"pinValue": 1},
		Labels: map[string]string{"role": "motor"},
	}
	clone := cloneDevice(original)
	clone.Pins[0] = "changed"
	clone.State["pinValue"] = 99
	clone.Labels["role"] = "changed"

	assert.Equal(t, "3", original.Pins[0])
	assert.Equal(t, 1, original.State["pinValue"])
	assert.Equal(t, "motor", original.Labels["role"])
}

func TestStoreAddAliasConflictKeepsExisting(t *testing.T) {
	store := newTestStore()
	store.addAliasLocked("alias", "board-a")
	store.addAliasLocked("alias", "board-b")

	// A conflicting alias must not overwrite the existing mapping.
	assert.Equal(t, "board-a", store.aliases["alias"])
}

func TestStoreRegisterBoardInvalidID(t *testing.T) {
	store := newTestStore()
	_, err := store.RegisterBoard("has space", "x", "", "", 0)
	assert.ErrorIs(t, err, ErrInvalidID)
}
