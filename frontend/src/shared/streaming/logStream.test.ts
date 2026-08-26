import { http, HttpResponse } from 'msw'
import { describe, expect, it, vi } from 'vitest'
import {
  followLogStream,
  isLogEvent,
  parseLogErrorEvent,
} from '@shared/streaming/logStream'
import { consumeNdjsonValues } from '@shared/streaming/ndjsonStream'
import { chunkedStream, streamResponse } from '@/test/stream'
import { server } from '@/test/server'

function logEvent(overrides: Partial<{ timestamp: string; level: string; message: string }> = {}) {
  return JSON.stringify({
    timestamp: '2026-08-26T10:00:00Z',
    level: 'INFO',
    message: 'server started',
    ...overrides,
  })
}

describe('consumeNdjsonValues', () => {
  it('parses each NDJSON line into a JSON value', async () => {
    const values: unknown[] = []
    await consumeNdjsonValues(chunkedStream([`${logEvent()}\n`, `${logEvent({ message: 'ready' })}\n`]), (v) =>
      values.push(v),
    )

    expect(values).toHaveLength(2)
    expect(values[0]).toEqual({ timestamp: '2026-08-26T10:00:00Z', level: 'INFO', message: 'server started' })
  })

  it('reassembles values split across reads', async () => {
    const values: unknown[] = []
    const event = `${logEvent()}\n`
    await consumeNdjsonValues(
      chunkedStream([event.slice(0, 20), event.slice(20, 40), event.slice(40)]),
      (v) => values.push(v),
    )

    expect(values).toHaveLength(1)
    expect((values[0] as { message: string }).message).toBe('server started')
  })

  it('skips malformed lines and keeps consuming', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const values: unknown[] = []
    await consumeNdjsonValues(
      chunkedStream(['not json\n', `${logEvent()}\n`]),
      (v) => values.push(v),
    )

    expect(values).toHaveLength(1)
    expect(warn).toHaveBeenCalledTimes(1)
    warn.mockRestore()
  })
})

describe('isLogEvent', () => {
  it('accepts the backend log event shape', () => {
    expect(
      isLogEvent({ timestamp: '2026-08-26T10:00:00Z', level: 'INFO', message: 'hi' }),
    ).toBe(true)
  })

  it('rejects other shapes', () => {
    expect(isLogEvent(null)).toBe(false)
    expect(isLogEvent({ operationId: 'op', type: 'progress' })).toBe(false)
    expect(isLogEvent({ timestamp: 't', level: 'INFO' })).toBe(false)
  })
})

describe('parseLogErrorEvent', () => {
  it('splits an ERROR-level event into code and message', () => {
    expect(
      parseLogErrorEvent({
        timestamp: 't',
        level: 'ERROR',
        message: 'CONTAINER_NOT_FOUND: no such container',
      }),
    ).toEqual({ code: 'CONTAINER_NOT_FOUND', message: 'no such container' })
  })

  it('returns null for non-error events', () => {
    expect(parseLogErrorEvent({ timestamp: 't', level: 'INFO', message: 'fine' })).toBeNull()
    expect(parseLogErrorEvent({ timestamp: 't', level: 'ERROR', message: 'no code prefix' })).toBeNull()
  })

  it('returns null for stderr lines that merely look like coded errors', () => {
    expect(
      parseLogErrorEvent({ timestamp: 't', level: 'ERROR', message: 'FATAL: could not bind port' }),
    ).toBeNull()
    expect(
      parseLogErrorEvent({ timestamp: 't', level: 'ERROR', message: 'ERROR: socket closed' }),
    ).toBeNull()
  })
})

describe('followLogStream', () => {
  it('emits each log event and skips non-log values', async () => {
    const events: { message: string }[] = []
    server.use(
      http.get('*/api/logs', () =>
        streamResponse(
          chunkedStream([
            `${logEvent()}\n`,
            `${JSON.stringify({ operationId: 'op', type: 'progress' })}\n`,
            `${logEvent({ message: 'second' })}\n`,
          ]),
        ),
      ),
    )

    await followLogStream('/api/logs', (event) => events.push({ message: event.message }), new AbortController().signal)

    expect(events.map((e) => e.message)).toEqual(['server started', 'second'])
  })

  it('throws the error envelope for a non-ok response', async () => {
    server.use(
      http.get('*/api/logs', () =>
        HttpResponse.json(
          { requestId: 'req-15', error: { message: 'no such container', code: 'CONTAINER_NOT_FOUND' } },
          { status: 404 },
        ),
      ),
    )

    await expect(
      followLogStream('/api/logs', () => undefined, new AbortController().signal),
    ).rejects.toMatchObject({ code: 'CONTAINER_NOT_FOUND', message: 'no such container' })
  })

  it('rejects when the request is aborted mid-stream', async () => {
    const controller = new AbortController()
    server.use(
      http.get('*/api/logs', () =>
        streamResponse(
          new ReadableStream<Uint8Array>({
            start(streamController) {
              streamController.enqueue(new TextEncoder().encode(`${logEvent()}\n`))
              controller.signal.addEventListener('abort', () =>
                streamController.error(new DOMException('Aborted', 'AbortError')),
              )
            },
          }),
        ),
      ),
    )

    const promise = followLogStream('/api/logs', () => undefined, controller.signal)
    controller.abort()

    await expect(promise).rejects.toThrow()
  })
})
