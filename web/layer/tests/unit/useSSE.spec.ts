import { describe, expect, it, vi } from 'vitest'
import type { SSEFrame } from '~/composables/useSSE'
import { createSSEParser, parseSSEFrame, useSSE } from '~/composables/useSSE'
import type { SimulatorEvent, Snapshot } from '~/types/simulator'

function makeSnapshot(currentSeq: number): Snapshot {
  return {
    meta: {
      version: '0.1.0',
      startedAt: '2026-11-04T10:00:00Z',
      uptimeSeconds: 1,
      currentSeq,
      connectedClients: 1,
      droppedEvents: 0,
    },
    boards: [],
  }
}

function makeEvent(seq: number): SimulatorEvent {
  return {
    seq,
    timestamp: '2026-11-04T10:00:04Z',
    type: 'pin.updated',
    source: 'ui',
    boardId: 'dfp',
    pin: '3',
  }
}

function sseResponse(chunks: string[]): Response {
  const encoder = new TextEncoder()
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const chunk of chunks) {
        controller.enqueue(encoder.encode(chunk))
      }
      controller.close()
    },
  })
  return new Response(body, { status: 200, headers: { 'Content-Type': 'text/event-stream' } })
}

function openSSEResponse(signal: AbortSignal): Response {
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      signal.addEventListener('abort', () => {
        try {
          controller.error(new DOMException('Aborted', 'AbortError'))
        } catch {
          // The stream may already be closed.
        }
      })
    },
  })
  return new Response(body, { status: 200, headers: { 'Content-Type': 'text/event-stream' } })
}

describe('SSE frame parsing', () => {
  it('parses id, event, and data fields while ignoring retry and comments', () => {
    const frame = parseSSEFrame(': keep-alive\nretry: 2000\nid: 12\nevent: pin.updated\ndata: {"seq":12}')
    expect(frame).toEqual({ id: '12', event: 'pin.updated', data: '{"seq":12}' })

    expect(parseSSEFrame('retry: 2000')).toBeNull()
    expect(parseSSEFrame(': comment')).toBeNull()
  })

  it('joins multi-line data fields', () => {
    const frame = parseSSEFrame('event: message\ndata: first\ndata: second')
    expect(frame?.data).toBe('first\nsecond')
  })

  it('reassembles frames split across chunks', () => {
    const frames: SSEFrame[] = []
    const parser = createSSEParser((frame) => frames.push(frame))

    parser.push('id: 7\nevent: pin.')
    parser.push('updated\ndata: {"seq":7}')
    parser.push('\n\n')

    expect(frames).toEqual([{ id: '7', event: 'pin.updated', data: '{"seq":7}' }])
  })

  it('emits multiple frames delivered in one chunk', () => {
    const frames: SSEFrame[] = []
    const parser = createSSEParser((frame) => frames.push(frame))

    parser.push('id: 1\nevent: a\ndata: {}\n\nid: 2\nevent: b\ndata: {}\n\n')

    expect(frames).toEqual([
      { id: '1', event: 'a', data: '{}' },
      { id: '2', event: 'b', data: '{}' },
    ])
  })

  it('handles CRLF frames across chunk boundaries', () => {
    const frames: SSEFrame[] = []
    const parser = createSSEParser((frame) => frames.push(frame))

    parser.push('id: 3\r')
    parser.push('\nevent: x\r\n')
    parser.push('data: hello\r\n\r\n')

    expect(frames).toEqual([{ id: '3', event: 'x', data: 'hello' }])
  })
})

describe('useSSE', () => {
  it('delivers a snapshot and deduplicates events by seq', async () => {
    const snapshot = makeSnapshot(5)
    const onSnapshot = vi.fn()
    const onEvent = vi.fn()
    const fetchImpl = vi.fn(async () =>
      sseResponse([
        'retry: 2000\n\n',
        `id: 5\nevent: snapshot\ndata: ${JSON.stringify(snapshot)}\n\n`,
        `id: 5\nevent: pin.updated\ndata: ${JSON.stringify(makeEvent(5))}\n\n`,
        `id: 6\nevent: pin.updated\ndata: ${JSON.stringify(makeEvent(6))}\n\n`,
      ]),
    )

    const sse = useSSE({ onEvent, onSnapshot, fetchImpl })
    sse.start()

    await vi.waitFor(() => expect(onEvent).toHaveBeenCalledTimes(1))
    expect(onSnapshot).toHaveBeenCalledWith(snapshot)
    expect(onEvent).toHaveBeenCalledWith(makeEvent(6))
    expect(sse.lastEventId.value).toBe(6)
    sse.stop()
  })

  it('sends Accept, Last-Event-ID, and a reactive Authorization header', async () => {
    let token = 'first'
    const requests: Array<Record<string, string>> = []
    const fetchImpl = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      requests.push(init?.headers as Record<string, string>)
      return sseResponse([`id: 4\nevent: pin.updated\ndata: ${JSON.stringify(makeEvent(4))}\n\n`])
    })

    const onEvent = vi.fn()
    const sse = useSSE({
      getToken: () => token,
      onEvent,
      fetchImpl,
    })
    sse.start()
    await vi.waitFor(() => expect(requests).toHaveLength(1))

    expect(requests[0]).toMatchObject({
      Accept: 'text/event-stream',
      Authorization: 'Bearer first',
    })

    await vi.waitFor(() => expect(onEvent).toHaveBeenCalled())
    token = 'second'
    sse.reconnect()
    await vi.waitFor(() => expect(requests).toHaveLength(2))

    expect(requests[1]).toMatchObject({
      Accept: 'text/event-stream',
      Authorization: 'Bearer second',
      'Last-Event-ID': '4',
    })
    sse.stop()
  })

  it('falls back to a static token when no getter is provided', async () => {
    const requests: Array<Record<string, string>> = []
    const fetchImpl = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      requests.push(init?.headers as Record<string, string>)
      return sseResponse([])
    })

    const sse = useSSE({ token: 'legacy', onEvent: vi.fn(), fetchImpl })
    sse.start()
    await vi.waitFor(() => expect(requests).toHaveLength(1))
    expect(requests[0].Authorization).toBe('Bearer legacy')
    sse.stop()
  })

  it('aborts the previous request on reconnect and on stop', async () => {
    const signals: AbortSignal[] = []
    const fetchImpl = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      const signal = init?.signal as AbortSignal
      signals.push(signal)
      return openSSEResponse(signal)
    })

    const sse = useSSE({ onEvent: vi.fn(), fetchImpl })
    sse.start()
    await vi.waitFor(() => expect(signals).toHaveLength(1))

    sse.reconnect()
    await vi.waitFor(() => expect(signals).toHaveLength(2))
    expect(signals[0].aborted).toBe(true)

    sse.stop()
    expect(signals[1].aborted).toBe(true)
    expect(sse.status.value).toBe('offline')
  })

  it('reconnects with backoff after a failed request', async () => {
    const fetchImpl = vi.fn(async () => {
      throw new Error('network down')
    })

    const sse = useSSE({ onEvent: vi.fn(), fetchImpl })
    sse.start()
    await vi.waitFor(() => expect(fetchImpl).toHaveBeenCalled())

    expect(sse.status.value).toBe('reconnecting')
    sse.stop()
  })
})

describe('useSSE trace events', () => {
  it('dispatches trace frames separately from state events', async () => {
    const onEvent = vi.fn()
    const onTrace = vi.fn()
    const fetchImpl = vi.fn(async () =>
      sseResponse([
        'id: 1\nevent: trace\ndata: {"seq":9,"kind":"pin-write","target":"3","value":1}\n\n',
        `id: 2\nevent: pin.updated\ndata: ${JSON.stringify(makeEvent(2))}\n\n`,
      ]),
    )
    const sse = useSSE({ onEvent, onTrace, fetchImpl })
    sse.start()
    await vi.waitFor(() => expect(onTrace).toHaveBeenCalledTimes(1))
    expect(onTrace).toHaveBeenCalledWith(
      expect.objectContaining({ seq: 9, kind: 'pin-write', target: '3' }),
    )
    await vi.waitFor(() => expect(onEvent).toHaveBeenCalledTimes(1))
    expect(sse.lastEventId.value).toBe(2)
    sse.stop()
  })
})
