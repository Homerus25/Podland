import { useCallback, useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { genericError, getApiError, getApiErrorFromResponse, getBearerToken } from '@shared/utils/api'
import { consumeNdjsonValues } from '@shared/streaming/ndjsonStream'
import type { ApiError } from '@shared/types/api'

const apiBaseUrl = import.meta.env.VITE_API_BASE_URL || '/api'

// Event and report shapes mirror the backend's AutoUpdateEvent
// (backend/internal/domain/quadlets/auto_update.go).
export interface AutoUpdateItem {
  unit?: string
  containerName: string
  imageName: string
  // The backend's outcome vocabulary: "updated" | "failed" | "pending" | "up-to-date".
  updated: string
  error?: string
}

export interface AutoUpdateReport {
  containers: AutoUpdateItem[]
  overall: string
  exitCode: number
}

export interface AutoUpdateLine {
  stream: string
  line: string
}

interface AutoUpdateEvent {
  operationId: string
  type: 'log' | 'complete' | 'error'
  stream?: string
  line?: string
  report?: AutoUpdateReport
  error?: { code: string; message: string }
}

function isAutoUpdateEvent(value: unknown): value is AutoUpdateEvent {
  if (typeof value !== 'object' || value === null) return false
  const event = value as Record<string, unknown>
  return (
    typeof event.operationId === 'string' &&
    (event.type === 'log' || event.type === 'complete' || event.type === 'error')
  )
}

export interface UseQuadletAutoUpdateResult {
  isRunning: boolean
  dryRun: boolean
  lines: AutoUpdateLine[]
  report: AutoUpdateReport | null
  // A terminal error event (machine unreachable, SSH failure) or an HTTP
  // failure — distinct from per-container failures inside the report.
  connectionError: ApiError | null
  run: (dryRun: boolean) => void
  cancel: () => void
}

// Consumes the auto-update NDJSON stream with plain fetch + AbortController
// (the image pull pattern). The stream carries its own "log"/"complete"/
// "error" event vocabulary, so it parses via the shared NDJSON reader with
// local shape validation instead of the generic StreamEvent consumer.
export function useQuadletAutoUpdate(machineId: string): UseQuadletAutoUpdateResult {
  const queryClient = useQueryClient()
  const [isRunning, setIsRunning] = useState(false)
  const [dryRun, setDryRun] = useState(false)
  const [lines, setLines] = useState<AutoUpdateLine[]>([])
  const [report, setReport] = useState<AutoUpdateReport | null>(null)
  const [connectionError, setConnectionError] = useState<ApiError | null>(null)
  const abortRef = useRef<AbortController | null>(null)

  const cancel = useCallback(() => {
    abortRef.current?.abort()
  }, [])

  useEffect(
    () => () => {
      abortRef.current?.abort()
    },
    [],
  )

  const run = useCallback(
    async (runDryRun: boolean) => {
      abortRef.current?.abort()
      const controller = new AbortController()
      abortRef.current = controller

      setDryRun(runDryRun)
      setConnectionError(null)
      setLines([])
      setReport(null)
      setIsRunning(true)

      // Terminal cleanup guarded against superseded/aborted runs, as in
      // usePullImage: a second terminal event cannot rerun its side effects.
      const finish = (): boolean => {
        if (abortRef.current !== controller) return false
        abortRef.current = null
        setIsRunning(false)
        return true
      }

      const handleEvent = (event: AutoUpdateEvent) => {
        if (controller.signal.aborted) return
        if (event.type === 'log') {
          if (event.line !== undefined) {
            setLines((current) => [...current, { stream: event.stream ?? 'stdout', line: event.line! }])
          }
        } else if (event.type === 'complete') {
          if (finish()) {
            setReport(event.report ?? null)
            // Restarted containers and pulled images only change on a real
            // run; a dry run mutates nothing, so its queries stay fresh.
            if (!runDryRun) {
              queryClient.invalidateQueries({ queryKey: ['machines', machineId, 'quadlets'] })
              queryClient.invalidateQueries({ queryKey: ['machines', machineId, 'containers'] })
              queryClient.invalidateQueries({ queryKey: ['machines', machineId, 'images'] })
            }
          }
        } else if (event.type === 'error') {
          if (finish()) {
            setConnectionError({
              code: event.error?.code ?? genericError.code,
              message: event.error?.message ?? genericError.message,
            })
          }
        }
      }

      let response: Response
      try {
        const token = getBearerToken()
        const params = new URLSearchParams()
        if (runDryRun) params.set('dryRun', 'true')
        const query = params.size > 0 ? `?${params.toString()}` : ''
        response = await fetch(`${apiBaseUrl}/machines/${machineId}/quadlets/auto-update${query}`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            ...(token ? { Authorization: `Bearer ${token}` } : {}),
          },
          signal: controller.signal,
        })
      } catch (fetchError) {
        if (finish() && !controller.signal.aborted) setConnectionError(getApiError(fetchError))
        return
      }

      if (!response.ok) {
        if (finish()) setConnectionError(await getApiErrorFromResponse(response))
        return
      }

      if (!response.body) {
        if (finish()) setConnectionError({ ...genericError })
        return
      }

      try {
        await consumeNdjsonValues(response.body, (value) => {
          if (isAutoUpdateEvent(value)) handleEvent(value)
          else console.warn('Skipping malformed stream event (unexpected shape):', value)
        })
        finish()
      } catch (streamError) {
        if (finish() && !controller.signal.aborted) setConnectionError(getApiError(streamError))
      }
    },
    [machineId, queryClient],
  )

  return { isRunning, dryRun, lines, report, connectionError, run, cancel }
}
