package state

// DeviceGroup groups device kinds into UI sections.
type DeviceGroup string

const (
	GroupInputs    DeviceGroup = "inputs"
	GroupOutputs   DeviceGroup = "outputs"
	GroupActuators DeviceGroup = "actuators"
	GroupSensors   DeviceGroup = "sensors"
	GroupDisplays  DeviceGroup = "displays"
	GroupExpanders DeviceGroup = "expanders"
	GroupPins      DeviceGroup = "pins"
	GroupExtras    DeviceGroup = "extras"
)

// Valid reports whether g is a known device group.
func (g DeviceGroup) Valid() bool {
	switch g {
	case GroupInputs, GroupOutputs, GroupActuators, GroupSensors,
		GroupDisplays, GroupExpanders, GroupPins, GroupExtras:
		return true
	default:
		return false
	}
}

// GroupForKind returns the UI group for a device kind.
func GroupForKind(kind DeviceKind) DeviceGroup {
	switch kind {
	case DeviceKindButton, DeviceKindCaptor, DeviceKindPIR:
		return GroupInputs
	case DeviceKindRelay, DeviceKindLED, DeviceKindRGBLED:
		return GroupOutputs
	case DeviceKindMotor, DeviceKindStepper, DeviceKindServo, DeviceKindPWM,
		DeviceKindBuzzer, DeviceKindAnalogActuator, DeviceKindDirectPin:
		return GroupActuators
	case DeviceKindSensor, DeviceKindAnalog, DeviceKindTemperature, DeviceKindIMU,
		DeviceKindADC, DeviceKindRFID, DeviceKindDistance:
		return GroupSensors
	case DeviceKindDisplay:
		return GroupDisplays
	case DeviceKindExpander:
		return GroupExpanders
	case DeviceKindValue, DeviceKindFunction:
		return GroupExtras
	default:
		return GroupExtras
	}
}
