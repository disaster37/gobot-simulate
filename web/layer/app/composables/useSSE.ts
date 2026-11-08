import { ref } from 'vue'
import type { SimulatorEvent, Snapshot } from '~/types/simulator'

export type SSEStatus = 'connecting' | 'connected' | 'reconnecting' | 'offline'

export interface SSEFrame {
  id?: string
  event?: string
  data?: string
}

export interface SSEParser {
  push: (chunk: string) => void
  reset: () => void
}

export interface UseSSEOptions {
  url?: string
  token?: string
  getToken?: () => string
  onEvent: (event: SimulatorEvent) => void
  onSnapshot?: (snapshot: Snapshot) => void
  fetchImpl?: typeof fetch
}

const maxReconnectDelay = 30_000
const offlineAfterAttempts = 5

function normalizeNewlines(value: string): string {
  return value.replace(/\r\n?/g, '\n')
}

/**
 * parseSSEFrame parses one SSE frame (the text between blank lines).
 * Unknown fields such as `retry` and comment lines are ignored.
 */
export function parseSSEFrame(raw: string): SSEFrame | null {
  const frame: SSEFrame = {}
  const data: string[] = []

  for (const line of normalizeNewlines(raw).split('\n')) {
    if (line === '' || line.startsWith(':')) {
      continue
    }
    const separator = line.indexOf(':')
    const field = separator === -1 ? line : line.slice(0, separator)
    let value = separator === -1 ? '' : line.slice(separator + 1)
    if (value.startsWith(' ')) {
      value = value.slice(1)
    }
    switch (field) {
      case 'id':
        frame.id = value
        break
      case 'event':
        frame.event = value
        break
      case 'data':
        data.push(value)
        break
      default:
        break
    }
  }

  if (data.length > 0) {
    frame.data = data.join('\n')
  }
  if (frame.id === undefined && frame.event === undefined && frame.data === undefined) {
    return null
  }
  return frame
}

/**
 * createSSEParser buffers arbitrary text chunks and emits complete frames.
 * Frames may span chunks, and one chunk may contain several frames.
 */
export function createSSEParser(onFrame: (frame: SSEFrame) => void): SSEParser {
  let buffer = ''

  function drain(): void {
    let boundary = buffer.indexOf('\n\n')
    while (boundary !== -1) {
      const raw = buffer.slice(0, boundary)
      buffer = buffer.slice(boundary + 2)
      const frame = parseSSEFrame(raw)
      if (frame) {
        onFrame(frame)
      }
      boundary = buffer.indexOf('\n\n')
    }
  }

  return {
    push(chunk: string): void {
      let combined = buffer + chunk
      // A chunk may end in the middle of a CRLF pair; keep the trailing CR so
      // it is not mistaken for a complete blank line.
      const trailingCR = combined.endsWith('\r')
      if (trailingCR) {
        combined = combined.slice(0, -1)
      }
      buffer = normalizeNewlines(combined)
      if (trailingCR) {
        buffer += '\r'
      }
      drain()
    },
    reset(): void {
      buffer = ''
    },
  }
}

export function useSSE(options: UseSSEOptions) {
  const status = ref<SSEStatus>('offline')
  const lastEventId = ref(0)
  const baseURL = options.url ?? '/api/v1/events'
  const fetcher: typeof fetch = options.fetchImpl ?? ((input, init) => globalThis.fetch(input, init))

  let controller: AbortController | null = null
  let attempts = 0
  let timer: ReturnType<typeof setTimeout> | null = null
  let stopped = true
  let generation = 0

  function currentToken(): string {
    if (options.getToken) {
      return options.getToken()
    }
    return options.token ?? ''
  }

  function buildHeaders(): Record<string, string> {
    const headers: Record<string, string> = { Accept: 'text/event-stream' }
    if (lastEventId.value > 0) {
      headers['Last-Event-ID'] = String(lastEventId.value)
    }
    const token = currentToken()
    if (token) {
      headers.Authorization = `Bearer ${token}`
    }
    return headers
  }

  function handleFrame(frame: SSEFrame): void {
    const eventType = frame.event ?? ''
    const raw = frame.data ?? ''

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
    if (!raw) {
      return
    }
    const event = JSON.parse(raw) as SimulatorEvent
    if (event.seq <= lastEventId.value) {
      return
    }
    lastEventId.value = event.seq
    options.onEvent(event)
  }

  function scheduleReconnect(): void {
    if (stopped) {
      return
    }
    const delay = Math.min(maxReconnectDelay, 1_000 * 2 ** attempts)
    attempts += 1
    status.value = attempts > offlineAfterAttempts ? 'offline' : 'reconnecting'
    if (timer) {
      clearTimeout(timer)
    }
    timer = setTimeout(() => {
      timer = null
      void connect()
    }, delay)
  }

  async function connect(): Promise<void> {
    if (stopped) {
      return
    }
    controller?.abort()
    const run = ++generation
    const active = new AbortController()
    controller = active
    status.value = attempts > 0 ? 'reconnecting' : 'connecting'

    try {
      const response = await fetcher(baseURL, {
        method: 'GET',
        headers: buildHeaders(),
        cache: 'no-store',
        signal: active.signal,
      })
      if (!response.ok || !response.body) {
        throw new Error(`SSE request failed with status ${response.status}`)
      }

      attempts = 0
      status.value = 'connected'

      const reader = response.body.getReader()
      const decoder = new TextDecoder()
      const parser = createSSEParser((frame) => {
        try {
          handleFrame(frame)
        } catch {
          // Ignore malformed frames; the stream remains usable.
        }
      })

      while (!stopped && run === generation) {
        const { done, value } = await reader.read()
        if (done) {
          break
        }
        parser.push(decoder.decode(value, { stream: true }))
      }

      if (!stopped && run === generation) {
        scheduleReconnect()
      }
    } catch {
      if (active.signal.aborted || stopped || run !== generation) {
        return
      }
      scheduleReconnect()
    } finally {
      if (run === generation) {
        controller = null
      }
    }
  }

  function start(): void {
    stopped = false
    attempts = 0
    void connect()
  }

  function reconnect(): void {
    if (timer) {
      clearTimeout(timer)
      timer = null
    }
    attempts = 0
    stopped = false
    void connect()
  }

  function stop(): void {
    stopped = true
    generation += 1
    if (timer) {
      clearTimeout(timer)
      timer = null
    }
    controller?.abort()
    controller = null
    status.value = 'offline'
  }

  return { status, lastEventId, start, stop, reconnect }
}
