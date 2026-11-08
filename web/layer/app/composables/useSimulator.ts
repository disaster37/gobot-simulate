import { computed, ref } from 'vue'
import type {
  BoardState,
  ChipEntry,
  DevicePatch,
  DeviceState,
  DisplayChipValues,
  FunctionResult,
  PinPatch,
  PinState,
  ScenarioRule,
  SimulatorEvent,
  Snapshot,
  TopologySpec,
  TraceRecord,
  WaveformSeries,
} from '../types/simulator'

export interface SimulatorOptions {
  baseURL?: string
  fetcher?: typeof fetch
  token?: string
}

const defaultBaseURL = '/api/v1'

const snapshot = ref<Snapshot | null>(null)
const lastSeq = ref(0)
const loading = ref(false)
const error = ref<string | null>(null)
const events = ref<SimulatorEvent[]>([])
const trace = ref<TraceRecord[]>([])
const authToken = ref('')

let baseURL = defaultBaseURL
let fetcher: typeof fetch = (...args) => globalThis.fetch(...args)

const boards = computed(() => snapshot.value?.boards ?? [])
const meta = computed(() => snapshot.value?.meta ?? null)
const version = computed(() => meta.value?.version ?? 'dev')

function configureSimulator(options: SimulatorOptions): void {
  if (options.baseURL) {
    baseURL = options.baseURL
  }
  if (options.fetcher) {
    fetcher = options.fetcher
  }
  if (options.token !== undefined) {
    authToken.value = options.token
  }
}

function resetSimulatorState(): void {
  snapshot.value = null
  lastSeq.value = 0
  loading.value = false
  error.value = null
  events.value = []
  trace.value = []
  baseURL = defaultBaseURL
  fetcher = (...args) => globalThis.fetch(...args)
  authToken.value = ''
}

function applySnapshot(next: Snapshot): void {
  snapshot.value = next
  lastSeq.value = Math.max(lastSeq.value, next.meta.currentSeq)
}

function findBoard(boardID: string): BoardState | undefined {
  return snapshot.value?.boards.find((board) => board.id === boardID || board.name === boardID)
}

function replaceBoard(next: BoardState): void {
  if (!snapshot.value) {
    return
  }
  const boards = snapshot.value.boards.slice()
  const index = boards.findIndex((board) => board.id === next.id)
  if (index === -1) {
    boards.push(next)
  } else {
    boards[index] = next
  }
  snapshot.value = { ...snapshot.value, boards }
}

function replaceDevice(boardID: string, next: DeviceState): void {
  const board = findBoard(boardID)
  if (!board) {
    return
  }
  board.devices = { ...board.devices, [next.id]: next }
}

function appendTrace(record: TraceRecord): void {
  trace.value = [...trace.value.slice(-499), record]
}

function applyEvent(event: SimulatorEvent): boolean {
  if (!event || typeof event.seq !== 'number' || event.seq <= lastSeq.value) {
    return false
  }
  lastSeq.value = event.seq
  events.value = [...events.value.slice(-199), event]

  const board = event.boardId ? findBoard(event.boardId) : undefined
  switch (event.type) {
    case 'board.updated':
    case 'board.started':
      if (event.board) {
        replaceBoard(event.board)
      }
      break
    case 'device.updated':
      if (board && event.device) {
        board.devices = { ...board.devices, [event.device.id]: event.device }
      }
      break
    case 'pin.updated':
      if (board && event.pinState) {
        board.pins = { ...board.pins, [event.pinState.pin]: event.pinState }
      }
      break
    case 'value.updated':
      if (board && event.valueName) {
        board.values = {
          ...board.values,
          [event.valueName]: {
            name: event.valueName,
            value: event.newValue,
            updatedAt: event.timestamp,
          },
        }
      }
      break
    case 'function.called':
      if (board && event.valueName) {
        const fn = board.functions[event.valueName]
        board.functions = {
          ...board.functions,
          [event.valueName]: {
            name: event.valueName,
            returnCode: Number(event.newValue ?? fn?.returnCode ?? 0),
            lastParameters: String(event.oldValue ?? fn?.lastParameters ?? ''),
            callCount: (fn?.callCount ?? 0) + 1,
            lastCalledAt: event.timestamp,
          },
        }
      }
      break
    case 'fault.changed':
      if (board) {
        board.fault = event.fault ?? { active: false }
      }
      break
    default:
      break
  }
  return true
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await requestRaw(path, init)
  if (response.status === 204) {
    return undefined as T
  }
  return (await response.json()) as T
}

async function requestText(path: string, init: RequestInit = {}): Promise<string> {
  const response = await requestRaw(path, init)
  return response.text()
}

async function requestRaw(path: string, init: RequestInit = {}): Promise<Response> {
  const headers = new Headers(init.headers)
  headers.set('Accept', 'application/json')
  if (init.body) {
    headers.set('Content-Type', 'application/json')
  }
  if (authToken.value) {
    headers.set('Authorization', `Bearer ${authToken.value}`)
  }
  const response = await fetcher(`${baseURL}${path}`, { ...init, headers })
  if (!response.ok) {
    const payload = await response.json().catch(() => null)
    const message =
      payload?.error?.message ?? `request failed with status ${response.status}`
    throw new Error(message)
  }
  return response
}

function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

async function fetchSnapshot(): Promise<Snapshot> {
  loading.value = true
  error.value = null
  try {
    const next = await request<Snapshot>('/snapshot')
    applySnapshot(next)
    return next
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    throw err
  } finally {
    loading.value = false
  }
}

async function fetchBoard(boardID: string): Promise<BoardState> {
  const board = await request<BoardState>(`/boards/${encodeURIComponent(boardID)}`)
  replaceBoard(board)
  return board
}

async function patchDevice(
  boardID: string,
  deviceID: string,
  patch: DevicePatch,
): Promise<DeviceState> {
  const board = findBoard(boardID)
  const previous = board?.devices[deviceID] ? clone(board.devices[deviceID]) : undefined
  if (board && previous) {
    const optimistic = { ...previous }
    if (patch.active !== undefined) {
      optimistic.active = patch.active
    }
    if (patch.value !== undefined) {
      optimistic.state = { ...optimistic.state, value: patch.value }
    }
    board.devices = { ...board.devices, [deviceID]: optimistic }
  }

  try {
    const updated = await request<DeviceState>(
      `/boards/${encodeURIComponent(boardID)}/devices/${encodeURIComponent(deviceID)}`,
      { method: 'PATCH', body: JSON.stringify(patch) },
    )
    replaceDevice(boardID, updated)
    return updated
  } catch (err) {
    if (board && previous) {
      board.devices = { ...board.devices, [deviceID]: previous }
    }
    error.value = err instanceof Error ? err.message : String(err)
    throw err
  }
}

async function commandDevice(
  boardID: string,
  deviceID: string,
  action: string,
  value?: number,
): Promise<DeviceState> {
  const updated = await request<DeviceState>(
    `/boards/${encodeURIComponent(boardID)}/devices/${encodeURIComponent(deviceID)}/command`,
    { method: 'POST', body: JSON.stringify(value === undefined ? { action } : { action, value }) },
  )
  replaceDevice(boardID, updated)
  return updated
}

async function pressDevice(
  boardID: string,
  deviceID: string,
  holdMs?: number,
): Promise<DeviceState> {
  const updated = await request<DeviceState>(
    `/boards/${encodeURIComponent(boardID)}/devices/${encodeURIComponent(deviceID)}/press`,
    { method: 'POST', body: JSON.stringify(holdMs ? { holdMs } : {}) },
  )
  replaceDevice(boardID, updated)
  return updated
}

async function releaseDevice(boardID: string, deviceID: string): Promise<DeviceState> {
  const updated = await request<DeviceState>(
    `/boards/${encodeURIComponent(boardID)}/devices/${encodeURIComponent(deviceID)}/release`,
    { method: 'POST', body: '{}' },
  )
  replaceDevice(boardID, updated)
  return updated
}

async function setDeviceFault(
  boardID: string,
  deviceID: string,
  fault: string,
): Promise<DeviceState> {
  const updated = await request<DeviceState>(
    `/boards/${encodeURIComponent(boardID)}/devices/${encodeURIComponent(deviceID)}/fault`,
    { method: 'POST', body: JSON.stringify({ fault }) },
  )
  replaceDevice(boardID, updated)
  return updated
}

async function clearDeviceFaults(boardID: string, deviceID: string): Promise<DeviceState> {
  const updated = await request<DeviceState>(
    `/boards/${encodeURIComponent(boardID)}/devices/${encodeURIComponent(deviceID)}/fault`,
    { method: 'DELETE' },
  )
  replaceDevice(boardID, updated)
  return updated
}

async function setPin(boardID: string, pin: string, patch: PinPatch): Promise<PinState> {
  const updated = await request<PinState>(
    `/boards/${encodeURIComponent(boardID)}/pins/${encodeURIComponent(pin)}`,
    { method: 'POST', body: JSON.stringify(patch) },
  )
  const board = findBoard(boardID)
  if (board) {
    board.pins = { ...board.pins, [pin]: updated }
  }
  return updated
}

async function setValue(boardID: string, name: string, value: unknown) {
  return request(`/boards/${encodeURIComponent(boardID)}/values/${encodeURIComponent(name)}`, {
    method: 'PUT',
    body: JSON.stringify({ value }),
  })
}

async function callFunction(
  boardID: string,
  name: string,
  parameters: string,
  returnCode?: number,
): Promise<FunctionResult> {
  return request<FunctionResult>(
    `/boards/${encodeURIComponent(boardID)}/functions/${encodeURIComponent(name)}`,
    {
      method: 'POST',
      body: JSON.stringify({
        parameters,
        ...(returnCode === undefined ? {} : { returnCode }),
        execute: true,
      }),
    },
  )
}

async function setFault(boardID: string, message: string) {
  return request(`/boards/${encodeURIComponent(boardID)}/fault`, {
    method: 'POST',
    body: JSON.stringify({ message }),
  })
}

async function clearFault(boardID: string) {
  return request(`/boards/${encodeURIComponent(boardID)}/fault`, { method: 'DELETE' })
}

async function fetchTrace(
  boardID: string,
  options: { since?: number; kinds?: string[]; target?: string } = {},
): Promise<TraceRecord[]> {
  const params = new URLSearchParams()
  if (options.since) {
    params.set('since', String(options.since))
  }
  if (options.kinds?.length) {
    params.set('kinds', options.kinds.join(','))
  }
  if (options.target) {
    params.set('target', options.target)
  }
  const query = params.toString()
  const response = await request<{ records: TraceRecord[] }>(
    `/boards/${encodeURIComponent(boardID)}/trace${query ? `?${query}` : ''}`,
  )
  for (const record of response.records) {
    if (!trace.value.some((entry) => entry.seq === record.seq)) {
      appendTrace(record)
    }
  }
  return response.records
}

async function fetchWaveform(
  boardID: string,
  options: { pins?: string[]; from?: string; to?: string; maxPoints?: number } = {},
): Promise<WaveformSeries[]> {
  const params = new URLSearchParams()
  if (options.pins?.length) {
    params.set('pins', options.pins.join(','))
  }
  if (options.from) {
    params.set('from', options.from)
  }
  if (options.to) {
    params.set('to', options.to)
  }
  if (options.maxPoints) {
    params.set('maxPoints', String(options.maxPoints))
  }
  const query = params.toString()
  const response = await request<{ series: WaveformSeries[] }>(
    `/boards/${encodeURIComponent(boardID)}/waveform${query ? `?${query}` : ''}`,
  )
  return response.series
}

async function postStep(
  boardID: string,
  name: string,
  status: string,
  detail: Record<string, unknown> = {},
) {
  return request(`/boards/${encodeURIComponent(boardID)}/steps`, {
    method: 'POST',
    body: JSON.stringify({ name, status, detail }),
  })
}

async function fetchTopology(): Promise<TopologySpec> {
  return request<TopologySpec>('/topology')
}

async function fetchTopologySource(): Promise<string> {
  return requestText('/topology/source')
}

async function fetchScenarios(): Promise<ScenarioRule[]> {
  const response = await request<{ scenarios: ScenarioRule[] }>('/scenarios')
  return response.scenarios
}

async function setScenarioEnabled(id: string, enabled: boolean): Promise<ScenarioRule> {
  return request<ScenarioRule>(`/scenarios/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    body: JSON.stringify({ enabled }),
  })
}

async function triggerScenario(id: string): Promise<void> {
  await request(`/scenarios/${encodeURIComponent(id)}/trigger`, { method: 'POST' })
}

async function fetchBuses(boardID: string): Promise<Array<Record<string, unknown>>> {
  const response = await request<{ buses: Array<Record<string, unknown>> }>(
    `/boards/${encodeURIComponent(boardID)}/buses`,
  )
  return response.buses
}

async function fetchChips(boardID: string): Promise<ChipEntry[]> {
  const response = await request<{ chips: ChipEntry[] }>(
    `/boards/${encodeURIComponent(boardID)}/chips`,
  )
  return response.chips
}

async function fetchChip(chipID: string): Promise<DisplayChipValues> {
  return request<DisplayChipValues>(`/chips/${encodeURIComponent(chipID)}`)
}

async function patchChip(
  chipID: string,
  field: string,
  value: unknown,
): Promise<DisplayChipValues> {
  return request<DisplayChipValues>(`/chips/${encodeURIComponent(chipID)}`, {
    method: 'PATCH',
    body: JSON.stringify({ field, value }),
  })
}

async function rawChip(
  chipID: string,
  write: number[],
  readLen: number,
): Promise<{ data?: number[]; values?: Record<string, unknown> }> {
  return request(`/chips/${encodeURIComponent(chipID)}/raw`, {
    method: 'POST',
    body: JSON.stringify({ write, readLen }),
  })
}

export function useSimulator(options: SimulatorOptions = {}) {
  configureSimulator(options)

  return {
    snapshot,
    boards,
    meta,
    version,
    loading,
    error,
    events,
    trace,
    lastSeq,
    authToken,
    fetchSnapshot,
    fetchBoard,
    applySnapshot,
    applyEvent,
    appendTrace,
    patchDevice,
    commandDevice,
    pressDevice,
    releaseDevice,
    setDeviceFault,
    clearDeviceFaults,
    setPin,
    setValue,
    callFunction,
    setFault,
    clearFault,
    fetchTrace,
    fetchWaveform,
    postStep,
    fetchTopology,
    fetchTopologySource,
    fetchScenarios,
    setScenarioEnabled,
    triggerScenario,
    fetchBuses,
    fetchChips,
    fetchChip,
    patchChip,
    rawChip,
  }
}

export { configureSimulator, resetSimulatorState }
