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

let createRequests: { count: number; body: unknown } = { count: 0, body: null }

function resetCounters() {
  createRequests = { count: 0, body: null }
}

function quadletsHandler(quadlets: () => QuadletSummary[]) {
  return http.get('*/api/machines/m-1/quadlets', () => HttpResponse.json(quadlets()))
}

function createHandler(
  status = 201,
  body?: Record<string, unknown>,
  onCreated?: () => void,
) {
  return http.post('*/api/machines/m-1/quadlets', async ({ request }) => {
    createRequests.count++
    createRequests.body = await request.json()
    if (status !== 201) return HttpResponse.json(body, { status })
    onCreated?.()
    return HttpResponse.json(
      {
        id: 'fresh',
        name: 'fresh',
        resource: 'volume',
        state: 'unknown',
        content: 'Image=docker.io/library/redis:7',
      },
      { status: 201 },
    )
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

async function openCreateModal() {
  renderQuadletList()
  await screen.findByText('existing')
  const user = userEvent.setup()
  await user.click(screen.getByTestId('create-quadlet-button'))
  expect(await screen.findByTestId('modal')).toBeInTheDocument()
  return user
}

describe('QuadletList create', () => {
  beforeEach(resetCounters)

  it('opens a modal form with name, resource selector, and monospace content textarea', async () => {
    server.use(quadletsHandler(() => [existing]), createHandler())
    await openCreateModal()

    expect(screen.getByLabelText(/name/i)).toBeRequired()
    const resourceSelect = screen.getByLabelText(/resource type/i) as HTMLSelectElement
    expect(resourceSelect).toBeRequired()
    expect(Array.from(resourceSelect.options).map((option) => option.value)).toEqual([
      '',
      'container',
      'pod',
      'image',
      'volume',
      'network',
    ])
    const content = screen.getByLabelText(/content/i)
    expect(content).toBeRequired()
    expect(content.className).toContain('chakra-textarea')
    expect((content as HTMLElement).tagName).toBe('TEXTAREA')
  })

  it('blocks submit with all fields empty and sends no request', async () => {
    server.use(quadletsHandler(() => [existing]), createHandler())
    const user = await openCreateModal()

    await user.click(screen.getByTestId('quadlet-form-submit'))

    expect(await screen.findByText('Name is required')).toBeInTheDocument()
    expect(screen.getByText('Resource type is required')).toBeInTheDocument()
    expect(screen.getByText('Content is required')).toBeInTheDocument()
    expect(createRequests.count).toBe(0)
    expect(screen.getByTestId('modal')).toBeInTheDocument()
  })

  it('blocks submit with a whitespace-only name and whitespace-only content', async () => {
    server.use(quadletsHandler(() => [existing]), createHandler())
    const user = await openCreateModal()

    await user.type(screen.getByLabelText(/name/i), '   ')
    await user.type(screen.getByLabelText(/content/i), '   ')
    await user.click(screen.getByTestId('quadlet-form-submit'))

    expect(await screen.findByText('Name is required')).toBeInTheDocument()
    expect(screen.getByText('Content is required')).toBeInTheDocument()
    expect(createRequests.count).toBe(0)
  })

  it('sends the create payload in the backend DTO shape', async () => {
    server.use(quadletsHandler(() => [existing]), createHandler())
    const user = await openCreateModal()

    await user.type(screen.getByLabelText(/name/i), 'cache')
    await user.selectOptions(screen.getByLabelText(/resource type/i), 'volume')
    await user.type(screen.getByLabelText(/content/i), 'Image=docker.io/library/redis:7')
    await user.click(screen.getByTestId('quadlet-form-submit'))

    await waitFor(() => expect(createRequests.count).toBe(1))
    expect(createRequests.body).toEqual({
      name: 'cache',
      resource: 'volume',
      content: 'Image=docker.io/library/redis:7',
    })
  })

  it('closes the modal and refetches the list so the new quadlet appears', async () => {
    let quadlets = [existing]
    server.use(
      quadletsHandler(() => quadlets),
      createHandler(201, undefined, () => {
        quadlets = [
          existing,
          quadletFixture({
            id: 'fresh',
            name: 'fresh',
            path: '/etc/containers/systemd/fresh.volume',
            unit: 'fresh-volume.service',
            resource: 'volume',
            state: 'unknown',
          }),
        ]
      }),
    )
    const user = await openCreateModal()

    await user.type(screen.getByLabelText(/name/i), 'fresh')
    await user.selectOptions(screen.getByLabelText(/resource type/i), 'volume')
    await user.type(screen.getByLabelText(/content/i), 'Image=docker.io/library/redis:7')
    await user.click(screen.getByTestId('quadlet-form-submit'))

    await waitFor(() => expect(screen.queryByTestId('modal')).not.toBeInTheDocument())
    expect(await screen.findByTestId('quadlets-table')).toBeInTheDocument()
    expect(await screen.findByText('fresh')).toBeInTheDocument()
    expect(screen.getAllByTestId('quadlet-row')).toHaveLength(2)
    expect(createRequests.count).toBe(1)
  })

  it('keeps the modal open and shows the error envelope on failure', async () => {
    server.use(
      quadletsHandler(() => [existing]),
      createHandler(409, {
        requestId: 'req-31',
        error: { message: 'Quadlet already exists', code: 'QUADLET_ALREADY_EXISTS' },
      }),
    )
    const user = await openCreateModal()

    await user.type(screen.getByLabelText(/name/i), 'existing')
    await user.selectOptions(screen.getByLabelText(/resource type/i), 'container')
    await user.type(screen.getByLabelText(/content/i), 'Image=docker.io/library/nginx:1.27')
    await user.click(screen.getByTestId('quadlet-form-submit'))

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('QUADLET_ALREADY_EXISTS')).toBeInTheDocument()
    expect(screen.getByText('Quadlet already exists')).toBeInTheDocument()
    expect(screen.getByTestId('modal')).toBeInTheDocument()
    expect(screen.getByDisplayValue('existing')).toBeInTheDocument()
  })

  it('offers the create action on an empty machine and creates the first quadlet', async () => {
    let quadlets: QuadletSummary[] = []
    server.use(
      quadletsHandler(() => quadlets),
      createHandler(201, undefined, () => {
        quadlets = [
          quadletFixture({
            id: 'fresh',
            name: 'fresh',
            path: '/etc/containers/systemd/fresh.container',
            unit: 'fresh.service',
            resource: 'container',
            state: 'unknown',
          }),
        ]
      }),
    )
    renderQuadletList()

    expect(await screen.findByTestId('quadlets-empty')).toBeInTheDocument()
    const user = userEvent.setup()
    await user.click(screen.getByTestId('create-quadlet-button'))
    await user.type(await screen.findByLabelText(/name/i), 'fresh')
    await user.selectOptions(screen.getByLabelText(/resource type/i), 'container')
    await user.type(screen.getByLabelText(/content/i), 'Image=docker.io/library/nginx:1.27')
    await user.click(screen.getByTestId('quadlet-form-submit'))

    await waitFor(() => expect(screen.queryByTestId('modal')).not.toBeInTheDocument())
    expect(await screen.findByText('fresh')).toBeInTheDocument()
    expect(createRequests.count).toBe(1)
  })
})
