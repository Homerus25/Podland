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
    id: 'existing',
    name: 'existing',
    path: '/etc/containers/systemd/existing.container',
    unit: 'existing.service',
    resource: 'container',
    state: 'running',
    ...overrides,
  }
}

const existing = quadletFixture()

const existingContent = '[Container]\nImage=docker.io/library/nginx:1.27'

let updateRequests: { count: number; body: unknown } = { count: 0, body: null }

function resetCounters() {
  updateRequests = { count: 0, body: null }
}

function quadletsHandler(quadlets: () => QuadletSummary[]) {
  return http.get('*/api/machines/m-1/quadlets', () => HttpResponse.json(quadlets()))
}

function getHandler(content = existingContent) {
  return http.get('*/api/machines/m-1/quadlets/:quadletId', () =>
    HttpResponse.json({ ...existing, content }),
  )
}

function updateHandler(status = 200, body?: Record<string, unknown>, onUpdated?: () => void) {
  return http.put('*/api/machines/m-1/quadlets/:quadletId', async ({ request }) => {
    updateRequests.count++
    updateRequests.body = await request.json()
    if (status !== 200) return HttpResponse.json(body, { status })
    onUpdated?.()
    return HttpResponse.json({ ...existing, content: (updateRequests.body as { content: string }).content })
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

async function openEditModal() {
  renderQuadletList()
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Edit existing' }))
  expect(await screen.findByTestId('modal')).toBeInTheDocument()
  return user
}

describe('QuadletList edit', () => {
  beforeEach(resetCounters)

  it('opens the editor seeded with the quadlet content from the get endpoint', async () => {
    server.use(quadletsHandler(() => [existing]), getHandler())
    await openEditModal()

    expect(await screen.findByLabelText(/content/i)).toHaveValue(existingContent)
  })

  it('renders the seeded name and resource type read-only; only content is editable', async () => {
    server.use(quadletsHandler(() => [existing]), getHandler())
    const user = await openEditModal()

    const name = (await screen.findByLabelText(/name/i)) as HTMLInputElement
    expect(name).toHaveValue('existing')
    expect(name).toHaveAttribute('readonly')

    const resource = screen.getByLabelText(/resource type/i) as HTMLInputElement
    expect(resource).toHaveValue('container')
    expect(resource).toHaveAttribute('readonly')

    const content = screen.getByLabelText(/content/i)
    await user.clear(content)
    await user.type(content, 'Image=docker.io/library/redis:7')
    expect(screen.getByLabelText(/content/i)).toHaveValue('Image=docker.io/library/redis:7')
  })

  it('saves with a content-only payload to the update endpoint', async () => {
    server.use(quadletsHandler(() => [existing]), getHandler(), updateHandler())
    const user = await openEditModal()

    const content = await screen.findByLabelText(/content/i)
    await user.clear(content)
    await user.type(content, 'Image=docker.io/library/redis:7')
    await user.click(screen.getByTestId('quadlet-form-submit'))

    await waitFor(() => expect(updateRequests.count).toBe(1))
    expect(updateRequests.body).toEqual({ content: 'Image=docker.io/library/redis:7' })
  })

  it('closes the editor and refetches the list so post-update state shows', async () => {
    let quadlets = [existing]
    server.use(
      quadletsHandler(() => quadlets),
      getHandler(),
      updateHandler(200, undefined, () => {
        // A content change plus systemd reload can change the quadlet's state.
        quadlets = [quadletFixture({ state: 'stopped' })]
      }),
    )
    const user = await openEditModal()

    const content = await screen.findByLabelText(/content/i)
    await user.clear(content)
    await user.type(content, 'Image=docker.io/library/redis:7')
    await user.click(screen.getByTestId('quadlet-form-submit'))

    await waitFor(() => expect(screen.queryByTestId('modal')).not.toBeInTheDocument())
    expect(await screen.findByTestId('quadlets-table')).toBeInTheDocument()
    await waitFor(() => {
      expect(screen.getAllByTestId('state-badge').map((badge) => badge.textContent)).toEqual([
        'stopped',
      ])
    })
    expect(updateRequests.count).toBe(1)
  })

  it('shows a loading state while the content is fetched', async () => {
    server.use(
      quadletsHandler(() => [existing]),
      http.get('*/api/machines/m-1/quadlets/:quadletId', async () => {
        await new Promise((resolve) => setTimeout(resolve, 50))
        return HttpResponse.json({ ...existing, content: existingContent })
      }),
    )
    renderQuadletList()
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Edit existing' }))

    expect(screen.getByTestId('quadlet-edit-loading')).toBeInTheDocument()
    expect(await screen.findByLabelText(/content/i)).toHaveValue(existingContent)
    expect(screen.queryByTestId('quadlet-edit-loading')).not.toBeInTheDocument()
  })

  it('shows the error envelope when the quadlet disappeared before opening', async () => {
    server.use(
      quadletsHandler(() => [existing]),
      http.get('*/api/machines/m-1/quadlets/:quadletId', () =>
        HttpResponse.json(
          { requestId: 'req-41', error: { message: 'Quadlet not found', code: 'QUADLET_NOT_FOUND' } },
          { status: 404 },
        ),
      ),
    )
    renderQuadletList()
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Edit existing' }))

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('QUADLET_NOT_FOUND')).toBeInTheDocument()
    expect(screen.getByText('Quadlet not found')).toBeInTheDocument()
    expect(screen.queryByTestId('quadlet-edit-loading')).not.toBeInTheDocument()
  })

  it('keeps the editor open and shows the error envelope when saving fails', async () => {
    server.use(
      quadletsHandler(() => [existing]),
      getHandler(),
      updateHandler(404, {
        requestId: 'req-42',
        error: { message: 'Quadlet not found', code: 'QUADLET_NOT_FOUND' },
      }),
    )
    const user = await openEditModal()

    const content = await screen.findByLabelText(/content/i)
    await user.clear(content)
    await user.type(content, 'Image=docker.io/library/redis:7')
    await user.click(screen.getByTestId('quadlet-form-submit'))

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('QUADLET_NOT_FOUND')).toBeInTheDocument()
    expect(screen.getByText('Quadlet not found', { selector: '.chakra-alert__desc' })).toBeInTheDocument()
    expect(screen.getByTestId('modal')).toBeInTheDocument()
    expect(screen.getByLabelText(/content/i)).toHaveValue('Image=docker.io/library/redis:7')
  })

  it('reopens with fresh content after a save rather than the pre-edit file', async () => {
    let servedContent = existingContent
    server.use(
      quadletsHandler(() => [existing]),
      http.get('*/api/machines/m-1/quadlets/:quadletId', () =>
        HttpResponse.json({ ...existing, content: servedContent }),
      ),
      http.put('*/api/machines/m-1/quadlets/:quadletId', async ({ request }) => {
        const body = (await request.json()) as { content: string }
        servedContent = body.content
        return HttpResponse.json({ ...existing, content: body.content })
      }),
    )
    const user = await openEditModal()

    const content = await screen.findByLabelText(/content/i)
    await user.clear(content)
    await user.type(content, 'Image=docker.io/library/redis:7')
    await user.click(screen.getByTestId('quadlet-form-submit'))
    await waitFor(() => expect(screen.queryByTestId('modal')).not.toBeInTheDocument())

    await user.click(screen.getByRole('button', { name: 'Edit existing' }))

    expect(await screen.findByLabelText(/content/i)).toHaveValue(
      'Image=docker.io/library/redis:7',
    )
  })
})
