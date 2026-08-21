import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import type { Machine } from '@shared/types/api'
import { renderApp } from '@/test/auth'
import { server } from '@/test/server'

const machineFixture: Machine = {
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
}

function machineHandler(machine: Machine) {
  return http.get(`*/api/machines/${machine.id}`, () => HttpResponse.json(machine))
}

function renderMachineDetailPage(id = 'm-1') {
  return renderApp(`/machines/${id}`, {
    resolveInitialState: async () => ({ username: 'alice' }),
  })
}

describe('MachineDetailPage', () => {
  it('navigates from the machines list into the detail page via the View action', async () => {
    server.use(http.get('*/api/machines', () => HttpResponse.json([machineFixture])))
    server.use(machineHandler(machineFixture))
    renderApp('/machines', {
      resolveInitialState: async () => ({ username: 'alice' }),
    })

    await userEvent.click(await screen.findByRole('button', { name: /view/i }))

    expect(await screen.findByText('prod-web-01')).toBeInTheDocument()
    expect(screen.getByText('web01.example.com:22')).toBeInTheDocument()
  })
  it('shows a loading state before the machine arrives', async () => {
    server.use(
      http.get('*/api/machines/m-1', async () => {
        await new Promise((resolve) => setTimeout(resolve, 50))
        return HttpResponse.json(machineFixture)
      }),
    )
    renderMachineDetailPage()

    expect(await screen.findByTestId('machine-loading')).toBeInTheDocument()
  })

  it('renders machine identity with permission badge', async () => {
    server.use(machineHandler(machineFixture))
    renderMachineDetailPage()

    expect(await screen.findByText('prod-web-01')).toBeInTheDocument()
    expect(screen.getByTestId('machine-endpoint')).toHaveTextContent('web01.example.com:22')
    expect(screen.getByTestId('machine-ssh-user')).toHaveTextContent('deploy')
    expect(screen.getByTestId('machine-permission')).toHaveTextContent('admin')
  })

  it('switches between the Containers, Images, Quadlets, and Settings tabs', async () => {
    server.use(machineHandler(machineFixture))
    renderMachineDetailPage()

    await screen.findByText('prod-web-01')
    expect(screen.getByTestId('tab-containers-placeholder')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('tab', { name: /images/i }))
    expect(screen.getByTestId('tab-images-placeholder')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('tab', { name: /quadlets/i }))
    expect(screen.getByTestId('tab-quadlets-placeholder')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('tab', { name: /settings/i }))
    expect(screen.getByTestId('tab-settings-placeholder')).toBeInTheDocument()
  })

  it('renders the error envelope for an unknown machine id', async () => {
    server.use(
      http.get('*/api/machines/m-missing', () =>
        HttpResponse.json(
          { requestId: 'req-9', error: { message: 'Machine not found', code: 'MACHINE_NOT_FOUND' } },
          { status: 404 },
        ),
      ),
    )
    renderMachineDetailPage('m-missing')

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('MACHINE_NOT_FOUND')).toBeInTheDocument()
    expect(screen.getByText('Machine not found')).toBeInTheDocument()
  })
})
