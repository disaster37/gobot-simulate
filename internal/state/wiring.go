package state

import (
	"errors"
	"fmt"
	"strings"
)

// Drive describes how a coil or load is driven by the pin level.
type Drive string

const (
	DriveActiveHigh Drive = "active-high"
	DriveActiveLow  Drive = "active-low"
)

// Contact describes the relay contact configuration.
type Contact string

const (
	ContactNO Contact = "no"
	ContactNC Contact = "nc"
)

// Mechanism describes the physical input mechanism.
type Mechanism string

const (
	MechanismNone      Mechanism = "none"
	MechanismMomentary Mechanism = "momentary"
	MechanismLatching  Mechanism = "latching"
	MechanismSelector  Mechanism = "selector"
)

// Pull describes the input bias resistor.
type Pull string

const (
	PullNone     Pull = "none"
	PullUp       Pull = "up"
	PullDown     Pull = "down"
	PullExternal Pull = "external"
)

// Wiring is the declared electrical wiring semantics for a device.
type Wiring struct {
	Drive     Drive     `json:"drive,omitempty" yaml:"drive,omitempty"`
	Contact   Contact   `json:"contact,omitempty" yaml:"contact,omitempty"`
	Mechanism Mechanism `json:"mechanism,omitempty" yaml:"mechanism,omitempty"`
	Pull      Pull      `json:"pull,omitempty" yaml:"pull,omitempty"`
	LoadLabel string    `json:"loadLabel,omitempty" yaml:"loadLabel,omitempty"`
}

// Normalized fills in the documented defaults.
func (w Wiring) Normalized() Wiring {
	if w.Drive == "" {
		w.Drive = DriveActiveHigh
	}
	if w.Contact == "" {
		w.Contact = ContactNO
	}
	if w.Mechanism == "" {
		w.Mechanism = MechanismNone
	}
	if w.Pull == "" {
		w.Pull = PullNone
	}
	return w
}

// IsZero reports whether no wiring was declared.
func (w Wiring) IsZero() bool {
	return w == Wiring{}
}

// Validate checks enum values and returns path-qualified errors.
func (w Wiring) Validate(path string) error {
	w = w.Normalized()
	var errs []error
	switch w.Drive {
	case DriveActiveHigh, DriveActiveLow:
	default:
		errs = append(errs, fmt.Errorf("%s.drive: unknown value %q", path, w.Drive))
	}
	switch w.Contact {
	case ContactNO, ContactNC:
	default:
		errs = append(errs, fmt.Errorf("%s.contact: unknown value %q", path, w.Contact))
	}
	switch w.Mechanism {
	case MechanismNone, MechanismMomentary, MechanismLatching, MechanismSelector:
	default:
		errs = append(errs, fmt.Errorf("%s.mechanism: unknown value %q", path, w.Mechanism))
	}
	switch w.Pull {
	case PullNone, PullUp, PullDown, PullExternal:
	default:
		errs = append(errs, fmt.Errorf("%s.pull: unknown value %q", path, w.Pull))
	}
	return errors.Join(errs...)
}

// Energised derives whether the coil is energised from the pin level.
func (w Wiring) Energised(pinLevel int) bool {
	w = w.Normalized()
	return (pinLevel == 1) != (w.Drive == DriveActiveLow)
}

// LoadPowered derives whether the contact passes power to the load.
func (w Wiring) LoadPowered(pinLevel int) bool {
	w = w.Normalized()
	return w.Energised(pinLevel) != (w.Contact == ContactNC)
}

// ExpectedIdleLevel derives the expected idle digital level for an input.
func (w Wiring) ExpectedIdleLevel() int {
	w = w.Normalized()
	activeHigh := true
	switch w.Pull {
	case PullUp:
		activeHigh = w.Contact == ContactNO
	case PullDown, PullNone, PullExternal:
		activeHigh = w.Contact != ContactNO
	}
	if activeHigh {
		return 1
	}
	return 0
}

// IsInput reports whether the device kind is an input.
func IsInput(kind DeviceKind) bool {
	switch kind {
	case DeviceKindButton, DeviceKindCaptor, DeviceKindPIR:
		return true
	default:
		return false
	}
}

// IsOutput reports whether the device kind is an output.
func IsOutput(kind DeviceKind) bool {
	switch kind {
	case DeviceKindRelay, DeviceKindLED, DeviceKindRGBLED, DeviceKindMotor,
		DeviceKindStepper, DeviceKindServo, DeviceKindPWM, DeviceKindBuzzer,
		DeviceKindAnalogActuator, DeviceKindDirectPin:
		return true
	default:
		return false
	}
}

// WiringWarnings derives non-fatal warnings for a device.
func WiringWarnings(d DeviceState, pin *PinState) []string {
	var warnings []string
	w := d.Wiring.Normalized()
	if d.Kind.Valid() {
		if want := w.Drive == DriveActiveLow; d.Inverted != want && (d.Kind == DeviceKindRelay || d.Kind == DeviceKindLED) {
			warnings = append(warnings, "WIRING_MISMATCH: driver inversion disagrees with wiring drive")
		}
	}
	if IsInput(d.Kind) {
		expected := w.ExpectedIdleLevel()
		if d.DefaultState != expected {
			warnings = append(warnings,
				fmt.Sprintf("WIRING_MISMATCH: driver default state %d disagrees with expected idle %d for %s with %s contact and pull-%s",
					d.DefaultState, expected, d.Kind, w.Contact, w.Pull))
		}
	}
	_ = pin
	return dedupeWarnings(warnings)
}

func dedupeWarnings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		key := strings.TrimSpace(value)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}
