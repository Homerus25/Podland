import { useEffect, useRef, useState } from 'react'
import { Box, Select, Switch, Text, VStack } from '@chakra-ui/react'
import { ErrorDisplay } from '@shared/components/ErrorDisplay'
import { parseLogErrorEvent, type LogEvent } from '@shared/streaming/logStream'
import { genericError } from '@shared/utils/api'
import type { ApiError } from '@shared/types/api'

// How a log viewer obtains its lines. The same component serves any backend
// log endpoint (container and quadlet logs today): the caller wires
// the two calls to its own URLs and params.
export interface LogSource {
  // Fetches the last `tail` lines without following.
  fetchTail: (tail: number) => Promise<string[]>
  // Opens the live stream; calls onEvent per streamed log event and resolves
  // when the stream ends. Must honor the AbortSignal so toggling follow off,
  // closing the viewer, or unmounting tears down the connection.
  follow: (
    tail: number,
    onEvent: (event: LogEvent) => void,
    signal: AbortSignal,
  ) => Promise<void>
}

const TAIL_OPTIONS = [50, 100, 200, 500]
const DEFAULT_TAIL = 100

function formatEvent(event: LogEvent): string {
  return `${event.timestamp} ${event.level} ${event.message}`
}

// Sources reject with ApiError-shaped objects (the stream helper does, and
// sources convert axios errors); anything else collapses to the generic one.
function toApiError(error: unknown): ApiError {
  if (typeof error === 'object' && error !== null) {
    const candidate = error as { code?: unknown; message?: unknown }
    if (typeof candidate.code === 'string' && typeof candidate.message === 'string') {
      return { code: candidate.code, message: candidate.message }
    }
  }
  return { ...genericError }
}

// Generic streaming log viewer: initial tail fetch, follow toggle driving a
// live NDJSON stream, auto-scroll while following, and error surfacing. Owns
// one AbortController per follow session; aborting on unmount guarantees no
// background connections leak.
export function LogViewer({ source }: { source: LogSource }) {
  const [tail, setTail] = useState(DEFAULT_TAIL)
  const [lines, setLines] = useState<string[]>([])
  const [isFollowing, setIsFollowing] = useState(false)
  const [error, setError] = useState<ApiError | null>(null)
  const abortRef = useRef<AbortController | null>(null)
  const scrollRef = useRef<HTMLDivElement>(null)

  const loadTail = async (requestedTail: number) => {
    setError(null)
    try {
      setLines(await source.fetchTail(requestedTail))
    } catch (tailError) {
      setLines([])
      setError(toApiError(tailError))
    }
  }

  const stopFollowing = () => {
    abortRef.current?.abort()
    abortRef.current = null
    setIsFollowing(false)
  }

  const startFollowing = async (requestedTail: number) => {
    abortRef.current?.abort()
    const controller = new AbortController()
    abortRef.current = controller
    setError(null)
    setIsFollowing(true)
    // The backend's follow mode replays the tail before streaming new lines,
    // so the initial fetch's lines are dropped to avoid showing them twice.
    setLines([])

    try {
      await source.follow(
        requestedTail,
        (event) => {
          if (controller.signal.aborted) return
          const logError = parseLogErrorEvent(event)
          if (logError) {
            setError(logError)
            return
          }
          setLines((previous) => [...previous, formatEvent(event)])
        },
        controller.signal,
      )
    } catch (streamError) {
      // An abort surfaces here as a rejected read; only real failures show,
      // preserving the envelope when the source rejected with one.
      if (!controller.signal.aborted) setError(toApiError(streamError))
    } finally {
      if (abortRef.current === controller) {
        abortRef.current = null
        setIsFollowing(false)
      }
    }
  }

  const onFollowToggle = () => {
    if (isFollowing) {
      stopFollowing()
    } else {
      void startFollowing(tail)
    }
  }

  useEffect(() => {
    if (isFollowing) {
      // Tail changed mid-follow: restart the stream so the new tail applies
      // (the replay repopulates the lines).
      void startFollowing(tail)
    } else {
      void loadTail(tail)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tail])

  useEffect(
    () => () => {
      abortRef.current?.abort()
    },
    [],
  )

  // Keep the newest line visible while following.
  useEffect(() => {
    if (isFollowing && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [lines, isFollowing])

  return (
    <VStack align="stretch" spacing={2} data-testid="log-viewer">
      {error ? <ErrorDisplay error={error} /> : null}
      <Box display="flex" gap={4} alignItems="center">
        <Text fontSize="sm">Tail</Text>
        <Select
          size="sm"
          width="auto"
          value={tail}
          onChange={(event) => setTail(Number(event.target.value))}
          data-testid="log-tail-select"
        >
          {TAIL_OPTIONS.map((option) => (
            <option key={option} value={option}>
              last {option} lines
            </option>
          ))}
        </Select>
        <Text fontSize="sm">Follow</Text>
        <Switch
          isChecked={isFollowing}
          onChange={onFollowToggle}
          data-testid="log-follow-toggle"
        />
      </Box>
      <Box
        ref={scrollRef}
        as="pre"
        fontFamily="mono"
        fontSize="sm"
        p={2}
        maxHeight="400px"
        overflowY="auto"
        whiteSpace="pre-wrap"
        data-testid="log-viewer-scroll"
      >
        {lines.length === 0 ? (
          <Text color="gray.500" data-testid="log-viewer-empty">
            No log output.
          </Text>
        ) : (
          lines.map((line, index) => (
            <Text key={index} data-testid="log-viewer-line">
              {line}
            </Text>
          ))
        )}
      </Box>
    </VStack>
  )
}
