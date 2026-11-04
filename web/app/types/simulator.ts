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
  | 'button'
  | 'captor'
  | 'sensor'
  | 'pwm'
  | 'servo'
  | 'analog'
  | 'value'
  | 'function'

export type EventSource = 'hardware-code' | 'ui' | 'simulation'

export type EventType =
  | 'snapshot'
  | 'board.updated'
  | 'device.updated'
  | 'pin.updated'
  | 'value.updated'
  | 'fault.changed'
  | 'heartbeat'

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
  source: EventSource
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
