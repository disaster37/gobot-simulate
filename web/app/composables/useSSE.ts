import { ref } from 'vue'
import type { SimulatorEvent, Snapshot } from '~/types/simulator'

export type SSEStatus = 'connecting' | 'connected' | 'reconnecting' | 'offline'

export interface UseSSEOptions {
  url?: string
  token?: string
  onEvent: (event: SimulatorEvent) => void
  onSnapshot?: (snapshot: Snapshot) => void
  eventSourceFactory?: (url: string) => EventSource
}

const eventTypes = [
  'snapshot',
  'board.updated',
  'device.updated',
  'pin.updated',
  'value.updated',
  'fault.changed',
  'heartbeat',
]

export function useSSE(options: UseSSEOptions) {
  const status = ref<SSEStatus>('offline')
  const lastEventId = ref(0)
  const baseURL = options.url ?? '/api/v1/events'
  let source: EventSource | null = null
  let attempts = 0
  let timer: ReturnType<typeof setTimeout> | null = null
  let stopped = true

  function buildURL(): string {
    const params = new URLSearchParams()
    if (lastEventId.value > 0) {
      params.set('lastEventId', String(lastEventId.value))
    }
    if (options.token) {
      params.set('token', options.token)
    }
    const query = params.toString()
    return query ? `${baseURL}?${query}` : baseURL
  }

  function scheduleReconnect(): void {
    if (stopped) {
      return
    }
    const delay = Math.min(30_000, 1_000 * 2 ** attempts)
    attempts += 1
    status.value = attempts > 5 ? 'offline' : 'reconnecting'
    timer = setTimeout(connect, delay)
  }

  function handleFrame(eventType: string, raw: string): void {
    if (eventType === 'heartbeat') {
      status.value = 'connected'
      return
    }
    if (eventType === 'snapshot') {
      const snapshot = JSON.parse(raw) as Snapshot
      lastEventId.value = Math.max(lastEventId.value, snapshot.meta.currentSeq)
      options.onSnapshot?.(snapshot)
      return
    }
    const event = JSON.parse(raw) as SimulatorEvent
    if (event.seq <= lastEventId.value) {
      return
    }
    lastEventId.value = event.seq
    options.onEvent(event)
  }

  function connect(): void {
    if (stopped) {
      return
    }
    if (source) {
      source.close()
      source = null
    }
    status.value = attempts > 0 ? 'reconnecting' : 'connecting'
    source = options.eventSourceFactory
      ? options.eventSourceFactory(buildURL())
      : new EventSource(buildURL())

    source.onopen = () => {
      attempts = 0
      status.value = 'connected'
    }
    source.onerror = () => {
      source?.close()
      source = null
      scheduleReconnect()
    }
    for (const eventType of eventTypes) {
      source.addEventListener(eventType, (message) => {
        try {
          handleFrame(eventType, (message as MessageEvent<string>).data)
        } catch {
          // Ignore malformed frames; the stream remains usable.
        }
      })
    }
  }

  function start(): void {
    stopped = false
    attempts = 0
    connect()
  }

  function reconnect(): void {
    if (timer) {
      clearTimeout(timer)
      timer = null
    }
    attempts = 0
    stopped = false
    connect()
  }

  function stop(): void {
    stopped = true
    if (timer) {
      clearTimeout(timer)
      timer = null
    }
    source?.close()
    source = null
    status.value = 'offline'
  }

  return { status, lastEventId, start, stop, reconnect }
}
