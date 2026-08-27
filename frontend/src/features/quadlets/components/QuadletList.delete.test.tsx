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

const web = quadletFixture()

let deleteRequests: { count: number; url: URL | null } = { count: 0, url: null }

function resetCounters() {
  deleteRequests = { count: 0, url: null }
}

function quadletsHandler(quadlets: () => QuadletSummary[]) {
  return http.get('*/api/machines/m-1/quadlets', () => HttpResponse.json(quadlets()))
}

function deleteHandler(status = 204, body?: Record<string, unknown>, onDeleted?: () => void) {
  return http.delete('*/api/machines/m-1/quadlets/:quadletId', ({ request }) => {
    deleteRequests.count++
    deleteRequests.url = new URL(request.url)
    if (status !== 204) return HttpResponse.json(body, { status })
    onDeleted?.()
    return new HttpResponse(null, { status: 204 })
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

async function openDeleteModal(name = 'web') {
  renderQuadletList()
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: `Delete ${name}` }))
  expect(await screen.findByTestId('modal')).toBeInTheDocument()
  return user
}

describe('QuadletList delete', () => {
  beforeEach(resetCounters)

  it('opens a confirmation modal naming the quadlet without sending a request', async () => {
    server.use(quadletsHandler(() => [web]), deleteHandler())
    await openDeleteModal()

    expect(screen.getByTestId('modal')).toHaveTextContent('web')
    expect(deleteRequests.count).toBe(0)
  })

  it('confirming deletes the quadlet with the default stop-first and no force', async () => {
    server.use(quadletsHandler(() => [web]), deleteHandler())
    const user = await openDeleteModal()

    await user.click(screen.getByTestId('confirm-delete-button'))

    await waitFor(() => expect(deleteRequests.count).toBe(1))
    expect(deleteRequests.url?.pathname).toBe('/api/machines/m-1/quadlets/web')
    expect(deleteRequests.url?.searchParams.get('stop')).toBe('true')
    expect(deleteRequests.url?.searchParams.get('force')).toBe('false')
  })

  it('maps the chosen options onto the stop and force query parameters', async () => {
    server.use(quadletsHandler(() => [web]), deleteHandler())
    const user = await openDeleteModal()

    await user.click(screen.getByRole('checkbox', { name: /stop/i }))
    await user.click(screen.getByRole('checkbox', { name: /force/i }))
    await user.click(screen.getByTestId('confirm-delete-button'))

    await waitFor(() => expect(deleteRequests.count).toBe(1))
    expect(deleteRequests.url?.searchParams.get('stop')).toBe('false')
    expect(deleteRequests.url?.searchParams.get('force')).toBe('true')
  })

  it('canceling closes the modal without sending a request', async () => {
    server.use(quadletsHandler(() => [web]), deleteHandler())
    const user = await openDeleteModal()

    await user.click(screen.getByRole('button', { name: 'Cancel' }))

    await waitFor(() => expect(screen.queryByTestId('modal')).not.toBeInTheDocument())
    expect(deleteRequests.count).toBe(0)
  })

  it('closes the modal and refetches the list so the deleted quadlet disappears', async () => {
    const survivor = quadletFixture({ id: 'db', name: 'db', unit: 'db.service' })
    let quadlets = [web, survivor]
    server.use(
      quadletsHandler(() => quadlets),
      deleteHandler(204, undefined, () => {
        quadlets = [survivor]
      }),
    )
    const user = await openDeleteModal()

    await user.click(screen.getByTestId('confirm-delete-button'))

    await waitFor(() => expect(screen.queryByTestId('modal')).not.toBeInTheDocument())
    expect(await screen.findByText('db')).toBeInTheDocument()
    await waitFor(() => expect(screen.queryByText('web')).not.toBeInTheDocument())
    expect(deleteRequests.count).toBe(1)
  })

  it('keeps the modal open and shows the error envelope when deleting fails', async () => {
    server.use(
      quadletsHandler(() => [web]),
      deleteHandler(404, {
        requestId: 'req-51',
        error: { message: 'Quadlet not found', code: 'QUADLET_NOT_FOUND' },
      }),
    )
    const user = await openDeleteModal()

    await user.click(screen.getByTestId('confirm-delete-button'))

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('QUADLET_NOT_FOUND')).toBeInTheDocument()
    expect(screen.getByText('Quadlet not found')).toBeInTheDocument()
    expect(screen.getByTestId('modal')).toBeInTheDocument()
    expect(screen.getByTestId('confirm-delete-button')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument()
  })

  it('surfaces a running resource with its envelope code and message', async () => {
    // Deleting without stop-first on a running quadlet is rejected by the
    // backend with a 400 envelope; ssh failures (502) are collapsed to the
    // generic error by the shared api error policy, so they share this path.
    server.use(
      quadletsHandler(() => [web]),
      deleteHandler(400, {
        requestId: 'req-52',
        error: {
          message: 'Quadlet-managed resource is running',
          code: 'QUADLET_RESOURCE_RUNNING',
        },
      }),
    )
    const user = await openDeleteModal()

    await user.click(screen.getByRole('checkbox', { name: /stop/i }))
    await user.click(screen.getByTestId('confirm-delete-button'))

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('QUADLET_RESOURCE_RUNNING')).toBeInTheDocument()
    expect(screen.getByText('Quadlet-managed resource is running')).toBeInTheDocument()
    expect(screen.getByTestId('modal')).toBeInTheDocument()
  })
})
