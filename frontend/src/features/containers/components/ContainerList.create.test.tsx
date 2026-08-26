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
    names: ['/existing'],
    image: 'docker.io/library/nginx:1.27',
    createdAt: '2026-08-20T10:00:00Z',
    state: 'running',
    status: 'Up 2 hours',
    restarts: 0,
    exited: false,
    ...overrides,
  }
}

const existing = containerFixture()

let createRequests: { count: number; body: unknown } = { count: 0, body: null }

function resetCounters() {
  createRequests = { count: 0, body: null }
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

function createHandler(status = 201, body?: Record<string, unknown>) {
  return http.post('*/api/machines/m-1/containers', async ({ request }) => {
    createRequests.count++
    createRequests.body = await request.json()
    if (status !== 201) return HttpResponse.json(body, { status })
    return HttpResponse.json(
      {
        id: 'c-new',
        name: 'fresh',
        state: 'created',
        image: 'docker.io/library/redis:7',
        createdAt: '2026-08-26T00:00:00Z',
      },
      { status: 201 },
    )
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

async function openCreateModal() {
  renderContainerList()
  await screen.findByText('existing')
  const user = userEvent.setup()
  await user.click(screen.getByTestId('create-container-button'))
  expect(await screen.findByTestId('modal')).toBeInTheDocument()
  return user
}

describe('ContainerList create', () => {
  it('opens a modal form with required image and optional name and command', async () => {
    resetCounters()
    server.use(containersHandler(() => [existing]), createHandler())
    await openCreateModal()

    expect(screen.getByLabelText(/image/i)).toBeRequired()
    expect(screen.getByLabelText(/name/i)).not.toBeRequired()
    expect(screen.getByLabelText(/command/i)).not.toBeRequired()
  })

  it('blocks submit with an empty image and sends no request', async () => {
    resetCounters()
    server.use(containersHandler(() => [existing]), createHandler())
    const user = await openCreateModal()

    await user.click(screen.getByTestId('container-form-submit'))

    expect(await screen.findByText('Image is required')).toBeInTheDocument()
    expect(createRequests.count).toBe(0)
    expect(screen.getByTestId('modal')).toBeInTheDocument()
  })

  it('blocks submit with a whitespace-only image and sends no request', async () => {
    resetCounters()
    server.use(containersHandler(() => [existing]), createHandler())
    const user = await openCreateModal()

    await user.type(screen.getByLabelText(/image/i), '   ')
    await user.click(screen.getByTestId('container-form-submit'))

    expect(await screen.findByText('Image is required')).toBeInTheDocument()
    expect(createRequests.count).toBe(0)
  })

  it('offers the create action on an empty machine and creates the first container', async () => {
    resetCounters()
    let containers: ContainerSummary[] = []
    server.use(
      containersHandler(() => containers),
      http.post('*/api/machines/m-1/containers', async ({ request }) => {
        createRequests.count++
        await request.json()
        containers = [
          containerFixture({
            id: 'c-new',
            names: ['/fresh'],
            image: 'docker.io/library/redis:7',
            state: 'created',
          }),
        ]
        return HttpResponse.json(
          {
            id: 'c-new',
            name: 'fresh',
            state: 'created',
            image: 'docker.io/library/redis:7',
            createdAt: '2026-08-26T00:00:00Z',
          },
          { status: 201 },
        )
      }),
    )
    renderContainerList()

    expect(await screen.findByTestId('containers-empty')).toBeInTheDocument()
    const user = userEvent.setup()
    await user.click(screen.getByTestId('create-container-button'))
    await user.type(await screen.findByLabelText(/image/i), 'docker.io/library/redis:7')
    await user.click(screen.getByTestId('container-form-submit'))

    await waitFor(() => expect(screen.queryByTestId('modal')).not.toBeInTheDocument())
    expect(await screen.findByText('fresh')).toBeInTheDocument()
    expect(createRequests.count).toBe(1)
  })

  it('sends the create payload in the backend DTO shape', async () => {
    resetCounters()
    server.use(containersHandler(() => [existing]), createHandler())
    const user = await openCreateModal()

    await user.type(screen.getByLabelText(/image/i), 'docker.io/library/redis:7')
    await user.type(screen.getByLabelText(/command/i), 'redis-server --appendonly yes')
    await user.click(screen.getByTestId('container-form-submit'))

    await waitFor(() => expect(createRequests.count).toBe(1))
    expect(createRequests.body).toEqual({
      image: 'docker.io/library/redis:7',
      command: ['redis-server', '--appendonly', 'yes'],
    })
  })

  it('includes the optional name when one is given', async () => {
    resetCounters()
    server.use(containersHandler(() => [existing]), createHandler())
    const user = await openCreateModal()

    await user.type(screen.getByLabelText(/image/i), 'docker.io/library/redis:7')
    await user.type(screen.getByLabelText(/name/i), 'cache')
    await user.click(screen.getByTestId('container-form-submit'))

    await waitFor(() => expect(createRequests.count).toBe(1))
    expect(createRequests.body).toEqual({
      image: 'docker.io/library/redis:7',
      name: 'cache',
    })
  })

  it('closes the modal and shows the new container after success', async () => {
    resetCounters()
    let containers = [existing]
    server.use(
      containersHandler(() => containers),
      http.post('*/api/machines/m-1/containers', async ({ request }) => {
        createRequests.count++
        await request.json()
        containers = [
          existing,
          containerFixture({
            id: 'c-new',
            names: ['/fresh'],
            image: 'docker.io/library/redis:7',
            state: 'created',
          }),
        ]
        return HttpResponse.json(
          {
            id: 'c-new',
            name: 'fresh',
            state: 'created',
            image: 'docker.io/library/redis:7',
            createdAt: '2026-08-26T00:00:00Z',
          },
          { status: 201 },
        )
      }),
    )
    const user = await openCreateModal()

    await user.type(screen.getByLabelText(/image/i), 'docker.io/library/redis:7')
    await user.click(screen.getByTestId('container-form-submit'))

    await waitFor(() => expect(screen.queryByTestId('modal')).not.toBeInTheDocument())
    expect(await screen.findByText('docker.io/library/redis:7')).toBeInTheDocument()
  })

  it('keeps the modal open and shows the error envelope on failure', async () => {
    resetCounters()
    server.use(
      containersHandler(() => [existing]),
      createHandler(404, {
        requestId: 'req-14',
        error: { message: 'image not found', code: 'IMAGE_NOT_FOUND' },
      }),
    )
    const user = await openCreateModal()

    await user.type(screen.getByLabelText(/image/i), 'docker.io/library/nope:latest')
    await user.click(screen.getByTestId('container-form-submit'))

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('IMAGE_NOT_FOUND')).toBeInTheDocument()
    expect(screen.getByText('image not found')).toBeInTheDocument()
    expect(screen.getByTestId('modal')).toBeInTheDocument()
    expect(screen.getByDisplayValue('docker.io/library/nope:latest')).toBeInTheDocument()
  })
})
