import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse, type DefaultBodyType } from 'msw'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { usePullImage } from '@features/images/hooks/usePullImage'
import { server } from '@/test/server'
import {
  abortableNdjsonResponse,
  ndjsonResponse,
  streamFixtures,
} from '@/test/stream'

type PullResponse = HttpResponse<DefaultBodyType>
type PullHandler = (request: Request) => PullResponse | Promise<PullResponse>

const { progressEvent, completeEvent, errorEvent } = streamFixtures

const pullUrl = '*/api/machines/:machineId/images/pull'

let requests: Request[] = []

// Routes the pull endpoint through MSW (the repo's single test seam), so the
// real fetch, the AbortController, and the hook's stream handling all run.
function mockPull(handler: PullHandler) {
  requests = []
  server.use(
    http.post(pullUrl, ({ request }) => {
      requests.push(request)
      return handler(request)
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
  vi.restoreAllMocks()
})

describe('usePullImage', () => {
  it('POSTs the image reference and exposes the latest progress event', async () => {
    mockPull(() =>
      ndjsonResponse([
        progressEvent({ state: { current: 10, total: 100, status: 'Downloading layers' } }),
        progressEvent({ state: { current: 50, total: 100, status: 'Downloading layers' } }),
        completeEvent(),
      ]),
    )
    const { result } = renderPullHook()

    await act(async () => {
      result.current.pull('nginx:latest')
    })

    expect(requests).toHaveLength(1)
    expect(requests[0].method).toBe('POST')
    expect(new URL(requests[0].url).pathname).toBe('/api/machines/m-1/images/pull')
    expect(await requests[0].json()).toEqual({ image: 'nginx:latest' })
    await waitFor(() => expect(result.current.isPulling).toBe(false))
    expect(result.current.progress?.state?.current).toBe(50)
    expect(result.current.error).toBeNull()
  })

  it('invalidates the images query on complete', async () => {
    mockPull(() => ndjsonResponse([completeEvent()]))
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
    mockPull(() => ndjsonResponse([errorEvent('REGISTRY_UNREACHABLE', 'Registry is down')]))
    const { result } = renderPullHook()

    await act(async () => {
      result.current.pull('nginx:latest')
    })

    await waitFor(() => expect(result.current.isPulling).toBe(false))
    expect(result.current.error).toEqual({ code: 'REGISTRY_UNREACHABLE', message: 'Registry is down' })
  })

  it('aborts the fetch on cancel and tears down the stream, not failing', async () => {
    let requestSignal: AbortSignal | undefined
    mockPull((request) => {
      requestSignal = request.signal
      return abortableNdjsonResponse([progressEvent()], request.signal)
    })
    const { result } = renderPullHook()

    await act(async () => {
      result.current.pull('nginx:latest')
    })
    await waitFor(() => expect(result.current.isPulling).toBe(true))
    expect(result.current.progress?.state?.status).toBe('pulling')

    await act(async () => {
      result.current.cancel()
    })

    // The aborted signal errors the body stream, the rejected read flows
    // through consumeNdjsonStream's teardown into finish(), and the pull ends
    // as cancelled — no error, no unhandled rejection (the backend's
    // OPERATION_CANCELLED path). Reader release is covered in
    // ndjsonStream.test.ts on the consumer-side stream.
    expect(requestSignal?.aborted).toBe(true)
    expect(result.current.isPulling).toBe(false)
    expect(result.current.error).toBeNull()
  })

  it('aborts the previous pull when a new one starts, without clobbering it', async () => {
    let releaseFirst: (() => void) | undefined
    let firstRequestSignal: AbortSignal | undefined
    mockPull(async (request) => {
      if (requests.length === 1) {
        firstRequestSignal = request.signal
        await new Promise<void>((resolve) => {
          releaseFirst = resolve
        })
      }
      return ndjsonResponse([completeEvent()])
    })
    const { result } = renderPullHook()

    await act(async () => {
      result.current.pull('nginx:1.27')
    })
    expect(requests).toHaveLength(1)
    expect(firstRequestSignal?.aborted).toBe(false)

    await act(async () => {
      result.current.pull('redis:7')
    })

    expect(requests).toHaveLength(2)
    expect(firstRequestSignal?.aborted).toBe(true)

    // The superseded pull's response arrives late; its events and cleanup
    // must not disturb the second pull's state.
    await act(async () => {
      releaseFirst?.()
    })
    await waitFor(() => expect(result.current.isPulling).toBe(false))
    expect(result.current.error).toBeNull()
  })

  it('exposes non-2xx HTTP failures through the error envelope', async () => {
    mockPull(() =>
      HttpResponse.json(
        { requestId: 'req-1', error: { code: 'MACHINE_NOT_FOUND', message: 'Machine not found' } },
        { status: 404 },
      ),
    )
    const { result } = renderPullHook()

    await act(async () => {
      result.current.pull('nginx:latest')
    })

    await waitFor(() => expect(result.current.isPulling).toBe(false))
    expect(result.current.error).toEqual({ code: 'MACHINE_NOT_FOUND', message: 'Machine not found' })
  })

  it('skips malformed lines and reassembles events split across reads', async () => {
    const good = JSON.stringify(progressEvent({ state: { current: 5, total: 10, status: 'Copying blobs' } }))
    const complete = JSON.stringify(completeEvent())
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    mockPull(() => ndjsonResponse([], ['garbage\n', good.slice(0, 20), `${good.slice(20)}\n${complete.slice(0, 3)}`, `${complete.slice(3)}\n`]))
    const { result } = renderPullHook()

    await act(async () => {
      result.current.pull('nginx:latest')
    })

    await waitFor(() => expect(result.current.isPulling).toBe(false))
    expect(result.current.progress?.state?.status).toBe('Copying blobs')
    expect(result.current.progress?.state?.current).toBe(5)
    expect(warn).toHaveBeenCalledTimes(1)
    expect(result.current.error).toBeNull()
  })
})
