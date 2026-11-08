# Topology and scenario schema (v1)

`SIMULATE_TOPOLOGY` (or `--topology`) points at a YAML or JSON document. The
same document can be applied through `simulate.LoadTopology` /
`LoadTopologyFile`.

Precedence: topology file > Go `DeviceSpec` hints > `AutoDiscover` inference.
Unknown fields are rejected. Validation errors are path-qualified, for example
`boards[1].devices[3].wiring.contact: unknown value "nf"`.

```yaml
schema: "1"
boards:
  - id: dfp
    name: DFP
    profile: raspi
    platform: raspberry-pi
    port: /dev/ttyUSB0
    latency: 5ms
    pins:
      - pin: "3"
        label: drum
        mode: output
      - pin: "11"
        mode: input
        pull: up
    devices:
      - id: relay_drum
        name: relay_drum
        label: Drum
        kind: relay
        pin: "3"
        wiring: { drive: active-high, contact: no, mechanism: none, pull: none, loadLabel: drum }
      - id: button_start
        kind: button
        pin: "11"
        defaultState: 1
        wiring: { contact: no, mechanism: momentary, pull: up }
    chips:
      - id: oled
        bus: i2c-1
        address: 60
        model: ssd1306
        options: {}
        faults: [bus-nak]
    values:
      distance: 0
    functions:
      ack: { returnCode: 0 }
scenarios:
  - id: wash
    name: DFP wash cycle
    enabled: true
    board: dfp
    trigger: { type: board.started }
    actions:
      - type: emitStep
        name: wash-start
        status: running
      - type: command
        target: relay_drum
        command: on
        delayMs: 500
```

## Boards

| Field | Type | Notes |
|---|---|---|
| `id` | identifier | required, unique |
| `name`, `profile`, `platform`, `port` | string | metadata |
| `latency` | duration | applied to every adaptor I/O call |
| `pins` | list | pin label, mode, pull, invert |
| `devices` | list | device bindings (see below) |
| `chips` | list | bus-attached chip models |
| `values` / `functions` | map | initial aREST value/function state |

Device kinds: `relay`, `led`, `rgb-led`, `button`, `captor`, `pir`, `sensor`,
`pwm`, `servo`, `analog`, `analog-actuator`, `motor`, `stepper`, `buzzer`,
`direct-pin`, `distance`, `temperature`, `display`, `expander`, `imu`, `adc`,
`rfid`, `value`, `function`.

Wiring fields: `drive` (`active-high`, `active-low`), `contact` (`no`, `nc`),
`mechanism` (`none`, `momentary`, `latching`, `selector`), `pull` (`none`, `up`,
`down`, `external`), `loadLabel`.

Pins: `pin`, `label`, `mode` (`unset`, `input`, `output`, `input-pullup`,
`pwm`, `servo`, `analog`), `pull`, `invert`.

Chips: `id`, `bus` (`i2c-1`, `spi-0`, `onewire`, `serial`, `ble`, `gpio`),
`address`, `model`, `options`, `faults`.

## Scenarios

A rule is `{id, name, enabled, board, trigger, conditions[], actions[]}`.

Triggers:

| Type | Extra fields | Fires when |
|---|---|---|
| `device.changed` | `device` | a device state changes |
| `pin.changed` | `pin` | a pin changes |
| `value.changed` | `name` | an aREST value changes |
| `function.called` | `name` | an aREST function is invoked |
| `bus.written` | `bus`, `address` | a bus transaction completes |
| `timer.every` | `every` | repeatedly on a duration |
| `delay.after` | `after` | once after a duration |
| `board.started` | — | board becomes connected |
| `manual` | — | `POST /scenarios/:id/trigger` |

Conditions: `{type, target, operator, value}` with `eq`, `ne`, `gt`, `gte`,
`lt`, `lte`, `truthy`, `falsy`. `type` may be `value`, `newValue`, `oldValue`,
`device`, `pin` or `source`.

Actions: `setPin`, `setValue`, `press`, `release`, `command`, `ramp`,
`setFault`, `clearFault`, `emitStep`. Every action may set `delayMs`.
`ramp` additionally uses `from`, `to`, `durationMs`, `stepMs`.

### Manual override semantics

A UI/API write marks the affected target as manually controlled. Rules acting on
that target drop their actions while the override is active. The override is
cleared by the next non-manual trigger on the same target, at which point rules
resume normally. Dropped actions are counted in `ScenarioDropped()`.

### Loop protection

Each rule has a sliding execution budget (100 executions per second) in addition
to the per-cycle depth limit. Executions beyond the budget are dropped and
counted; this prevents action/trigger feedback loops from spinning forever.
