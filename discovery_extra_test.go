package simulate

import (
	"testing"

	"github.com/disaster37/gobot-simulate/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindHintScoring(t *testing.T) {
	hints := []DeviceSpec{
		{ID: "button_start", Pin: "11", Name: "start"},
		{Pin: "12", Name: "other"},
		{Name: "relay_drum"},
	}

	// ID match wins (score 3).
	h := findHint(hints, "button_start", "11")
	require.NotNil(t, h)
	assert.Equal(t, "button_start", h.ID)

	// Pin match (score 2) beats name match (score 1).
	h = findHint(hints, "unknown", "12")
	require.NotNil(t, h)
	assert.Equal(t, "12", h.Pin)

	// Name-only match (score 1).
	h = findHint(hints, "relay_drum", "99")
	require.NotNil(t, h)
	assert.Equal(t, "relay_drum", h.Name)

	assert.Nil(t, findHint(hints, "missing", "99"))
}

func TestUniqueDeviceID(t *testing.T) {
	sim := New()
	a := sim.Adaptor("dfp")
	board := a.board

	assert.Equal(t, "relay", uniqueDeviceID(board, "relay", "3"))

	board.RegisterDevice(state.DeviceState{ID: "relay", Kind: DeviceKindRelay, Pins: []string{"3"}})
	assert.Equal(t, "relay_3", uniqueDeviceID(board, "relay", "3"))

	board.RegisterDevice(state.DeviceState{ID: "relay_3", Kind: DeviceKindRelay, Pins: []string{"3"}})
	assert.Equal(t, "relay_2", uniqueDeviceID(board, "relay", "3"))
}

func TestDiscoveryTruthy(t *testing.T) {
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
		{"int zero", int(0), false},
		{"int nonzero", int(2), true},
		{"nil", nil, false},
		{"default", struct{}{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, truthy(tt.in))
		})
	}
}

func TestPinValueAndBoardValueHelpers(t *testing.T) {
	sim := New()
	a := sim.Adaptor("dfp")
	a.SetDigitalPinState("3", 1)
	a.SetValueReadState("distance", 12.5)

	assert.Equal(t, 1, pinValue(a.board, "3"))
	assert.Equal(t, 0, pinValue(a.board, "missing"))
	assert.Equal(t, 12.5, boardValue(a.board, "distance"))
	assert.Nil(t, boardValue(a.board, "missing"))
}

func TestRegisterDiscoveredRejectsInvalidIDs(t *testing.T) {
	sim := New()
	a := sim.Adaptor("dfp")

	err := sim.registerDiscovered(a.board, discoveredSpec{
		DeviceSpec: DeviceSpec{ID: "dev", Pin: "bad pin"},
	})
	assert.ErrorIs(t, err, ErrInvalidID)

	err = sim.registerDiscovered(a.board, discoveredSpec{
		DeviceSpec: DeviceSpec{ID: "bad id", Pin: "3"},
	})
	assert.ErrorIs(t, err, ErrInvalidID)
}
