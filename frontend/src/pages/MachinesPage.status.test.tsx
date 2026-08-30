import { fireEvent, screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import type { Machine } from '@shared/types/api'
import { renderApp } from '@/test/auth'
import { server } from '@/test/server'

const machinesFixture: Machine[] = [
  {
    id: 'm-1',
    name: 'prod-web-01',
    description: '',
    hostname: 'web01.example.com',
    port: 22,
    sshUsername: 'deploy',
    sshKeyFingerprint: 'SHA256:aaa',
    podmanSocketPath: '/run/podman/podman.sock',
    ownerId: 'u-owner',
    permission: 'admin',
    createdAt: '2026-08-01T10:00:00Z',
    updatedAt: '2026-08-01T10:00:00Z',
  },
  {
    id: 'm-2',
    name: 'ci-runner',
    description: '',
    hostname: 'ci.internal',
    port: 2222,
    sshUsername: 'ci',
    sshKeyFingerprint: 'SHA256:bbb',
    podmanSocketPath: '/run/podman/podman.sock',
    ownerId: 'u-owner',
    permission: 'read',
    createdAt: '2026-08-02T10:00:00Z',
    updatedAt: '2026-08-02T10:00:00Z',
  },
]

function machinesHandler(body: Machine[]) {
  return http.get('*/api/machines', () => HttpResponse.json(body))
}

function statusHandler(
  respond: (machineId: string) => Promise<Response> | Response,
  seen: string[] = [],
) {
  return http.get('*/api/machines/:id/status', async ({ params }) => {
    seen.push(String(params.id))
    return respond(String(params.id))
  })
}

function renderMachinesPage() {
  return renderApp('/machines', {
    resolveInitialState: async () => ({ username: 'alice' }),
  })
}

describe('MachinesPage status badges', () => {
  it('renders online and offline badges from the status endpoint', async () => {
    server.use(
      machinesHandler(machinesFixture),
      statusHandler((id) =>
        HttpResponse.json({
          status: id === 'm-1' ? 'online' : 'offline',
          checkedAt: '2026-08-29T10:00:00Z',
        }),
      ),
    )
    renderMachinesPage()

    expect(await screen.findByText('prod-web-01')).toBeInTheDocument()
    expect(await screen.findByText('online')).toBeInTheDocument()
    expect(await screen.findByText('offline')).toBeInTheDocument()
  })

  it('shows checking while the request is in flight, then the result', async () => {
    let release: () => void = () => {}
    const gate = new Promise<void>((resolve) => {
      release = resolve
    })
    server.use(
      machinesHandler(machinesFixture),
      statusHandler(async () => {
        await gate
        return HttpResponse.json({ status: 'online', checkedAt: '2026-08-29T10:00:00Z' })
      }),
    )
    renderMachinesPage()

    expect(await screen.findByText('prod-web-01')).toBeInTheDocument()
    expect((await screen.findAllByText('checking')).length).toBe(2)

    release()
    await waitFor(() => expect(screen.getAllByText('online')).toHaveLength(2))
  })

  it('shows unknown when the status request fails', async () => {
    server.use(
      machinesHandler(machinesFixture),
      statusHandler(() => new HttpResponse(null, { status: 500 })),
    )
    renderMachinesPage()

    expect(await screen.findByText('prod-web-01')).toBeInTheDocument()
    expect(await screen.findByText('unknown')).toBeInTheDocument()
  })

  it('fires exactly one status request per machine id', async () => {
    const seen: string[] = []
    server.use(
      machinesHandler(machinesFixture),
      statusHandler(
        () => HttpResponse.json({ status: 'online', checkedAt: '2026-08-29T10:00:00Z' }),
        seen,
      ),
    )
    renderMachinesPage()

    await waitFor(() => {
      expect(screen.getAllByTestId('machine-status-badge')).toHaveLength(2)
      expect(screen.getAllByText('online')).toHaveLength(2)
    })
    expect(seen.sort()).toEqual(['m-1', 'm-2'])
  })

  it('exposes the checkedAt timestamp as a tooltip on the badge', async () => {
    server.use(
      machinesHandler(machinesFixture),
      statusHandler(() =>
        HttpResponse.json({ status: 'online', checkedAt: '2026-08-29T10:00:00Z' }),
      ),
    )
    renderMachinesPage()

    await screen.findAllByText('online')
    const badge = screen.getAllByTestId('machine-status-badge')[0]
    fireEvent.focus(badge)
    const tooltip = await screen.findByRole('tooltip')
    expect(tooltip).toHaveTextContent('Checked at 2026-08-29T10:00:00Z')
    expect(badge).toHaveAttribute('aria-label', 'Checked at 2026-08-29T10:00:00Z')
  })

  it('re-probes every machine when the overview is re-opened', async () => {
    const seen: string[] = []
    server.use(
      machinesHandler(machinesFixture),
      statusHandler(
        () => HttpResponse.json({ status: 'online', checkedAt: '2026-08-29T10:00:00Z' }),
        seen,
      ),
    )
    const { unmount } = renderMachinesPage()
    await screen.findAllByText('online')
    unmount()

    renderMachinesPage()
    await screen.findAllByText('online')
    expect(seen.sort()).toEqual(['m-1', 'm-1', 'm-2', 'm-2'])
  })
})
