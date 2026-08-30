import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { QuadletList } from '@features/quadlets/components/QuadletList'
import type { QuadletSummary } from '@features/quadlets/services/quadletsApi'
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

const web = quadletFixture()

type Permission = 'admin' | 'write' | 'read'

function renderQuadletList(permission: Permission = 'admin') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  server.use(http.get('*/api/machines/m-1/quadlets', () => HttpResponse.json([web])))
  render(
    <QueryClientProvider client={queryClient}>
      <QuadletList machineId="m-1" permission={permission} />
    </QueryClientProvider>,
  )
}

// Auto-update events mirror the backend's AutoUpdateEvent shapes.
type AutoUpdateEvent = Record<string, unknown> & { type: string; operationId: string }

function logEvent(line: string): AutoUpdateEvent {
  return { operationId: 'op-1', type: 'log', stream: 'stdout', line, machineId: 'm-1' }
}

function completeEvent(containers: Record<string, unknown>[], overall = 'completed'): AutoUpdateEvent {
  return {
    operationId: 'op-1',
    type: 'complete',
    machineId: 'm-1',
    report: { containers, overall, exitCode: 0 },
  }
}

function errorEvent(code: string, message: string): AutoUpdateEvent {
  return { operationId: 'op-1', type: 'error', machineId: 'm-1', error: { code, message } }
}

const item = (overrides: Record<string, unknown>) => ({
  containerName: 'web',
  imageName: 'nginx:1.27',
  updated: 'updated',
  ...overrides,
})

let autoUpdateRequests: Request[] = []

// A gate so tests can hold the stream open and observe live line rendering.
let releaseStream: (() => void) | undefined
let pendingEvents: AutoUpdateEvent[] = []

function mockAutoUpdate(events: AutoUpdateEvent[] = [], holdOpen = false) {
  autoUpdateRequests = []
  pendingEvents = events
  releaseStream = undefined
  server.use(
    http.post('*/api/machines/m-1/quadlets/auto-update', ({ request }) => {
      autoUpdateRequests.push(request)
      const framed = pendingEvents.map((event) => `${JSON.stringify(event)}\n`)
      if (!holdOpen) {
        return new HttpResponse(framed.join(''), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      const encoder = new TextEncoder()
      const stream = new ReadableStream<Uint8Array>({
        async start(controller) {
          for (const chunk of framed) {
            await new Promise((resolve) => setTimeout(resolve, 20))
            controller.enqueue(encoder.encode(chunk))
          }
          await new Promise<void>((resolve) => {
            releaseStream = resolve
          })
          controller.close()
        },
      })
      return new HttpResponse(stream, {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    }),
  )
}

async function clickCheckForUpdates() {
  renderQuadletList()
  const user = userEvent.setup()
  await user.click(await screen.findByTestId('check-updates-button'))
  return user
}

async function clickUpdateAllThroughConfirm() {
  renderQuadletList()
  const user = userEvent.setup()
  await user.click(await screen.findByTestId('update-all-button'))
  expect(await screen.findByTestId('modal')).toBeInTheDocument()
  expect(autoUpdateRequests).toHaveLength(0)
  await user.click(screen.getByTestId('confirm-auto-update-button'))
  return user
}

describe('QuadletList auto-update', () => {
  it('hides both toolbar buttons from read-only users', async () => {
    renderQuadletList('read')

    expect(await screen.findByTestId('quadlets-table')).toBeInTheDocument()
    expect(screen.queryByTestId('check-updates-button')).not.toBeInTheDocument()
    expect(screen.queryByTestId('update-all-button')).not.toBeInTheDocument()
  })

  it('shows both toolbar buttons to users with write access', async () => {
    renderQuadletList('write')

    expect(await screen.findByTestId('check-updates-button')).toBeInTheDocument()
    expect(screen.getByTestId('update-all-button')).toBeInTheDocument()
  })

  it('runs "Check for updates" directly as a dry run without confirmation', async () => {
    mockAutoUpdate([completeEvent([item({ updated: 'pending' })], 'dry-run')])
    await clickCheckForUpdates()

    await waitFor(() => expect(autoUpdateRequests).toHaveLength(1))
    expect(new URL(autoUpdateRequests[0].url).searchParams.get('dryRun')).toBe('true')
    expect(await screen.findByTestId('modal')).toBeInTheDocument()
  })

  it('requires confirmation before "Update all" fires, then runs without dryRun', async () => {
    mockAutoUpdate([completeEvent([])])
    await clickUpdateAllThroughConfirm()

    await waitFor(() => expect(autoUpdateRequests).toHaveLength(1))
    expect(new URL(autoUpdateRequests[0].url).searchParams.has('dryRun')).toBe(false)
  })

  it('renders live log lines while the stream is open', async () => {
    mockAutoUpdate([logEvent('Checking web.service')], true)
    await clickCheckForUpdates()

    expect(await screen.findByTestId('modal')).toBeInTheDocument()
    expect(await screen.findByText('Checking web.service')).toBeInTheDocument()
    expect(screen.getByTestId('auto-update-in-progress')).toBeInTheDocument()

    releaseStream?.()
    await waitFor(() =>
      expect(screen.queryByTestId('auto-update-in-progress')).not.toBeInTheDocument(),
    )
  })

  it('renders the dry-run report: pending containers with images are distinct from updated', async () => {
    mockAutoUpdate([
      completeEvent([
        item({ containerName: 'web', updated: 'pending' }),
        item({ containerName: 'db', imageName: 'postgres:16', updated: 'up-to-date' }),
      ], 'dry-run'),
    ])
    await clickCheckForUpdates()

    expect(await screen.findByTestId('modal')).toBeInTheDocument()
    expect(screen.getByTestId('auto-update-dry-run-badge')).toBeInTheDocument()
    expect(screen.getByTestId('auto-update-item-pending-web')).toBeInTheDocument()
    expect(screen.getByTestId('auto-update-item-pending-web')).toHaveTextContent('nginx:1.27')
    expect(screen.getByTestId('auto-update-item-up-to-date-db')).toBeInTheDocument()
  })

  it('renders per-container failures with their errors in the report', async () => {
    mockAutoUpdate([
      completeEvent([
        item({ containerName: 'web', updated: 'updated' }),
        item({
          containerName: 'cache',
          imageName: 'redis:7',
          updated: 'failed',
          error: 'health check timed out',
        }),
      ], 'completed-with-failures'),
    ])
    await clickUpdateAllThroughConfirm()

    expect(await screen.findByTestId('auto-update-item-updated-web')).toBeInTheDocument()
    const failed = screen.getByTestId('auto-update-item-failed-cache')
    expect(failed).toHaveTextContent('redis:7')
    expect(failed).toHaveTextContent('health check timed out')
  })

  it('surfaces a terminal error event as a connection error, not per-container failures', async () => {
    mockAutoUpdate([errorEvent('SSH_FAILED', 'Machine unreachable')])
    await clickCheckForUpdates()

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('SSH_FAILED')
    expect(alert).toHaveTextContent('Machine unreachable')
    expect(screen.queryByText(/failed/i, { selector: '[data-testid^="auto-update-item"]' })).toBeNull()
  })
})
