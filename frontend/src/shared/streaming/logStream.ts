import { consumeNdjsonValues } from '@shared/streaming/ndjsonStream'
import { genericError, getApiErrorFromResponse, getBearerToken } from '@shared/utils/api'
import type { ApiError } from '@shared/types/api'

// A single log line as streamed by the backend's log endpoints in follow mode
// (e.g. backend/internal/domain/containers/dto.go ContainerLogEvent).
export interface LogEvent {
  timestamp: string
  level: string
  message: string
}

export function isLogEvent(value: unknown): value is LogEvent {
  if (typeof value !== 'object' || value === null) return false
  const event = value as Record<string, unknown>
  return (
    typeof event.timestamp === 'string' &&
    typeof event.level === 'string' &&
    typeof event.message === 'string'
  )
}

// The backend encodes stream failures as ERROR-level events whose message is
// "CODE: human message" (backend/internal/domain/containers/handler.go
// streamLogs). Only UPPER_SNAKE codes with at least one underscore qualify:
// stderr lines also arrive as level ERROR with arbitrary text ("FATAL: could
// not bind"), and the backend's error codes are always multi-word.
export function parseLogErrorEvent(event: LogEvent): ApiError | null {
  if (event.level !== 'ERROR') return null
  const separator = event.message.indexOf(': ')
  if (separator === -1) return null
  const code = event.message.slice(0, separator)
  if (!/^[A-Z][A-Z0-9]*(?:_[A-Z0-9]+)+$/.test(code)) return null
  return { code, message: event.message.slice(separator + 2) }
}

// Streams a URL as an NDJSON log event stream with plain fetch (axios would
// buffer the whole response). Emits LogEvent-shaped values; other values are
// warned and skipped. Resolves when the stream ends, rejects with an ApiError
// on transport failure or a non-ok response, and tears down the connection on
// abort via the signal.
export async function followLogStream(
  url: string,
  onEvent: (event: LogEvent) => void,
  signal: AbortSignal,
): Promise<void> {
  const token = getBearerToken()
  let response: Response
  try {
    response = await fetch(url, {
      headers: token ? { Authorization: `Bearer ${token}` } : undefined,
      signal,
    })
  } catch (fetchError) {
    if (signal.aborted) throw fetchError
    throw { ...genericError }
  }

  if (!response.ok) {
    throw await getApiErrorFromResponse(response)
  }

  if (!response.body) {
    throw { ...genericError }
  }

  await consumeNdjsonValues(response.body, (value) => {
    if (isLogEvent(value)) {
      onEvent(value)
    } else {
      console.warn('Skipping non-log stream value (unexpected shape):', value)
    }
  })
}
