import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { ContainerList } from '@features/containers/components/ContainerList'
import type { ContainerSummary } from '@features/containers/services/containersApi'
import { server } from '@/test/server'

function containerFixture(overrides: Partial<ContainerSummary> = {}): ContainerSummary {
  return {
    id: 'abc123def4567890',
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

function containersResponse(containers: ContainerSummary[]) {
  return { containers, total: containers.length, timestamp: '2026-08-26T00:00:00Z' }
}

function mockContainers(containers: ContainerSummary[]) {
  server.use(
    http.get('*/api/machines/m-1/containers', () => HttpResponse.json(containersResponse(containers))),
  )
}

function renderContainerList() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const view = render(
    <QueryClientProvider client={queryClient}>
      <ContainerList machineId="m-1" />
    </QueryClientProvider>,
  )
  return { queryClient, ...view }
}

describe('ContainerList', () => {
  it('renders one row per container with name, image, and a state badge', async () => {
    mockContainers([
      containerFixture(),
      containerFixture({
        id: 'fedcba0987654321',
        names: ['/other-container'],
        image: 'docker.io/library/redis:7',
        state: 'exited',
      }),
    ])
    renderContainerList()

    await waitFor(() => {
      expect(screen.queryAllByTestId('container-row')).toHaveLength(2)
    })
    expect(screen.getByText('my-container')).toBeInTheDocument()
    expect(screen.getByText('docker.io/library/nginx:1.27')).toBeInTheDocument()
    expect(screen.getByText('other-container')).toBeInTheDocument()
    expect(screen.getByText('docker.io/library/redis:7')).toBeInTheDocument()
    expect(screen.getAllByTestId('state-badge').map((badge) => badge.textContent)).toEqual([
      'running',
      'exited',
    ])
  })

  it('strips the leading slash podman puts on container names', async () => {
    mockContainers([containerFixture({ names: ['/slashy'] })])
    renderContainerList()

    expect(await screen.findByText('slashy')).toBeInTheDocument()
    expect(screen.queryByText('/slashy')).not.toBeInTheDocument()
  })

  it('falls back to the short container id when the container has no name', async () => {
    mockContainers([containerFixture({ names: [] })])
    renderContainerList()

    expect(await screen.findByText('abc123def456')).toBeInTheDocument()
  })

  it.each(['running', 'exited', 'paused', 'dead', 'created', 'restarting', 'configured'])(
    'shows a badge for a container in state %s',
    async (state) => {
      mockContainers([containerFixture({ state })])
      renderContainerList()

      expect(await screen.findByText(state)).toBeInTheDocument()
    },
  )

  it('caches containers under the per-machine query key hierarchy', async () => {
    mockContainers([containerFixture()])
    const { queryClient } = renderContainerList()

    await screen.findByText('my-container')
    const cached = queryClient.getQueryData<ContainerSummary[]>(['machines', 'm-1', 'containers'])
    expect(cached).toHaveLength(1)
    expect(cached?.[0].id).toBe('abc123def4567890')
  })

  it('shows a loading state before containers arrive', async () => {
    server.use(
      http.get('*/api/machines/m-1/containers', async () => {
        await new Promise((resolve) => setTimeout(resolve, 50))
        return HttpResponse.json(containersResponse([containerFixture()]))
      }),
    )
    renderContainerList()

    expect(screen.getByTestId('containers-loading')).toBeInTheDocument()
  })

  it('shows an explanatory empty state when the machine has no containers', async () => {
    mockContainers([])
    renderContainerList()

    const empty = await screen.findByTestId('containers-empty')
    expect(empty.textContent).toMatch(/no containers/i)
    expect(screen.queryByTestId('container-row')).not.toBeInTheDocument()
  })

  it('renders the backend error envelope with code and message', async () => {
    server.use(
      http.get('*/api/machines/m-1/containers', () =>
        HttpResponse.json(
          { requestId: 'req-11', error: { message: 'Machine not found', code: 'MACHINE_NOT_FOUND' } },
          { status: 404 },
        ),
      ),
    )
    renderContainerList()

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('MACHINE_NOT_FOUND')).toBeInTheDocument()
    expect(screen.getByText('Machine not found')).toBeInTheDocument()
  })
})
