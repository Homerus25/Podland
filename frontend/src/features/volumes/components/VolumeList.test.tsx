import { describe, expect, it } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { VolumeList } from '@features/volumes/components/VolumeList'
import type { VolumeSummary } from '@features/volumes/services/volumesApi'
import { server } from '@/test/server'

function volumeFixture(overrides: Partial<VolumeSummary> = {}): VolumeSummary {
  return {
    name: 'app-data',
    driver: 'local',
    createdAt: '2026-08-20T10:00:00Z',
    anonymous: false,
    labels: {},
    options: {},
    mountpoint: '/var/lib/containers/storage/volumes/app-data/_data',
    ...overrides,
  }
}

function mockVolumes(volumes: VolumeSummary[]) {
  server.use(http.get('*/api/machines/m-1/volumes', () => HttpResponse.json(volumes)))
}

function renderVolumeList() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <VolumeList machineId="m-1" />
    </QueryClientProvider>,
  )
}

describe('VolumeList', () => {
  it('renders volume rows with name, type badge, driver, created date and size', async () => {
    mockVolumes([
      volumeFixture({ name: 'app-data', anonymous: false, size: 1500000 }),
      volumeFixture({ name: 'a1b2c3d4', anonymous: true, createdAt: '2026-08-25T10:00:00Z' }),
    ])
    renderVolumeList()

    const rows = await screen.findAllByTestId('volume-row')
    expect(rows).toHaveLength(2)

    const named = within(rows[0])
    expect(named.getByText('app-data')).toBeInTheDocument()
    expect(named.getByTestId('volume-type-badge')).toHaveTextContent(/named/i)
    expect(named.getByText('local')).toBeInTheDocument()
    expect(named.getByText(/1\.5 MB/)).toBeInTheDocument()

    const anonymous = within(rows[1])
    expect(anonymous.getByText('a1b2c3d4')).toBeInTheDocument()
    expect(anonymous.getByTestId('volume-type-badge')).toHaveTextContent(/anonymous/i)
  })

  it('renders a placeholder cell when the API reports no size', async () => {
    mockVolumes([volumeFixture({ size: undefined })])
    renderVolumeList()

    const row = await screen.findByTestId('volume-row')
    expect(within(row).getAllByRole('cell')).toHaveLength(5)
    expect(within(row).getByText('—')).toBeInTheDocument()
  })

  it('shows a distinct loading state while fetching', () => {
    mockVolumes([])
    renderVolumeList()

    expect(screen.getByTestId('volumes-loading')).toBeInTheDocument()
    expect(screen.queryByTestId('volumes-empty')).not.toBeInTheDocument()
  })

  it('shows a distinct error state when the request fails', async () => {
    server.use(
      http.get('*/api/machines/m-1/volumes', () =>
        HttpResponse.json(
          { requestId: 'r-1', error: { code: 'INTERNAL_ERROR', message: 'ssh failed' } },
          { status: 500 },
        ),
      ),
    )
    renderVolumeList()

    await waitFor(() => expect(screen.getByTestId('volumes-error')).toBeInTheDocument())
    expect(screen.queryByTestId('volumes-loading')).not.toBeInTheDocument()
    expect(screen.queryByTestId('volumes-empty')).not.toBeInTheDocument()
  })

  it('shows a distinct empty state when the machine has no volumes', async () => {
    mockVolumes([])
    renderVolumeList()

    expect(await screen.findByTestId('volumes-empty')).toBeInTheDocument()
    expect(screen.queryByTestId('volumes-loading')).not.toBeInTheDocument()
  })
})
