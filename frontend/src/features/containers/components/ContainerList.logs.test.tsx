import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { ContainerList } from '@features/containers/components/ContainerList'
import type { ContainerSummary } from '@features/containers/services/containersApi'
import { chunkedStream, streamResponse } from '@/test/stream'
import { server } from '@/test/server'

function containerFixture(overrides: Partial<ContainerSummary> = {}): ContainerSummary {
  return {
    id: 'c-1',
    names: ['/my-container'],
    image: 'docker.io/library/nginx:1.27',
    createdAt: '2026-08-20T10:00:00Z',
    state: 'running',
    status: 'Up 2 hours',
    restarts: 0,
    exited: false,
    ...overrides,
  }
}

function logEvent(overrides: Partial<{ timestamp: string; level: string; message: string }> = {}) {
  return JSON.stringify({
    timestamp: '2026-08-26T10:00:00Z',
    level: 'INFO',
    message: 'booted',
    ...overrides,
  })
}

let logQueries: { tail: string | null; follow: string | null; count: number } = {
  tail: null,
  follow: null,
  count: 0,
}

function resetCounters() {
  logQueries = { tail: null, follow: null, count: 0 }
}

function containersHandler(containers: ContainerSummary[]) {
  return http.get('*/api/machines/m-1/containers', () =>
    HttpResponse.json({
      containers,
      total: containers.length,
      timestamp: '2026-08-26T00:00:00Z',
    }),
  )
}

function logsHandler() {
  return http.get('*/api/machines/m-1/containers/c-1/logs', ({ request }) => {
    const url = new URL(request.url)
    logQueries.count++
    logQueries.tail = url.searchParams.get('tail')
    logQueries.follow = url.searchParams.get('follow')
    if (url.searchParams.get('follow') === 'true') {
      return streamResponse(chunkedStream([`${logEvent()}\n`, `${logEvent({ message: 'live line' })}\n`]))
    }
    return HttpResponse.json({ id: 'c-1', logs: 'hist-line-1\nhist-line-2' })
  })
}

function renderContainerList() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <ContainerList machineId="m-1" />
    </QueryClientProvider>,
  )
}

async function openLogsViewer() {
  renderContainerList()
  await screen.findByText('my-container')
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: /logs.*my-container/i }))
  return user
}

describe('ContainerList logs', () => {
  it('opens the viewer and fetches the last 100 lines by default', async () => {
    resetCounters()
    server.use(containersHandler([containerFixture()]), logsHandler())
    await openLogsViewer()

    expect(await screen.findAllByTestId('log-viewer-line')).toHaveLength(2)
    expect(screen.getByText(/hist-line-1/)).toBeInTheDocument()
    expect(screen.getByText(/hist-line-2/)).toBeInTheDocument()
    await waitFor(() => expect(logQueries.count).toBeGreaterThanOrEqual(1))
    expect(logQueries.tail).toBe('100')
    expect(logQueries.follow).not.toBe('true')
  })

  it('streams live lines when follow is enabled', async () => {
    resetCounters()
    server.use(containersHandler([containerFixture()]), logsHandler())
    const user = await openLogsViewer()
    await screen.findAllByTestId('log-viewer-line')

    await user.click(screen.getByTestId('log-follow-toggle'))

    expect(await screen.findByText(/live line/)).toBeInTheDocument()
    await waitFor(() => expect(logQueries.follow).toBe('true'))
  })

  it('aborts the stream when the viewer is closed', async () => {
    resetCounters()
    let streamSignal: AbortSignal | null = null
    server.use(
      containersHandler([containerFixture()]),
      http.get('*/api/machines/m-1/containers/c-1/logs', ({ request }) => {
        const url = new URL(request.url)
        if (url.searchParams.get('follow') === 'true') {
          streamSignal = request.signal
          return streamResponse(
            new ReadableStream<Uint8Array>({
              start(controller) {
                controller.enqueue(new TextEncoder().encode(`${logEvent()}\n`))
                request.signal.addEventListener('abort', () =>
                  controller.error(new DOMException('Aborted', 'AbortError')),
                )
              },
            }),
          )
        }
        return HttpResponse.json({ id: 'c-1', logs: 'hist-line-1' })
      }),
    )
    const user = await openLogsViewer()
    await screen.findAllByTestId('log-viewer-line')
    await user.click(screen.getByTestId('log-follow-toggle'))
    await waitFor(() => expect(streamSignal).not.toBeNull())

    await user.click(screen.getByRole('button', { name: /close dialog/i }))

    await waitFor(() => expect(streamSignal?.aborted).toBe(true))
    await waitFor(() => expect(screen.queryByTestId('log-viewer')).not.toBeInTheDocument())
  })

  it('surfaces the error envelope when the initial fetch fails', async () => {
    resetCounters()
    server.use(
      containersHandler([containerFixture()]),
      http.get('*/api/machines/m-1/containers/c-1/logs', () =>
        HttpResponse.json(
          { requestId: 'req-16', error: { message: 'no such container', code: 'CONTAINER_NOT_FOUND' } },
          { status: 404 },
        ),
      ),
    )
    await openLogsViewer()

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('CONTAINER_NOT_FOUND')).toBeInTheDocument()
    expect(screen.getByText('no such container')).toBeInTheDocument()
  })
})
