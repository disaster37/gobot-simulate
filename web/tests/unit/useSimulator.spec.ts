import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { BoardState, DeviceState, SimulatorEvent, Snapshot } from '~/types/simulator'
import { resetSimulatorState, useSimulator } from '~/composables/useSimulator'

function makeDevice(overrides: Partial<DeviceState> = {}): DeviceState {
  return {
    id: 'button_start',
    boardId: 'dfp',
    name: 'start',
    label: 'Start',
    kind: 'button',
    driver: '*gpio.ButtonDriver',
    pins: ['11'],
    inverted: false,
    defaultState: 1,
    active: false,
    state: { pinValue: 1, defaultState: 1 },
    capabilities: { readable: true, writable: true, pressable: true, toggleable: true, callable: false },
    updatedAt: '2026-11-04T10:00:00Z',
    ...overrides,
  }
}

function makeBoard(overrides: Partial<BoardState> = {}): BoardState {
  return {
    id: 'dfp',
    name: 'fat',
    platform: 'raspberry-pi',
    connected: true,
    online: true,
    reconnectCount: 0,
    simulatedLagMs: 0,
    fault: { active: false },
    pins: {},
    devices: { button_start: makeDevice() },
    values: {},
    functions: {},
    createdAt: '2026-11-04T10:00:00Z',
    updatedAt: '2026-11-04T10:00:00Z',
    ...overrides,
  }
}

function makeSnapshot(board = makeBoard()): Snapshot {
  return {
    meta: {
      version: '0.1.0',
      startedAt: '2026-11-04T10:00:00Z',
      uptimeSeconds: 1,
      currentSeq: 0,
      connectedClients: 1,
      droppedEvents: 0,
    },
    boards: [board],
  }
}

describe('useSimulator', () => {
  beforeEach(() => {
    resetSimulatorState()
  })

  it('applies a snapshot to the reactive board list', () => {
    const sim = useSimulator()
    sim.applySnapshot(makeSnapshot())

    expect(sim.boards.value).toHaveLength(1)
    expect(sim.boards.value[0].id).toBe('dfp')
    expect(sim.meta.value?.version).toBe('0.1.0')
  })

  it('deduplicates events by sequence and updates nested state', () => {
    const sim = useSimulator()
    sim.applySnapshot(makeSnapshot())

    const event: SimulatorEvent = {
      seq: 4,
      timestamp: '2026-11-04T10:00:04Z',
      type: 'device.updated',
      source: 'ui',
      boardId: 'dfp',
      deviceId: 'button_start',
      device: makeDevice({ active: true, state: { pinValue: 0 } }),
    }

    expect(sim.applyEvent(event)).toBe(true)
    expect(sim.applyEvent(event)).toBe(false)
    expect(sim.boards.value[0].devices.button_start.active).toBe(true)
    expect(sim.lastSeq.value).toBe(4)
    expect(sim.events.value).toHaveLength(1)

    const next: SimulatorEvent = {
      seq: 5,
      timestamp: '2026-11-04T10:00:05Z',
      type: 'value.updated',
      source: 'ui',
      boardId: 'dfp',
      valueName: 'distance',
      newValue: 42.5,
    }
    expect(sim.applyEvent(next)).toBe(true)
    expect(sim.boards.value[0].values.distance.value).toBe(42.5)
  })

  it('rolls back optimistic device changes when the request fails', async () => {
    let rejectRequest: (reason?: unknown) => void = () => {}
    const fetcher = vi.fn(
      () =>
        new Promise<Response>((_, reject) => {
          rejectRequest = reject
        }),
    ) as unknown as typeof fetch
    const sim = useSimulator({ fetcher })
    sim.applySnapshot(makeSnapshot())

    const pending = sim.patchDevice('dfp', 'button_start', { active: true })
    await Promise.resolve()
    expect(sim.boards.value[0].devices.button_start.active).toBe(true)

    rejectRequest(new Error('boom'))
    await expect(pending).rejects.toThrow('boom')
    expect(sim.boards.value[0].devices.button_start.active).toBe(false)
    expect(sim.error.value).toBe('boom')
  })

  it('replaces a device with the server response on success', async () => {
    const updated = makeDevice({ active: true, state: { pinValue: 0 } })
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(updated), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    ) as unknown as typeof fetch
    const sim = useSimulator({ fetcher })
    sim.applySnapshot(makeSnapshot())

    const result = await sim.patchDevice('dfp', 'button_start', { active: true })

    expect(result.active).toBe(true)
    expect(sim.boards.value[0].devices.button_start.active).toBe(true)
    expect(fetcher).toHaveBeenCalledOnce()
  })
})
