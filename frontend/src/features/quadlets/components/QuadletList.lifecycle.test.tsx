import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it } from 'vitest'
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

const web = quadletFixture({ state: 'stopped' })

function quadletsHandler(quadlets: () => QuadletSummary[]) {
  return http.get('*/api/machines/m-1/quadlets', () => HttpResponse.json(quadlets()))
}

let lifecycleRequests: URL[] = []

function resetRequests() {
  lifecycleRequests = []
}

// The lifecycle endpoints answer 200 with {id, state, unit}; the response
// body itself is not used by the UI (the list refetch tells the story), so
// the handler only records what it saw. A delay keeps a request in flight
// for the pending-state tests.
function captureLifecycle(action: 'start' | 'stop' | 'restart', delayMs = 0) {
  return http.post(`*/api/machines/m-1/quadlets/:quadletId/${action}`, async ({ request }) => {
    if (delayMs > 0) await new Promise((resolve) => setTimeout(resolve, delayMs))
    lifecycleRequests.push(new URL(request.url))
    return HttpResponse.json({ id: 'web', state: 'running', unit: 'web.service' })
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

async function openOptionsModal(action: 'stop' | 'restart', name = 'web') {
  const user = userEvent.setup()
  renderQuadletList()
  const label = action === 'stop' ? 'Stop' : 'Restart'
  await user.click(await screen.findByRole('button', { name: `${label} ${name}` }))
  expect(await screen.findByTestId('modal')).toHaveTextContent(name)
  return user
}

describe('QuadletList lifecycle', () => {
  beforeEach(resetRequests)

  it('posts to the start endpoint with no options on Start click', async () => {
    server.use(quadletsHandler(() => [web]), captureLifecycle('start'))
    renderQuadletList()

    await userEvent.click(await screen.findByTestId('start-quadlet-button'))

    await waitFor(() => expect(lifecycleRequests).toHaveLength(1))
    expect(lifecycleRequests[0].pathname).toBe('/api/machines/m-1/quadlets/web/start')
    expect(lifecycleRequests[0].search).toBe('')
  })

  it('opens the stop options modal naming the quadlet without sending a request', async () => {
    server.use(quadletsHandler(() => [quadletFixture()]), captureLifecycle('stop'))
    await openOptionsModal('stop')

    expect(screen.getByRole('checkbox', { name: /force/i })).not.toBeChecked()
    expect(screen.getByRole('spinbutton', { name: /timeout/i })).toHaveValue(null)
    expect(lifecycleRequests).toHaveLength(0)
  })

  it('confirming stop with the defaults sends force=false and no timeout parameter', async () => {
    server.use(quadletsHandler(() => [quadletFixture()]), captureLifecycle('stop'))
    const user = await openOptionsModal('stop')

    await user.click(screen.getByTestId('confirm-lifecycle-button'))

    await waitFor(() => expect(lifecycleRequests).toHaveLength(1))
    expect(lifecycleRequests[0].pathname).toBe('/api/machines/m-1/quadlets/web/stop')
    expect(lifecycleRequests[0].searchParams.get('force')).toBe('false')
    expect(lifecycleRequests[0].searchParams.has('timeout')).toBe(false)
  })

  it('maps the chosen stop options onto the force and timeout query parameters', async () => {
    server.use(quadletsHandler(() => [quadletFixture()]), captureLifecycle('stop'))
    const user = await openOptionsModal('stop')

    await user.click(screen.getByRole('checkbox', { name: /force/i }))
    await user.type(screen.getByRole('spinbutton', { name: /timeout/i }), '30')
    await user.click(screen.getByTestId('confirm-lifecycle-button'))

    await waitFor(() => expect(lifecycleRequests).toHaveLength(1))
    expect(lifecycleRequests[0].searchParams.get('force')).toBe('true')
    expect(lifecycleRequests[0].searchParams.get('timeout')).toBe('30')
  })

  it('restarts through the options modal onto the restart endpoint', async () => {
    server.use(quadletsHandler(() => [quadletFixture()]), captureLifecycle('restart'))
    const user = await openOptionsModal('restart')

    expect(screen.getByTestId('modal')).toHaveTextContent('Restart')
    await user.click(screen.getByRole('checkbox', { name: /force/i }))
    await user.type(screen.getByRole('spinbutton', { name: /timeout/i }), '45')
    await user.click(screen.getByTestId('confirm-lifecycle-button'))

    await waitFor(() => expect(lifecycleRequests).toHaveLength(1))
    expect(lifecycleRequests[0].pathname).toBe('/api/machines/m-1/quadlets/web/restart')
    expect(lifecycleRequests[0].searchParams.get('force')).toBe('true')
    expect(lifecycleRequests[0].searchParams.get('timeout')).toBe('45')
  })

  it('confirming restart with defaults sends force=false and no timeout parameter', async () => {
    server.use(quadletsHandler(() => [quadletFixture()]), captureLifecycle('restart'))
    const user = await openOptionsModal('restart')

    await user.click(screen.getByTestId('confirm-lifecycle-button'))

    await waitFor(() => expect(lifecycleRequests).toHaveLength(1))
    expect(lifecycleRequests[0].pathname).toBe('/api/machines/m-1/quadlets/web/restart')
    expect(lifecycleRequests[0].searchParams.get('force')).toBe('false')
    expect(lifecycleRequests[0].searchParams.has('timeout')).toBe(false)
  })

  it('disables the row lifecycle actions while a start runs and resists double clicks', async () => {
    server.use(
      quadletsHandler(() => [web, quadletFixture({ id: 'db', name: 'db', unit: 'db.service' })]),
      captureLifecycle('start', 100),
    )
    const user = userEvent.setup()
    renderQuadletList()

    const startButtons = await screen.findAllByTestId('start-quadlet-button')
    expect(startButtons).toHaveLength(2)
    await user.click(startButtons[0])
    await user.click(startButtons[0])

    await waitFor(() => {
      expect(startButtons[0]).toBeDisabled()
      expect(startButtons[0]).toHaveAttribute('data-loading')
    })
    expect(screen.getAllByTestId('stop-quadlet-button')[0]).toBeDisabled()
    expect(screen.getAllByTestId('restart-quadlet-button')[0]).toBeDisabled()
    expect(startButtons[1]).toBeEnabled()
    expect(screen.getAllByTestId('stop-quadlet-button')[1]).toBeEnabled()

    await waitFor(() => expect(lifecycleRequests).toHaveLength(1))
  })

  it('shows the confirm button as pending while a stop runs', async () => {
    server.use(quadletsHandler(() => [quadletFixture()]), captureLifecycle('stop', 100))
    const user = await openOptionsModal('stop')

    const confirm = screen.getByTestId('confirm-lifecycle-button')
    await user.click(confirm)

    await waitFor(() => {
      expect(confirm).toBeDisabled()
      expect(confirm).toHaveAttribute('data-loading')
    })
  })

  it('refetches the list on success so the state badge shows the new state', async () => {
    let stopped = false
    server.use(
      quadletsHandler(() => [quadletFixture({ state: stopped ? 'stopped' : 'running' })]),
      http.post('*/api/machines/m-1/quadlets/:quadletId/stop', () => {
        stopped = true
        return HttpResponse.json({ id: 'web', state: 'stopped', unit: 'web.service' })
      }),
    )
    const user = userEvent.setup()
    renderQuadletList()

    expect(await screen.findByTestId('state-badge')).toHaveTextContent('running')
    await user.click(await screen.findByTestId('stop-quadlet-button'))
    await user.click(await screen.findByTestId('confirm-lifecycle-button'))

    await waitFor(() => expect(screen.getByTestId('state-badge')).toHaveTextContent('stopped'))
  })

  it('shows the error envelope under the row when starting fails', async () => {
    server.use(
      quadletsHandler(() => [quadletFixture()]),
      http.post('*/api/machines/m-1/quadlets/:quadletId/start', () =>
        HttpResponse.json(
          {
            requestId: 'req-61',
            error: { message: 'Unit is masked', code: 'QUADLET_START_FAILED' },
          },
          { status: 409 },
        ),
      ),
    )
    const user = userEvent.setup()
    renderQuadletList()

    const startButton = await screen.findByTestId('start-quadlet-button')
    await user.click(startButton)

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('QUADLET_START_FAILED')).toBeInTheDocument()
    expect(screen.getByText('Unit is masked')).toBeInTheDocument()
    await waitFor(() => expect(startButton).toBeEnabled())
  })

  it('keeps the options modal open with the error envelope when stopping fails', async () => {
    server.use(
      quadletsHandler(() => [quadletFixture()]),
      http.post('*/api/machines/m-1/quadlets/:quadletId/stop', () =>
        HttpResponse.json(
          {
            requestId: 'req-62',
            error: { message: 'Quadlet not found', code: 'QUADLET_NOT_FOUND' },
          },
          { status: 404 },
        ),
      ),
    )
    const user = await openOptionsModal('stop')

    await user.click(screen.getByTestId('confirm-lifecycle-button'))

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('QUADLET_NOT_FOUND')).toBeInTheDocument()
    expect(screen.getByText('Quadlet not found')).toBeInTheDocument()
    expect(screen.getByTestId('modal')).toBeInTheDocument()
    expect(screen.getByTestId('confirm-lifecycle-button')).toBeEnabled()
  })
})
