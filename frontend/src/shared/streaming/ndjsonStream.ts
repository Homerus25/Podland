// Shared machinery for Newline-delimited JSON (NDJSON) byte streams, as
// produced by the backend's streaming endpoints (image pull, container logs).
// Malformed lines are logged and skipped so one bad line never kills a
// long-running stream; values split across reads are reassembled by buffering
// until a newline arrives.

export interface StreamEventState {
  current?: number
  total?: number
  status: string
  message?: string
}

export interface StreamEventError {
  code: string
  message: string
}

export interface StreamEvent {
  operationId: string
  type: 'progress' | 'complete' | 'error'
  machineId?: string
  state?: StreamEventState
  error?: StreamEventError
  timestamp?: string
}

export function isStreamEvent(value: unknown): value is StreamEvent {
  if (typeof value !== 'object' || value === null) return false
  const event = value as Record<string, unknown>
  return (
    typeof event.operationId === 'string' &&
    (event.type === 'progress' || event.type === 'complete' || event.type === 'error')
  )
}

// Parses a newline-delimited JSON byte stream into raw JSON values. Callers
// apply their own event-shape validation on top.
export async function consumeNdjsonValues(
  stream: ReadableStream<Uint8Array>,
  onValue: (value: unknown) => void,
): Promise<void> {
  const reader = stream.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      let newlineIndex = buffer.indexOf('\n')
      while (newlineIndex !== -1) {
        const line = buffer.slice(0, newlineIndex).trim()
        buffer = buffer.slice(newlineIndex + 1)
        if (line) {
          try {
            onValue(JSON.parse(line))
          } catch {
            console.warn('Skipping malformed stream line (invalid JSON):', line)
          }
        }
        newlineIndex = buffer.indexOf('\n')
      }
    }
  } finally {
    // Cancelling closes the reader and the underlying stream. On abort the
    // pending read() above rejects before we get here; cancelling propagates
    // the teardown so the connection is actually closed (the backend's
    // OPERATION_CANCELLED path). It is a no-op on already-done streams, and
    // must complete before releaseLock, which throws while a read is pending.
    await reader.cancel().catch(() => undefined)
    reader.releaseLock()
  }
}

export async function consumeNdjsonStream(
  stream: ReadableStream<Uint8Array>,
  onEvent: (event: StreamEvent) => void,
): Promise<void> {
  await consumeNdjsonValues(stream, (value) => {
    if (isStreamEvent(value)) {
      onEvent(value)
    } else {
      console.warn('Skipping malformed stream event (unexpected shape):', value)
    }
  })
}
