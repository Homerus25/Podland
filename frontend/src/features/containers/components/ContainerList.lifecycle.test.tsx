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

function containersHandler(containers: ContainerSummary[]) {
  return http.get('*/api/machines/m-1/containers', () =>
    HttpResponse.json({
      containers,
      total: containers.length,
      timestamp: '2026-08-26T00:00:00Z',
    }),
  )
}

function lifecycleHandler(action: 'start' | 'stop', containerId: string, delayMs = 0) {
  return http.post(`*/api/machines/m-1/containers/${containerId}/${action}`, async () => {
    if (delayMs > 0) await new Promise((resolve) => setTimeout(resolve, delayMs))
    return HttpResponse.json({ id: containerId })
  })
}

function lifecycleErrorHandler(action: 'start' | 'stop', containerId: string) {
  return http.post(`*/api/machines/m-1/containers/${containerId}/${action}`, () =>
    HttpResponse.json(
      {
        requestId: 'req-12',
        error: { message: 'container failed to start', code: 'CONTAINER_START_FAILED' },
      },
      { status: 409 },
    ),
  )
}

function renderContainerList() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <ContainerList machineId="m-1" />
    </QueryClientProvider>,
  )
}

describe('ContainerList lifecycle', () => {
  it('shows Stop for a running container and Start for an exited one', async () => {
    server.use(
      containersHandler([
        containerFixture({ id: 'c-run', names: ['/runner'], state: 'running' }),
        containerFixture({ id: 'c-stop', names: ['/sleepy'], state: 'exited' }),
      ]),
    )
    renderContainerList()

    await screen.findByText('runner')
    expect(screen.getByTestId('stop-container-button')).toBeInTheDocument()
    expect(screen.getByTestId('start-container-button')).toBeInTheDocument()
  })

  it('posts to the start endpoint for that container on Start click', async () => {
    const requests: { method: string; pathname: string }[] = []
    server.use(
      containersHandler([containerFixture({ id: 'c-1', names: ['/sleepy'], state: 'exited' })]),
      http.post('*/api/machines/m-1/containers/c-1/start', ({ request }) => {
        requests.push({ method: request.method, pathname: new URL(request.url).pathname })
        return HttpResponse.json({ id: 'c-1' })
      }),
    )
    renderContainerList()

    await userEvent.click(await screen.findByTestId('start-container-button'))

    await waitFor(() => expect(requests).toHaveLength(1))
    expect(requests[0]).toEqual({
      method: 'POST',
      pathname: '/api/machines/m-1/containers/c-1/start',
    })
  })

  it('posts to the stop endpoint for that container on Stop click', async () => {
    const requests: { method: string; pathname: string }[] = []
    server.use(
      containersHandler([containerFixture({ id: 'c-2', names: ['/runner'] })]),
      http.post('*/api/machines/m-1/containers/c-2/stop', ({ request }) => {
        requests.push({ method: request.method, pathname: new URL(request.url).pathname })
        return HttpResponse.json({ id: 'c-2' })
      }),
    )
    renderContainerList()

    await userEvent.click(await screen.findByTestId('stop-container-button'))

    await waitFor(() => expect(requests).toHaveLength(1))
    expect(requests[0]).toEqual({
      method: 'POST',
      pathname: '/api/machines/m-1/containers/c-2/stop',
    })
  })

  it('marks only the clicked row as pending while the operation runs', async () => {
    server.use(
      containersHandler([
        containerFixture({ id: 'c-1', names: ['/sleepy'], state: 'exited' }),
        containerFixture({ id: 'c-2', names: ['/other'], state: 'exited' }),
      ]),
      lifecycleHandler('start', 'c-1', 100),
      lifecycleHandler('start', 'c-2', 0),
    )
    const user = userEvent.setup()
    renderContainerList()

    const buttons = await screen.findAllByTestId('start-container-button')
    expect(buttons).toHaveLength(2)
    await user.click(buttons[0])

    await waitFor(() => {
      expect(buttons[0]).toBeDisabled()
      expect(buttons[0]).toHaveAttribute('data-loading')
    })
    expect(buttons[1]).toBeEnabled()
    expect(buttons[1]).not.toHaveAttribute('data-loading')
  })

  it('invalidates the containers query on success so the new state renders', async () => {
    let started = false
    server.use(
      http.get('*/api/machines/m-1/containers', () =>
        HttpResponse.json({
          containers: [
            containerFixture({ id: 'c-1', names: ['/sleepy'], state: started ? 'running' : 'exited' }),
          ],
          total: 1,
          timestamp: '2026-08-26T00:00:00Z',
        }),
      ),
      http.post('*/api/machines/m-1/containers/c-1/start', () => {
        started = true
        return HttpResponse.json({ id: 'c-1' })
      }),
    )
    renderContainerList()

    await userEvent.click(await screen.findByTestId('start-container-button'))

    expect(await screen.findByTestId('stop-container-button')).toBeInTheDocument()
    expect(screen.queryByTestId('start-container-button')).not.toBeInTheDocument()
  })

  it('shows the error envelope and clears pending when the action fails', async () => {
    server.use(
      containersHandler([containerFixture({ id: 'c-1', names: ['/sleepy'], state: 'exited' })]),
      lifecycleErrorHandler('start', 'c-1'),
    )
    const user = userEvent.setup()
    renderContainerList()

    const button = await screen.findByTestId('start-container-button')
    await user.click(button)

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('CONTAINER_START_FAILED')).toBeInTheDocument()
    expect(screen.getByText('container failed to start')).toBeInTheDocument()
    await waitFor(() => expect(button).toBeEnabled())
  })
})
