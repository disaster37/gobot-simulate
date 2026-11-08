package state

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWiringDerivations(t *testing.T) {
	tests := []struct {
		name      string
		wiring    Wiring
		pinLevel  int
		energised bool
		powered   bool
	}{
		{"active-high NO low", Wiring{Drive: DriveActiveHigh, Contact: ContactNO}, 0, false, false},
		{"active-high NO high", Wiring{Drive: DriveActiveHigh, Contact: ContactNO}, 1, true, true},
		{"active-high NC low", Wiring{Drive: DriveActiveHigh, Contact: ContactNC}, 0, false, true},
		{"active-high NC high", Wiring{Drive: DriveActiveHigh, Contact: ContactNC}, 1, true, false},
		{"active-low NO low", Wiring{Drive: DriveActiveLow, Contact: ContactNO}, 0, true, true},
		{"active-low NO high", Wiring{Drive: DriveActiveLow, Contact: ContactNO}, 1, false, false},
		{"active-low NC low", Wiring{Drive: DriveActiveLow, Contact: ContactNC}, 0, true, false},
		{"active-low NC high", Wiring{Drive: DriveActiveLow, Contact: ContactNC}, 1, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.energised, tt.wiring.Energised(tt.pinLevel))
			assert.Equal(t, tt.powered, tt.wiring.LoadPowered(tt.pinLevel))
		})
	}
}

func TestExpectedIdleLevel(t *testing.T) {
	tests := []struct {
		contact Contact
		pull    Pull
		want    int
	}{
		{ContactNO, PullUp, 1},
		{ContactNC, PullUp, 0},
		{ContactNO, PullDown, 0},
		{ContactNC, PullDown, 1},
		{ContactNO, PullNone, 0},
		{ContactNC, PullNone, 1},
		{ContactNO, PullExternal, 0},
		{ContactNC, PullExternal, 1},
	}
	for _, tt := range tests {
		w := Wiring{Contact: tt.contact, Pull: tt.pull}
		assert.Equal(t, tt.want, w.ExpectedIdleLevel(), "contact=%s pull=%s", tt.contact, tt.pull)
	}
}

func TestWiringValidate(t *testing.T) {
	require.NoError(t, Wiring{}.Validate("x"))
	err := Wiring{Drive: "sideways", Contact: "nf", Mechanism: "sticky", Pull: "middle"}.Validate("devices[0].wiring")
	require.Error(t, err)
	message := err.Error()
	assert.Contains(t, message, "devices[0].wiring.drive")
	assert.Contains(t, message, "devices[0].wiring.contact")
	assert.Contains(t, message, "devices[0].wiring.mechanism")
	assert.Contains(t, message, "devices[0].wiring.pull")
}

func TestWiringWarningsDetectsMismatch(t *testing.T) {
	device := DeviceState{
		Kind:         DeviceKindButton,
		DefaultState: 0,
		Wiring:       Wiring{Contact: ContactNO, Pull: PullUp, Mechanism: MechanismMomentary},
	}
	warnings := WiringWarnings(device, nil)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "WIRING_MISMATCH")
	assert.Contains(t, warnings[0], "driver default state 0")
	assert.Contains(t, warnings[0], "expected idle 1")

	device.DefaultState = 1
	assert.Empty(t, WiringWarnings(device, nil))
}

func TestWiringWarningsDriveMismatch(t *testing.T) {
	relay := DeviceState{
		Kind:     DeviceKindRelay,
		Inverted: false,
		Wiring:   Wiring{Drive: DriveActiveLow, Contact: ContactNO},
	}
	warnings := WiringWarnings(relay, nil)
	require.Len(t, warnings, 1)
	assert.True(t, strings.Contains(warnings[0], "WIRING_MISMATCH"))

	relay.Inverted = true
	assert.Empty(t, WiringWarnings(relay, nil))
}

func TestGroupForKindCoversEveryKind(t *testing.T) {
	for _, kind := range DeviceKinds() {
		assert.True(t, kind.Valid(), "kind %s should be valid", kind)
		assert.True(t, GroupForKind(kind).Valid(), "kind %s should have a valid group", kind)
	}
	assert.False(t, DeviceKind("bogus").Valid())
}
