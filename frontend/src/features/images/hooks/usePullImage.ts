import { useCallback, useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { getApiError, getBearerToken } from '@shared/utils/api'
import { consumeNdjsonStream, type StreamEvent } from '@shared/streaming/ndjsonStream'
import type { ApiError } from '@shared/types/api'

const apiBaseUrl = import.meta.env.VITE_API_BASE_URL || '/api'

export interface PullProgress {
  operationId: string
  state?: { current?: number; total?: number; status: string; message?: string }
}

interface UsePullImageResult {
  isPulling: boolean
  progress: PullProgress | null
  error: ApiError | null
  pull: (image: string) => void
  cancel: () => void
}

// Consumes the backend's NDJSON pull stream with plain fetch + AbortController
// (axios would buffer the whole response). The pattern — streaming fetch,
// event/state ownership, abort handling — is deliberately non-image-specific
// so later streaming flows (e.g. container logs) can follow it.
export function usePullImage(machineId: string): UsePullImageResult {
  const queryClient = useQueryClient()
  const [isPulling, setIsPulling] = useState(false)
  const [progress, setProgress] = useState<PullProgress | null>(null)
  const [error, setError] = useState<ApiError | null>(null)
  const abortRef = useRef<AbortController | null>(null)

  const cancel = useCallback(() => {
    abortRef.current?.abort()
    abortRef.current = null
    setIsPulling(false)
  }, [])

  // Aborting the fetch is how the backend learns to cancel the operation,
  // so unmount must not leave a pull running.
  useEffect(() => () => abortRef.current?.abort(), [])

  const pull = useCallback(
    async (image: string) => {
      // One pull at a time per hook instance; a new pull aborts the old one.
      abortRef.current?.abort()
      const controller = new AbortController()
      abortRef.current = controller

      setError(null)
      setProgress(null)
      setIsPulling(true)

      const handleEvent = (event: StreamEvent) => {
        if (controller.signal.aborted) return
        if (event.type === 'progress') {
          setProgress({ operationId: event.operationId, state: event.state })
        } else if (event.type === 'complete') {
          setIsPulling(false)
          abortRef.current = null
          queryClient.invalidateQueries({ queryKey: ['machines', machineId, 'images'] })
        } else if (event.type === 'error') {
          setIsPulling(false)
          abortRef.current = null
          setError({
            code: event.error?.code ?? 'INTERNAL_ERROR',
            message: event.error?.message ?? 'An unexpected error occurred',
          })
        }
      }

      let response: Response
      try {
        const token = getBearerToken()
        response = await fetch(`${apiBaseUrl}/machines/${machineId}/images/pull`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            ...(token ? { Authorization: `Bearer ${token}` } : {}),
          },
          body: JSON.stringify({ image }),
          signal: controller.signal,
        })
      } catch (fetchError) {
        if (controller.signal.aborted) return
        setIsPulling(false)
        abortRef.current = null
        setError(getApiError(fetchError))
        return
      }

      if (!response.ok) {
        const envelope = (await response.json().catch(() => null)) as {
          error?: { code?: string; message?: string }
        } | null
        setIsPulling(false)
        abortRef.current = null
        setError(
          envelope?.error?.code && envelope.error.message
            ? { code: envelope.error.code, message: envelope.error.message }
            : getApiError(response),
        )
        return
      }

      if (!response.body) {
        setIsPulling(false)
        abortRef.current = null
        setError({ code: 'INTERNAL_ERROR', message: 'An unexpected error occurred' })
        return
      }

      await consumeNdjsonStream(response.body, handleEvent)
      if (!controller.signal.aborted && abortRef.current === controller) {
        setIsPulling(false)
        abortRef.current = null
      }
    },
    [machineId, queryClient],
  )

  return { isPulling, progress, error, pull, cancel }
}
