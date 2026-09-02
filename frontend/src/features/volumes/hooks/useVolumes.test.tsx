import { describe, expect, it } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { listVolumes, type VolumeSummary } from '@features/volumes/services/volumesApi'
import { useVolumes } from '@features/volumes/hooks/useVolumes'
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

describe('listVolumes', () => {
  it('fetches the machine volumes through the API', async () => {
    const volumes = [volumeFixture()]
    server.use(http.get('*/api/machines/m-1/volumes', () => HttpResponse.json(volumes)))

    await expect(listVolumes('m-1')).resolves.toEqual(volumes)
  })
})

describe('useVolumes', () => {
  it('returns and caches the volume list per machine', async () => {
    const volumes = [volumeFixture(), volumeFixture({ name: 'tmp', anonymous: true, size: 4096 })]
    server.use(http.get('*/api/machines/m-1/volumes', () => HttpResponse.json(volumes)))

    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )

    const { result } = renderHook(() => useVolumes('m-1'), { wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data).toEqual(volumes)

    expect(queryClient.getQueryData(['machines', 'm-1', 'volumes'])).toEqual(volumes)
  })

  it('surfaces request errors without retrying', async () => {
    server.use(
      http.get('*/api/machines/m-1/volumes', () =>
        HttpResponse.json(
          { requestId: 'r-1', error: { code: 'INTERNAL_ERROR', message: 'ssh failed' } },
          { status: 500 },
        ),
      ),
    )

    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )

    const { result } = renderHook(() => useVolumes('m-1'), { wrapper })
    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.data).toBeUndefined()
  })
})
