export type PinMode =
  | 'unset'
  | 'input'
  | 'output'
  | 'input-pullup'
  | 'pwm'
  | 'servo'
  | 'analog'

export type DeviceKind =
  | 'relay'
  | 'led'
  | 'rgb-led'
  | 'button'
  | 'captor'
  | 'pir'
  | 'sensor'
  | 'pwm'
  | 'servo'
  | 'analog'
  | 'analog-actuator'
  | 'motor'
  | 'stepper'
  | 'buzzer'
  | 'direct-pin'
  | 'distance'
  | 'temperature'
  | 'display'
  | 'expander'
  | 'imu'
  | 'adc'
  | 'rfid'
  | 'value'
  | 'function'

export type DeviceGroup =
  | 'inputs'
  | 'outputs'
  | 'actuators'
  | 'sensors'
  | 'displays'
  | 'expanders'
  | 'pins'
  | 'extras'

export type SimulatorEventSource = 'hardware-code' | 'ui' | 'simulation'

export type EventType =
  | 'snapshot'
  | 'board.updated'
  | 'board.started'
  | 'device.updated'
  | 'pin.updated'
  | 'value.updated'
  | 'function.called'
  | 'fault.changed'
  | 'trace'
  | 'heartbeat'

export type DeviceFault =
  | 'stuck-high'
  | 'stuck-low'
  | 'welded-contact'
  | 'open-coil'
  | 'disconnected-load'
  | 'noisy'
  | 'bus-nak'
  | 'bus-timeout'

export interface Wiring {
  drive?: 'active-high' | 'active-low'
  contact?: 'no' | 'nc'
  mechanism?: 'none' | 'momentary' | 'latching' | 'selector'
  pull?: 'none' | 'up' | 'down' | 'external'
  loadLabel?: string
}

export interface PinState {
  pin: string
  label?: string
  mode: PinMode
  digitalValue: number
  byteValue: number
  analogValue: number
  invertInitial: boolean
  pullup: boolean
  updatedAt: string
}

export interface DeviceCapabilities {
  readable: boolean
  writable: boolean
  pressable: boolean
  toggleable: boolean
  callable: boolean
  commandable: boolean
  minValue?: number
  maxValue?: number
  unit?: string
}

export interface DeviceState {
  id: string
  boardId: string
  name: string
  label: string
  kind: DeviceKind
  group: DeviceGroup
  driver: string
  pins: string[]
  valueName?: string
  functionName?: string
  inverted: boolean
  defaultState: number
  active: boolean
  state: Record<string, unknown>
  capabilities: DeviceCapabilities
  labels?: Record<string, string>
  wiring: Wiring
  energised: boolean
  loadPowered: boolean
  expectedIdleLevel: number
  warnings?: string[]
  faults?: DeviceFault[]
  source?: string
  updatedAt: string
}

export interface ExtraValue {
  name: string
  value: unknown
  updatedAt: string
}

export interface ExtraFunction {
  name: string
  returnCode: number
  lastParameters?: string
  callCount: number
  lastCalledAt?: string
}

export interface FaultState {
  active: boolean
  message?: string
  updatedAt?: string
}

export interface BoardState {
  id: string
  name: string
  platform: string
  port?: string
  connected: boolean
  online: boolean
  reconnectCount: number
  simulatedLagMs: number
  fault: FaultState
  pins: Record<string, PinState>
  devices: Record<string, DeviceState>
  values: Record<string, ExtraValue>
  functions: Record<string, ExtraFunction>
  createdAt: string
  updatedAt: string
}

export interface ServerMetadata {
  version: string
  startedAt: string
  uptimeSeconds: number
  currentSeq: number
  connectedClients: number
  droppedEvents: number
}

export interface Snapshot {
  meta: ServerMetadata
  boards: BoardState[]
}

export interface SimulatorEvent {
  seq: number
  timestamp: string
  type: EventType
  source: SimulatorEventSource
  boardId?: string
  deviceId?: string
  pin?: string
  valueName?: string
  oldValue?: unknown
  newValue?: unknown
  board?: BoardState
  device?: DeviceState
  pinState?: PinState
  fault?: FaultState
}

export type TraceKind =
  | 'pin-read'
  | 'pin-write'
  | 'pwm'
  | 'servo'
  | 'analog'
  | 'bus-txn'
  | 'value-read'
  | 'value-write'
  | 'function-call'
  | 'driver-event'
  | 'fault'
  | 'step'

export interface TraceRecord {
  seq: number
  timestamp: string
  boardId?: string
  kind: TraceKind
  target?: string
  operation?: string
  value?: unknown
  source?: SimulatorEventSource
  durationNs?: number
  error?: string
  detail?: Record<string, unknown>
}

export interface WaveformPoint {
  timestamp: string
  value: number
}

export interface WaveformSeries {
  pin: string
  points: WaveformPoint[]
  downsampled: boolean
}

export interface ScenarioTrigger {
  type:
    | 'device.changed'
    | 'pin.changed'
    | 'value.changed'
    | 'function.called'
    | 'bus.written'
    | 'timer.every'
    | 'delay.after'
    | 'board.started'
    | 'manual'
  device?: string
  pin?: string
  name?: string
  bus?: string
  every?: string
  after?: string
}

export interface ScenarioAction {
  type:
    | 'setPin'
    | 'setValue'
    | 'press'
    | 'release'
    | 'command'
    | 'ramp'
    | 'setFault'
    | 'clearFault'
    | 'emitStep'
  target?: string
  pin?: string
  name?: string
  command?: string
  value?: unknown
  from?: number
  to?: number
  durationMs?: number
  stepMs?: number
  delayMs?: number
  fault?: string
  status?: string
  detail?: Record<string, unknown>
}

export interface ScenarioCondition {
  type: string
  target: string
  operator?: string
  value?: unknown
}

export interface ScenarioRule {
  id: string
  name?: string
  enabled?: boolean
  board?: string
  trigger: ScenarioTrigger
  conditions?: ScenarioCondition[]
  actions: ScenarioAction[]
}

export interface TopologySpec {
  schema: string
  boards: Array<Record<string, unknown>>
  scenarios?: ScenarioRule[]
}

export interface ChipDescriptor {
  model: string
  name: string
  transport: string
  address?: number
  registers?: Array<Record<string, unknown>>
  fields?: Array<{
    name: string
    bits?: string
    unit?: string
    min?: number
    max?: number
    widget?: string
  }>
  widget?: string
}

export interface ChipEntry {
  id: string
  boardId: string
  transport: string
  bus: string
  busNumber?: number
  address?: number
  model: string
  descriptor: ChipDescriptor
  faults?: DeviceFault[]
}

export interface DisplaySnapshot {
  model: string
  width: number
  height: number
  pixels?: number[]
  lines?: string[]
  enabled: boolean
  zoom?: number
}

export interface DisplayChipValues {
  descriptor: ChipDescriptor
  values?: Record<string, unknown>
  display?: DisplaySnapshot
  chip?: ChipEntry
}

export interface DevicePatch {
  active?: boolean
  value?: number
  byteValue?: number
  angle?: number
}

export interface PinPatch {
  digitalValue?: number
  byteValue?: number
  analogValue?: number
  mode?: PinMode
}

export interface FunctionResult {
  function: ExtraFunction
  result: number
}
