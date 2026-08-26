import { TextEncoder } from 'node:util'
import { HttpResponse } from 'msw'
import type { DefaultBodyType } from 'msw'
import type { StreamEvent } from '@shared/streaming/ndjsonStream'

// The stream event shape lives in @shared/streaming so tests can never drift
// from the production parser; only fixtures and stream builders live here.
export type { StreamEvent }

function progressEvent(overrides: Partial<StreamEvent> = {}): StreamEvent {
  return {
    operationId: 'op-1',
    type: 'progress',
    state: { current: 0, total: 100, status: 'pulling' },
    ...overrides,
  }
}

function completeEvent(overrides: Partial<StreamEvent> = {}): StreamEvent {
  return { operationId: 'op-1', type: 'complete', ...overrides }
}

function errorEvent(code: string, message: string): StreamEvent {
  return { operationId: 'op-1', type: 'error', error: { code, message } }
}

// Builds a ReadableStream that emits the given strings as separate reads,
// so tests can exercise chunk splitting (events split across reads).
export function chunkedStream(chunks: string[]): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder()
  return new ReadableStream<Uint8Array>({
    start(controller) {
      for (const chunk of chunks) controller.enqueue(encoder.encode(chunk))
      controller.close()
    },
  })
}

function eventStream(events: StreamEvent[]): ReadableStream<Uint8Array> {
  return chunkedStream(events.map((event) => `${JSON.stringify(event)}\n`))
}

// Emits one chunk per macrotask so the UI renders each progress update live,
// as it would over a real network. Accepts raw strings so malformed lines and
// split events can be interleaved.
export function liveStream(chunks: string[], delayMs = 100): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder()
  return new ReadableStream<Uint8Array>({
    async pull(controller) {
      for (const chunk of chunks) {
        await new Promise((resolve) => setTimeout(resolve, delayMs))
        controller.enqueue(encoder.encode(chunk))
      }
      controller.close()
    },
  })
}

// Emits the chunks, then stays open until `signal` aborts, at which point the
// stream errors — how a real fetch body behaves when its request is aborted
// mid-stream. The consumer's reader is released and the stream torn down.
export function abortableStream(chunks: string[], signal: AbortSignal): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder()
  return new ReadableStream<Uint8Array>({
    start(controller) {
      for (const chunk of chunks) controller.enqueue(encoder.encode(chunk))
      signal.addEventListener('abort', () => controller.error(new DOMException('Aborted', 'AbortError')))
    },
  })
}

export function streamResponse(stream: ReadableStream<Uint8Array>): HttpResponse<DefaultBodyType> {
  return new HttpResponse(stream, { status: 200, headers: { 'Content-Type': 'application/json' } })
}

// MSW response streaming NDJSON events. Pass raw `chunks` to override the
// one-line-per-event framing (malformed lines, events split across reads).
export function ndjsonResponse(events: StreamEvent[], chunks?: string[]): HttpResponse<DefaultBodyType> {
  return streamResponse(chunks ? chunkedStream(chunks) : eventStream(events))
}

export function liveNdjsonResponse(chunks: string[], delayMs?: number): HttpResponse<DefaultBodyType> {
  return streamResponse(liveStream(chunks, delayMs))
}

export function abortableNdjsonResponse(
  events: StreamEvent[],
  signal: AbortSignal,
): HttpResponse<DefaultBodyType> {
  return streamResponse(abortableStream(events.map((event) => `${JSON.stringify(event)}\n`), signal))
}

export const streamFixtures = { progressEvent, completeEvent, errorEvent }
