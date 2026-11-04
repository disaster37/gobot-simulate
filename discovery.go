package simulate

import (
	"errors"
	"fmt"
	"strings"

	"github.com/disaster37/gobot-simulate/internal/state"
	"gobot.io/x/gobot/v2"
	"gobot.io/x/gobot/v2/drivers/aio"
	"gobot.io/x/gobot/v2/drivers/gpio"
)

// AutoDiscover inspects a Gobot robot's exported driver methods and registers
// matching devices on the simulated board. Hints take precedence over
// inference and are matched by pin, ID, or driver name.
func (s *Simulator) AutoDiscover(robot *gobot.Robot, hints ...DeviceSpec) error {
	if robot == nil {
		return errors.New("simulate: nil robot")
	}

	var adaptor *Adaptor
	robot.Connections().Each(func(connection gobot.Connection) {
		if a, ok := connection.(*Adaptor); ok && adaptor == nil {
			adaptor = a
		}
	})
	if adaptor == nil {
		return errors.New("simulate: robot has no simulated adaptor")
	}

	if adaptor.Name() == "test" && robot.Name != "" {
		adaptor.SetName(robot.Name)
	}

	board := adaptor.board
	var errs []error
	robot.Devices().Each(func(device gobot.Device) {
		if err := s.discoverDevice(board, device, robot.Running(), hints); err != nil {
			errs = append(errs, err)
		}
	})
	return errors.Join(errs...)
}

type discoveredSpec struct {
	DeviceSpec
	Driver string
	State  map[string]interface{}
	Active bool
}

func (s *Simulator) discoverDevice(board *state.Board, device gobot.Device, running bool, hints []DeviceSpec) error {
	switch d := device.(type) {
	case *gpio.RelayDriver:
		hint := findHint(hints, d.Name(), d.Pin())
		spec := baseSpec(hint, device, d.Pin(), state.DeviceKindRelay)
		spec.Inverted = d.IsInverted() || spec.Inverted
		spec.Active = d.State()
		spec.State = map[string]interface{}{"pinValue": pinValue(board, d.Pin()), "active": d.State()}
		return s.registerDiscovered(board, spec)

	case *gpio.LedDriver:
		hint := findHint(hints, d.Name(), d.Pin())
		spec := baseSpec(hint, device, d.Pin(), state.DeviceKindLED)
		spec.Active = d.State()
		spec.State = map[string]interface{}{"pinValue": pinValue(board, d.Pin()), "active": d.State()}
		return s.registerDiscovered(board, spec)

	case *gpio.ButtonDriver:
		hint := findHint(hints, d.Name(), d.Pin())
		pin, _ := board.Pin(d.Pin())
		kind := state.DeviceKindButton
		if hint != nil && hint.Kind.Valid() {
			kind = hint.Kind
		} else if pin.InvertInitial || strings.Contains(strings.ToLower(d.Name()+" "+pin.Label), "captor") {
			kind = state.DeviceKindCaptor
		}
		spec := baseSpec(hint, device, d.Pin(), kind)
		spec.DefaultState = IntPtr(inferButtonDefaultState(hint, d, pin, running))
		spec.Active = d.Active()
		spec.State = map[string]interface{}{
			"pinValue":     pin.DigitalValue,
			"defaultState": *spec.DefaultState,
			"active":       d.Active(),
		}
		return s.registerDiscovered(board, spec)

	case *gpio.ServoDriver:
		hint := findHint(hints, d.Name(), d.Pin())
		spec := baseSpec(hint, device, d.Pin(), state.DeviceKindServo)
		spec.State = map[string]interface{}{"angle": int(d.Angle())}
		return s.registerDiscovered(board, spec)

	case *aio.AnalogSensorDriver:
		hint := findHint(hints, d.Name(), d.Pin())
		spec := baseSpec(hint, device, d.Pin(), state.DeviceKindSensor)
		spec.State = map[string]interface{}{"rawValue": d.RawValue(), "value": d.Value()}
		return s.registerDiscovered(board, spec)

	default:
		if value, ok := device.(interface {
			gobot.Device
			ValueName() string
		}); ok {
			hint := findHint(hints, device.Name(), "")
			spec := baseSpec(hint, device, "", state.DeviceKindValue)
			spec.ValueName = value.ValueName()
			spec.State = map[string]interface{}{"value": boardValue(board, spec.ValueName)}
			spec.Active = truthy(spec.State["value"])
			return s.registerDiscovered(board, spec)
		}
		if function, ok := device.(interface {
			gobot.Device
			FunctionName() string
			Parameters() string
		}); ok {
			hint := findHint(hints, device.Name(), "")
			spec := baseSpec(hint, device, "", state.DeviceKindFunction)
			spec.FunctionName = function.FunctionName()
			fn, _ := board.Function(spec.FunctionName)
			spec.State = map[string]interface{}{
				"callCount":      fn.CallCount,
				"returnCode":     fn.ReturnCode,
				"lastParameters": function.Parameters(),
			}
			return s.registerDiscovered(board, spec)
		}
		return nil
	}
}

func baseSpec(hint *DeviceSpec, device gobot.Device, pin string, kind state.DeviceKind) discoveredSpec {
	if hint != nil && hint.Kind.Valid() {
		kind = hint.Kind
	}
	name := device.Name()
	if hint != nil && hint.Name != "" {
		name = hint.Name
	}
	id := ""
	if hint != nil {
		id = hint.ID
	}
	if id == "" {
		id = name
	}
	if id == "" {
		id = string(kind)
	}
	return discoveredSpec{
		DeviceSpec: DeviceSpec{
			ID:           state.NormalizeIdentifier(id),
			Name:         name,
			Label:        deviceLabel(hint, name),
			Kind:         kind,
			Pin:          pin,
			Inverted:     hint != nil && hint.Inverted,
			Labels:       hintLabels(hint),
			Capabilities: hintCapabilities(hint),
		},
		Driver: fmt.Sprintf("%T", device),
	}
}

func (s *Simulator) registerDiscovered(board *state.Board, spec discoveredSpec) error {
	id := uniqueDeviceID(board, spec.ID, spec.Pin)
	pins := devicePins(spec.DeviceSpec)
	for _, pin := range pins {
		if err := state.ValidateIdentifier("pin", pin); err != nil {
			return err
		}
	}
	if err := state.ValidateIdentifier("device", id); err != nil {
		return err
	}
	kind := spec.Kind
	if !kind.Valid() {
		kind = state.DeviceKindValue
	}
	label := spec.Label
	if label == "" {
		label = id
	}
	defaultState := 0
	if spec.DefaultState != nil {
		defaultState = *spec.DefaultState
	}
	caps := state.DefaultCapabilities(kind)
	if spec.Capabilities != nil {
		caps = *spec.Capabilities
	}
	deviceState := state.DeviceState{
		ID:           id,
		BoardID:      board.ID(),
		Name:         spec.Name,
		Label:        label,
		Kind:         kind,
		Driver:       spec.Driver,
		Pins:         pins,
		ValueName:    spec.ValueName,
		FunctionName: spec.FunctionName,
		Inverted:     spec.Inverted,
		DefaultState: defaultState,
		Active:       spec.Active,
		State:        spec.State,
		Capabilities: caps,
		Labels:       spec.Labels,
	}
	board.RegisterDevice(deviceState)
	return nil
}

func inferButtonDefaultState(hint *DeviceSpec, d *gpio.ButtonDriver, pin state.PinState, running bool) int {
	if hint != nil && hint.DefaultState != nil {
		return *hint.DefaultState
	}
	if running {
		if !d.Active() {
			return pin.DigitalValue
		}
		return 1 - pin.DigitalValue
	}
	if pin.Pullup && !pin.InvertInitial {
		return 1
	}
	return 0
}

func findHint(hints []DeviceSpec, name, pin string) *DeviceSpec {
	bestScore := 0
	var best *DeviceSpec
	for i := range hints {
		hint := &hints[i]
		score := 0
		switch {
		case hint.ID != "" && hint.ID == name:
			score = 3
		case hint.Pin != "" && hint.Pin == pin:
			score = 2
		case hint.Name != "" && hint.Name == name:
			score = 1
		}
		if score > bestScore {
			bestScore = score
			best = hint
		}
	}
	return best
}

func uniqueDeviceID(board *state.Board, id, pin string) string {
	if id == "" {
		id = "device"
	}
	if !board.HasDevice(id) {
		return id
	}
	if pin != "" {
		candidate := state.NormalizeIdentifier(id + "_" + pin)
		if !board.HasDevice(candidate) {
			return candidate
		}
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s_%d", id, i)
		if !board.HasDevice(candidate) {
			return candidate
		}
	}
}

func deviceLabel(hint *DeviceSpec, fallback string) string {
	if hint != nil && hint.Label != "" {
		return hint.Label
	}
	return fallback
}

func hintLabels(hint *DeviceSpec) map[string]string {
	if hint == nil {
		return nil
	}
	return hint.Labels
}

func hintCapabilities(hint *DeviceSpec) *DeviceCapabilities {
	if hint == nil {
		return nil
	}
	return hint.Capabilities
}

func pinValue(board *state.Board, pin string) int {
	if p, ok := board.Pin(pin); ok {
		return p.DigitalValue
	}
	return 0
}

func boardValue(board *state.Board, name string) interface{} {
	if value, ok := board.Value(name); ok {
		return value.Value
	}
	return nil
}

func truthy(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return v != ""
	case float64:
		return v != 0
	case int:
		return v != 0
	case nil:
		return false
	default:
		return true
	}
}
