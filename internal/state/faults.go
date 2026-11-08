package state

import (
	"errors"
	"fmt"
)

// DeviceFault enumerates injectable device-level hardware faults.
type DeviceFault string

const (
	FaultStuckHigh     DeviceFault = "stuck-high"
	FaultStuckLow      DeviceFault = "stuck-low"
	FaultWeldedContact DeviceFault = "welded-contact"
	FaultOpenCoil      DeviceFault = "open-coil"
	FaultDisconnected  DeviceFault = "disconnected-load"
	FaultNoisy         DeviceFault = "noisy"
	FaultBusNAK        DeviceFault = "bus-nak"
	FaultBusTimeout    DeviceFault = "bus-timeout"
	FaultUnknown       DeviceFault = "unknown"
	faultNoiseModulus              = 7
)

// DeviceFaults returns every supported device fault.
func DeviceFaults() []DeviceFault {
	return []DeviceFault{
		FaultStuckHigh, FaultStuckLow, FaultWeldedContact, FaultOpenCoil,
		FaultDisconnected, FaultNoisy, FaultBusNAK, FaultBusTimeout,
	}
}

// Valid reports whether f is a known device fault.
func (f DeviceFault) Valid() bool {
	for _, candidate := range DeviceFaults() {
		if f == candidate {
			return true
		}
	}
	return false
}

// ErrInvalidFault is returned for an unknown device fault.
var ErrInvalidFault = errors.New("simulate: invalid fault")

// ParseDeviceFault validates and normalizes a fault name.
func ParseDeviceFault(raw string) (DeviceFault, error) {
	f := DeviceFault(raw)
	if !f.Valid() {
		return "", fmt.Errorf("%w: %q", ErrInvalidFault, raw)
	}
	return f, nil
}

// HasFault reports whether d has f injected.
func (d DeviceState) HasFault(f DeviceFault) bool {
	for _, candidate := range d.Faults {
		if candidate == f {
			return true
		}
	}
	return false
}

// effectiveDigital applies device read faults to a pin value. It returns the
// value observed by a driver read and whether the load is actually powered.
func effectiveDigital(pin PinState, devices []DeviceState, noiseSeq uint64) (int, bool) {
	value := pin.DigitalValue
	loadOK := true
	welded := false
	for _, d := range devices {
		if !containsString(d.Pins, pin.Pin) {
			continue
		}
		switch {
		case d.HasFault(FaultStuckHigh):
			value = 1
		case d.HasFault(FaultStuckLow):
			value = 0
		case d.HasFault(FaultWeldedContact):
			welded = true
		}
		if d.HasFault(FaultOpenCoil) || d.HasFault(FaultDisconnected) {
			loadOK = false
		}
		if d.HasFault(FaultNoisy) && noiseSeq%faultNoiseModulus == 0 {
			value = 1 - value
		}
	}
	if welded {
		value = 1 - value
	}
	return value, loadOK
}

// writeFaulted applies write faults to a commanded pin value.
func writeFaulted(pin string, commanded int, devices []DeviceState) int {
	value := commanded
	for _, d := range devices {
		if !containsString(d.Pins, pin) {
			continue
		}
		switch {
		case d.HasFault(FaultStuckHigh):
			value = 1
		case d.HasFault(FaultStuckLow):
			value = 0
		}
	}
	return value
}
