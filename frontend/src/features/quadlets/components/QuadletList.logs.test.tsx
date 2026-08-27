import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { QuadletList } from '@features/quadlets/components/QuadletList'
import type { QuadletSummary } from '@features/quadlets/services/quadletsApi'
import { chunkedStream, streamResponse } from '@/test/stream'
import { server } from '@/test/server'

function quadletFixture(overrides: Partial<QuadletSummary> = {}): QuadletSummary {
  return {
    id: 'web',
    name: 'web',
    path: '/etc/containers/systemd/web.container',
    unit: 'web.service',
    resource: 'container',
    state: 'running',
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

function quadletsHandler(quadlets: QuadletSummary[]) {
  return http.get('*/api/machines/m-1/quadlets', () => HttpResponse.json(quadlets))
}

function logsHandler() {
  return http.get('*/api/machines/m-1/quadlets/web/logs', ({ request }) => {
    const url = new URL(request.url)
    logQueries.count++
    logQueries.tail = url.searchParams.get('tail')
    logQueries.follow = url.searchParams.get('follow')
    if (url.searchParams.get('follow') === 'true') {
      return streamResponse(chunkedStream([`${logEvent()}\n`, `${logEvent({ message: 'live line' })}\n`]))
    }
    return HttpResponse.json({ id: 'web', logs: 'hist-line-1\nhist-line-2' })
  })
}

function renderQuadletList() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <QuadletList machineId="m-1" />
    </QueryClientProvider>,
  )
}

async function openLogsViewer() {
  renderQuadletList()
  await screen.findByText('web')
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: /logs.*web/i }))
  return user
}

describe('QuadletList logs', () => {
  it('opens the viewer and fetches the last 100 lines by default', async () => {
    resetCounters()
    server.use(quadletsHandler([quadletFixture()]), logsHandler())
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
    server.use(quadletsHandler([quadletFixture()]), logsHandler())
    const user = await openLogsViewer()
    await screen.findAllByTestId('log-viewer-line')

    await user.click(screen.getByTestId('log-follow-toggle'))

    expect(await screen.findByText(/live line/)).toBeInTheDocument()
    await waitFor(() => expect(logQueries.follow).toBe('true'))
    expect(logQueries.tail).toBe('100')
  })

  it('aborts the stream when the viewer is closed', async () => {
    resetCounters()
    let streamSignal: AbortSignal | null = null
    server.use(
      quadletsHandler([quadletFixture()]),
      http.get('*/api/machines/m-1/quadlets/web/logs', ({ request }) => {
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
        return HttpResponse.json({ id: 'web', logs: 'hist-line-1' })
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
      quadletsHandler([quadletFixture()]),
      http.get('*/api/machines/m-1/quadlets/web/logs', () =>
        HttpResponse.json(
          { requestId: 'req-61', error: { message: 'quadlet not found', code: 'QUADLET_NOT_FOUND' } },
          { status: 404 },
        ),
      ),
    )
    await openLogsViewer()

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('QUADLET_NOT_FOUND')).toBeInTheDocument()
    expect(screen.getByText('quadlet not found')).toBeInTheDocument()
  })

  it('sends the newly selected tail on the next fetch', async () => {
    resetCounters()
    server.use(quadletsHandler([quadletFixture()]), logsHandler())
    const user = await openLogsViewer()
    await screen.findAllByTestId('log-viewer-line')

    await user.selectOptions(screen.getByTestId('log-tail-select'), '500')

    await waitFor(() => expect(logQueries.tail).toBe('500'))
    expect(logQueries.follow).not.toBe('true')
  })

  it('surfaces the error envelope when the follow stream fails to connect', async () => {
    resetCounters()
    server.use(
      quadletsHandler([quadletFixture()]),
      http.get('*/api/machines/m-1/quadlets/web/logs', ({ request }) => {
        const url = new URL(request.url)
        if (url.searchParams.get('follow') === 'true') {
          // The quadlet disappearing between open and follow: a 4xx envelope
          // passes through unmasked (5xx is deliberately collapsed).
          return HttpResponse.json(
            { requestId: 'req-62', error: { message: 'quadlet not found', code: 'QUADLET_NOT_FOUND' } },
            { status: 404 },
          )
        }
        return HttpResponse.json({ id: 'web', logs: 'hist-line-1' })
      }),
    )
    const user = await openLogsViewer()
    await screen.findAllByTestId('log-viewer-line')

    await user.click(screen.getByTestId('log-follow-toggle'))

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('QUADLET_NOT_FOUND')).toBeInTheDocument()
    expect(screen.getByText('quadlet not found')).toBeInTheDocument()
  })
})
