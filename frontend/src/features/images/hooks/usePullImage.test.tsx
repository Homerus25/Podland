import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { usePullImage } from '@features/images/hooks/usePullImage'
import { eventStream, streamFixtures, type StreamEvent } from '@/test/stream'

const { progressEvent, completeEvent, errorEvent } = streamFixtures

let fetchCalls: { url: string; init: RequestInit }[] = []
let abortListenerCount = 0

function stubFetch(events: StreamEvent[]) {
  fetchCalls = []
  abortListenerCount = 0
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string | URL, init: RequestInit = {}) => {
      fetchCalls.push({ url: String(url), init })
      init.signal?.addEventListener('abort', () => abortListenerCount++)
      return new Response(eventStream(events), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    }),
  )
}

function renderPullHook() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
  const hook = renderHook(() => usePullImage('m-1'), { wrapper })
  return { ...hook, queryClient }
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('usePullImage', () => {
  it('POSTs the image reference and exposes the latest progress event', async () => {
    stubFetch([
      progressEvent({ state: { current: 10, total: 100, status: 'Downloading layers' } }),
      progressEvent({ state: { current: 50, total: 100, status: 'Downloading layers' } }),
      completeEvent(),
    ])
    const { result } = renderPullHook()

    await act(async () => {
      result.current.pull('nginx:latest')
    })

    expect(fetchCalls).toHaveLength(1)
    expect(fetchCalls[0].url).toContain('/api/machines/m-1/images/pull')
    expect(JSON.parse(String(fetchCalls[0].init.body))).toEqual({ image: 'nginx:latest' })
    await waitFor(() => expect(result.current.isPulling).toBe(false))
    expect(result.current.progress?.state?.current).toBe(50)
    expect(result.current.error).toBeNull()
  })

  it('invalidates the images query on complete', async () => {
    stubFetch([completeEvent()])
    const { result, queryClient } = renderPullHook()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    await act(async () => {
      result.current.pull('nginx:latest')
    })

    await waitFor(() => expect(result.current.isPulling).toBe(false))
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['machines', 'm-1', 'images'],
    })
  })

  it('routes the error envelope out of the stream', async () => {
    stubFetch([errorEvent('REGISTRY_UNREACHABLE', 'Registry is down')])
    const { result } = renderPullHook()

    await act(async () => {
      result.current.pull('nginx:latest')
    })

    await waitFor(() => expect(result.current.isPulling).toBe(false))
    expect(result.current.error).toEqual({ code: 'REGISTRY_UNREACHABLE', message: 'Registry is down' })
  })

  it('aborts the in-flight fetch on cancel and treats it as cancelled, not failed', async () => {
    fetchCalls = []
    let releaseStream: ((events: StreamEvent[]) => void) | undefined
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string | URL, init: RequestInit = {}) => {
        fetchCalls.push({ url: String(url), init })
        const events = await new Promise<StreamEvent[]>((resolve) => {
          releaseStream = resolve
        })
        return new Response(eventStream(events), { status: 200 })
      }),
    )
    const { result } = renderPullHook()

    await act(async () => {
      result.current.pull('nginx:latest')
    })
    expect(result.current.isPulling).toBe(true)

    await act(async () => {
      result.current.cancel()
    })

    const signal = fetchCalls[0].init.signal as AbortSignal
    expect(signal.aborted).toBe(true)
    expect(result.current.isPulling).toBe(false)
    expect(result.current.error).toBeNull()

    releaseStream?.([errorEvent('OPERATION_CANCELLED', 'Cancelled')])
    await act(async () => {})
  })

  it('aborts the previous pull when a new one starts', async () => {
    fetchCalls = []
    let releaseFirst: (() => void) | undefined
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string | URL, init: RequestInit = {}) => {
        fetchCalls.push({ url: String(url), init })
        if (fetchCalls.length === 1) {
          await new Promise<void>((resolve) => {
            releaseFirst = resolve
          })
        }
        return new Response(eventStream([completeEvent()]), { status: 200 })
      }),
    )
    const { result } = renderPullHook()

    await act(async () => {
      result.current.pull('nginx:1.27')
    })
    expect(fetchCalls).toHaveLength(1)
    const firstSignal = fetchCalls[0].init.signal as AbortSignal

    await act(async () => {
      result.current.pull('redis:7')
    })

    expect(fetchCalls).toHaveLength(2)
    expect(firstSignal.aborted).toBe(true)
    releaseFirst?.()
    await act(async () => {})
  })

  it('exposes non-2xx HTTP failures through the error envelope', async () => {
    fetchCalls = []
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        new Response(
          JSON.stringify({
            requestId: 'req-1',
            error: { code: 'MACHINE_NOT_FOUND', message: 'Machine not found' },
          }),
          { status: 404 },
        ),
      ),
    )
    const { result } = renderPullHook()

    await act(async () => {
      result.current.pull('nginx:latest')
    })

    await waitFor(() => expect(result.current.isPulling).toBe(false))
    expect(result.current.error).toEqual({ code: 'MACHINE_NOT_FOUND', message: 'Machine not found' })
  })
})
