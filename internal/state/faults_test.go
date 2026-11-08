package state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDeviceFault(t *testing.T) {
	for _, fault := range DeviceFaults() {
		parsed, err := ParseDeviceFault(string(fault))
		require.NoError(t, err)
		assert.Equal(t, fault, parsed)
	}
	_, err := ParseDeviceFault("melted")
	require.ErrorIs(t, err, ErrInvalidFault)
}

func TestEffectiveDigitalAppliesReadFaults(t *testing.T) {
	pin := PinState{Pin: "3", DigitalValue: 1}
	tests := []struct {
		name   string
		faults []DeviceFault
		value  int
		loadOK bool
	}{
		{"no fault", nil, 1, true},
		{"stuck low", []DeviceFault{FaultStuckLow}, 0, true},
		{"stuck high", []DeviceFault{FaultStuckHigh}, 1, true},
		{"welded contact", []DeviceFault{FaultWeldedContact}, 0, true},
		{"open coil", []DeviceFault{FaultOpenCoil}, 1, false},
		{"disconnected load", []DeviceFault{FaultDisconnected}, 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			devices := []DeviceState{{ID: "dev", Pins: []string{"3"}, Faults: tt.faults}}
			value, loadOK := effectiveDigital(pin, devices, 0)
			assert.Equal(t, tt.value, value)
			assert.Equal(t, tt.loadOK, loadOK)
		})
	}
}

func TestEffectiveDigitalIgnoresOtherPins(t *testing.T) {
	pin := PinState{Pin: "3", DigitalValue: 0}
	devices := []DeviceState{{ID: "dev", Pins: []string{"4"}, Faults: []DeviceFault{FaultStuckHigh}}}
	value, loadOK := effectiveDigital(pin, devices, 0)
	assert.Equal(t, 0, value)
	assert.True(t, loadOK)
}

func TestEffectiveDigitalNoisyFlipsPeriodically(t *testing.T) {
	pin := PinState{Pin: "3", DigitalValue: 0}
	devices := []DeviceState{{ID: "dev", Pins: []string{"3"}, Faults: []DeviceFault{FaultNoisy}}}
	value, _ := effectiveDigital(pin, devices, faultNoiseModulus)
	assert.Equal(t, 1, value)
	value, _ = effectiveDigital(pin, devices, 1)
	assert.Equal(t, 0, value)
}

func TestWriteFaulted(t *testing.T) {
	devices := []DeviceState{
		{ID: "stuck-high", Pins: []string{"3"}, Faults: []DeviceFault{FaultStuckHigh}},
		{ID: "stuck-low", Pins: []string{"4"}, Faults: []DeviceFault{FaultStuckLow}},
	}
	assert.Equal(t, 1, writeFaulted("3", 0, devices))
	assert.Equal(t, 0, writeFaulted("4", 1, devices))
	assert.Equal(t, 1, writeFaulted("5", 1, devices))
}

func TestDeviceHasFault(t *testing.T) {
	device := DeviceState{Faults: []DeviceFault{FaultStuckHigh, FaultNoisy}}
	assert.True(t, device.HasFault(FaultStuckHigh))
	assert.True(t, device.HasFault(FaultNoisy))
	assert.False(t, device.HasFault(FaultOpenCoil))
}
