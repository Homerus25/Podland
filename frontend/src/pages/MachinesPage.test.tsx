import { screen } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import type { Machine } from '@shared/types/api'
import { renderApp } from '@/test/auth'
import { server } from '@/test/server'

// Mirrors GET /api/machines item shape
// (backend/internal/domain/machines/service.go MachineResponse +
// doku/description_machines_api.md).
const machinesFixture: Machine[] = [
  {
    id: 'm-1',
    name: 'prod-web-01',
    description: 'Production web server',
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
    description: 'Shared CI runner',
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

function machinesHandler(body: Machine[], status = 200) {
  return http.get('*/api/machines', () => HttpResponse.json(body, { status }))
}

function renderMachinesPage() {
  return renderApp('/machines', {
    resolveInitialState: async () => ({ username: 'alice' }),
  })
}

describe('MachinesPage', () => {
  it('renders one card per machine with name, description, hostname:port, and permission badge', async () => {
    server.use(machinesHandler(machinesFixture))
    renderMachinesPage()

    expect(await screen.findByText('prod-web-01')).toBeInTheDocument()
    expect(screen.getByText('Production web server')).toBeInTheDocument()
    expect(screen.getByText('web01.example.com:22')).toBeInTheDocument()
    expect(screen.getByText('admin')).toBeInTheDocument()

    expect(screen.getByText('ci-runner')).toBeInTheDocument()
    expect(screen.getByText('Shared CI runner')).toBeInTheDocument()
    expect(screen.getByText('ci.internal:2222')).toBeInTheDocument()
    expect(screen.getByText('read')).toBeInTheDocument()
  })

  it('shows a loading state before machines arrive', async () => {
    server.use(
      http.get('*/api/machines', async () => {
        await new Promise((resolve) => setTimeout(resolve, 50))
        return HttpResponse.json(machinesFixture)
      }),
    )
    renderMachinesPage()

    expect(await screen.findByTestId('machines-loading')).toBeInTheDocument()
  })

  it('shows an explanatory empty state when the user has no machines', async () => {
    server.use(machinesHandler([]))
    renderMachinesPage()

    const empty = await screen.findByTestId('machines-empty')
    expect(empty).toBeInTheDocument()
    expect(empty.textContent).toMatch(/no machines/i)
    expect(screen.queryByTestId('machine-card')).not.toBeInTheDocument()
  })

  it('renders the backend error envelope with code and message', async () => {
    server.use(
      http.get('*/api/machines', () =>
        HttpResponse.json(
          { requestId: 'req-2', error: { message: 'Token has expired', code: 'TOKEN_EXPIRED' } },
          { status: 401 },
        ),
      ),
    )
    renderMachinesPage()

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('TOKEN_EXPIRED')).toBeInTheDocument()
    expect(screen.getByText('Token has expired')).toBeInTheDocument()
  })

  it('shows generic text for internal error codes, hiding leaked details', async () => {
    server.use(
      http.get('*/api/machines', () =>
        HttpResponse.json(
          { requestId: 'req-3', error: { message: 'database is on fire', code: 'INTERNAL_ERROR' } },
          { status: 500 },
        ),
      ),
    )
    renderMachinesPage()

    expect(await screen.findByText('An unexpected error occurred')).toBeInTheDocument()
    expect(screen.getByText('INTERNAL_ERROR')).toBeInTheDocument()
    expect(screen.queryByText('database is on fire')).not.toBeInTheDocument()
  })
})
