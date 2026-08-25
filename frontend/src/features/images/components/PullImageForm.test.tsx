import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { PullImageForm } from '@features/images/components/PullImageForm'
import { ImageList } from '@features/images/components/ImageList'
import { server } from '@/test/server'
import { streamFixtures, type StreamEvent } from '@/test/stream'

const { progressEvent, completeEvent, errorEvent } = streamFixtures

let fetchSignals: AbortSignal[] = []

function stubFetch(events: StreamEvent[] | string[]) {
  fetchSignals = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (_url: string | URL, init: RequestInit = {}) => {
      fetchSignals.push(init.signal as AbortSignal)
      const encoder = new TextEncoder()
      const body = new ReadableStream<Uint8Array>({
        async pull(controller) {
          for (const event of events) {
            // Yield to the event loop between events so the UI renders each
            // progress update live, as it would over a real network.
            await new Promise((resolve) => setTimeout(resolve, 100))
            const line = typeof event === 'string' ? event : `${JSON.stringify(event)}\n`
            controller.enqueue(encoder.encode(line))
          }
          controller.close()
        },
      })
      return new Response(body, { status: 200, headers: { 'Content-Type': 'application/json' } })
    }),
  )
}

function renderForm() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <PullImageForm machineId="m-1" />
    </QueryClientProvider>,
  )
}

// The pull form lives inside the Images tab next to the list it refreshes.
function renderTab() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <PullImageForm machineId="m-1" />
      <ImageList machineId="m-1" />
    </QueryClientProvider>,
  )
}

async function startPull(imageRef = 'nginx:latest') {
  await userEvent.type(screen.getByLabelText(/image reference/i), imageRef)
  await userEvent.click(screen.getByRole('button', { name: /pull/i }))
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('PullImageForm', () => {
  it('requires an image reference before starting a pull', async () => {
    stubFetch([completeEvent()])
    renderForm()

    expect(screen.getByRole('button', { name: /pull/i })).toBeDisabled()
  })

  it('streams live status text and a byte progress bar during the pull', async () => {
    stubFetch([
      progressEvent({ state: { current: 30, total: 100, status: 'Downloading layers', message: 'layer 1/2' } }),
      progressEvent({ state: { current: 80, total: 100, status: 'Downloading layers', message: 'layer 2/2' } }),
      completeEvent(),
    ])
    renderForm()
    await startPull()

    expect(await screen.findByText('Downloading layers')).toBeInTheDocument()
    expect(await screen.findByText('30 B / 100 B')).toBeInTheDocument()
    await waitFor(() => expect(screen.getByText('80 B / 100 B')).toBeInTheDocument())
    expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '80')
    expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuemax', '100')
    await waitFor(() => expect(screen.queryByRole('progressbar')).not.toBeInTheDocument())
  })

  it('cancels the in-flight pull via the cancel button', async () => {
    fetchSignals = []
    let releaseStream: (() => void) | undefined
    vi.stubGlobal(
      'fetch',
      vi.fn(async (_url: string | URL, init: RequestInit = {}) => {
        fetchSignals.push(init.signal as AbortSignal)
        await new Promise<void>((resolve) => {
          releaseStream = resolve
        })
        return new Response(null, { status: 200 })
      }),
    )
    renderForm()
    await startPull()

    await userEvent.click(await screen.findByRole('button', { name: /cancel/i }))

    expect(fetchSignals[0].aborted).toBe(true)
    await waitFor(() => expect(screen.queryByRole('button', { name: /cancel/i })).not.toBeInTheDocument())
    releaseStream?.()
  })

  it('invalidates the images query on complete so the list refetches', async () => {
    let imagesRequests = 0
    let pulled = false
    server.use(
      http.get('*/api/machines/m-1/images', () => {
        imagesRequests++
        const list = pulled
          ? [
              {
                id: 'sha256:new123',
                repoTags: ['docker.io/library/nginx:latest'],
                repoDigests: [],
                createdAt: '2026-08-21T10:00:00Z',
                size: 1000,
              },
            ]
          : []
        return HttpResponse.json(list)
      }),
    )
    stubFetch([
      progressEvent({ state: { current: 1, total: 2, status: 'Downloading layers' } }),
      completeEvent(),
    ])
    renderTab()
    await waitFor(() => expect(imagesRequests).toBe(1))
    pulled = true
    await startPull()

    expect(await screen.findByText('docker.io/library/nginx:latest')).toBeInTheDocument()
    expect(imagesRequests).toBeGreaterThanOrEqual(2)
  })

  it('renders the streamed error envelope through the shared error display', async () => {
    stubFetch([errorEvent('REGISTRY_AUTH_FAILED', 'Invalid registry credentials')])
    renderForm()
    await startPull()

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('REGISTRY_AUTH_FAILED')).toBeInTheDocument()
    expect(screen.getByText('Invalid registry credentials')).toBeInTheDocument()
  })

  it('skips malformed lines and still completes when chunks split events', async () => {
    const good = JSON.stringify(progressEvent({ state: { current: 5, total: 10, status: 'Copying blobs' } }))
    const complete = JSON.stringify(completeEvent())
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    stubFetch([
      'garbage\n',
      `${good.slice(0, 20)}`,
      `${good.slice(20)}\n${complete.slice(0, 3)}`,
      `${complete.slice(3)}\n`,
    ])
    renderForm()
    await startPull()

    expect(await screen.findByText('Copying blobs')).toBeInTheDocument()
    await waitFor(() => expect(screen.queryByRole('progressbar')).not.toBeInTheDocument())
    expect(warn).toHaveBeenCalled()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    warn.mockRestore()
  })
})
