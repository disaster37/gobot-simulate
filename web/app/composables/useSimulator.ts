import { computed, ref } from 'vue'
import type {
  BoardState,
  DevicePatch,
  DeviceState,
  FunctionResult,
  PinPatch,
  PinState,
  SimulatorEvent,
  Snapshot,
} from '~/types/simulator'

export interface SimulatorOptions {
  baseURL?: string
  fetcher?: typeof fetch
  token?: string
}

const snapshot = ref<Snapshot | null>(null)
const lastSeq = ref(0)
const loading = ref(false)
const error = ref<string | null>(null)
const events = ref<SimulatorEvent[]>([])
const authToken = ref('')

let baseURL = '/api/v1'
let fetcher: typeof fetch = (...args) => globalThis.fetch(...args)

const boards = computed(() => snapshot.value?.boards ?? [])
const meta = computed(() => snapshot.value?.meta ?? null)

function resetSimulatorState(): void {
  snapshot.value = null
  lastSeq.value = 0
  loading.value = false
  error.value = null
  events.value = []
  baseURL = '/api/v1'
  fetcher = (...args) => globalThis.fetch(...args)
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

function applyEvent(event: SimulatorEvent): boolean {
  if (!event || typeof event.seq !== 'number' || event.seq <= lastSeq.value) {
    return false
  }
  lastSeq.value = event.seq
  events.value = [...events.value.slice(-199), event]

  const board = event.boardId ? findBoard(event.boardId) : undefined
  switch (event.type) {
    case 'board.updated':
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
  if (response.status === 204) {
    return undefined as T
  }
  return (await response.json()) as T
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

export function useSimulator(options: SimulatorOptions = {}) {
  if (options.baseURL) {
    baseURL = options.baseURL
  }
  if (options.fetcher) {
    fetcher = options.fetcher
  }
  if (options.token !== undefined) {
    authToken.value = options.token
  }

  return {
    snapshot,
    boards,
    meta,
    loading,
    error,
    events,
    lastSeq,
    authToken,
    fetchSnapshot,
    fetchBoard,
    applySnapshot,
    applyEvent,
    patchDevice,
    pressDevice,
    releaseDevice,
    setPin,
    setValue,
    callFunction,
    setFault,
    clearFault,
  }
}

export { resetSimulatorState }
