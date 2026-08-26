import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { ContainerList } from '@features/containers/components/ContainerList'
import type { ContainerSummary } from '@features/containers/services/containersApi'
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

const first = containerFixture({ id: 'c-1', names: ['/victim'] })
const second = containerFixture({ id: 'c-2', names: ['/survivor'], state: 'exited' })

let deleteRequests: { method: string; pathname: string; search: string; count: number } = {
  method: '',
  pathname: '',
  search: '',
  count: 0,
}

function resetCounters() {
  deleteRequests = { method: '', pathname: '', search: '', count: 0 }
}

function containersHandler(containers: () => ContainerSummary[]) {
  return http.get('*/api/machines/m-1/containers', () =>
    HttpResponse.json({
      containers: containers(),
      total: containers().length,
      timestamp: '2026-08-26T00:00:00Z',
    }),
  )
}

function deleteHandler(onSuccess?: () => void) {
  return http.delete('*/api/machines/m-1/containers/*', ({ request }) => {
    deleteRequests.count++
    deleteRequests.method = request.method
    const url = new URL(request.url)
    deleteRequests.pathname = url.pathname
    deleteRequests.search = url.search
    onSuccess?.()
    return new HttpResponse(null, { status: 204 })
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

async function openDeleteModal() {
  renderContainerList()
  await screen.findByText('victim')
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: /delete victim/i }))
  return user
}

describe('ContainerList delete', () => {
  it('opens the confirmation modal and sends no delete request before confirming', async () => {
    resetCounters()
    server.use(containersHandler(() => [first]), deleteHandler())
    await openDeleteModal()

    expect(screen.getByTestId('modal')).toBeInTheDocument()
    expect(deleteRequests.count).toBe(0)
  })

  it('cancelling the modal sends no request and leaves the list untouched', async () => {
    resetCounters()
    server.use(containersHandler(() => [first]), deleteHandler())
    const user = await openDeleteModal()

    await user.click(screen.getByRole('button', { name: /cancel/i }))

    await waitFor(() => expect(screen.queryByTestId('modal')).not.toBeInTheDocument())
    expect(deleteRequests.count).toBe(0)
    expect(screen.getByText('victim')).toBeInTheDocument()
  })

  it('confirming deletes the targeted container with the right request', async () => {
    resetCounters()
    let remaining = [first, second]
    server.use(
      containersHandler(() => remaining),
      deleteHandler(() => {
        remaining = [second]
      }),
    )
    const user = await openDeleteModal()

    await user.click(screen.getByTestId('confirm-delete-button'))

    await waitFor(() => expect(deleteRequests.count).toBe(1))
    expect(deleteRequests.method).toBe('DELETE')
    expect(deleteRequests.pathname).toBe('/api/machines/m-1/containers/c-1')
    expect(deleteRequests.search).toBe('')
    expect(await screen.findByText('survivor')).toBeInTheDocument()
    await waitFor(() => expect(screen.queryByText('victim')).not.toBeInTheDocument())
  })

  it('shows pending state on the confirm action while the delete runs', async () => {
    resetCounters()
    server.use(
      containersHandler(() => [first]),
      http.delete('*/api/machines/m-1/containers/*', async () => {
        await new Promise((resolve) => setTimeout(resolve, 100))
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const user = await openDeleteModal()

    const confirm = screen.getByTestId('confirm-delete-button')
    await user.click(confirm)

    await waitFor(() => {
      expect(confirm).toBeDisabled()
      expect(confirm).toHaveAttribute('data-loading')
    })
  })

  it('shows the error envelope and keeps the row when the delete fails', async () => {
    resetCounters()
    server.use(
      containersHandler(() => [first]),
      http.delete('*/api/machines/m-1/containers/*', () =>
        HttpResponse.json(
          {
            requestId: 'req-13',
            error: { message: 'container is running', code: 'CONTAINER_RUNNING' },
          },
          { status: 409 },
        ),
      ),
    )
    const user = await openDeleteModal()

    const confirm = screen.getByTestId('confirm-delete-button')
    await user.click(confirm)

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('CONTAINER_RUNNING')).toBeInTheDocument()
    expect(screen.getByText('container is running')).toBeInTheDocument()
    await waitFor(() => expect(confirm).toBeEnabled())
    await waitFor(() => expect(screen.getByText('victim')).toBeInTheDocument())
  })
})
