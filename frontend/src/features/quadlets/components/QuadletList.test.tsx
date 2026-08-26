import { render, screen, waitFor } from '@testing-library/react'
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

function mockQuadlets(quadlets: QuadletSummary[]) {
  server.use(
    http.get('*/api/machines/m-1/quadlets', () => HttpResponse.json(quadlets)),
  )
}

function renderQuadletList() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const view = render(
    <QueryClientProvider client={queryClient}>
      <QuadletList machineId="m-1" />
    </QueryClientProvider>,
  )
  return { queryClient, ...view }
}

describe('QuadletList', () => {
  it('renders one row per quadlet with name, resource, and a state badge', async () => {
    mockQuadlets([
      quadletFixture(),
      quadletFixture({
        id: 'db',
        name: 'db',
        path: '/etc/containers/systemd/db.volume',
        unit: 'db-volume.service',
        resource: 'volume',
        state: 'stopped',
      }),
    ])
    renderQuadletList()

    await waitFor(() => {
      expect(screen.queryAllByTestId('quadlet-row')).toHaveLength(2)
    })
    expect(screen.getByText('web')).toBeInTheDocument()
    expect(screen.getByText('db')).toBeInTheDocument()
    expect(screen.getAllByTestId('state-badge').map((badge) => badge.textContent)).toEqual([
      'running',
      'stopped',
    ])
  })

  it('renders every quadlet resource type the backend reports', async () => {
    mockQuadlets([
      quadletFixture(),
      quadletFixture({
        id: 'front',
        name: 'front',
        path: '/etc/containers/systemd/front.pod',
        resource: 'pod',
      }),
      quadletFixture({
        id: 'app',
        name: 'app',
        path: '/etc/containers/systemd/app.image',
        resource: 'image',
      }),
      quadletFixture({
        id: 'db',
        name: 'db',
        path: '/etc/containers/systemd/db.volume',
        resource: 'volume',
      }),
      quadletFixture({
        id: 'net',
        name: 'net',
        path: '/etc/containers/systemd/net.network',
        resource: 'network',
      }),
    ])
    renderQuadletList()

    await waitFor(() => {
      expect(screen.queryAllByTestId('quadlet-row')).toHaveLength(5)
    })
    expect(screen.getByText('container')).toBeInTheDocument()
    expect(screen.getByText('pod')).toBeInTheDocument()
    expect(screen.getByText('image')).toBeInTheDocument()
    expect(screen.getByText('volume')).toBeInTheDocument()
    expect(screen.getByText('network')).toBeInTheDocument()
  })

  it('caches quadlets under the per-machine query key hierarchy', async () => {
    mockQuadlets([quadletFixture()])
    const { queryClient } = renderQuadletList()

    await screen.findByText('web')
    const cached = queryClient.getQueryData<QuadletSummary[]>(['machines', 'm-1', 'quadlets'])
    expect(cached).toHaveLength(1)
    expect(cached?.[0].id).toBe('web')
  })

  it.each(['running', 'stopped', 'error', 'unknown'])(
    'shows a badge for a quadlet in state %s',
    async (state) => {
      mockQuadlets([quadletFixture({ state })])
      renderQuadletList()

      expect(await screen.findByTestId('state-badge')).toHaveTextContent(state)
    },
  )

  it('shows a loading state before quadlets arrive', async () => {
    server.use(
      http.get('*/api/machines/m-1/quadlets', async () => {
        await new Promise((resolve) => setTimeout(resolve, 50))
        return HttpResponse.json([quadletFixture()])
      }),
    )
    renderQuadletList()

    expect(screen.getByTestId('quadlets-loading')).toBeInTheDocument()
  })

  it('shows an explanatory empty state when the machine has no quadlets', async () => {
    mockQuadlets([])
    renderQuadletList()

    const empty = await screen.findByTestId('quadlets-empty')
    expect(empty.textContent).toMatch(/no quadlets/i)
    expect(screen.queryByTestId('quadlet-row')).not.toBeInTheDocument()
  })

  it('renders the backend error envelope with code and message', async () => {
    server.use(
      http.get('*/api/machines/m-1/quadlets', () =>
        HttpResponse.json(
          { requestId: 'req-21', error: { message: 'Machine not found', code: 'MACHINE_NOT_FOUND' } },
          { status: 404 },
        ),
      ),
    )
    renderQuadletList()

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('MACHINE_NOT_FOUND')).toBeInTheDocument()
    expect(screen.getByText('Machine not found')).toBeInTheDocument()
  })
})
