import { useCallback, useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { genericError, getApiError, getApiErrorFromResponse, getBearerToken } from '@shared/utils/api'
import { consumeNdjsonStream, type StreamEvent, type StreamEventState } from '@shared/streaming/ndjsonStream'
import type { ApiError } from '@shared/types/api'

const apiBaseUrl = import.meta.env.VITE_API_BASE_URL || '/api'

export interface PullProgress {
  operationId: string
  state?: StreamEventState
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

  // Aborting is how the backend learns to cancel the operation, so cancel()
  // only aborts; the rejected stream read then flows through pull()'s catch
  // blocks into finish(), which owns all state cleanup.
  const cancel = useCallback(() => {
    abortRef.current?.abort()
  }, [])

  useEffect(
    () => () => {
      abortRef.current?.abort()
    },
    [],
  )

  const pull = useCallback(
    async (image: string) => {
      // One pull at a time per hook instance; a new pull aborts the old one.
      abortRef.current?.abort()
      const controller = new AbortController()
      abortRef.current = controller

      setError(null)
      setProgress(null)
      setIsPulling(true)

      // Terminal cleanup. Returns false once this pull has already finished
      // or been superseded by a newer one, so a second terminal event (e.g.
      // complete after error) cannot rerun its side effects.
      const finish = (): boolean => {
        if (abortRef.current !== controller) return false
        abortRef.current = null
        setIsPulling(false)
        return true
      }

      const handleEvent = (event: StreamEvent) => {
        if (controller.signal.aborted) return
        if (event.type === 'progress') {
          setProgress({ operationId: event.operationId, state: event.state })
        } else if (event.type === 'complete') {
          if (finish()) queryClient.invalidateQueries({ queryKey: ['machines', machineId, 'images'] })
        } else if (event.type === 'error') {
          if (finish()) {
            setError({
              code: event.error?.code ?? genericError.code,
              message: event.error?.message ?? genericError.message,
            })
          }
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
        if (finish() && !controller.signal.aborted) setError(getApiError(fetchError))
        return
      }

      if (!response.ok) {
        if (finish()) setError(await getApiErrorFromResponse(response))
        return
      }

      if (!response.body) {
        if (finish()) setError({ ...genericError })
        return
      }

      try {
        await consumeNdjsonStream(response.body, handleEvent)
        finish()
      } catch (streamError) {
        // An abort surfaces here as a rejected read; consumeNdjsonStream has
        // already released the reader. Anything else is a real stream failure.
        if (finish() && !controller.signal.aborted) setError(getApiError(streamError))
      }
    },
    [machineId, queryClient],
  )

  return { isPulling, progress, error, pull, cancel }
}
