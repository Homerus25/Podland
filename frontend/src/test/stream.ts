import { TextEncoder } from 'node:util'
import { HttpResponse } from 'msw'
import type { DefaultBodyType } from 'msw'

export interface StreamEvent {
  operationId: string
  type: 'progress' | 'complete' | 'error'
  machineId?: string
  state?: { current?: number; total?: number; status: string; message?: string }
  error?: { code: string; message: string }
  timestamp?: string
}

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

export function eventStream(events: StreamEvent[]): ReadableStream<Uint8Array> {
  return chunkedStream(events.map((event) => `${JSON.stringify(event)}\n`))
}

export function ndjsonResponse(events: StreamEvent[], chunks?: string[]): HttpResponse<DefaultBodyType> {
  const body = chunks ? chunkedStream(chunks) : eventStream(events)
  return new HttpResponse(body, { status: 200, headers: { 'Content-Type': 'application/json' } })
}

export const streamFixtures = { progressEvent, completeEvent, errorEvent }
