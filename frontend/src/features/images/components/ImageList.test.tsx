import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ImageList } from '@features/images/components/ImageList'
import { server } from '@/test/server'

const imagesFixture = [
  {
    id: 'sha256:abc123def4567890abc123def4567890abc123def4567890abc123def4567890',
    repoTags: ['docker.io/library/nginx:1.27'],
    repoDigests: [],
    createdAt: '2026-08-18T10:00:00Z',
    size: 157286400,
  },
]

function renderImageList() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <ImageList machineId="m-1" />
    </QueryClientProvider>,
  )
}

describe('ImageList', () => {
  it('renders one row per image with repo tag, human size, and relative date', async () => {
    server.use(
      http.get('*/api/machines/m-1/images', () => HttpResponse.json(imagesFixture)),
    )
    renderImageList()

    await waitFor(() => {
      expect(screen.queryByTestId('images-loading')).not.toBeInTheDocument()
    })
    expect(await screen.findByText('docker.io/library/nginx:1.27')).toBeInTheDocument()
    expect(screen.getByText('157.3 MB')).toBeInTheDocument()
  })

  it('shows all repo tags and falls back to <none> with the short id for a dangling image', async () => {
    server.use(
      http.get('*/api/machines/m-1/images', () =>
        HttpResponse.json([
          {
            id: 'sha256:abc123def4567890abc123def4567890abc123def4567890abc123def4567890',
            repoTags: ['docker.io/library/nginx:1.27', 'docker.io/library/nginx:latest'],
            repoDigests: [],
            createdAt: '2026-08-18T10:00:00Z',
            size: 157286400,
          },
          {
            id: 'sha256:fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321',
            repoTags: null,
            repoDigests: [],
            createdAt: '2026-08-20T10:00:00Z',
            size: 3145728,
          },
        ]),
      ),
    )
    renderImageList()

    await waitFor(() => {
      expect(screen.queryByTestId('images-loading')).not.toBeInTheDocument()
    })
    expect(
      await screen.findByText('docker.io/library/nginx:1.27, docker.io/library/nginx:latest'),
    ).toBeInTheDocument()
    expect(screen.getAllByText('<none>')).toHaveLength(1)
    expect(screen.getByText('fedcba098765')).toBeInTheDocument()
    expect(screen.getByText('3.1 MB')).toBeInTheDocument()
  })

  it('shows the relative creation age of each image', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    vi.setSystemTime(new Date('2026-08-21T12:00:00Z'))
    server.use(
      http.get('*/api/machines/m-1/images', () => HttpResponse.json(imagesFixture)),
    )
    renderImageList()

    expect(await screen.findByText('3 days ago')).toBeInTheDocument()
  })

  it('shows a loading state before images arrive', async () => {
    server.use(
      http.get('*/api/machines/m-1/images', async () => {
        await new Promise((resolve) => setTimeout(resolve, 50))
        return HttpResponse.json(imagesFixture)
      }),
    )
    renderImageList()

    expect(screen.getByTestId('images-loading')).toBeInTheDocument()
  })

  it('shows an explanatory empty state when the machine has no images', async () => {
    server.use(http.get('*/api/machines/m-1/images', () => HttpResponse.json([])))
    renderImageList()

    const empty = await screen.findByTestId('images-empty')
    expect(empty.textContent).toMatch(/no images/i)
    expect(screen.queryByTestId('image-row')).not.toBeInTheDocument()
  })

  it('renders the backend error envelope with code and message', async () => {
    server.use(
      http.get('*/api/machines/m-1/images', () =>
        HttpResponse.json(
          { requestId: 'req-9', error: { message: 'SSH connection failed', code: 'MACHINE_UNREACHABLE' } },
          { status: 502 },
        ),
      ),
    )
    renderImageList()

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('An unexpected error occurred')).toBeInTheDocument()
  })

  it('shows the machine-scoped error message for client errors', async () => {
    server.use(
      http.get('*/api/machines/m-1/images', () =>
        HttpResponse.json(
          { requestId: 'req-10', error: { message: 'Machine not found', code: 'MACHINE_NOT_FOUND' } },
          { status: 404 },
        ),
      ),
    )
    renderImageList()

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('MACHINE_NOT_FOUND')).toBeInTheDocument()
    expect(screen.getByText('Machine not found')).toBeInTheDocument()
  })
})

afterEach(() => {
  vi.useRealTimers()
})
